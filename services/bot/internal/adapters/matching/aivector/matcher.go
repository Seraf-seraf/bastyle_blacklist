package aivector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image/png"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type matcher struct {
	contentFrames *contentFrameExtractor
	client        Client
	modelName     string
	modelRevision string
	threshold     float64
	topK          int
	rule          MatchRule
}

type Options struct {
	Downloader          ports.MediaDownloader
	ImageFrameExtractor ports.MediaExtractor
	VideoFrameExtractor ports.MediaExtractor
	Client              Client
	ModelName           string
	ModelRevision       string
	Threshold           float64
	TopK                int
	Plan                domain.MediaExtractionPlan
	Limits              Limits
	Rule                MatchRule
}

type Limits struct {
	MaxAnimationDuration    time.Duration
	MaxVideoStickerDuration time.Duration
	MaxAnimationSize        int64
	MaxVideoStickerSize     int64
}

type MatchRule struct {
	MinMatchedFrames int
	MinMatchedRatio  float64
}

func NewMatcher(options Options) (*matcher, error) {
	const methodCtx = "aivector/NewMatcher"

	if err := validateOptions(options); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &matcher{
		contentFrames: newContentFrameExtractor(
			options.Downloader,
			options.ImageFrameExtractor,
			options.VideoFrameExtractor,
			options.Plan,
			options.Limits,
		),
		client:        options.Client,
		modelName:     options.ModelName,
		modelRevision: options.ModelRevision,
		threshold:     options.Threshold,
		topK:          options.TopK,
		rule:          options.Rule,
	}, nil
}

func (m *matcher) IsBlocked(ctx context.Context, chatID int64, content domain.Content) (bool, error) {
	const methodCtx = "aivector/matcher.IsBlocked"

	frames, err := m.contentFrames.Extract(ctx, content)
	if err != nil {
		if errors.Is(err, ports.ErrUnsupportedContent) {
			return false, nil
		}
		return false, apperrors.Wrap(methodCtx, err)
	}

	response, err := m.client.Search(ctx, SearchRequest{
		ChatID: chatID,
		TopK:   m.topK,
		Frames: frames,
	})
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	if len(frames) == 1 {
		return staticMatched(response, m.threshold), nil
	}

	return videoLikeMatched(response, len(frames), m.threshold, m.rule), nil
}

