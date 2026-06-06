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
