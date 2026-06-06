package kml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func Parse(reader io.Reader) ([]catalog.TrackPoint, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	points, err := parseXML(bytes.NewReader(data))
	if err != nil {
		salvaged := salvageCoordinateTuples(string(data))
		if len(salvaged) > 0 {
			return salvaged, nil
		}
		return nil, err
	}
	return points, nil
}

func parseXML(reader io.Reader) ([]catalog.TrackPoint, error) {
	decoder := xml.NewDecoder(reader)
	var points []catalog.TrackPoint
	var stack []string
	var text strings.Builder
	var gxTimes []time.Time
	var gxCoords []catalog.TrackPoint
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if (len(points) > 0 || len(gxCoords) > 0) && recoverableXMLError(err) {
				appendGXTrack(&points, gxCoords, gxTimes)
				return points, nil
			}
			return nil, fmt.Errorf("parse kml: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			text.Reset()
		case xml.CharData:
			text.Write([]byte(t))
		case xml.EndElement:
			raw := strings.TrimSpace(text.String())
			switch t.Name.Local {
			case "coordinates":
				parsed := parseCoordinates(raw, sourceForStack(stack))
				points = append(points, parsed...)
			case "when":
				if parsed, err := parseTime(raw); err == nil {
					gxTimes = append(gxTimes, parsed.UTC())
				}
			case "coord":
				if pt, ok := parseGXCoord(raw); ok {
					gxCoords = append(gxCoords, pt)
				}
			case "Track":
				appendGXTrack(&points, gxCoords, gxTimes)
				gxTimes = nil
				gxCoords = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			text.Reset()
		}
	}
	return points, nil
}

var coordinateTuplePattern = regexp.MustCompile(`[-+]?\d+(?:\.\d+)?\s*,\s*[-+]?\d+(?:\.\d+)?(?:\s*,\s*[-+]?\d+(?:\.\d+)?)?`)

func salvageCoordinateTuples(raw string) []catalog.TrackPoint {
	matches := coordinateTuplePattern.FindAllString(raw, -1)
	points := make([]catalog.TrackPoint, 0, len(matches))
	for _, match := range matches {
		parsed := parseCoordinates(match, "kml_salvaged_coordinates")
		points = append(points, parsed...)
	}
	return points
}

func appendGXTrack(points *[]catalog.TrackPoint, gxCoords []catalog.TrackPoint, gxTimes []time.Time) {
	for i := range gxCoords {
		if i < len(gxTimes) {
			gxCoords[i].RecordedAt = gxTimes[i]
		}
		*points = append(*points, gxCoords[i])
	}
}

func recoverableXMLError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "unexpected EOF") || strings.Contains(text, "EOF")
}

func ParseKMZ(reader io.ReaderAt, size int64) ([]catalog.TrackPoint, error) {
	zr, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, fmt.Errorf("parse kmz: %w", err)
	}
	var selected *zip.File
	for _, file := range zr.File {
		if strings.EqualFold(filepath.Base(file.Name), "doc.kml") {
			selected = file
			break
		}
	}
	if selected == nil {
		for _, file := range zr.File {
			if strings.EqualFold(filepath.Ext(file.Name), ".kml") {
				selected = file
				break
			}
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("kmz contains no kml document")
	}
	rc, err := selected.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return Parse(rc)
}

func ParseKMZBytes(data []byte) ([]catalog.TrackPoint, error) {
	return ParseKMZ(bytes.NewReader(data), int64(len(data)))
}

func parseCoordinates(raw, source string) []catalog.TrackPoint {
	fields := strings.Fields(raw)
	points := make([]catalog.TrackPoint, 0, len(fields))
	for _, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) < 2 {
			continue
		}
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLon != nil || errLat != nil {
			continue
		}
		var ele *float64
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err == nil {
				ele = &parsed
			}
		}
		points = append(points, catalog.TrackPoint{Lat: lat, Lon: lon, ElevationM: ele, Source: source})
	}
	return points
}

func parseGXCoord(raw string) (catalog.TrackPoint, bool) {
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return catalog.TrackPoint{}, false
	}
	lon, errLon := strconv.ParseFloat(fields[0], 64)
	lat, errLat := strconv.ParseFloat(fields[1], 64)
	if errLon != nil || errLat != nil {
		return catalog.TrackPoint{}, false
	}
	var ele *float64
	if len(fields) >= 3 {
		if parsed, err := strconv.ParseFloat(fields[2], 64); err == nil {
			ele = &parsed
		}
	}
	return catalog.TrackPoint{Lat: lat, Lon: lon, ElevationM: ele, Source: "kml_gx_track"}, true
}

func sourceForStack(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case "Point":
			return "kml_point"
		case "LineString":
			return "kml_linestring"
		case "LinearRing":
			return "kml_linearring"
		}
	}
	return "kml"
}

func parseTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var last error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
		last = err
	}
	return time.Time{}, last
}
