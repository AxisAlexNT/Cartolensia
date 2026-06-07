package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/transcoding"
)

type transcodeSession struct {
	ID        string    `json:"id"`
	AssetID   string    `json:"asset_id"`
	Profile   string    `json:"profile"`
	Hardware  string    `json:"hardware,omitempty"`
	Encoder   string    `json:"encoder,omitempty"`
	Dir       string    `json:"dir"`
	Playlist  string    `json:"playlist_url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Error     string    `json:"error,omitempty"`
	Args      []string  `json:"-"`
	cmd       *exec.Cmd
	stderr    *lockedBuffer
}

var transcodeSessions = struct {
	sync.Mutex
	items map[string]*transcodeSession
}{items: map[string]*transcodeSession{}}

type lockedBuffer struct {
	sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	if b.buf.Len() > 64*1024 {
		b.buf.Reset()
	}
	return b.buf.Write(p)
}

func (b *lockedBuffer) Tail() string {
	b.Lock()
	defer b.Unlock()
	text := b.buf.String()
	if len(text) > 4096 {
		return text[len(text)-4096:]
	}
	return text
}

func (s *Server) handleTranscodeSessionStart(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "transcoding.start") {
		return
	}
	var req struct {
		Profile string                     `json:"profile"`
		Preset  *catalog.TranscodingPreset `json:"preset"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		profile = "h264_720p_lan"
	}
	asset, err := s.deps.Store.GetAsset(r.Context(), assetID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if asset.MediaKind != "video" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("transcode sessions are available for video assets only"))
		return
	}
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	input, _, err := s.deps.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	inputPath := input.Name()
	_ = input.Close()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("ffmpeg is not available"))
		return
	}
	sessionID := id.NewUUID()
	sessionDir := filepath.Join(s.deps.Config.Cache.Dir, "transcode", sessionID)
	if err := ensurePathInside(filepath.Join(s.deps.Config.Cache.Dir, "transcode"), sessionDir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	args, resolvedPreset, err := s.hlsArgsForProfile(r.Context(), profile, req.Preset, inputPath, sessionDir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Dir = sessionDir
	cmd.Stdout = nil
	stderr := &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	session := &transcodeSession{
		ID:        sessionID,
		AssetID:   assetID,
		Profile:   profile,
		Hardware:  resolvedPreset.Hardware,
		Encoder:   resolvedPreset.FFmpegEncoder,
		Dir:       sessionDir,
		Playlist:  "/api/v1/media/transcode-sessions/" + sessionID + "/master.m3u8",
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		Args:      args,
		cmd:       cmd,
		stderr:    stderr,
	}
	transcodeSessions.Lock()
	transcodeSessions.items[sessionID] = session
	transcodeSessions.Unlock()
	go func() {
		err := cmd.Wait()
		cancel()
		transcodeSessions.Lock()
		defer transcodeSessions.Unlock()
		if current := transcodeSessions.items[sessionID]; current != nil {
			if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
				current.Status = "failed"
				current.Error = strings.TrimSpace(err.Error() + "\n" + current.stderr.Tail())
			} else if current.Status == "pending" || current.Status == "ready" {
				current.Status = "finished"
			}
		}
	}()
	waitForHLSReady(session, 4*time.Second)
	writeJSON(w, http.StatusAccepted, publicTranscodeSession(session))
}

