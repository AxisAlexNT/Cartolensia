package kml

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseLineString(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml><Document><Placemark><LineString><coordinates>
44.1,40.1,100 44.2,40.2,110
</coordinates></LineString></Placemark></Document></kml>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].Lon != 44.1 || points[0].Lat != 40.1 || points[0].Source != "kml_linestring" {
		t.Fatalf("unexpected first point %#v", points[0])
	}
	if points[0].ElevationM == nil || *points[0].ElevationM != 100 {
		t.Fatalf("expected elevation, got %#v", points[0].ElevationM)
	}
}

func TestParsePlacemarkPoint(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml><Placemark><Point><coordinates>44.3,40.3</coordinates></Point></Placemark></kml>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(points) != 1 || points[0].Source != "kml_point" {
		t.Fatalf("unexpected points %#v", points)
	}
}

func TestParseGXTrack(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml xmlns:gx="http://www.google.com/kml/ext/2.2"><Placemark><gx:Track>
<when>2026-01-02T03:04:05Z</when><gx:coord>44.4 40.4 120</gx:coord>
<when>2026-01-02T03:04:06Z</when><gx:coord>44.5 40.5 121</gx:coord>
</gx:Track></Placemark></kml>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].RecordedAt.IsZero() || points[0].Source != "kml_gx_track" {
		t.Fatalf("expected timestamped gx point, got %#v", points[0])
	}
}

func TestParseKMZ(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("doc.kml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`<kml><Placemark><Point><coordinates>44.6,40.6</coordinates></Point></Placemark></kml>`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	points, err := ParseKMZBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("parse kmz: %v", err)
	}
	if len(points) != 1 || points[0].Lon != 44.6 {
		t.Fatalf("unexpected points %#v", points)
	}
}

func TestParseTruncatedKMLSalvagesCoordinates(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml><Document><Placemark><LineString><coordinates>
44.1,40.1 44.2,40.2
</coordinates></LineString></Placemark>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("expected salvaged points, got %#v", points)
	}
}

func TestParseTruncatedKMLSalvagesUnclosedCoordinateBlock(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml><Document><Placemark><LineString><coordinates>
44.1,40.1 44.2,40.2`))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || points[0].Source != "kml_salvaged_coordinates" {
		t.Fatalf("expected raw coordinate salvage, got %#v", points)
	}
}

func TestParseNoTimeKMLStillReturnsGeometry(t *testing.T) {
	points, err := Parse(strings.NewReader(`<kml><Placemark><LineString><coordinates>44,40 45,41</coordinates></LineString></Placemark></kml>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || !points[0].RecordedAt.IsZero() {
		t.Fatalf("unexpected no-time kml points: %#v", points)
	}
}

func TestInvalidKML(t *testing.T) {
	if _, err := Parse(strings.NewReader(`<kml><Placemark>`)); err == nil {
		t.Fatal("expected invalid kml error")
	}
}
