package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type FFProbeInfo struct {
	Available       bool     `json:"available"`
	Path            string   `json:"path,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	Width           *int     `json:"width,omitempty"`
	Height          *int     `json:"height,omitempty"`
	Codec           string   `json:"codec,omitempty"`
	AudioCodec      string   `json:"audio_codec,omitempty"`
	Container       string   `json:"container,omitempty"`
	BitrateBPS      *int64   `json:"bitrate_bps,omitempty"`
	FrameRate       *float64 `json:"frame_rate,omitempty"`
	SampleRateHz    *int     `json:"sample_rate_hz,omitempty"`
	Channels        *int     `json:"channels,omitempty"`
	HasVideo        bool     `json:"has_video,omitempty"`
	HasAudio        bool     `json:"has_audio,omitempty"`
}

func DetectFFProbe() FFProbeInfo {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return FFProbeInfo{Available: false}
	}
	return FFProbeInfo{Available: true, Path: path}
}

func ProbeVideo(ctx context.Context, path string) (FFProbeInfo, error) {
	return ProbeMedia(ctx, path)
}

func ProbeMedia(ctx context.Context, path string) (FFProbeInfo, error) {
	info := DetectFFProbe()
	if !info.Available {
		return info, fmt.Errorf("ffprobe not found")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, info.Path,
		"-v", "error",
		"-show_entries", "format=duration,format_name,bit_rate:stream=codec_name,codec_type,width,height,avg_frame_rate,sample_rate,channels",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return info, err
	}
	var parsed struct {
		Streams []struct {
			CodecType    string `json:"codec_type"`
			CodecName    string `json:"codec_name"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgFrameRate string `json:"avg_frame_rate"`
			SampleRate   string `json:"sample_rate"`
			Channels     int    `json:"channels"`
		} `json:"streams"`
		Format struct {
			Duration   string `json:"duration"`
			FormatName string `json:"format_name"`
			BitRate    string `json:"bit_rate"`
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
	info.Container = parsed.Format.FormatName
	if parsed.Format.BitRate != "" {
		if value, err := strconv.ParseInt(parsed.Format.BitRate, 10, 64); err == nil {
			info.BitrateBPS = &value
		}
	}
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			info.HasVideo = true
		case "audio":
			info.HasAudio = true
		}
		if stream.Width > 0 && stream.Height > 0 && info.Width == nil {
			width := stream.Width
			height := stream.Height
			info.Width = &width
			info.Height = &height
			info.Codec = stream.CodecName
			if frameRate, ok := parseFrameRate(stream.AvgFrameRate); ok {
				info.FrameRate = &frameRate
			}
		}
		if (stream.CodecType == "audio" || stream.SampleRate != "" || stream.Channels > 0) && info.AudioCodec == "" {
			info.HasAudio = true
			info.AudioCodec = stream.CodecName
			if stream.SampleRate != "" {
				if value, err := strconv.Atoi(stream.SampleRate); err == nil {
					info.SampleRateHz = &value
				}
			}
			if stream.Channels > 0 {
				channels := stream.Channels
				info.Channels = &channels
			}
		}
	}
	return info, nil
}

func parseFrameRate(raw string) (float64, bool) {
	if raw == "" || raw == "0/0" {
		return 0, false
	}
	if num, den, ok := strings.Cut(raw, "/"); ok {
		numerator, err1 := strconv.ParseFloat(num, 64)
		denominator, err2 := strconv.ParseFloat(den, 64)
		if err1 == nil && err2 == nil && denominator != 0 {
			return numerator / denominator, true
		}
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil
}
