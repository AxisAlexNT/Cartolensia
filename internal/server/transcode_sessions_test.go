package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/transcoding"
)

func TestHLSArgsProfiles(t *testing.T) {
	dir := t.TempDir()
	args, err := hlsArgs("h264_low_bitrate", "/tmp/input.mp4", dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, arg := range args {
		joined += arg + " "
	}
	if !strings.Contains(joined, "libx264") || !strings.Contains(joined, "master.m3u8") || !strings.Contains(joined, "hls") {
		t.Fatalf("unexpected hls args: %s", joined)
	}
	if _, err := hlsArgs("unknown", "/tmp/input.mp4", dir); err == nil {
		t.Fatal("expected unsupported profile error")
	}
}

func TestHLSReadyRequiresPlaylistAndSegment(t *testing.T) {
	dir := t.TempDir()
	if ready, _ := hlsReady(dir, 10); ready {
		t.Fatal("empty directory should not be ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, _ := hlsReady(dir, 10); ready {
		t.Fatal("playlist without segment should not be ready")
	}
	if err := os.WriteFile(filepath.Join(dir, "segment_00000.ts"), []byte("segment"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, _ := hlsReady(dir, 10); ready {
		t.Fatal("playlist and short unfinished segment should wait for ready threshold")
	}
	playlist := "#EXTM3U\n#EXTINF:2.000,\nsegment_00000.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, seconds := hlsReady(dir, 10); !ready || seconds != 2 {
		t.Fatalf("finished short playlist should be ready, ready=%v seconds=%v", ready, seconds)
	}
}

func TestTranscodePresetValidationAndNVENCArgs(t *testing.T) {
	preset := catalog.TranscodingPreset{
		ID:             "nv-test",
		Name:           "NVENC test",
		Hardware:       "nvidia",
		Codec:          "h264",
		FFmpegEncoder:  "h264_nvenc",
		Mode:           "bitrate",
		ParameterValue: "750",
		Container:      "hls",
	}
	if warnings := transcodePresetWarnings(preset, transcoding.Capabilities{}); len(warnings) == 0 {
		t.Fatal("expected bare numeric bitrate warning")
	}
	args, err := videoArgsForPreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"h264_nvenc", "-preset p5", "-b:v 750k", "-maxrate 750k", "-bufsize 1500k"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
	if got := safeBitrateParameter("1500k", "750k"); got != "1500k" {
		t.Fatalf("unexpected explicit bitrate %q", got)
	}
	if got := safeBitrateParameter("1500", "750k"); got != "1500k" {
		t.Fatalf("unexpected normalized bitrate %q", got)
	}
	av1 := preset
	av1.Codec = "av1"
	av1.FFmpegEncoder = "av1_nvenc"
	if warnings := transcodePresetWarnings(av1, transcoding.Capabilities{}); len(warnings) == 0 {
		t.Fatal("expected RTX 3090 Ti AV1 warning")
	}
	if _, err := hlsArgsForPreset(av1, "/tmp/input.mp4", t.TempDir()); err == nil || !strings.Contains(err.Error(), "AV1 live HLS playback is disabled") {
		t.Fatalf("expected AV1 HLS disabled error, got %v", err)
	}
}

func TestBuiltInTranscodingPresetsExposeGPUWhenAvailable(t *testing.T) {
	caps := transcoding.Capabilities{
		FFmpeg: transcoding.ToolInfo{Available: true},
		Hardware: transcoding.HardwareHints{
			NvidiaSMI: true,
			VAAPI:     true,
			DevDRI:    true,
		},
		Encoders: []transcoding.Encoder{
			{Name: "libx264"},
			{Name: "libsvtav1"},
			{Name: "h264_nvenc"},
			{Name: "h264_vaapi"},
			{Name: "av1_nvenc"},
			{Name: "av1_vaapi"},
		},
	}
	presets := builtInTranscodingPresets(caps)
	byID := map[string]catalog.TranscodingPreset{}
	for _, preset := range presets {
		byID[preset.ID] = preset
	}
	for _, id := range []string{"h264_nvenc_720p_lan", "h264_vaapi_720p_lan", "av1_nvenc_low_bitrate", "av1_vaapi_low_bitrate"} {
		if !byID[id].Available {
			t.Fatalf("expected %s to be available in %#v", id, byID[id])
		}
	}
}

func TestBuiltInTranscodingPresetExactResolvesAdvertisedProfiles(t *testing.T) {
	caps := transcoding.Capabilities{
		FFmpeg: transcoding.ToolInfo{Available: true},
		Hardware: transcoding.HardwareHints{
			NvidiaSMI: true,
			VAAPI:     true,
			DevDRI:    true,
		},
		Encoders: []transcoding.Encoder{
			{Name: "libx264"},
			{Name: "h264_nvenc"},
			{Name: "h264_vaapi"},
		},
	}
	for _, id := range []string{"h264_720p_lan", "h264_nvenc_720p_lan", "h264_vaapi_720p_lan"} {
		preset, ok := builtInPresetExact(id, caps)
		if !ok {
			t.Fatalf("expected exact built-in preset %s", id)
		}
		if _, err := transcodeArgsForPreset(preset, "/tmp/input.mp4", t.TempDir()); err != nil {
			t.Fatalf("expected advertised preset %s to produce args: %v", id, err)
		}
	}
	if _, ok := builtInPresetExact("does_not_exist", caps); ok {
		t.Fatal("unknown profile should not resolve as a built-in preset")
	}
}

func TestVAAPITranscodeArgsIncludeDeviceAndUploadFilter(t *testing.T) {
	t.Setenv("CARTOLENSIA_VAAPI_DEVICE", "/dev/dri/renderD128")
	preset := catalog.TranscodingPreset{
		ID:             "vaapi-test",
		Name:           "VAAPI test",
		Hardware:       "vaapi",
		Codec:          "h264",
		FFmpegEncoder:  "h264_vaapi",
		Mode:           "quantizer",
		ParameterValue: "24",
		Container:      "hls",
	}
	args, err := hlsArgsForPreset(preset, "/tmp/input.mp4", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-vaapi_device", "/dev/dri/renderD128", "h264_vaapi", "format=nv12,hwupload"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
	if strings.Contains(joined, "-pix_fmt yuv420p") {
		t.Fatalf("VAAPI command should not force software pix_fmt: %s", joined)
	}
	dryRunArgs, err := ffmpegDryRunArgs(preset, "/tmp/input.mp4", 1)
	if err != nil {
		t.Fatal(err)
	}
	joinedDryRun := strings.Join(dryRunArgs, " ")
	for _, want := range []string{"-vaapi_device", "/dev/dri/renderD128", "h264_vaapi", "format=nv12,hwupload"} {
		if !strings.Contains(joinedDryRun, want) {
			t.Fatalf("expected %q in dry-run args %q", want, joinedDryRun)
		}
	}
	if strings.Contains(joinedDryRun, "-pix_fmt yuv420p") {
		t.Fatalf("VAAPI dry-run command should not force software pix_fmt: %s", joinedDryRun)
	}
}
