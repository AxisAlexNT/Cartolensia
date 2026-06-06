package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestGeoSnapInterpolatesAndDoesNotOverwriteEXIF(t *testing.T) {
	ctx := context.Background()
	store := catalog.NewMemoryStore()
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	trackAsset, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/tracks/route.gpx",
		RelativePath: "tracks/route.gpx",
		Name:         "route.gpx",
		Extension:    "gpx",
		MIME:         "application/gpx+xml",
		MediaKind:    "track",
		SizeBytes:    10,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTrackPoints(ctx, trackAsset.Asset.ID, []catalog.TrackPoint{
		{RecordedAt: now, Lat: 40, Lon: 44, Source: "gpx"},
		{RecordedAt: now.Add(10 * time.Minute), Lat: 41, Lon: 45, Source: "gpx"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertGPSTrackSummary(ctx, catalog.TrackSummary{
		TrackAssetID: trackAsset.Asset.ID,
		Name:         "route.gpx",
		PointCount:   2,
		StartTime:    &now,
		EndTime:      ptrTime(now.Add(10 * time.Minute)),
		MinLat:       ptrFloat(40),
		MinLon:       ptrFloat(44),
		MaxLat:       ptrFloat(41),
		MaxLon:       ptrFloat(45),
	}, nil)
	photoA, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/a.jpg",
		RelativePath: "photos/a.jpg",
		Name:         "a.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    10,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	taken := now.Add(5 * time.Minute)
	if err := store.UpdateAssetMetadata(ctx, photoA.Asset.ID, &taken, nil); err != nil {
		t.Fatal(err)
	}
	photoB, err := store.UpsertDiscoveredFile(ctx, storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/b.jpg",
		RelativePath: "photos/b.jpg",
		Name:         "b.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    10,
		MTime:        now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAssetMetadata(ctx, photoB.Asset.ID, &taken, nil); err != nil {
		t.Fatal(err)
	}
	confidence := 1.0
	if _, err := store.UpsertAssetGeo(ctx, catalog.AssetGeo{AssetID: photoB.Asset.ID, Lat: 1, Lon: 2, Source: "exif", Confidence: &confidence}, false); err != nil {
		t.Fatal(err)
	}
	job, err := store.EnqueueJob(ctx, jobs.New("geo_snap", GeoSnapPayload{TrackAssetID: trackAsset.Asset.ID, IncludeGeotagged: true}))
	if err != nil {
		t.Fatal(err)
	}
	if err := (Runner{Store: store}).SnapToTrack(ctx, &job); err != nil {
		t.Fatal(err)
	}
	geoA, err := store.GetAssetGeo(ctx, photoA.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if geoA.Source != "track_snapped" || geoA.Lat != 40.5 || geoA.Lon != 44.5 {
		t.Fatalf("unexpected snapped geotag: %#v", geoA)
	}
	geoB, err := store.GetAssetGeo(ctx, photoB.Asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if geoB.Source != "exif" || geoB.Lat != 1 || geoB.Lon != 2 {
		t.Fatalf("expected EXIF geotag to be preserved, got %#v", geoB)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
func ptrFloat(value float64) *float64    { return &value }
