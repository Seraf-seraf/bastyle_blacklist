package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
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
	if binary == "" {
		return nil, errors.New("ffmpeg frame extractor binary is not configured")
	}
	if timeout <= 0 {
		return nil, errors.New("ffmpeg frame extractor timeout must be positive")
	}

	return &ffmpegFrameExtractor{
		binary:  binary,
		timeout: timeout,
	}, nil
}

func (e *ffmpegFrameExtractor) Extract(ctx context.Context, media domain.MediaFile, plan domain.MediaExtractionPlan) (domain.ExtractedMedia, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExtractedMedia{}, err
	}
	if err := validateFFmpegPlan(plan); err != nil {
		return domain.ExtractedMedia{}, err
	}
	if len(media.Data) == 0 {
		return domain.ExtractedMedia{}, errors.New("ffmpeg frame extractor media data is empty")
	}

	tempDir, err := os.MkdirTemp("", "bastyle-frames-*")
	if err != nil {
		return domain.ExtractedMedia{}, err
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input"+mediaFileExtension(media.FilePath))
	if err := os.WriteFile(inputPath, media.Data, 0600); err != nil {
		return domain.ExtractedMedia{}, err
	}

	framePattern := filepath.Join(tempDir, "frame-%03d.png")
	if err := e.runFFmpeg(ctx, inputPath, framePattern, plan); err != nil {
		return domain.ExtractedMedia{}, err
	}

	frames, err := readFrameImages(tempDir, plan.MaxFrames)
	if err != nil {
		return domain.ExtractedMedia{}, err
	}
	if len(frames) == 0 {
		return domain.ExtractedMedia{}, errors.New("ffmpeg frame extractor returned no frames")
	}

	return domain.ExtractedMedia{Frames: frames}, nil
}

func (e *ffmpegFrameExtractor) runFFmpeg(ctx context.Context, inputPath string, framePattern string, plan domain.MediaExtractionPlan) error {
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
			return ffmpegCtx.Err()
		}

		return err
	}

	return nil
}

func validateFFmpegPlan(plan domain.MediaExtractionPlan) error {
	if plan.MaxFrames <= 0 {
		return errors.New("ffmpeg frame extractor max frames must be positive")
	}
	if plan.MaxFrames > maxFrameExtractionFrames {
		return errors.New("ffmpeg frame extractor max frames is too large")
	}
	if plan.TargetWidth <= 0 {
		return errors.New("ffmpeg frame extractor target width must be positive")
	}
	if plan.TargetWidth > maxFrameExtractionDimension {
		return errors.New("ffmpeg frame extractor target width is too large")
	}
	if plan.TargetHeight <= 0 {
		return errors.New("ffmpeg frame extractor target height must be positive")
	}
	if plan.TargetHeight > maxFrameExtractionDimension {
		return errors.New("ffmpeg frame extractor target height is too large")
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
	paths, err := filepath.Glob(filepath.Join(dir, "frame-*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) > maxFrames {
		paths = paths[:maxFrames]
	}

	frames := make([]domain.ExtractedFrame, 0, len(paths))
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}

		img, err := decodeBoundedImage(file)
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}

		frames = append(frames, domain.ExtractedFrame{Image: img})
	}

	return frames, nil
}
