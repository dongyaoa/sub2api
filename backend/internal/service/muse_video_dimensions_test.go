//go:build unit

package service

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMuseVideoTargetDimensions(t *testing.T) {
	for _, tc := range []struct {
		ratio  string
		target videoDimensions
	}{
		{"16:9", videoDimensions{1920, 1080}},
		{"9:16", videoDimensions{1080, 1920}},
		{"1:1", videoDimensions{1080, 1080}},
		{"4:3", videoDimensions{1440, 1080}},
		{"3:4", videoDimensions{1080, 1440}},
		{"3:2", videoDimensions{1620, 1080}},
		{"2:3", videoDimensions{1080, 1620}},
		{"1080x1920", videoDimensions{1080, 1920}},
		{"", videoDimensions{}},
	} {
		target, err := museVideoTargetDimensions("1080p", tc.ratio)
		require.NoError(t, err)
		require.Equal(t, tc.target, target, tc.ratio)
	}
	for _, ratio := range []string{"0:0", "1:0", "0:1", "1:99999999"} {
		_, err := museVideoTargetDimensions("720p", ratio)
		require.Error(t, err, ratio)
	}
}

func TestMuseVideoContentOutputDimensions(t *testing.T) {
	ffmpeg := os.Getenv("VIDEO_FFMPEG_PATH")
	if ffmpeg == "" {
		var err error
		ffmpeg, err = exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("FFmpeg is required to test video output dimensions")
		}
	}
	input := filepath.Join(t.TempDir(), "source.mp4")
	// 160x120 with non-square pixels displays as 2:1. The source does not
	// match the requested portrait frame, as can happen with Muse ratio hints.
	output, err := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "color=red:s=160x120,setsar=3/2",
		"-frames:v", "1", "-c:v", "libx264", input).CombinedOutput()
	require.NoError(t, err, string(output))
	source, err := os.ReadFile(input)
	require.NoError(t, err)

	for _, tc := range []struct {
		name, resolution, aspect string
		width, height            int
	}{
		{"auto", "auto", "9:16", 160, 120},
		{"portrait", "1080p", "9:16", 1080, 1920},
		{"landscape", "720p", "16:9", 1280, 720},
		{"square", "720p", "1:1", 720, 720},
		{"source display aspect", "720p", "", 1440, 720},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := VideoTaskOwner{UserID: 1, APIKeyID: 2}
			storage := &recordingVideoStorage{}
			uploader := NewImageResultUploader(storage, "images/", 0, nil)
			tasks := NewVideoTaskService(newMemoryVideoTaskStore(), func() (*ImageResultUploader, bool) { return uploader, true })
			metadata := VideoTaskMetadata{Model: "muse-video", Resolution: tc.resolution, AspectRatio: tc.aspect}
			require.NoError(t, tasks.RecordSubmission(context.Background(), owner, 3, 4, "task_muse", metadata, nil))
			record, err := tasks.GetRecord(context.Background(), owner, "task_muse")
			require.NoError(t, err)
			ctx := WithMuseVideoOutput(context.Background(), record.Metadata)
			ctx = WithGrokVideoContentPersister(ctx, func(ctx context.Context, id, mime string, data []byte) error {
				return tasks.StoreContent(ctx, owner, id, mime, data)
			})
			upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
				grokMediaContentStatusResponse(`{"id":"task_muse","status":"succeeded","result":{"url":"/v1/media/vid_generated.mp4"}}`),
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(bytes.NewReader(source))},
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			c, response := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse/content", nil)
			_, err = svc.ForwardMuseVideo(ctx, c, museVideoTestAccount(), GrokMediaEndpointVideoContent, "task_muse", nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "video/mp4", storage.contentType)
			require.Equal(t, storage.data, response.Body.Bytes())

			// Independently decode the delivered bytes as PNG to check the real
			// raster, including the padding when Muse produced a different shape.
			path := filepath.Join(t.TempDir(), "delivered.mp4")
			require.NoError(t, os.WriteFile(path, response.Body.Bytes(), 0o600))
			frame, err := exec.Command(ffmpeg, "-loglevel", "error", "-i", path, "-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "-").Output()
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(frame))
			require.NoError(t, err)
			require.Equal(t, tc.width, img.Bounds().Dx())
			require.Equal(t, tc.height, img.Bounds().Dy())
			if tc.name == "portrait" {
				red, _, _, _ := img.At(tc.width/2, tc.height/2).RGBA()
				require.Greater(t, red, uint32(0xf000))
				red, _, _, _ = img.At(tc.width/2, 5).RGBA()
				require.Less(t, red, uint32(0x0800))
			}
		})
	}
}
