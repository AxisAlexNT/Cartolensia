package server

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func (s *Server) handleTrackPreview(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	maxPoints := queryInt(r, "max_points", 1200)
	if maxPoints <= 0 || maxPoints > 5000 {
		maxPoints = 1200
	}
	detail, err := s.deps.Store.GetTrack(r.Context(), assetID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	points, err := s.deps.Store.QueryTrackPoints(r.Context(), catalog.TrackPointQuery{TrackAssetID: assetID, Simplify: true, MaxPoints: maxPoints})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	coords := make([][]float64, 0, len(points))
	for _, point := range points {
		coords = append(coords, []float64{point.Lon, point.Lat})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":    "FeatureCollection",
		"summary": detail.Summary,
		"features": []map[string]any{{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": map[string]any{
				"id":            detail.Summary.TrackAssetID,
				"name":          detail.Summary.Name,
				"kind":          "track",
				"source_format": detail.Summary.SourceFormat,
				"point_count":   detail.Summary.PointCount,
				"preview":       true,
			},
		}},
	})
}

func (s *Server) handleTrackThumbnail(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	width := queryInt(r, "width", 360)
	height := queryInt(r, "height", 220)
	if width < 64 || width > 1024 {
		width = 360
	}
	if height < 64 || height > 1024 {
		height = 220
	}
	cacheRoot := filepath.Join(s.deps.Config.Cache.Dir, "track-thumbnails")
	target := filepath.Join(cacheRoot, assetID+"-"+strconv.Itoa(width)+"x"+strconv.Itoa(height)+".png")
	if err := ensurePathInside(cacheRoot, target); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := os.Stat(target); err == nil {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, target)
		return
	}
	points, err := s.deps.Store.QueryTrackPoints(r.Context(), catalog.TrackPointQuery{TrackAssetID: assetID, Simplify: true, MaxPoints: 2500})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if len(points) == 0 {
		writeError(w, http.StatusNotFound, catalog.ErrNotFound)
		return
	}
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	file, err := os.Create(target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	img := renderTrackThumbnail(points, width, height)
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil {
		writeError(w, http.StatusInternalServerError, encodeErr)
		return
	}
	if closeErr != nil {
		writeError(w, http.StatusInternalServerError, closeErr)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, target)
}

func renderTrackThumbnail(points []catalog.TrackPoint, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{R: 16, G: 24, B: 39, A: 255}
	grid := color.RGBA{R: 31, G: 42, B: 61, A: 255}
	line := color.RGBA{R: 235, G: 245, B: 255, A: 255}
	start := color.RGBA{R: 46, G: 204, B: 113, A: 255}
	end := color.RGBA{R: 255, G: 99, B: 71, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, bg)
		}
	}
	for x := 0; x < width; x += 36 {
		drawLine(img, x, 0, x, height-1, grid)
	}
	for y := 0; y < height; y += 36 {
		drawLine(img, 0, y, width-1, y, grid)
	}
	minLon, maxLon := points[0].Lon, points[0].Lon
	minLat, maxLat := points[0].Lat, points[0].Lat
	for _, point := range points[1:] {
		minLon = minFloat(minLon, point.Lon)
		maxLon = maxFloat(maxLon, point.Lon)
		minLat = minFloat(minLat, point.Lat)
		maxLat = maxFloat(maxLat, point.Lat)
	}
	if maxLon == minLon {
		maxLon += 0.0001
		minLon -= 0.0001
	}
	if maxLat == minLat {
		maxLat += 0.0001
		minLat -= 0.0001
	}
	pad := 16.0
	project := func(point catalog.TrackPoint) (int, int) {
		x := pad + (point.Lon-minLon)/(maxLon-minLon)*(float64(width)-2*pad)
		y := pad + (maxLat-point.Lat)/(maxLat-minLat)*(float64(height)-2*pad)
		return int(x + 0.5), int(y + 0.5)
	}
	prevX, prevY := project(points[0])
	for _, point := range points[1:] {
		x, y := project(point)
		drawLine(img, prevX, prevY, x, y, line)
		prevX, prevY = x, y
	}
	x, y := project(points[0])
	drawDot(img, x, y, 4, start)
	x, y = project(points[len(points)-1])
	drawDot(img, x, y, 4, end)
	return img
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		drawDot(img, x0, y0, 1, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func drawDot(img *image.RGBA, cx, cy, radius int, c color.Color) {
	bounds := img.Bounds()
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if x < bounds.Min.X || x >= bounds.Max.X || y < bounds.Min.Y || y >= bounds.Max.Y {
				continue
			}
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
