package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type FFProbeInfo struct {
	Available       bool     `json:"available"`
	Path            string   `json:"path,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	Width           *int     `json:"width,omitempty"`
	Height          *int     `json:"height,omitempty"`
}

func DetectFFProbe() FFProbeInfo {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return FFProbeInfo{Available: false}
	}
	return FFProbeInfo{Available: true, Path: path}
}

func ProbeVideo(ctx context.Context, path string) (FFProbeInfo, error) {
	info := DetectFFProbe()
	if !info.Available {
		return info, fmt.Errorf("ffprobe not found")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, info.Path,
		"-v", "error",
		"-show_entries", "format=duration:stream=width,height",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return info, err
	}
	var parsed struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return info, err
	}
	if parsed.Format.Duration != "" {
		if value, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil {
			info.DurationSeconds = &value
		}
	}
	for _, stream := range parsed.Streams {
		if stream.Width > 0 && stream.Height > 0 {
			width := stream.Width
			height := stream.Height
			info.Width = &width
			info.Height = &height
			break
		}
	}
	return info, nil
}