func (m *matcher) PrepareBlock(ctx context.Context, chatID int64, content domain.Content) (ports.PreparedBlock, error) {
	const methodCtx = "aivector/matcher.PrepareBlock"

	frames, err := m.contentFrames.Extract(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	embedding, err := m.client.Embed(ctx, EmbedRequest{Frames: frames})
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if embedding.Dimension <= 0 {
		return nil, apperrors.New(methodCtx, "AI-vector сервис вернул неположительную размерность")
	}
	if len(embedding.Frames) == 0 {
		return nil, apperrors.New(methodCtx, "AI-vector сервис не вернул векторы кадров")
	}

	vectorFrames := make([]preparedVectorFrame, 0, len(embedding.Frames))
	for _, frame := range embedding.Frames {
		if len(frame.Vector) != embedding.Dimension {
			return nil, apperrors.New(methodCtx, "размерность AI-vector кадра не совпадает с ответом сервиса")
		}
		vectorFrames = append(vectorFrames, preparedVectorFrame{
			FrameIndex:     frame.FrameIndex,
			PositionMillis: frame.FrameIndex * 1000,
			Vector:         frame.Vector,
		})
	}

	return &preparedBlock{
		chatID:        chatID,
		fileUniqueID:  content.FileUniqueID,
		mediaType:     content.Type,
		modelName:     m.modelName,
		modelRevision: m.modelRevision,
		vectorDim:     embedding.Dimension,
		vectorFrames:  vectorFrames,
	}, nil
}

type contentFrameExtractor struct {
	downloader          ports.MediaDownloader
	imageFrameExtractor ports.MediaExtractor
	videoFrameExtractor ports.MediaExtractor
	plan                domain.MediaExtractionPlan
	limits              Limits
}

func newContentFrameExtractor(
	downloader ports.MediaDownloader,
	imageFrameExtractor ports.MediaExtractor,
	videoFrameExtractor ports.MediaExtractor,
	plan domain.MediaExtractionPlan,
	limits Limits,
) *contentFrameExtractor {
	return &contentFrameExtractor{
		downloader:          downloader,
		imageFrameExtractor: imageFrameExtractor,
		videoFrameExtractor: videoFrameExtractor,
		plan:                plan,
		limits:              limits,
	}
}

func (e *contentFrameExtractor) Extract(ctx context.Context, content domain.Content) ([]FrameFile, error) {
	const methodCtx = "aivector/contentFrameExtractor.Extract"

	if !content.CanDownload() {
		return nil, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	switch content.Type {
	case domain.MediaPhoto:
		return e.imageFrames(ctx, content)
	case domain.MediaStickerStatic:
		media, err := e.downloader.Download(ctx, content)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		if isWebMFile(media.FilePath) {
			if err := e.checkVideoSticker(media); err != nil {
				return nil, apperrors.Wrap(methodCtx, err)
			}

			frames, err := e.videoFrames(ctx, media)
			return frames, apperrors.Wrap(methodCtx, err)
		}

		frames, err := e.extractFrames(ctx, e.imageFrameExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
		return frames, apperrors.Wrap(methodCtx, err)
	case domain.MediaAnimation:
		if err := e.checkAnimationMetadata(content); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		media, err := e.downloader.Download(ctx, content)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		if err := e.checkAnimationFile(media); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		frames, err := e.videoFrames(ctx, media)
		return frames, apperrors.Wrap(methodCtx, err)
	default:
		return nil, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}
}

func (e *contentFrameExtractor) imageFrames(ctx context.Context, content domain.Content) ([]FrameFile, error) {
	const methodCtx = "aivector/contentFrameExtractor.imageFrames"

	media, err := e.downloader.Download(ctx, content)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if isVideoFile(media.FilePath) {
		return nil, apperrors.Wrap(methodCtx, ports.ErrUnsupportedContent)
	}

	frames, err := e.extractFrames(ctx, e.imageFrameExtractor, media, domain.MediaExtractionPlan{MaxFrames: 1})
	return frames, apperrors.Wrap(methodCtx, err)
}

func (e *contentFrameExtractor) videoFrames(ctx context.Context, media domain.MediaFile) ([]FrameFile, error) {
	const methodCtx = "aivector/contentFrameExtractor.videoFrames"

	frames, err := e.extractFrames(ctx, e.videoFrameExtractor, media, e.plan)
	return frames, apperrors.Wrap(methodCtx, err)
}

func (e *contentFrameExtractor) extractFrames(
	ctx context.Context,
	extractor ports.MediaExtractor,
	media domain.MediaFile,
	plan domain.MediaExtractionPlan,
) ([]FrameFile, error) {
	const methodCtx = "aivector/contentFrameExtractor.extractFrames"

	extracted, err := extractor.Extract(ctx, media, plan)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if len(extracted.Frames) == 0 {
		return nil, apperrors.New(methodCtx, "извлекатель не вернул кадров")
	}

	frames := make([]FrameFile, 0, len(extracted.Frames))
	for _, frame := range extracted.Frames {
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, frame.Image); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		frames = append(frames, FrameFile{
			Name: frameFileName(frame.Index),
			Data: buffer.Bytes(),
		})
	}

	return frames, nil
}

func staticMatched(response SearchResponse, threshold float64) bool {
	for _, frame := range response.Frames {
		for _, hit := range frame.Hits {
			if hit.Score >= threshold {
				return true
			}
		}
	}

	return false
}

func videoLikeMatched(response SearchResponse, checkedFrames int, threshold float64, rule MatchRule) bool {
	if checkedFrames <= 0 {
		return false
	}

	matchedByBan := make(map[int]int)
	for _, frame := range response.Frames {
		matchedInFrame := make(map[int]bool)
		for _, hit := range frame.Hits {
			if hit.Score < threshold {
				continue
			}
			matchedInFrame[hit.BanID] = true
		}
		for banID := range matchedInFrame {
			matchedByBan[banID]++
		}
	}

	for _, matchedFrames := range matchedByBan {
		if matchedFrames >= rule.MinMatchedFrames &&
			float64(matchedFrames)/float64(checkedFrames) >= rule.MinMatchedRatio {
			return true
		}
	}

	return false
}

func validateOptions(options Options) error {
	const methodCtx = "aivector/validateOptions"

	if options.Downloader == nil {
		return apperrors.New(methodCtx, "загрузчик не настроен")
	}
	if options.ImageFrameExtractor == nil {
		return apperrors.New(methodCtx, "извлекатель изображений не настроен")
	}
	if options.VideoFrameExtractor == nil {
		return apperrors.New(methodCtx, "извлекатель видео не настроен")
	}
	if options.Client == nil {
		return apperrors.New(methodCtx, "клиент не настроен")
	}
	if options.ModelName == "" {
		return apperrors.New(methodCtx, "model_name не настроен")
	}
	if options.ModelRevision == "" {
		return apperrors.New(methodCtx, "model_revision не настроен")
	}
	if options.Threshold < 0 || options.Threshold > 1 {
		return apperrors.New(methodCtx, "порог должен быть от 0 до 1")
	}
	if options.TopK <= 0 {
		return apperrors.New(methodCtx, "top_k должен быть положительным")
	}
	if options.Plan.MaxFrames <= 0 {
		return apperrors.New(methodCtx, "максимальное количество кадров должно быть положительным")
	}
	if options.Plan.TargetWidth <= 0 {
		return apperrors.New(methodCtx, "целевая ширина должна быть положительной")
	}
	if options.Plan.TargetHeight <= 0 {
		return apperrors.New(methodCtx, "целевая высота должна быть положительной")
	}
	if err := options.Limits.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if err := options.Rule.validate(); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func (l Limits) validate() error {
	const methodCtx = "aivector/Limits.validate"

	if l.MaxAnimationDuration <= 0 {
		return apperrors.New(methodCtx, "максимальная длительность анимации должна быть положительной")
	}
	if l.MaxVideoStickerDuration <= 0 {
		return apperrors.New(methodCtx, "максимальная длительность видеостикера должна быть положительной")
	}
	if l.MaxAnimationSize <= 0 {
		return apperrors.New(methodCtx, "максимальный размер анимации должен быть положительным")
	}
	if l.MaxVideoStickerSize <= 0 {
		return apperrors.New(methodCtx, "максимальный размер видеостикера должен быть положительным")
	}

	return nil
}

func (r MatchRule) validate() error {
	const methodCtx = "aivector/MatchRule.validate"

	if r.MinMatchedFrames <= 0 {
		return apperrors.New(methodCtx, "минимальное количество совпавших кадров должно быть положительным")
	}
	if r.MinMatchedRatio <= 0 || r.MinMatchedRatio > 1 {
		return apperrors.New(methodCtx, "минимальная доля совпавших кадров должна быть от 0 до 1")
	}

	return nil
}

func (e *contentFrameExtractor) checkAnimationMetadata(content domain.Content) error {
	const methodCtx = "aivector/contentFrameExtractor.checkAnimationMetadata"

	if time.Duration(content.DurationSec)*time.Second > e.limits.MaxAnimationDuration {
		return apperrors.New(methodCtx, "длительность анимации превышает лимит")
	}
	if content.SizeBytes > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер анимации превышает лимит")
	}

	return nil
}

func (e *contentFrameExtractor) checkAnimationFile(media domain.MediaFile) error {
	const methodCtx = "aivector/contentFrameExtractor.checkAnimationFile"

	if media.Content.SizeBytes > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер анимации превышает лимит")
	}
	if int64(len(media.Data)) > e.limits.MaxAnimationSize {
		return apperrors.New(methodCtx, "размер загруженной анимации превышает лимит")
	}

	return nil
}

func (e *contentFrameExtractor) checkVideoSticker(media domain.MediaFile) error {
	const methodCtx = "aivector/contentFrameExtractor.checkVideoSticker"

	if time.Duration(media.Content.DurationSec)*time.Second > e.limits.MaxVideoStickerDuration {
		return apperrors.New(methodCtx, "длительность видеостикера превышает лимит")
	}
	if media.Content.SizeBytes > e.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "размер видеостикера превышает лимит")
	}
	if int64(len(media.Data)) > e.limits.MaxVideoStickerSize {
		return apperrors.New(methodCtx, "размер загруженного видеостикера превышает лимит")
	}

	return nil
}

