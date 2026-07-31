package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const maxStoredVideoBytes int64 = 512 << 20

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
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "video"
	}
	return out.String()
}
