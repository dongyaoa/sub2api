package service

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type videoDimensions struct {
	width, height int
}

var museNumericAspectRatio = regexp.MustCompile(`^([0-9]+)\s*[:xX×]\s*([0-9]+)$`)

// A fixed tier sets the short edge. An omitted aspect ratio uses the source
// video's display aspect ratio after FFmpeg applies rotation and pixel aspect.
func museVideoTargetDimensions(resolution, aspectRatio string) (videoDimensions, error) {
	switch resolution {
	case "", "auto":
		return videoDimensions{}, nil
	case "720p", "1080p":
	default:
		return videoDimensions{}, fmt.Errorf("Muse output resolution must be auto, 720p or 1080p")
	}
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" || strings.EqualFold(aspectRatio, "auto") {
		return videoDimensions{}, nil
	}
	parts := museNumericAspectRatio.FindStringSubmatch(aspectRatio)
	if parts == nil {
		return videoDimensions{}, fmt.Errorf("fixed Muse output requires a numeric aspect ratio or size, such as 9:16 or 1080x1920")
	}
	width, _ := strconv.ParseFloat(parts[1], 64)
	height, _ := strconv.ParseFloat(parts[2], 64)
	return museDimensionsForRatio(resolution, width/height)
}

func museDimensionsForRatio(resolution string, ratio float64) (videoDimensions, error) {
	shortEdge := 720
	if resolution == "1080p" {
		shortEdge = 1080
	}
	// H.264 with yuv420p requires even dimensions. Bound extreme ratios
	// before converting to int or asking FFmpeg to allocate the canvas.
	longEdge := 2 * math.Round(float64(shortEdge)*math.Max(ratio, 1/ratio)/2)
	if !(ratio > 0 && longEdge <= 8192) {
		return videoDimensions{}, fmt.Errorf("Muse output aspect ratio must give positive dimensions no larger than 8192 pixels")
	}
	if ratio < 1 {
		return videoDimensions{shortEdge, int(longEdge)}, nil
	}
	return videoDimensions{int(longEdge), shortEdge}, nil
}

func (d videoDimensions) scaleFilter() string {
	// Fit the displayed picture inside square-pixel output, including sources
	// with non-square pixels. Pad symmetrically if Muse ignored the ratio hint.
	return fmt.Sprintf("scale=w='max(2,round(min(%d,%d*dar)/2)*2)':h='max(2,round(min(%d,%d/dar)/2)*2)',setsar=1,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		d.width, d.height, d.height, d.width, d.width, d.height)
}

// The framecrc muxer reports dimensions and sample aspect ratio of the first
// decoded frame. It ships with FFmpeg, including single-binary installations.
func readVideoFrameDimensions(ctx context.Context, ffmpegPath, path string, autorotate bool) (videoDimensions, float64, error) {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if !autorotate {
		args = append(args, "-noautorotate")
	}
	args = append(args, "-i", path, "-map", "0:v:0", "-frames:v", "1", "-f", "framecrc", "-")
	//nolint:gosec // The operator configures ffmpegPath and path is a private temporary file.
	out, err := exec.CommandContext(ctx, ffmpegPath, args...).Output()
	if err != nil {
		return videoDimensions{}, 0, fmt.Errorf("inspect video frame with ffmpeg: %w", err)
	}
	var d videoDimensions
	var sarWidth, sarHeight int
	hasFrame := false
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "#dimensions 0:"):
			_, _ = fmt.Sscanf(line, "#dimensions 0: %dx%d", &d.width, &d.height)
		case strings.HasPrefix(line, "#sar 0:"):
			_, _ = fmt.Sscanf(line, "#sar 0: %d/%d", &sarWidth, &sarHeight)
		case strings.HasPrefix(line, "0,"):
			hasFrame = true
		}
	}
	if !hasFrame || d.width <= 0 || d.height <= 0 {
		return videoDimensions{}, 0, fmt.Errorf("ffmpeg could not determine the video frame dimensions")
	}
	pixelAspect := 1.0
	if sarWidth > 0 && sarHeight > 0 {
		pixelAspect = float64(sarWidth) / float64(sarHeight)
	}
	return d, pixelAspect, nil
}
