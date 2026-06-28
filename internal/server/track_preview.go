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
	hideJumps := mapHideTrackJumps(r)
	jumpThresholdM := mapTrackJumpThresholdM(r)
	geometry, segmentCount, hiddenJumps := trackGeometryFromPoints(points, hideJumps, jumpThresholdM)
	if geometry == nil {
		geometry = map[string]any{"type": "LineString", "coordinates": [][]float64{}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":    "FeatureCollection",
		"summary": detail.Summary,
		"track_filter": map[string]any{
			"hide_jumps":       hideJumps,
			"jump_threshold_m": jumpThresholdM,
			"hidden_jumps":     hiddenJumps,
			"segment_count":    segmentCount,
		},
		"features": []map[string]any{{
			"type":     "Feature",
			"geometry": geometry,
			"properties": map[string]any{
				"id":                   detail.Summary.TrackAssetID,
				"name":                 detail.Summary.Name,
				"kind":                 "track",
				"source_format":        detail.Summary.SourceFormat,
				"point_count":          detail.Summary.PointCount,
				"preview":              true,
				"segment_count":        segmentCount,
				"jump_filter_enabled":  hideJumps,
				"jump_threshold_m":     jumpThresholdM,
				"hidden_jump_segments": hiddenJumps,
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
	hideJumps := mapHideTrackJumps(r)
	jumpThresholdM := mapTrackJumpThresholdM(r)
	filterKey := "raw"
	if hideJumps {
		filterKey = "nojumps" + strconv.Itoa(int(jumpThresholdM+0.5))
	}
	target := filepath.Join(cacheRoot, assetID+"-"+strconv.Itoa(width)+"x"+strconv.Itoa(height)+"-"+filterKey+".png")
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
	img := renderTrackThumbnail(points, width, height, hideJumps, jumpThresholdM)
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

func renderTrackThumbnail(points []catalog.TrackPoint, width, height int, hideJumps bool, jumpThresholdM float64) image.Image {
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
	segments, _ := splitTrackPointsByJump(points, hideJumps, jumpThresholdM)
	var drawable []catalog.TrackPoint
	for _, segment := range segments {
		if len(segment) >= 2 {
			drawable = append(drawable, segment...)
		}
	}
	if len(drawable) == 0 {
		drawable = points
	}
	minLon, maxLon := drawable[0].Lon, drawable[0].Lon
	minLat, maxLat := drawable[0].Lat, drawable[0].Lat
	for _, point := range drawable[1:] {
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
	for _, segment := range segments {
		if len(segment) < 2 {
			continue
		}
		prevX, prevY := project(segment[0])
		for _, point := range segment[1:] {
			x, y := project(point)
			drawLine(img, prevX, prevY, x, y, line)
			prevX, prevY = x, y
		}
	}
	x, y := project(drawable[0])
	drawDot(img, x, y, 4, start)
	x, y = project(drawable[len(drawable)-1])
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