func (s *Server) handleTranscodeSession(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	sessionID := parts[1]
	transcodeSessions.Lock()
	session := transcodeSessions.items[sessionID]
	transcodeSessions.Unlock()
	if session == nil {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 2 {
		s.stopTranscodeSession(w, sessionID, session)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 3 && parts[2] == "stop" {
		s.stopTranscodeSession(w, sessionID, session)
		return
	}
	if len(parts) == 3 && parts[2] == "status" {
		updateTranscodeReadiness(session)
		writeJSON(w, http.StatusOK, publicTranscodeSession(session))
		return
	}
	if len(parts) == 3 && parts[2] == "master.m3u8" {
		updateTranscodeReadiness(session)
		if session.Status == "pending" {
			writeError(w, http.StatusAccepted, fmt.Errorf("transcode playlist is not ready yet"))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		http.ServeFile(w, r, filepath.Join(session.Dir, "master.m3u8"))
		return
	}
	if len(parts) == 3 {
		name := filepath.Base(parts[2])
		target := filepath.Join(session.Dir, name)
		if err := ensurePathInside(session.Dir, target); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.HasSuffix(name, ".ts") {
			w.Header().Set("Content-Type", "video/mp2t")
		} else if strings.HasSuffix(name, ".m4s") {
			w.Header().Set("Content-Type", "video/iso.segment")
		} else if strings.HasSuffix(name, ".m3u8") {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		}
		http.ServeFile(w, r, target)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) stopTranscodeSession(w http.ResponseWriter, sessionID string, session *transcodeSession) {
	if session.cmd != nil && session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}
	if err := ensurePathInside(filepath.Join(s.deps.Config.Cache.Dir, "transcode"), session.Dir); err == nil {
		_ = os.RemoveAll(session.Dir)
	}
	transcodeSessions.Lock()
	delete(transcodeSessions.items, sessionID)
	transcodeSessions.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleTranscodingPresetValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	req, err := decodeTranscodeValidationRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	response := s.validateTranscodeRequest(r.Context(), req)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleTranscodingHardwareTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.requireWrite(w, r, "transcoding.test") {
		return
	}
	req, err := decodeTranscodeValidationRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.DryRun = true
	if req.DurationSeconds <= 0 || req.DurationSeconds > 3 {
		req.DurationSeconds = 2
	}
	response := s.validateTranscodeRequest(r.Context(), req)
	writeJSON(w, http.StatusOK, response)
}

type transcodeValidationRequest struct {
	Preset          catalog.TranscodingPreset
	AssetID         string
	DryRun          bool
	DurationSeconds int
}

func decodeTranscodeValidationRequest(r *http.Request) (transcodeValidationRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return transcodeValidationRequest{}, err
	}
	var req transcodeValidationRequest
	if value, ok := raw["preset"]; ok {
		if err := json.Unmarshal(value, &req.Preset); err != nil {
			return req, err
		}
	} else {
		encoded, _ := json.Marshal(raw)
		if err := json.Unmarshal(encoded, &req.Preset); err != nil {
			return req, err
		}
	}
	_ = json.Unmarshal(raw["asset_id"], &req.AssetID)
	_ = json.Unmarshal(raw["dry_run"], &req.DryRun)
	_ = json.Unmarshal(raw["duration_seconds"], &req.DurationSeconds)
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 2
	}
	if req.DurationSeconds > 3 {
		req.DurationSeconds = 3
	}
	if req.Preset.ID == "" {
		req.Preset.ID = "custom_inline"
	}
	if req.Preset.Name == "" {
		req.Preset.Name = "Custom inline"
	}
	if req.Preset.Container == "" {
		req.Preset.Container = "hls"
	}
	return req, nil
}

func (s *Server) validateTranscodeRequest(ctx context.Context, req transcodeValidationRequest) map[string]any {
	caps := transcoding.Detect(ctx)
	warnings := transcodePresetWarnings(req.Preset, caps)
	if err := validateTranscodingPreset(req.Preset, caps); err != nil {
		return map[string]any{"valid": false, "error": err.Error(), "warnings": warnings, "preset": req.Preset}
	}
	args, err := hlsArgsForPreset(req.Preset, "/tmp/input.mp4", "/tmp/cartolensia-transcode-preview")
	if err != nil {
		return map[string]any{"valid": false, "error": err.Error(), "warnings": warnings, "preset": req.Preset}
	}
	response := map[string]any{
		"valid":    true,
		"preset":   req.Preset,
		"warnings": warnings,
		"command":  ffmpegCommandSummary(args),
	}
	if req.DryRun {
		if req.AssetID == "" {
			response["valid"] = false
			response["error"] = "asset_id is required for hardware dry-run"
			return response
		}
		result := s.runTranscodeDryRun(ctx, req)
		for key, value := range result {
			response[key] = value
		}
		if ok, _ := result["dry_run_ok"].(bool); !ok {
			response["valid"] = false
		}
	}
	return response
}

func transcodePresetWarnings(preset catalog.TranscodingPreset, caps transcoding.Capabilities) []string {
	var warnings []string
	encoder := preset.FFmpegEncoder
	if encoder == "" {
		encoder = defaultEncoderForCodec(preset.Codec)
	}
	if strings.Contains(encoder, "av1_nvenc") {
		warnings = append(warnings, "AV1 NVENC is advertised by ffmpeg, but it must be dry-run validated; RTX 3090 Ti-class GPUs do not provide AV1 encode.")
	}
	if isDRIBasedHardware(preset.Hardware, encoder) && !caps.Hardware.DevDRI {
		warnings = append(warnings, "VAAPI/QSV hardware requires /dev/dri access and is unverified in this runtime.")
	}
	if preset.Mode == "bitrate" && strings.TrimSpace(preset.ParameterValue) != "" && isDigitsOnly(preset.ParameterValue) {
		warnings = append(warnings, "Bare numeric bitrate values are interpreted as kilobits per second.")
	}
	return warnings
}

func (s *Server) runTranscodeDryRun(ctx context.Context, req transcodeValidationRequest) map[string]any {
	asset, err := s.deps.Store.GetAsset(ctx, req.AssetID)
	if err != nil {
		return map[string]any{"dry_run_ok": false, "dry_run_error": err.Error()}
	}
	if asset.MediaKind != "video" {
		return map[string]any{"dry_run_ok": false, "dry_run_error": "hardware test requires a video asset"}
	}
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return map[string]any{"dry_run_ok": false, "dry_run_error": "video asset has no storage location"}
	}
	input, _, err := s.deps.Registry.OpenByURL(loc.StorageURL)
	if err != nil {
		return map[string]any{"dry_run_ok": false, "dry_run_error": err.Error()}
	}
	inputPath := input.Name()
	_ = input.Close()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return map[string]any{"dry_run_ok": false, "dry_run_error": "ffmpeg is not available"}
	}
	args, err := ffmpegDryRunArgs(req.Preset, inputPath, req.DurationSeconds)
	if err != nil {
		return map[string]any{"dry_run_ok": false, "dry_run_error": err.Error()}
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(req.DurationSeconds+8)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil
	started := time.Now()
	err = cmd.Run()
	elapsed := time.Since(started)
	tail := stderr.String()
	if len(tail) > 4096 {
		tail = tail[len(tail)-4096:]
	}
	result := map[string]any{
		"dry_run":          true,
		"dry_run_ok":       err == nil,
		"duration_seconds": req.DurationSeconds,
		"elapsed_ms":       elapsed.Milliseconds(),
		"stderr_tail":      tail,
		"command":          ffmpegCommandSummary(args),
	}
	if err != nil {
		result["dry_run_error"] = err.Error()
	}
	return result
}

