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
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
)

type transcodeSession struct {
	ID        string    `json:"id"`
	AssetID   string    `json:"asset_id"`
	Profile   string    `json:"profile"`
	Dir       string    `json:"dir"`
	Playlist  string    `json:"playlist_url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Error     string    `json:"error,omitempty"`
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
		Profile string `json:"profile"`
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
	args, err := hlsArgs(profile, inputPath, sessionDir)
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
		Dir:       sessionDir,
		Playlist:  "/api/v1/media/transcode-sessions/" + sessionID + "/master.m3u8",
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
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

func publicTranscodeSession(session *transcodeSession) map[string]any {
	return map[string]any{
		"id":           session.ID,
		"asset_id":     session.AssetID,
		"profile":      session.Profile,
		"playlist_url": session.Playlist,
		"status":       session.Status,
		"created_at":   session.CreatedAt,
		"error":        session.Error,
		"stderr_tail":  session.stderrTail(),
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

func hlsArgs(profile, inputPath, sessionDir string) ([]string, error) {
	videoArgs := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "24", "-vf", "scale='min(1280,iw)':-2"}
	audioArgs := []string{"-c:a", "aac", "-b:a", "128k"}
	switch profile {
	case "h264_720p_lan", "h264-720p-lan", "browser-h264":
	case "h264_low_bitrate", "h264-low", "h264-low-bitrate":
		videoArgs = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-vf", "scale='min(854,iw)':-2"}
		audioArgs = []string{"-c:a", "aac", "-b:a", "96k"}
	case "av1_low_bitrate", "av1-preview":
		return nil, fmt.Errorf("av1 transcode profile is not enabled yet")
	default:
		return nil, fmt.Errorf("unsupported transcode profile %q", profile)
	}
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
	return args, nil
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
