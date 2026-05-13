package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

const (
	maxFrameExtractionFrames    = 20
	maxFrameExtractionDimension = 1024
)

type ffmpegFrameExtractor struct {
	binary  string
	timeout time.Duration
}

func NewFFmpegFrameExtractor(binary string, timeout time.Duration) (ports.MediaExtractor, error) {
	const methodCtx = "media/NewFFmpegFrameExtractor"

	if binary == "" {
		return nil, apperrors.New(methodCtx, "бинарный файл ffmpeg для извлечения кадров не настроен")
	}
	if timeout <= 0 {
		return nil, apperrors.New(methodCtx, "таймаут извлечения кадров ffmpeg должен быть положительным")
	}

	return &ffmpegFrameExtractor{
		binary:  binary,
		timeout: timeout,
	}, nil
}

func (e *ffmpegFrameExtractor) Extract(ctx context.Context, media domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	const methodCtx = "media/ffmpegFrameExtractor.Extract"

	if err := ctx.Err(); err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}
	if err := validateFFmpegPlan(plan); err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}
	if len(media.Data) == 0 {
		return domain.ExtractedMedia{}, apperrors.New(methodCtx, "данные медиа для извлечения кадров ffmpeg пустые")
	}

	tempDir, err := os.MkdirTemp("", "bastyle-frames-*")
	if err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input"+mediaFileExtension(media.FilePath))
	if err := os.WriteFile(inputPath, media.Data, 0600); err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}

	framePattern := filepath.Join(tempDir, "frame-%03d.png")
	if err := e.runFFmpeg(ctx, inputPath, framePattern, plan); err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}

	frames, err := readFrameImages(tempDir, plan.MaxFrames)
	if err != nil {
		return domain.ExtractedMedia{}, apperrors.Wrap(methodCtx, err)
	}
	if len(frames) == 0 {
		return domain.ExtractedMedia{}, apperrors.New(methodCtx, "извлечение кадров ffmpeg не вернуло кадров")
	}

	return domain.ExtractedMedia{Frames: frames}, nil
}

func (e *ffmpegFrameExtractor) runFFmpeg(ctx context.Context, inputPath string, framePattern string, plan domain.MediaExtractionPlan) error {
	const methodCtx = "media/ffmpegFrameExtractor.runFFmpeg"

	ffmpegCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-y",
		"-i", inputPath,
		"-vf", videoFrameFilter(plan),
		"-frames:v", strconv.Itoa(plan.MaxFrames),
		framePattern,
	}

	cmd := exec.CommandContext(ffmpegCtx, e.binary, args...)

	if err := cmd.Run(); err != nil {
		if ffmpegCtx.Err() != nil {
			return apperrors.Wrap(methodCtx, ffmpegCtx.Err())
		}

		return apperrors.Wrap(methodCtx, err)
	}

	return nil
}

func validateFFmpegPlan(plan domain.MediaExtractionPlan) error {
	const methodCtx = "media/validateFFmpegPlan"

	if plan.MaxFrames <= 0 {
		return apperrors.New(methodCtx, "максимальное количество кадров ffmpeg должно быть положительным")
	}
	if plan.MaxFrames > maxFrameExtractionFrames {
		return apperrors.New(methodCtx, "максимальное количество кадров ffmpeg слишком большое")
	}
	if plan.TargetWidth <= 0 {
		return apperrors.New(methodCtx, "целевая ширина кадра ffmpeg должна быть положительной")
	}
	if plan.TargetWidth > maxFrameExtractionDimension {
		return apperrors.New(methodCtx, "целевая ширина кадра ffmpeg слишком большая")
	}
	if plan.TargetHeight <= 0 {
		return apperrors.New(methodCtx, "целевая высота кадра ffmpeg должна быть положительной")
	}
	if plan.TargetHeight > maxFrameExtractionDimension {
		return apperrors.New(methodCtx, "целевая высота кадра ffmpeg слишком большая")
	}

	return nil
}

func videoFrameFilter(plan domain.MediaExtractionPlan) string {
	return strings.Join([]string{
		"fps=1",
		"scale=w=" + strconv.Itoa(plan.TargetWidth) + ":h=" + strconv.Itoa(plan.TargetHeight) + ":force_original_aspect_ratio=decrease",
	}, ",")
}

func mediaFileExtension(filePath string) string {
	ext := filepath.Ext(filePath)
	if ext == "" {
		return ".media"
	}

	return ext
}

func readFrameImages(dir string, maxFrames int) ([]domain.ExtractedFrame, error) {
	const methodCtx = "media/readFrameImages"

	paths, err := filepath.Glob(filepath.Join(dir, "frame-*.png"))
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	sort.Strings(paths)
	if len(paths) > maxFrames {
		paths = paths[:maxFrames]
	}

	frames := make([]domain.ExtractedFrame, 0, len(paths))
	for i, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}

		img, err := decodeBoundedImage(file)
		closeErr := file.Close()
		if err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
		if closeErr != nil {
			return nil, apperrors.Wrap(methodCtx, closeErr)
		}

		frames = append(frames, domain.ExtractedFrame{
			Index:          i,
			PositionMillis: i * 1000,
			Image:          img,
		})
	}

	return frames, nil
}