func ffmpegDryRunArgs(preset catalog.TranscodingPreset, inputPath string, durationSeconds int) ([]string, error) {
	videoArgs, err := videoArgsForPreset(preset)
	if err != nil {
		return nil, err
	}
	args := []string{"-hide_banner", "-nostdin", "-y", "-t", strconv.Itoa(durationSeconds), "-i", inputPath, "-map", "0:v:0"}
	args = append(args, videoArgs...)
	args = append(args, "-an", "-pix_fmt", "yuv420p", "-f", "null", "-")
	return args, nil
}

func publicTranscodeSession(session *transcodeSession) map[string]any {
	return map[string]any{
		"id":            session.ID,
		"asset_id":      session.AssetID,
		"profile":       session.Profile,
		"hardware":      session.Hardware,
		"encoder":       session.Encoder,
		"playlist_url":  session.Playlist,
		"status":        session.Status,
		"created_at":    session.CreatedAt,
		"error":         session.Error,
		"stderr_tail":   session.stderrTail(),
		"command":       ffmpegCommandSummary(session.Args),
		"segment_count": transcodeSegmentCount(session.Dir),
		"output_bytes":  transcodeOutputBytes(session.Dir),
	}
}

func (s *transcodeSession) stderrTail() string {
	if s.stderr == nil {
		return ""
	}
	return s.stderr.Tail()
}