func frameFileName(index int) string {
	return "frame-" + strings.Repeat("0", max(0, 3-len(strconv.Itoa(index)))) + strconv.Itoa(index) + ".png"
}

type preparedBlock struct {
	chatID        int64
	fileUniqueID  string
	mediaType     domain.MediaType
	modelName     string
	modelRevision string
	vectorDim     int
	vectorFrames  []preparedVectorFrame
	banID         int64
}

type preparedVectorFrame struct {
	FrameIndex     int
	PositionMillis int
	Vector         []float32
}

func (m *matcher) PersistBlock(ctx context.Context, tx pgx.Tx, banUID uuid.UUID, block ports.PreparedBlock) (bool, error) {
	const methodCtx = "aivector/matcher.PersistBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok || prepared == nil {
		return false, apperrors.New(methodCtx, "неверный тип prepared block AI-vector")
	}

	signature, err := vectorSignature(prepared.vectorFrames, prepared.vectorDim)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	var storedBanUID uuid.UUID
	err = tx.QueryRow(ctx, `
WITH inserted AS (
    INSERT INTO ai_vector_ban (
        ban_uid,
        chat_id,
        file_unique_id,
        media_type,
        model_name,
        model_revision,
        vector_dim,
        frames_count,
        active,
        vector_signature
    )
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9)
    ON CONFLICT (chat_id, model_name, model_revision, vector_signature) DO NOTHING
    RETURNING id, ban_uid
)
SELECT id, ban_uid FROM inserted
UNION ALL
SELECT id, ban_uid FROM ai_vector_ban
WHERE chat_id = $2
  AND model_name = $5
  AND model_revision = $6
  AND vector_signature = $9
LIMIT 1
`,
		banUID,
		prepared.chatID,
		prepared.fileUniqueID,
		string(prepared.mediaType),
		prepared.modelName,
		prepared.modelRevision,
		prepared.vectorDim,
		len(prepared.vectorFrames),
		signature,
	).Scan(&prepared.banID, &storedBanUID)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	created := storedBanUID == banUID

	frameIndexes := make([]int32, 0, len(prepared.vectorFrames))
	positionMillis := make([]int32, 0, len(prepared.vectorFrames))
	vectorBlobs := make([][]byte, 0, len(prepared.vectorFrames))

	for _, frame := range prepared.vectorFrames {
		blob, err := vectorBlob(frame.Vector, prepared.vectorDim)
		if err != nil {
			return false, apperrors.Wrap(methodCtx, err)
		}

		frameIndexes = append(frameIndexes, int32(frame.FrameIndex))
		positionMillis = append(positionMillis, int32(frame.PositionMillis))
		vectorBlobs = append(vectorBlobs, blob)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO ai_vector_frame (
    ban_id,
    frame_index,
    position_millis,
    vector_blob
)
SELECT
    $1,
    frame_index,
    position_millis,
    vector_blob
FROM unnest(
    $2::int[],
    $3::int[],
    $4::bytea[]
) AS f(frame_index, position_millis, vector_blob)
ON CONFLICT (ban_id, frame_index) DO NOTHING
`,
		prepared.banID,
		frameIndexes,
		positionMillis,
		vectorBlobs,
	)
	if err != nil {
		return false, apperrors.Wrap(methodCtx, err)
	}

	return created, nil
}

func (m *matcher) ApplyBlock(_ context.Context, block ports.PreparedBlock) error {
	const methodCtx = "aivector/matcher.ApplyBlock"

	prepared, ok := block.(*preparedBlock)
	if !ok {
		return apperrors.New(methodCtx, "неверный тип prepared block AI-vector")
	}
	if prepared.banID <= 0 {
		return apperrors.New(methodCtx, "AI-vector ban_id не получен после сохранения")
	}

	return nil
}

func vectorSignature(frames []preparedVectorFrame, dimension int) (string, error) {
	digest := sha256.New()
	for _, frame := range frames {
		var frameIndex [4]byte
		var positionMillis [8]byte
		binary.BigEndian.PutUint32(frameIndex[:], uint32(int32(frame.FrameIndex)))
		binary.BigEndian.PutUint64(positionMillis[:], uint64(int64(frame.PositionMillis)))
		digest.Write(frameIndex[:])
		digest.Write(positionMillis[:])

		blob, err := vectorBlob(frame.Vector, dimension)
		if err != nil {
			return "", err
		}
		digest.Write(blob)
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

func vectorBlob(vector []float32, dimension int) ([]byte, error) {
	const methodCtx = "aivector/vectorBlob"

	if len(vector) != dimension {
		return nil, apperrors.New(methodCtx, "размерность AI-vector не совпадает")
	}

	blob := make([]byte, 0, len(vector)*4)
	for _, value := range vector {
		var raw [4]byte
		binary.LittleEndian.PutUint32(raw[:], math.Float32bits(value))
		blob = append(blob, raw[:]...)
	}

	return blob, nil
}

func isVideoFile(filePath string) bool {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".mp4", ".webm", ".mov", ".mkv":
		return true
	default:
		return false
	}
}

func isWebMFile(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".webm")
}

var _ ports.ContentBlockMatcher = (*matcher)(nil)
