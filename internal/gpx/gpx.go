package gpx

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

type document struct {
	Tracks    []track `xml:"trk"`
	Routes    []route `xml:"rte"`
	Waypoints []point `xml:"wpt"`
}

type track struct {
	Name     string    `xml:"name"`
	Segments []segment `xml:"trkseg"`
}

type segment struct {
	Points []point `xml:"trkpt"`
}

type route struct {
	Name   string  `xml:"name"`
	Points []point `xml:"rtept"`
}

type point struct {
	Lat  string `xml:"lat,attr"`
	Lon  string `xml:"lon,attr"`
	Ele  string `xml:"ele"`
	Time string `xml:"time"`
}

func Parse(reader io.Reader) ([]catalog.TrackPoint, error) {
	var doc document
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse gpx: %w", err)
	}
	var points []catalog.TrackPoint
	for _, trk := range doc.Tracks {
		for _, seg := range trk.Segments {
			for _, pt := range seg.Points {
				parsed, err := parsePoint(pt, "gpx")
				if err != nil {
					return nil, err
				}
				points = append(points, parsed)
			}
		}
	}
	for _, rte := range doc.Routes {
		for _, pt := range rte.Points {
			parsed, err := parsePoint(pt, "gpx_route")
			if err != nil {
				return nil, err
			}
			points = append(points, parsed)
		}
	}
	for _, pt := range doc.Waypoints {
		parsed, err := parsePoint(pt, "gpx_waypoint")
		if err != nil {
			return nil, err
		}
		points = append(points, parsed)
	}
	return points, nil
}

type Analysis struct {
	PointCount      int        `json:"point_count"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	MinLat          *float64   `json:"min_lat,omitempty"`
	MinLon          *float64   `json:"min_lon,omitempty"`
	MaxLat          *float64   `json:"max_lat,omitempty"`
	MaxLon          *float64   `json:"max_lon,omitempty"`
	DistanceM       float64    `json:"distance_m"`
	DurationSeconds *float64   `json:"duration_seconds,omitempty"`
	ElevationMinM   *float64   `json:"elevation_min_m,omitempty"`
	ElevationMaxM   *float64   `json:"elevation_max_m,omitempty"`
	AverageSpeedMPS *float64   `json:"average_speed_mps,omitempty"`
}

func Analyze(points []catalog.TrackPoint) Analysis {
	var out Analysis
	out.PointCount = len(points)
	for i, point := range points {
		lat := point.Lat
		lon := point.Lon
		if out.MinLat == nil || lat < *out.MinLat {
			out.MinLat = &lat
		}
		if out.MaxLat == nil || lat > *out.MaxLat {
			out.MaxLat = &lat
		}
		if out.MinLon == nil || lon < *out.MinLon {
			out.MinLon = &lon
		}
		if out.MaxLon == nil || lon > *out.MaxLon {
			out.MaxLon = &lon
		}
		if !point.RecordedAt.IsZero() {
			t := point.RecordedAt.UTC()
			if out.StartTime == nil || t.Before(*out.StartTime) {
				out.StartTime = &t
			}
			if out.EndTime == nil || t.After(*out.EndTime) {
				out.EndTime = &t
			}
		}
		if point.ElevationM != nil {
			ele := *point.ElevationM
			if out.ElevationMinM == nil || ele < *out.ElevationMinM {
				out.ElevationMinM = &ele
			}
			if out.ElevationMaxM == nil || ele > *out.ElevationMaxM {
				out.ElevationMaxM = &ele
			}
		}
		if i > 0 {
			out.DistanceM += HaversineMeters(points[i-1].Lat, points[i-1].Lon, point.Lat, point.Lon)
		}
	}
	if out.StartTime != nil && out.EndTime != nil && out.EndTime.After(*out.StartTime) {
		duration := out.EndTime.Sub(*out.StartTime).Seconds()
		out.DurationSeconds = &duration
		if out.DistanceM > 0 {
			avg := out.DistanceM / duration
			out.AverageSpeedMPS = &avg
		}
	}
	return out
}

func Simplify(points []catalog.TrackPoint, maxPoints int) []catalog.TrackPoint {
	if maxPoints <= 0 || len(points) <= maxPoints {
		return append([]catalog.TrackPoint(nil), points...)
	}
	if maxPoints == 1 {
		return []catalog.TrackPoint{points[0]}
	}
	step := float64(len(points)-1) / float64(maxPoints-1)
	out := make([]catalog.TrackPoint, 0, maxPoints)
	lastIndex := -1
	for i := 0; i < maxPoints; i++ {
		idx := int(math.Round(float64(i) * step))
		if idx <= lastIndex {
			idx = lastIndex + 1
		}
		if idx >= len(points) {
			idx = len(points) - 1
		}
		out = append(out, points[idx])
		lastIndex = idx
	}
	return out
}

func HaversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const radiusM = 6371008.8
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLambda := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * radiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func parsePoint(pt point, source string) (catalog.TrackPoint, error) {
	lat, err := strconv.ParseFloat(strings.TrimSpace(pt.Lat), 64)
	if err != nil {
		return catalog.TrackPoint{}, fmt.Errorf("parse gpx latitude: %w", err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(pt.Lon), 64)
	if err != nil {
		return catalog.TrackPoint{}, fmt.Errorf("parse gpx longitude: %w", err)
	}
	var recordedAt time.Time
	if raw := strings.TrimSpace(pt.Time); raw != "" {
		parsed, err := parseTime(raw)
		if err != nil {
			return catalog.TrackPoint{}, fmt.Errorf("parse gpx point time: %w", err)
		}
		recordedAt = parsed.UTC()
	}
	var ele *float64
	if strings.TrimSpace(pt.Ele) != "" {
		value, err := strconv.ParseFloat(strings.TrimSpace(pt.Ele), 64)
		if err != nil {
			return catalog.TrackPoint{}, fmt.Errorf("parse gpx elevation: %w", err)
		}
		ele = &value
	}
	return catalog.TrackPoint{RecordedAt: recordedAt, Lat: lat, Lon: lon, ElevationM: ele, Source: source}, nil
}

func parseTime(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
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