func waitForHLSReady(session *transcodeSession, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		updateTranscodeReadiness(session)
		if session.Status == "ready" || session.Status == "finished" || session.Status == "failed" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func updateTranscodeReadiness(session *transcodeSession) {
	transcodeSessions.Lock()
	defer transcodeSessions.Unlock()
	if session.Status != "pending" {
		return
	}
	if hlsReady(session.Dir) {
		session.Status = "ready"
	}
}

func hlsReady(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "master.m3u8")); err != nil {
		return false
	}
	matches, err := filepath.Glob(filepath.Join(dir, "segment_*.ts"))
	return err == nil && len(matches) > 0
}

func (s *Server) hlsArgsForProfile(ctx context.Context, profile string, inlinePreset *catalog.TranscodingPreset, inputPath, sessionDir string) ([]string, catalog.TranscodingPreset, error) {
	if inlinePreset != nil {
		preset := *inlinePreset
		if preset.ID == "" {
			preset.ID = "custom_inline"
		}
		if preset.Name == "" {
			preset.Name = "Custom inline"
		}
		if err := validateTranscodingPreset(preset, transcoding.Detect(ctx)); err != nil {
			return nil, catalog.TranscodingPreset{}, err
		}
		args, err := hlsArgsForPreset(preset, inputPath, sessionDir)
		return args, preset, err
	}
	if args, err := hlsArgs(profile, inputPath, sessionDir); err == nil {
		return args, builtInPresetByID(profile, transcoding.Detect(ctx)), nil
	}
	custom, err := s.deps.Store.ListTranscodingPresets(ctx)
	if err != nil {
		return nil, catalog.TranscodingPreset{}, err
	}
	for _, preset := range custom {
		if preset.ID == profile {
			args, err := hlsArgsForPreset(preset, inputPath, sessionDir)
			return args, preset, err
		}
	}
	return nil, catalog.TranscodingPreset{}, fmt.Errorf("unsupported transcode profile %q", profile)
}

