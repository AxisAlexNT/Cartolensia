package database

import "testing"

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
