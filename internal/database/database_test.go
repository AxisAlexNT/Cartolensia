package database

import (
	"testing"
	"testing/fstest"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

func TestLoadMigrationsSorted(t *testing.T) {
	migrations, err := LoadMigrations("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected migrations")
	}
	if migrations[0].Version != "001_core" {
		t.Fatalf("unexpected first migration %q", migrations[0].Version)
	}
	if migrations[0].Checksum == "" || migrations[0].SQL == "" {
		t.Fatalf("migration not populated: %#v", migrations[0])
	}
}

func TestLoadEmbeddedMigrations(t *testing.T) {
	migrations, err := LoadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 3 {
		t.Fatalf("expected embedded migrations, got %d", len(migrations))
	}
	if migrations[0].Version != "001_core" {
		t.Fatalf("unexpected first embedded migration %q", migrations[0].Version)
	}
}

func TestLoadMigrationsFSSortedAndChecksummed(t *testing.T) {
	fsys := fstest.MapFS{
		"002_second.sql": {Data: []byte("select 2;")},
		"001_first.sql":  {Data: []byte("select 1;")},
		"README.md":      {Data: []byte("ignored")},
	}
	migrations, err := LoadMigrationsFS(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}
	if migrations[0].Version != "001_first" || migrations[1].Version != "002_second" {
		t.Fatalf("migrations not sorted: %#v", migrations)
	}
	if migrations[0].Checksum == "" || migrations[0].Checksum == migrations[1].Checksum {
		t.Fatalf("unexpected checksums: %#v", migrations)
	}
}

func TestApplyTrackMetadata(t *testing.T) {
	summary := catalog.TrackSummary{}
	applyTrackMetadata(&summary, `{"distance_m":123.5,"duration_seconds":45,"elevation_min_m":10,"elevation_max_m":40}`)
	if summary.DistanceM != 123.5 {
		t.Fatalf("distance not applied: %#v", summary)
	}
	if summary.DurationSec == nil || *summary.DurationSec != 45 {
		t.Fatalf("duration not applied: %#v", summary)
	}
	if summary.ElevationMin == nil || *summary.ElevationMin != 10 || summary.ElevationMax == nil || *summary.ElevationMax != 40 {
		t.Fatalf("elevation not applied: %#v", summary)
	}
}

func TestNormalizeGeoPageAllowsMapScaleQueries(t *testing.T) {
	limit, offset := normalizeGeoPage(0, -10)
	if limit != 10000 || offset != 0 {
		t.Fatalf("unexpected geo defaults: limit=%d offset=%d", limit, offset)
	}

	limit, offset = normalizeGeoPage(50000, 25)
	if limit != 50000 || offset != 25 {
		t.Fatalf("geo query should keep large map limits: limit=%d offset=%d", limit, offset)
	}

	limit, _ = normalizeGeoPage(500000, 0)
	if limit != 100000 {
		t.Fatalf("geo query should still cap extreme requests, got %d", limit)
	}
}

func TestTrackRenderDetailLevelForMaxPoints(t *testing.T) {
	cases := []struct {
		maxPoints int
		want      string
	}{
		{0, "overview"},
		{4, "overview"},
		{5, "z6"},
		{16, "z6"},
		{17, "z10"},
		{64, "z10"},
		{65, "z13"},
		{256, "z13"},
		{257, "z16"},
		{1024, "z16"},
		{1025, ""},
	}
	for _, tc := range cases {
		if got := trackRenderDetailLevelForMaxPoints(tc.maxPoints); got != tc.want {
			t.Fatalf("maxPoints=%d got %q want %q", tc.maxPoints, got, tc.want)
		}
	}
}

func TestTrackRenderLevelsDownsample(t *testing.T) {
	points := make([]catalog.TrackPoint, 0, 100)
	for i := 0; i < 100; i++ {
		points = append(points, catalog.TrackPoint{Lat: float64(i), Lon: float64(i)})
	}
	for _, level := range trackRenderLevels() {
		sampled := downsampleTrackPoints(points, level.MaxPoints)
		if len(sampled) > level.MaxPoints {
			t.Fatalf("%s sampled %d > %d", level.Name, len(sampled), level.MaxPoints)
		}
		if len(sampled) == 0 || sampled[0].Lat != points[0].Lat || sampled[len(sampled)-1].Lat != points[len(points)-1].Lat {
			t.Fatalf("%s did not preserve endpoints: %#v", level.Name, sampled)
		}
	}
}