func builtInTranscodingPresets(caps transcoding.Capabilities) []catalog.TranscodingPreset {
	now := time.Now().UTC()
	h264Available := caps.FFmpeg.Available && encoderAvailable(caps, "libx264")
	av1Available := false
	h264Disabled := ""
	if !h264Available {
		h264Disabled = "ffmpeg/libx264 is unavailable"
	}
	av1Disabled := ""
	if !av1Available {
		av1Disabled = "AV1 encoder is unavailable or not configured for browser-safe HLS"
	}
	return []catalog.TranscodingPreset{
		{
			ID:             "original",
			Name:           "Original/direct",
			BuiltIn:        true,
			Available:      true,
			Hardware:       "none",
			Codec:          "copy",
			FFmpegEncoder:  "copy",
			Mode:           "direct",
			ParameterValue: "original",
			Container:      "original",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "h264_720p_lan",
			Name:           "H.264 720p LAN",
			BuiltIn:        true,
			Available:      h264Available,
			DisabledReason: h264Disabled,
			Hardware:       "cpu",
			Codec:          "h264",
			FFmpegEncoder:  "libx264",
			Mode:           "quality",
			ParameterValue: "24",
			Container:      "hls",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "h264_low_bitrate",
			Name:           "H.264 low bitrate",
			BuiltIn:        true,
			Available:      h264Available,
			DisabledReason: h264Disabled,
			Hardware:       "cpu",
			Codec:          "h264",
			FFmpegEncoder:  "libx264",
			Mode:           "quality",
			ParameterValue: "30",
			Container:      "hls",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "av1_low_bitrate",
			Name:           "AV1 low bitrate",
			BuiltIn:        true,
			Available:      av1Available,
			DisabledReason: av1Disabled,
			Hardware:       "cpu",
			Codec:          "av1",
			FFmpegEncoder:  "libsvtav1",
			Mode:           "quality",
			ParameterValue: "34",
			Container:      "hls",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}
}

func builtInPresetByID(profile string, caps transcoding.Capabilities) catalog.TranscodingPreset {
	for _, preset := range builtInTranscodingPresets(caps) {
		if preset.ID == profile {
			return preset
		}
	}
	if profile == "h264-720p-lan" || profile == "browser-h264" {
		for _, preset := range builtInTranscodingPresets(caps) {
			if preset.ID == "h264_720p_lan" {
				return preset
			}
		}
	}
	if profile == "h264-low" || profile == "h264-low-bitrate" {
		for _, preset := range builtInTranscodingPresets(caps) {
			if preset.ID == "h264_low_bitrate" {
				return preset
			}
		}
	}
	return catalog.TranscodingPreset{ID: profile, Hardware: "cpu", Codec: "h264", FFmpegEncoder: "libx264", Mode: "quality", ParameterValue: "24", Container: "hls"}
}

func validateTranscodingPreset(preset catalog.TranscodingPreset, caps transcoding.Capabilities) error {
	if strings.TrimSpace(preset.Name) == "" {
		return fmt.Errorf("preset name is required")
	}
	if preset.Codec == "" {
		return fmt.Errorf("codec is required")
	}
	if preset.FFmpegEncoder == "" {
		preset.FFmpegEncoder = defaultEncoderForCodec(preset.Codec)
	}
	if preset.Mode != "quality" && preset.Mode != "quantizer" && preset.Mode != "bitrate" {
		return fmt.Errorf("mode must be quality, quantizer, or bitrate")
	}
	if preset.ParameterValue == "" {
		return fmt.Errorf("parameter_value is required")
	}
	if !encoderAvailable(caps, preset.FFmpegEncoder) {
		return fmt.Errorf("ffmpeg encoder %q is unavailable", preset.FFmpegEncoder)
	}
	if preset.Hardware != "" && preset.Hardware != "cpu" && !hardwareAvailable(caps, preset.Hardware) {
		return fmt.Errorf("hardware %q is unavailable", preset.Hardware)
	}
	if isDRIBasedHardware(preset.Hardware, preset.FFmpegEncoder) {
		if _, err := os.Stat("/dev/dri"); err != nil {
			return fmt.Errorf("%s requires /dev/dri access, which is not available in this runtime", preset.Hardware)
		}
	}
	return nil
}

func hlsArgsForPreset(preset catalog.TranscodingPreset, inputPath, sessionDir string) ([]string, error) {
	videoArgs, err := videoArgsForPreset(preset)
	if err != nil {
		return nil, err
	}
	audioArgs := []string{"-c:a", "aac", "-b:a", "128k"}
	return hlsArgsWithVideoAudio(inputPath, sessionDir, videoArgs, audioArgs), nil
}

func videoArgsForPreset(preset catalog.TranscodingPreset) ([]string, error) {
	encoder := preset.FFmpegEncoder
	if encoder == "" {
		encoder = defaultEncoderForCodec(preset.Codec)
	}
	videoArgs := []string{"-c:v", encoder}
	if strings.Contains(encoder, "nvenc") {
		videoArgs = append(videoArgs, "-preset", "p5")
	} else {
		videoArgs = append(videoArgs, "-preset", "veryfast")
	}
	switch preset.Mode {
	case "quality":
		if strings.Contains(encoder, "nvenc") {
			videoArgs = append(videoArgs, "-rc", "vbr", "-cq", safeNumericParameter(preset.ParameterValue, "24"), "-b:v", "0")
		} else {
			videoArgs = append(videoArgs, "-crf", safeNumericParameter(preset.ParameterValue, "24"))
		}
	case "quantizer":
		if strings.Contains(encoder, "nvenc") {
			videoArgs = append(videoArgs, "-rc", "constqp", "-qp", safeNumericParameter(preset.ParameterValue, "24"))
		} else {
			videoArgs = append(videoArgs, "-qp", safeNumericParameter(preset.ParameterValue, "24"))
		}
	case "bitrate":
		bitrate := safeBitrateParameter(preset.ParameterValue, "1500k")
		videoArgs = append(videoArgs, "-b:v", bitrate)
		if strings.Contains(encoder, "nvenc") {
			videoArgs = append(videoArgs, "-maxrate", bitrate, "-bufsize", doubleBitrateParameter(bitrate))
		}
	default:
		return nil, fmt.Errorf("unsupported preset mode %q", preset.Mode)
	}
	if preset.Codec == "h264" || strings.Contains(encoder, "264") {
		videoArgs = append(videoArgs, "-vf", "scale=w=1280:h=-2")
	}
	return videoArgs, nil
}

func hlsArgs(profile, inputPath, sessionDir string) ([]string, error) {
	videoArgs := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "24", "-vf", "scale=w=1280:h=-2"}
	audioArgs := []string{"-c:a", "aac", "-b:a", "128k"}
	switch profile {
	case "h264_720p_lan", "h264-720p-lan", "browser-h264":
	case "h264_low_bitrate", "h264-low", "h264-low-bitrate":
		videoArgs = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-vf", "scale=w=854:h=-2"}
		audioArgs = []string{"-c:a", "aac", "-b:a", "96k"}
	case "av1_low_bitrate", "av1-preview":
		return nil, fmt.Errorf("av1 transcode profile is not enabled yet")
	default:
		return nil, fmt.Errorf("unsupported transcode profile %q", profile)
	}
	return hlsArgsWithVideoAudio(inputPath, sessionDir, videoArgs, audioArgs), nil
}

