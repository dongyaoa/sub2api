package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxStoredVideoBytes          int64 = 512 << 20
	defaultVideoTranscodeTimeout       = 5 * time.Minute
)

type browserVideoTranscoder func(ctx context.Context, contentType string, data []byte) ([]byte, error)

type grokVideoContentPersisterContextKey struct{}

type GrokVideoContentPersister func(ctx context.Context, requestID, contentType string, data []byte) error

func WithGrokVideoContentPersister(ctx context.Context, persist GrokVideoContentPersister) context.Context {
	if persist == nil {
		return ctx
	}
	return context.WithValue(ctx, grokVideoContentPersisterContextKey{}, persist)
}

func grokVideoContentPersisterFromContext(ctx context.Context) GrokVideoContentPersister {
	if ctx == nil {
		return nil
	}
	persist, _ := ctx.Value(grokVideoContentPersisterContextKey{}).(GrokVideoContentPersister)
	return persist
}

func ensureBrowserPlayableVideo(ctx context.Context, declaredContentType string, data []byte) ([]byte, string, error) {
	return prepareBrowserPlayableVideo(ctx, declaredContentType, data, transcodeVideoWithFFmpeg)
}

func prepareBrowserPlayableVideo(
	ctx context.Context,
	declaredContentType string,
	data []byte,
	transcode browserVideoTranscoder,
) ([]byte, string, error) {
	contentType, _, err := normalizeGeneratedVideoContent(declaredContentType, data)
	if err != nil {
		return nil, "", err
	}
	// MP4 sample-entry tags alone cannot prove browser decodability (for
	// example, 10-bit H.264 can still carry avc1). Canonicalize every upstream
	// file once, then use the persisted playback format version on later reads.
	if transcode == nil {
		return nil, "", errors.New("video requires browser-compatible transcoding")
	}
	converted, err := transcode(ctx, contentType, data)
	if err != nil {
		return nil, "", err
	}
	if int64(len(converted)) > maxStoredVideoBytes {
		return nil, "", fmt.Errorf("transcoded video exceeds %d bytes", maxStoredVideoBytes)
	}
	convertedType, _, err := normalizeGeneratedVideoContent("video/mp4", converted)
	if err != nil {
		return nil, "", fmt.Errorf("validate transcoded video: %w", err)
	}
	if convertedType != "video/mp4" || !isBrowserCompatibleMP4(converted) {
		return nil, "", errors.New("transcoder did not produce a browser-compatible H.264 MP4")
	}
	return converted, "video/mp4", nil
}

func isBrowserCompatibleMP4(data []byte) bool {
	if !bytes.Contains(data, []byte("avc1")) && !bytes.Contains(data, []byte("avc3")) {
		return false
	}
	for _, codec := range [][]byte{
		[]byte("hvc1"), []byte("hev1"), []byte("dvh1"), []byte("dvhe"),
		[]byte("av01"), []byte("vp09"), []byte("ac-3"), []byte("ec-3"),
		[]byte("Opus"),
	} {
		if bytes.Contains(data, codec) {
			return false
		}
	}
	return true
}

func transcodeVideoWithFFmpeg(ctx context.Context, contentType string, data []byte) ([]byte, error) {
	ffmpegPath := strings.TrimSpace(os.Getenv("VIDEO_FFMPEG_PATH"))
	if ffmpegPath == "" {
		var err error
		ffmpegPath, err = exec.LookPath("ffmpeg")
		if err != nil {
			return nil, errors.New("ffmpeg is required to convert this video for browser playback")
		}
	}

	tempDir, err := os.MkdirTemp("", "sub2api-video-")
	if err != nil {
		return nil, fmt.Errorf("create video transcode directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	inputExtension := ".mp4"
	if contentType == "video/webm" {
		inputExtension = ".webm"
	}
	inputPath := filepath.Join(tempDir, "input"+inputExtension)
	outputPath := filepath.Join(tempDir, "output.mp4")
	if err := os.WriteFile(inputPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("write video transcode input: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, defaultVideoTranscodeTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inputPath,
		"-map", "0:v:0", "-map", "0:a?", "-sn", "-dn",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-pix_fmt", "yuv420p", "-profile:v", "high", "-tag:v", "avc1",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", "-max_muxing_queue_size", "1024",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("transcode video: %w", runCtx.Err())
		}
		detail := strings.TrimSpace(string(output))
		if len(detail) > 2048 {
			detail = detail[len(detail)-2048:]
		}
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("transcode video with ffmpeg: %s", detail)
	}
	converted, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read transcoded video: %w", err)
	}
	if len(converted) == 0 {
		return nil, errors.New("ffmpeg produced an empty video")
	}
	return converted, nil
}

// StoreVideo validates generated video bytes and stores them under the image
// storage prefix's videos/ subdirectory, reusing the existing R2/S3 settings.
func (u *ImageResultUploader) StoreVideo(ctx context.Context, taskID, declaredContentType string, data []byte) (string, string, error) {
	if u == nil || u.storage == nil {
		return "", "", errors.New("video object storage is unavailable")
	}
	if len(data) == 0 {
		return "", "", errors.New("upstream returned an empty video")
	}
	if int64(len(data)) > maxStoredVideoBytes {
		return "", "", fmt.Errorf("video exceeds %d bytes", maxStoredVideoBytes)
	}
	contentType, extension, err := normalizeGeneratedVideoContent(declaredContentType, data)
	if err != nil {
		return "", "", err
	}
	key := u.prefix + "videos/" + sanitizeVideoObjectName(taskID) + extension
	url, err := u.storage.Save(ctx, key, contentType, data)
	if err != nil {
		return "", "", fmt.Errorf("upload video to object storage: %w", err)
	}
	return url, contentType, nil
}

func normalizeGeneratedVideoContent(declaredContentType string, data []byte) (string, string, error) {
	declaredContentType = strings.ToLower(strings.TrimSpace(strings.Split(declaredContentType, ";")[0]))
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
		return "video/mp4", ".mp4", nil
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) && (declaredContentType == "video/webm" || detected == "video/webm") {
		return "video/webm", ".webm", nil
	}
	return "", "", fmt.Errorf("upstream returned unsupported video content (declared %q, detected %q)", declaredContentType, detected)
}

func sanitizeVideoObjectName(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			_, _ = out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "video"
	}
	return out.String()
}
