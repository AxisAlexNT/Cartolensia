package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

func TestPostgresIntegrationPhase1(t *testing.T) {
	if os.Getenv("CARTOLENSIA_RUN_DB_TESTS") != "1" {
		t.Skip("set CARTOLENSIA_RUN_DB_TESTS=1 to run PostgreSQL integration tests")
	}
	databaseURL := os.Getenv("CARTOLENSIA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set CARTOLENSIA_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openIsolatedTestDB(t, ctx, databaseURL)
	defer db.Close()

	if err := db.ApplyEmbeddedMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.ApplyEmbeddedMigrations(ctx); err != nil {
		t.Fatalf("migrations are not idempotent: %v", err)
	}

	cfg := config.Defaults()
	if err := db.SnapshotConfig(ctx, cfg); err != nil {
		t.Fatalf("snapshot config: %v", err)
	}
	if err := db.UpsertStorages(ctx, cfg.Storages); err != nil {
		t.Fatalf("upsert storages: %v", err)
	}
	if err := db.UpsertPlugins(ctx, plugins.BuiltIns()); err != nil {
		t.Fatalf("upsert plugins: %v", err)
	}

	info := storage.FileInfo{
		StorageName:  "fixture",
		StorageURL:   "fs://fixture/photos/one.jpg",
		RelativePath: "photos/one.jpg",
		Name:         "one.jpg",
		Extension:    "jpg",
		MIME:         "image/jpeg",
		MediaKind:    "photo",
		SizeBytes:    12,
		MTime:        time.Unix(20, 0).UTC(),
	}
	first, err := db.UpsertDiscoveredFile(ctx, info)
	if err != nil {
		t.Fatalf("first discovery upsert: %v", err)
	}
	second, err := db.UpsertDiscoveredFile(ctx, info)
	if err != nil {
		t.Fatalf("second discovery upsert: %v", err)
	}
	if !first.Created || second.Created || first.Asset.ID != second.Asset.ID {
		t.Fatalf("discovery upsert not idempotent: first=%#v second=%#v", first, second)
	}
	const sha512Hex = "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	if err := db.UpdateLocationHash(ctx, first.Asset.ID, sha512Hex, 12); err != nil {
		t.Fatalf("update hash: %v", err)
	}
	stats, err := db.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Assets != 1 || stats.Hashed != 1 || stats.Unhashed != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}

	job, err := db.EnqueueJob(ctx, jobs.New("hash", nil))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	leased, err := db.LeaseNextJob(ctx, "worker-a", []string{"hash"}, time.Second)
	if err != nil {
		t.Fatalf("lease: %v", err)
	}
	if leased.ID != job.ID || leased.WorkerID != "worker-a" || leased.Attempts != 1 {
		t.Fatalf("unexpected leased job: %#v", leased)
	}
	if err := db.HeartbeatJob(ctx, leased.ID, "worker-a", time.Second); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := db.CompleteLeasedJob(ctx, leased, "worker-b"); err == nil {
		t.Fatal("stale worker completed leased job")
	}
	cancelled, err := db.RequestCancelJob(ctx, leased.ID)
	if err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	if cancelled.Status != jobs.StatusCancelRequested {
		t.Fatalf("expected cancel_requested, got %#v", cancelled)
	}
	if err := db.CancelLeasedJob(ctx, leased, "worker-a"); err != nil {
		t.Fatalf("cancel leased: %v", err)
	}
	final, err := db.GetJob(ctx, leased.ID)
	if err != nil {
		t.Fatalf("get final job: %v", err)
	}
	if final.Status != jobs.StatusCanceled {
		t.Fatalf("expected canceled, got %#v", final)
	}
}

func openIsolatedTestDB(t *testing.T, ctx context.Context, databaseURL string) *DB {
	t.Helper()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "cartolensia_test_" + time.Now().UTC().Format("20060102150405") + "_" + randSuffix()
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+identifier); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		defer admin.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, "drop schema if exists "+identifier+" cascade")
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return &DB{pool: pool}
}

func randSuffix() string {
	return time.Now().UTC().Format("150405000000000")
}

var _ catalog.Store = (*DB)(nil)