func hlsArgsWithVideoAudio(inputPath, sessionDir string, videoArgs, audioArgs []string) []string {
	segmentPattern := filepath.Join(sessionDir, "segment_%05d.ts")
	playlist := filepath.Join(sessionDir, "master.m3u8")
	args := []string{"-hide_banner", "-nostdin", "-y", "-i", inputPath, "-map", "0:v:0", "-map", "0:a?"}
	args = append(args, videoArgs...)
	args = append(args, audioArgs...)
	args = append(args,
		"-pix_fmt", "yuv420p",
		"-sc_threshold", "0",
		"-g", "48",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "0",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", segmentPattern,
		playlist,
	)
	return args
}

func defaultEncoderForCodec(codec string) string {
	switch strings.ToLower(codec) {
	case "h264", "h.264":
		return "libx264"
	case "h265", "hevc", "h.265":
		return "libx265"
	case "av1":
		return "libsvtav1"
	default:
		return "libx264"
	}
}

func encoderAvailable(caps transcoding.Capabilities, encoder string) bool {
	if encoder == "" || encoder == "copy" {
		return true
	}
	for _, item := range caps.Encoders {
		if item.Name == encoder {
			return true
		}
	}
	return false
}

func hardwareAvailable(caps transcoding.Capabilities, hardware string) bool {
	switch strings.ToLower(hardware) {
	case "", "cpu", "none":
		return true
	case "nvidia", "nvidia_gpu", "nvidia_nvenc":
		return caps.Hardware.NvidiaSMI
	case "amd", "amd_gpu", "vaapi":
		return caps.Hardware.VAAPI
	case "intel", "intel_gpu", "qsv":
		return caps.Hardware.QSV || caps.Hardware.DevDRI
	default:
		return false
	}
}

func safeNumericParameter(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallback
		}
	}
	return value
}

func isDigitsOnly(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func safeBitrateParameter(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	for i, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		if i == len(value)-1 && (r == 'k' || r == 'm') {
			return value
		}
		return fallback
	}
	return value + "k"
}

func doubleBitrateParameter(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	multiplier := int64(1)
	unit := ""
	if strings.HasSuffix(value, "k") || strings.HasSuffix(value, "m") {
		unit = value[len(value)-1:]
		value = strings.TrimSuffix(value, unit)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return "3000k"
	}
	return fmt.Sprintf("%d%s", parsed*2*multiplier, unit)
}

func isDRIBasedHardware(hardware, encoder string) bool {
	hardware = strings.ToLower(hardware)
	encoder = strings.ToLower(encoder)
	return hardware == "amd" || hardware == "intel" || hardware == "vaapi" || hardware == "qsv" || strings.Contains(encoder, "vaapi") || strings.Contains(encoder, "qsv")
}

func ffmpegCommandSummary(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	redacted := append([]string(nil), args...)
	for i, arg := range redacted {
		if strings.Contains(arg, "/mnt/Models/rclone") {
			redacted[i] = filepath.Base(arg)
		}
	}
	return redacted
}

func transcodeSegmentCount(dir string) int {
	matches, err := filepath.Glob(filepath.Join(dir, "segment_*.ts"))
	if err != nil {
		return 0
	}
	return len(matches)
}

func transcodeOutputBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func ensurePathInside(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes configured cache directory")
	}
	return nil
}
