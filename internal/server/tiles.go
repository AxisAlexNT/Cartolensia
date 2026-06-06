package server

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const osmTileURL = "https://tile.openstreetmap.org/%d/%d/%d.png"

func (s *Server) handleTiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	parts := pathParts(strings.TrimPrefix(r.URL.Path, "/api/v1/tiles/"))
	if len(parts) != 4 {
		http.NotFound(w, r)
		return
	}
	source, z, x, y, err := parseTileRequest(parts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cachePath, err := safeTileCachePath(filepath.Join(s.deps.Config.Cache.Dir, "tiles"), source, z, x, y)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := os.Stat(cachePath); err == nil {
		http.ServeFile(w, r, cachePath)
		return
	}
	if source != "osm" {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown tile source %q", source))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(osmTileURL, z, x, y), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	req.Header.Set("User-Agent", "Cartolensia/0.1 local real-peek tile cache")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("tile source unavailable: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Errorf("tile source returned %s", resp.Status))
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tmp := cachePath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 2<<20)); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeFile(w, r, cachePath)
}

func parseTileRequest(parts []string) (string, int, int, int, error) {
	source := strings.TrimSpace(parts[0])
	if source != "osm" {
		return "", 0, 0, 0, fmt.Errorf("unknown tile source %q", source)
	}
	z, err := strconv.Atoi(parts[1])
	if err != nil || z < 0 || z > 19 {
		return "", 0, 0, 0, fmt.Errorf("tile z must be between 0 and 19")
	}
	x, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("tile x must be an integer")
	}
	yPart := strings.TrimSuffix(parts[3], ".png")
	if yPart == parts[3] || strings.Contains(yPart, ".") {
		return "", 0, 0, 0, fmt.Errorf("tile path must end in .png")
	}
	y, err := strconv.Atoi(yPart)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("tile y must be an integer")
	}
	max := int(math.Pow(2, float64(z)))
	if x < 0 || y < 0 || x >= max || y >= max {
		return "", 0, 0, 0, fmt.Errorf("tile coordinates are outside zoom bounds")
	}
	return source, z, x, y, nil
}

func safeTileCachePath(cacheRoot, source string, z, x, y int) (string, error) {
	root, err := filepath.Abs(cacheRoot)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, source, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
	cleanTarget := filepath.Clean(target)
	rel, err := filepath.Rel(root, cleanTarget)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("tile cache path escapes cache root")
	}
	return cleanTarget, nil
}
