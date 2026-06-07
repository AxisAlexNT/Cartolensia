package transcoding

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type ToolInfo struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

type Encoder struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CodecFamily string `json:"codec_family,omitempty"`
	Hardware    string `json:"hardware,omitempty"`
}

type HardwareHints struct {
	NvidiaSMI bool `json:"nvidia_smi"`
	DevDRI    bool `json:"dev_dri"`
	VAAPI     bool `json:"vaapi"`
	QSV       bool `json:"qsv"`
}

type Capabilities struct {
	FFmpeg   ToolInfo      `json:"ffmpeg"`
	FFprobe  ToolInfo      `json:"ffprobe"`
	Encoders []Encoder     `json:"encoders"`
	Hardware HardwareHints `json:"hardware"`
	Safety   string        `json:"safety"`
}

func Detect(ctx context.Context) Capabilities {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ffmpeg := detectTool(ctx, "ffmpeg")
	ffprobe := detectTool(ctx, "ffprobe")
	var encoders []Encoder
	if ffmpeg.Available {
		output, err := exec.CommandContext(ctx, ffmpeg.Path, "-hide_banner", "-encoders").CombinedOutput()
		if err == nil {
			encoders = ParseEncoders(string(output))
		}
	}
	return Capabilities{
		FFmpeg:   ffmpeg,
		FFprobe:  ffprobe,
		Encoders: encoders,
		Hardware: HardwareHints{
			NvidiaSMI: lookPath("nvidia-smi"),
			DevDRI:    exists("/dev/dri"),
			VAAPI:     hasEncoder(encoders, "vaapi") && exists("/dev/dri"),
			QSV:       hasEncoder(encoders, "qsv") && exists("/dev/dri"),
		},
		Safety: "user-triggered HLS transcode sessions write only to the configured Cartolensia cache; originals remain immutable",
	}
}

func ParseEncoders(output string) []Encoder {
	var out []Encoder
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "Encoders:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		flags := fields[0]
		if len(flags) < 2 || !strings.Contains(flags, "V") {
			continue
		}
		name := fields[1]
		desc := strings.TrimSpace(strings.TrimPrefix(line, flags+" "+name))
		out = append(out, Encoder{Name: name, Description: desc, CodecFamily: codecFamily(name, desc), Hardware: hardwareKind(name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Status(ctx context.Context) map[string]any {
	return map[string]any{
		"capabilities": Detect(ctx),
		"presets": []map[string]any{
			{"id": "archive-h264", "name": "Archive H.264", "implemented": false},
			{"id": "archive-av1", "name": "Archive AV1", "implemented": false},
		},
		"jobs_implemented": false,
		"safety":           "transcoding outputs are future work and must never be written into original storage by default",
	}
}

func detectTool(ctx context.Context, name string) ToolInfo {
	path, err := exec.LookPath(name)
	if err != nil {
		return ToolInfo{Available: false}
	}
	info := ToolInfo{Available: true, Path: path}
	output, err := exec.CommandContext(ctx, path, "-version").Output()
	if err == nil {
		first, _, _ := strings.Cut(string(output), "\n")
		info.Version = strings.TrimSpace(first)
	}
	return info
}

func codecFamily(name, desc string) string {
	text := strings.ToLower(name + " " + desc)
	switch {
	case strings.Contains(text, "av1"):
		return "av1"
	case strings.Contains(text, "hevc") || strings.Contains(text, "h265") || strings.Contains(text, "h.265"):
		return "hevc"
	case strings.Contains(text, "h264") || strings.Contains(text, "h.264"):
		return "h264"
	default:
		return ""
	}
}

func hardwareKind(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "nvenc"):
		return "nvidia_nvenc"
	case strings.Contains(name, "vaapi"):
		return "vaapi"
	case strings.Contains(name, "qsv"):
		return "intel_qsv"
	case strings.Contains(name, "amf"):
		return "amd_amf"
	default:
		return ""
	}
}

func hasEncoder(encoders []Encoder, needle string) bool {
	for _, encoder := range encoders {
		if strings.Contains(strings.ToLower(encoder.Name), needle) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func AcceleratorHints() map[string]any {
	dockerRuntime, dockerErr := dockerNvidiaRuntime()
	gpuName, gpuMemory := nvidiaGPUInfo()
	return map[string]any{
		"goos":                  runtime.GOOS,
		"goarch":                runtime.GOARCH,
		"cpu":                   true,
		"dev_dri":               exists("/dev/dri"),
		"nvidia_smi":            lookPath("nvidia-smi"),
		"nvidia_name":           gpuName,
		"nvidia_memory_mb":      gpuMemory,
		"docker_available":      lookPath("docker"),
		"docker_nvidia_runtime": dockerRuntime,
		"docker_error":          dockerErr,
	}
}

func nvidiaGPUInfo() (string, int) {
	if !lookPath("nvidia-smi") {
		return "", 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return "", 0
	}
	line := strings.TrimSpace(strings.Split(string(output), "\n")[0])
	name, memText, ok := strings.Cut(line, ",")
	if !ok {
		return strings.TrimSpace(line), 0
	}
	mem := 0
	_, _ = fmt.Sscanf(strings.TrimSpace(memText), "%d", &mem)
	return strings.TrimSpace(name), mem
}

func dockerNvidiaRuntime() (bool, string) {
	if !lookPath("docker") {
		return false, "docker command not found"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .Runtimes}}").CombinedOutput()
	if err != nil {
		return false, strings.TrimSpace(string(output))
	}
	if strings.Contains(strings.ToLower(string(output)), "\"nvidia\"") {
		return true, ""
	}
	return false, "nvidia runtime not listed"
}
