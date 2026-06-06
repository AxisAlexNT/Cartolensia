package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

type DB struct {
	pool *pgxpool.Pool
}

type Migration struct {
	Version  string
	Path     string
	SQL      string
	Checksum string
}

type Capability struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Installed bool   `json:"installed"`
}

func Open(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{pool: pool}, nil
}

func (db *DB) Close() {
	if db != nil && db.pool != nil {
		db.pool.Close()
	}
}

func LoadMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)
	migrations := make([]Migration, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		version := strings.TrimSuffix(filepath.Base(path), ".sql")
		migrations = append(migrations, Migration{
			Version:  version,
			Path:     path,
			SQL:      string(data),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return migrations, nil
}

func (db *DB) ApplyMigrations(ctx context.Context, dir string) error {
	migrations, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	if _, err := db.pool.Exec(ctx, `create table if not exists schema_migrations (version text primary key, checksum text not null, applied_at timestamptz not null default now())`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	for _, migration := range migrations {
		var appliedChecksum string
		err := db.pool.QueryRow(ctx, `select checksum from schema_migrations where version=$1`, migration.Version).Scan(&appliedChecksum)
		if err == nil {
			if appliedChecksum != migration.Checksum {
				return fmt.Errorf("migration %s checksum mismatch", migration.Version)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", migration.Version, err)
		}
		tx, err := db.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version, checksum) values($1, $2)`, migration.Version, migration.Checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.Version, err)
		}
	}
	return nil
}

func (db *DB) Capabilities(ctx context.Context) ([]Capability, error) {
	names := []string{"postgis", "vector", "pg_trgm"}
	out := make([]Capability, 0, len(names))
	for _, name := range names {
		var cap Capability
		cap.Name = name
		err := db.pool.QueryRow(ctx, `
			select
				exists(select 1 from pg_available_extensions where name=$1),
				exists(select 1 from pg_extension where extname=$1)
		`, name).Scan(&cap.Available, &cap.Installed)
		if err != nil {
			return nil, err
		}
		out = append(out, cap)
	}
	return out, nil
}

func (db *DB) SnapshotConfig(ctx context.Context, cfg config.Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `insert into config_snapshots(id, source, effective_config) values($1, $2, $3)`, id.NewUUID(), cfg.Source, data)
	return err
}

func (db *DB) UpsertStorages(ctx context.Context, storages []config.StorageConfig) error {
	for _, st := range storages {
		if _, err := db.ensureStorage(ctx, st.Name, st.Kind, st.Root, st.Mode); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) UpsertPlugins(ctx context.Context, manifests []plugins.Manifest) error {
	for _, manifest := range manifests {
		data, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		_, err = db.pool.Exec(ctx, `
			insert into plugins(id, name, version, enabled, runtime, status, manifest_json, loaded_at)
			values($1, $2, $3, true, $4, $5, $6, now())
			on conflict(id) do update set
				name=excluded.name,
				version=excluded.version,
				runtime=excluded.runtime,
				status=excluded.status,
				manifest_json=excluded.manifest_json,
				loaded_at=now()
		`, manifest.ID, manifest.Name, manifest.Version, manifest.Runtime, manifest.Status, data)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) UpsertDiscoveredFile(ctx context.Context, info storage.FileInfo) (catalog.UpsertResult, error) {
	storageID, err := db.ensureStorage(ctx, info.StorageName, "fs", "", "strict_read_only")
	if err != nil {
		return catalog.UpsertResult{}, err
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return catalog.UpsertResult{}, err
	}
	defer rollback(ctx, tx)

	var existingAssetID string
	err = tx.QueryRow(ctx, `select asset_id from asset_locations where url=$1`, info.StorageURL).Scan(&existingAssetID)
	if err == nil {
		_, err = tx.Exec(ctx, `
			update asset_locations
			set relative_path=$1, file_name=$2, extension=$3, mime_type=$4, media_kind=$5, size_bytes=$6, mtime=$7, last_seen_at=now()
			where url=$8
		`, info.RelativePath, info.Name, info.Extension, info.MIME, info.MediaKind, info.SizeBytes, info.MTime, info.StorageURL)
		if err != nil {
			return catalog.UpsertResult{}, err
		}
		_, err = tx.Exec(ctx, `update assets set media_kind=$1, display_name=$2, updated_at=now() where id=$3`, info.MediaKind, info.Name, existingAssetID)
		if err != nil {
			return catalog.UpsertResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return catalog.UpsertResult{}, err
		}
		asset, err := db.GetAsset(ctx, existingAssetID)
		return catalog.UpsertResult{Asset: asset}, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return catalog.UpsertResult{}, err
	}

	assetID := id.NewUUID()
	locationID := id.NewUUID()
	_, err = tx.Exec(ctx, `insert into assets(id, media_kind, display_name) values($1, $2, $3)`, assetID, info.MediaKind, info.Name)
	if err != nil {
		return catalog.UpsertResult{}, err
	}
	_, err = tx.Exec(ctx, `
		insert into asset_locations(id, asset_id, storage_id, url, relative_path, file_name, extension, mime_type, media_kind, size_bytes, mtime, hash_status)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'unhashed')
	`, locationID, assetID, storageID, info.StorageURL, info.RelativePath, info.Name, info.Extension, info.MIME, info.MediaKind, info.SizeBytes, info.MTime)
	if err != nil {
		return catalog.UpsertResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return catalog.UpsertResult{}, err
	}
	asset, err := db.GetAsset(ctx, assetID)
	return catalog.UpsertResult{Asset: asset, Created: true}, err
}

func (db *DB) ListAssets(ctx context.Context) ([]catalog.Asset, error) {
	rows, err := db.pool.Query(ctx, `
		select a.id::text, a.media_kind, a.display_name, a.first_seen_at, a.updated_at,
			l.id::text, l.storage_id::text, s.name, l.url, l.relative_path, l.file_name, l.extension, l.mime_type, l.media_kind,
			l.size_bytes, l.mtime, l.hash_status, coalesce(encode(c.sha512, 'hex'), ''), coalesce(l.content_id::text, ''), l.last_seen_at
		from assets a
		left join asset_locations l on l.asset_id=a.id
		left join storage_backends s on s.id=l.storage_id
		left join contents c on c.id=l.content_id
		order by l.url
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssets(rows)
}

func (db *DB) GetAsset(ctx context.Context, assetID string) (catalog.Asset, error) {
	rows, err := db.pool.Query(ctx, `
		select a.id::text, a.media_kind, a.display_name, a.first_seen_at, a.updated_at,
			l.id::text, l.storage_id::text, s.name, l.url, l.relative_path, l.file_name, l.extension, l.mime_type, l.media_kind,
			l.size_bytes, l.mtime, l.hash_status, coalesce(encode(c.sha512, 'hex'), ''), coalesce(l.content_id::text, ''), l.last_seen_at
		from assets a
		left join asset_locations l on l.asset_id=a.id
		left join storage_backends s on s.id=l.storage_id
		left join contents c on c.id=l.content_id
		where a.id=$1
		order by l.url
	`, assetID)
	if err != nil {
		return catalog.Asset{}, err
	}
	defer rows.Close()
	assets, err := scanAssets(rows)
	if err != nil {
		return catalog.Asset{}, err
	}
	if len(assets) == 0 {
		return catalog.Asset{}, catalog.ErrNotFound
	}
	return assets[0], nil
}

func (db *DB) UpdateLocationHash(ctx context.Context, assetID, sha512Hex string, bytes int64) error {
	hashBytes, err := hex.DecodeString(sha512Hex)
	if err != nil {
		return err
	}
	contentID := id.NewUUID()
	var existingContentID string
	err = db.pool.QueryRow(ctx, `select id::text from contents where sha512=$1`, hashBytes).Scan(&existingContentID)
	if err == nil {
		contentID = existingContentID
	} else if errors.Is(err, pgx.ErrNoRows) {
		_, err = db.pool.Exec(ctx, `insert into contents(id, sha512, size_bytes) values($1, $2, $3)`, contentID, hashBytes, bytes)
		if err != nil {
			return err
		}
	} else {
		return err
	}
	cmd, err := db.pool.Exec(ctx, `
		update asset_locations
		set content_id=$1, hash_status='hashed', size_bytes=$2
		where asset_id=$3
	`, contentID, bytes, assetID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	_, err = db.pool.Exec(ctx, `update assets set updated_at=now() where id=$1`, assetID)
	return err
}

func (db *DB) Stats(ctx context.Context) (catalog.Stats, error) {
	var stats catalog.Stats
	err := db.pool.QueryRow(ctx, `
		select
			(select count(*) from assets),
			(select count(*) from asset_locations),
			coalesce(sum(case when media_kind='photo' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='video' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='track' then 1 else 0 end), 0),
			coalesce(sum(case when hash_status='hashed' then 1 else 0 end), 0),
			coalesce(sum(case when hash_status <> 'hashed' then 1 else 0 end), 0),
			coalesce(sum(size_bytes), 0)
		from asset_locations
	`).Scan(&stats.Assets, &stats.Locations, &stats.Photos, &stats.Videos, &stats.Tracks, &stats.Hashed, &stats.Unhashed, &stats.TotalBytes)
	return stats, err
}

func (db *DB) EnqueueJob(ctx context.Context, job jobs.Job) (jobs.Job, error) {
	payload, counters, err := marshalJobJSON(job)
	if err != nil {
		return jobs.Job{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into jobs(id, kind, status, payload_json, counters_json, progress_current, progress_total, attempts, max_attempts, created_at, started_at, finished_at, error)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, job.ID, job.Kind, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts, job.CreatedAt, job.StartedAt, job.FinishedAt, nullString(job.Error))
	return job, err
}

func (db *DB) UpdateJob(ctx context.Context, job jobs.Job) error {
	payload, counters, err := marshalJobJSON(job)
	if err != nil {
		return err
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	cmd, err := tx.Exec(ctx, `
		update jobs
		set status=$2, payload_json=$3, counters_json=$4, progress_current=$5, progress_total=$6, attempts=$7, max_attempts=$8,
			started_at=$9, finished_at=$10, error=$11
		where id=$1
	`, job.ID, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts, job.StartedAt, job.FinishedAt, nullString(job.Error))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `delete from job_logs where job_id=$1`, job.ID); err != nil {
		return err
	}
	for _, line := range job.Logs {
		if _, err := tx.Exec(ctx, `insert into job_logs(job_id, level, message, created_at) values($1, $2, $3, $4)`, job.ID, line.Level, line.Message, line.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (db *DB) ListJobs(ctx context.Context) ([]jobs.Job, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, kind, status, payload_json, counters_json, progress_current, progress_total, attempts, max_attempts, created_at, started_at, finished_at, coalesce(error, '')
		from jobs order by created_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		logs, err := db.jobLogs(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Logs = logs
	}
	return out, nil
}

func (db *DB) GetJob(ctx context.Context, jobID string) (jobs.Job, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, kind, status, payload_json, counters_json, progress_current, progress_total, attempts, max_attempts, created_at, started_at, finished_at, coalesce(error, '')
		from jobs where id=$1
	`, jobID)
	if err != nil {
		return jobs.Job{}, err
	}
	defer rows.Close()
	out, err := scanJobs(rows)
	if err != nil {
		return jobs.Job{}, err
	}
	if len(out) == 0 {
		return jobs.Job{}, catalog.ErrNotFound
	}
	out[0].Logs, err = db.jobLogs(ctx, out[0].ID)
	return out[0], err
}

func (db *DB) ensureStorage(ctx context.Context, name, kind, root, mode string) (string, error) {
	var storageID string
	err := db.pool.QueryRow(ctx, `select id::text from storage_backends where name=$1`, name).Scan(&storageID)
	if err == nil {
		if root != "" {
			_, _ = db.pool.Exec(ctx, `update storage_backends set kind=$2, root=$3, mode=$4, updated_at=now() where id=$1`, storageID, kind, root, mode)
		}
		return storageID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	storageID = id.NewUUID()
	if kind == "" {
		kind = "fs"
	}
	if mode == "" {
		mode = "strict_read_only"
	}
	_, err = db.pool.Exec(ctx, `insert into storage_backends(id, name, kind, root, mode) values($1, $2, $3, $4, $5)`, storageID, name, kind, root, mode)
	return storageID, err
}

func scanAssets(rows pgx.Rows) ([]catalog.Asset, error) {
	byID := map[string]int{}
	var assets []catalog.Asset
	for rows.Next() {
		var asset catalog.Asset
		var loc catalog.Location
		var storageID string
		if err := rows.Scan(&asset.ID, &asset.MediaKind, &asset.DisplayName, &asset.FirstSeenAt, &asset.UpdatedAt,
			&loc.ID, &storageID, &loc.StorageName, &loc.StorageURL, &loc.RelativePath, &loc.FileName, &loc.Extension, &loc.MIME, &loc.MediaKind,
			&loc.SizeBytes, &loc.MTime, &loc.HashStatus, &loc.SHA512Hex, &loc.ContentID, &loc.LastSeenAt); err != nil {
			return nil, err
		}
		idx, ok := byID[asset.ID]
		if !ok {
			asset.Locations = nil
			assets = append(assets, asset)
			idx = len(assets) - 1
			byID[asset.ID] = idx
		}
		if loc.ID != "" {
			loc.AssetID = asset.ID
			assets[idx].Locations = append(assets[idx].Locations, loc)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

func scanJobs(rows pgx.Rows) ([]jobs.Job, error) {
	var out []jobs.Job
	for rows.Next() {
		var job jobs.Job
		var status string
		var payloadBytes []byte
		var countersBytes []byte
		if err := rows.Scan(&job.ID, &job.Kind, &status, &payloadBytes, &countersBytes, &job.ProgressCurrent, &job.ProgressTotal, &job.Attempts, &job.MaxAttempts, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.Error); err != nil {
			return nil, err
		}
		job.Status = jobs.Status(status)
		if len(payloadBytes) > 0 {
			job.Payload = json.RawMessage(payloadBytes)
		}
		if len(countersBytes) > 0 {
			_ = json.Unmarshal(countersBytes, &job.Counters)
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (db *DB) jobLogs(ctx context.Context, jobID string) ([]jobs.LogLine, error) {
	rows, err := db.pool.Query(ctx, `select level, message, created_at from job_logs where job_id=$1 order by created_at`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []jobs.LogLine
	for rows.Next() {
		var line jobs.LogLine
		if err := rows.Scan(&line.Level, &line.Message, &line.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func marshalJobJSON(job jobs.Job) ([]byte, []byte, error) {
	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		return nil, nil, err
	}
	if job.Payload != nil {
		payload, err = json.Marshal(job.Payload)
		if err != nil {
			return nil, nil, err
		}
	}
	counters, err := json.Marshal(job.Counters)
	return payload, counters, err
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func IsConnectError(err error) bool {
	var pgErr *pgconn.ConnectError
	return errors.As(err, &pgErr)
}

var _ catalog.Store = (*DB)(nil)

func NowUTC() time.Time { return time.Now().UTC() }
