package gpx

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

type document struct {
	Tracks []track `xml:"trk"`
}

type track struct {
	Name     string    `xml:"name"`
	Segments []segment `xml:"trkseg"`
}

type segment struct {
	Points []point `xml:"trkpt"`
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
				lat, err := strconv.ParseFloat(strings.TrimSpace(pt.Lat), 64)
				if err != nil {
					return nil, fmt.Errorf("parse gpx latitude: %w", err)
				}
				lon, err := strconv.ParseFloat(strings.TrimSpace(pt.Lon), 64)
				if err != nil {
					return nil, fmt.Errorf("parse gpx longitude: %w", err)
				}
				recordedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(pt.Time))
				if err != nil {
					return nil, fmt.Errorf("parse gpx point time: %w", err)
				}
				var ele *float64
				if strings.TrimSpace(pt.Ele) != "" {
					value, err := strconv.ParseFloat(strings.TrimSpace(pt.Ele), 64)
					if err != nil {
						return nil, fmt.Errorf("parse gpx elevation: %w", err)
					}
					ele = &value
				}
				points = append(points, catalog.TrackPoint{
					RecordedAt: recordedAt.UTC(),
					Lat:        lat,
					Lon:        lon,
					ElevationM: ele,
					Source:     "gpx",
				})
			}
		}
	}
	return points, nil
}
