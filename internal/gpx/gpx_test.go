package gpx

import (
	"strings"
	"testing"
)

func TestParseGPXTrackPoints(t *testing.T) {
	points, err := Parse(strings.NewReader(`<?xml version="1.0"?>
<gpx version="1.1">
  <trk><name>Ride</name><trkseg>
    <trkpt lat="40.1" lon="44.2"><ele>1234.5</ele><time>2024-06-01T10:00:00Z</time></trkpt>
    <trkpt lat="40.2" lon="44.3"><time>2024-06-01T10:05:00Z</time></trkpt>
  </trkseg></trk>
</gpx>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %#v", points)
	}
	if points[0].ElevationM == nil || *points[0].ElevationM != 1234.5 {
		t.Fatalf("unexpected first point: %#v", points[0])
	}
	if points[1].Lat != 40.2 || points[1].Lon != 44.3 {
		t.Fatalf("unexpected second point: %#v", points[1])
	}
}
