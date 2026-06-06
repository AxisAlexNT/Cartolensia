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
	analysis := Analyze(points)
	if analysis.PointCount != 2 || analysis.DistanceM <= 0 || analysis.DurationSeconds == nil {
		t.Fatalf("unexpected analysis: %#v", analysis)
	}
}

func TestParseGPXRouteWaypointAndMissingTime(t *testing.T) {
	const data = `<?xml version="1.0"?>
<gpx version="1.1" creator="test">
  <rte><name>Route</name><rtept lat="40.0" lon="44.0"><ele>1</ele></rtept></rte>
  <wpt lat="40.1" lon="44.1"><time>2024-06-01 10:00:00</time></wpt>
</gpx>`
	points, err := Parse(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("expected route point and waypoint, got %#v", points)
	}
	if !points[0].RecordedAt.IsZero() {
		t.Fatalf("missing time should be tolerated as zero, got %s", points[0].RecordedAt)
	}
	simplified := Simplify(points, 1)
	if len(simplified) != 1 {
		t.Fatalf("unexpected simplification: %#v", simplified)
	}
}

func TestParseTruncatedGPXSalvagesCompletePoints(t *testing.T) {
	points, err := Parse(strings.NewReader(`<gpx><trk><trkseg>
<trkpt lat="40.1" lon="44.1"><time>2026-01-01T00:00:00Z</time></trkpt>
<trkpt lat="40.2" lon="44.2"><time>2026-01-01T00:01:00Z</time></trkpt>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("expected salvaged points, got %#v", points)
	}
}
