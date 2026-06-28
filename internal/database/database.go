package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
	embeddedmigrations "github.com/AxisAlexNT/Cartolensia/migrations"
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
	return loadMigrations(os.DirFS(dir), ".", func(name string) string {
		return filepath.Join(dir, name)
	})
}

func LoadEmbeddedMigrations() ([]Migration, error) {
	return LoadMigrationsFS(embeddedmigrations.FS, ".")
}

func LoadMigrationsFS(fsys fs.FS, dir string) ([]Migration, error) {
	return loadMigrations(fsys, dir, func(name string) string {
		return path.Join(dir, name)
	})
}

func loadMigrations(fsys fs.FS, dir string, displayPath func(string) string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	migrations := make([]Migration, 0, len(names))
	for _, name := range names {
		fsysPath := path.Join(dir, name)
		data, err := fs.ReadFile(fsys, fsysPath)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", fsysPath, err)
		}
		sum := sha256.Sum256(data)
		version := strings.TrimSuffix(name, ".sql")
		migrations = append(migrations, Migration{
			Version:  version,
			Path:     displayPath(name),
			SQL:      string(data),
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return migrations, nil
}

func (db *DB) ApplyMigrations(ctx context.Context, dir string) error {
	return db.ApplyMigrationsFromDir(ctx, dir)
}

func (db *DB) ApplyMigrationsFromDir(ctx context.Context, dir string) error {
	migrations, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	return db.applyMigrations(ctx, migrations)
}

func (db *DB) ApplyEmbeddedMigrations(ctx context.Context) error {
	migrations, err := LoadEmbeddedMigrations()
	if err != nil {
		return err
	}
	return db.applyMigrations(ctx, migrations)
}

func (db *DB) applyMigrations(ctx context.Context, migrations []Migration) error {
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

func (db *DB) EnsureOptionalVectorSchema(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `
do $$
begin
    create extension if not exists vector;
exception when others then
    raise notice 'pgvector extension is not available: %', sqlerrm;
end $$;

do $$
begin
    if exists(select 1 from pg_extension where extname = 'vector') then
        execute 'alter table asset_embeddings add column if not exists embedding_vector vector(512)';
        execute 'create index if not exists idx_asset_embeddings_vector_cosine on asset_embeddings using ivfflat (embedding_vector vector_cosine_ops) with (lists = 100)';
    end if;
exception when others then
    raise notice 'optional pgvector embedding setup skipped: %', sqlerrm;
end $$;`)
	return err
}

func (db *DB) PGVectorReady(ctx context.Context) bool {
	var ready bool
	err := db.pool.QueryRow(ctx, `
		select exists(select 1 from pg_extension where extname='vector')
		   and exists(select 1 from information_schema.columns where table_name='asset_embeddings' and column_name='embedding_vector')`).Scan(&ready)
	return err == nil && ready
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
		if _, err := db.ensureStorage(ctx, st.Name, st.Kind, st.Root, st.Mode, storageConfigJSON(st)); err != nil {
			return err
		}
	}
	return nil
}

func storageConfigJSON(st config.StorageConfig) []byte {
	data, err := json.Marshal(map[string]any{
		"source_url": st.SourceURL,
		"smb":        st.SMB,
	})
	if err != nil {
		return []byte(`{}`)
	}
	return data
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

func (db *DB) BootstrapAdmin(ctx context.Context, user auth.User) (auth.User, bool, error) {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return auth.User{}, false, err
	}
	defer rollback(ctx, tx)
	var existing auth.User
	err = tx.QueryRow(ctx, `
		select id::text, email, display_name, coalesce(password_hash, ''), role, disabled_at
		from users where email=$1 for update
	`, user.Email).Scan(&existing.ID, &existing.Email, &existing.DisplayName, &existing.PasswordHash, &existing.Role, &existing.DisabledAt)
	if err == nil {
		if existing.PasswordHash == "" && user.PasswordHash != "" {
			if _, err := tx.Exec(ctx, `update users set password_hash=$2, updated_at=now() where id=$1`, existing.ID, user.PasswordHash); err != nil {
				return auth.User{}, false, err
			}
			existing.PasswordHash = user.PasswordHash
		}
		if err := tx.Commit(ctx); err != nil {
			return auth.User{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		insert into users(id, email, display_name, password_hash, role)
		values($1, $2, $3, $4, $5)
	`, user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role); err != nil {
		return auth.User{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return auth.User{}, false, err
	}
	return user, true, nil
}

func (db *DB) UserByEmail(ctx context.Context, email string) (auth.User, error) {
	var user auth.User
	err := db.pool.QueryRow(ctx, `
		select id::text, email, display_name, coalesce(password_hash, ''), role, disabled_at
		from users where email=$1
	`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.DisabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	return user, err
}

func (db *DB) UserByID(ctx context.Context, userID string) (auth.User, error) {
	var user auth.User
	err := db.pool.QueryRow(ctx, `
		select id::text, email, display_name, coalesce(password_hash, ''), role, disabled_at
		from users where id=$1
	`, userID).Scan(&user.ID, &user.Email, &user.DisplayName, &user.PasswordHash, &user.Role, &user.DisabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	return user, err
}

func (db *DB) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	cmd, err := db.pool.Exec(ctx, `update users set password_hash=$2, updated_at=now() where id=$1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return auth.ErrNotFound
	}
	_, _ = db.pool.Exec(ctx, `delete from sessions where user_id=$1`, userID)
	return nil
}

func (db *DB) CreateSession(ctx context.Context, sessionID, userID string, tokenHash []byte, expiresAt time.Time) error {
	_, err := db.pool.Exec(ctx, `
		insert into sessions(id, user_id, token_hash, expires_at)
		values($1, $2, $3, $4)
	`, sessionID, userID, tokenHash, expiresAt)
	return err
}

func (db *DB) PrincipalBySessionHash(ctx context.Context, tokenHash []byte, now time.Time) (auth.Principal, error) {
	var principal auth.Principal
	err := db.pool.QueryRow(ctx, `
		update sessions set last_seen_at=$2
		where token_hash=$1 and expires_at > $2
		returning user_id::text
	`, tokenHash, now).Scan(&principal.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return auth.Principal{}, err
	}
	err = db.pool.QueryRow(ctx, `
		select id::text, display_name, email, role from users where id=$1 and disabled_at is null
	`, principal.ID).Scan(&principal.ID, &principal.Name, &principal.Email, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	principal.AuthMethod = auth.AuthMethodSession
	return principal, err
}

func (db *DB) DeleteSessionByHash(ctx context.Context, tokenHash []byte) error {
	_, err := db.pool.Exec(ctx, `delete from sessions where token_hash=$1`, tokenHash)
	return err
}

func (db *DB) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	cmd, err := db.pool.Exec(ctx, `delete from sessions where expires_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (db *DB) PrincipalByAPITokenHash(ctx context.Context, tokenHash []byte, now time.Time) (auth.Principal, error) {
	var principal auth.Principal
	var scopes []string
	err := db.pool.QueryRow(ctx, `
		update api_tokens set last_used_at=$2
		where token_hash=$1
			and revoked_at is null
			and (expires_at is null or expires_at > $2)
		returning user_id::text, scopes
	`, tokenHash, now).Scan(&principal.ID, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return auth.Principal{}, err
	}
	err = db.pool.QueryRow(ctx, `
		select id::text, display_name, email, role from users where id=$1 and disabled_at is null
	`, principal.ID).Scan(&principal.ID, &principal.Name, &principal.Email, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	principal.AuthMethod = auth.AuthMethodAPIToken
	principal.Scopes = scopes
	return principal, err
}

func (db *DB) CreateAPIToken(ctx context.Context, token auth.APIToken, tokenHash []byte) error {
	_, err := db.pool.Exec(ctx, `
		insert into api_tokens(id, user_id, name, token_hash, scopes, expires_at, created_at)
		values($1, $2, $3, $4, $5, $6, $7)
	`, token.ID, token.UserID, token.Name, tokenHash, token.Scopes, token.ExpiresAt, token.CreatedAt)
	return err
}

func (db *DB) ListAPITokens(ctx context.Context, userID string) ([]auth.APIToken, error) {
	rows, err := db.pool.Query(ctx, `
		select id::text, user_id::text, name, scopes, expires_at, created_at, last_used_at, revoked_at
		from api_tokens where user_id=$1 order by created_at desc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []auth.APIToken
	for rows.Next() {
		var token auth.APIToken
		if err := rows.Scan(&token.ID, &token.UserID, &token.Name, &token.Scopes, &token.ExpiresAt, &token.CreatedAt, &token.LastUsedAt, &token.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, token)
	}
	return out, rows.Err()
}

func (db *DB) RevokeAPIToken(ctx context.Context, userID, tokenID string) error {
	cmd, err := db.pool.Exec(ctx, `update api_tokens set revoked_at=now() where id=$1 and user_id=$2`, tokenID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return auth.ErrNotFound
	}
	return nil
}

func (db *DB) UpsertDiscoveredFile(ctx context.Context, info storage.FileInfo) (catalog.UpsertResult, error) {
	storageID, err := db.ensureStorage(ctx, info.StorageName, "fs", "", "strict_read_only", []byte(`{}`))
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
		select a.id::text, a.media_kind, a.display_name, a.taken_at, a.metadata_json, a.first_seen_at, a.updated_at,
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
		select a.id::text, a.media_kind, a.display_name, a.taken_at, a.metadata_json, a.first_seen_at, a.updated_at,
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
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	contentID := id.NewUUID()
	var existingContentID string
	err = tx.QueryRow(ctx, `select id::text from contents where sha512=$1 and size_bytes=$2`, hashBytes, bytes).Scan(&existingContentID)
	if err == nil {
		contentID = existingContentID
	} else if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `insert into contents(id, sha512, size_bytes) values($1, $2, $3)`, contentID, hashBytes, bytes)
		if err != nil {
			return err
		}
	} else {
		return err
	}

	targetAssetID := assetID
	var existingAssetID string
	err = tx.QueryRow(ctx, `
		select asset_id::text
		from asset_locations
		where content_id=$1 and asset_id<>$2
		order by last_seen_at, id
		limit 1
	`, contentID, assetID).Scan(&existingAssetID)
	if err == nil {
		targetAssetID = existingAssetID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	cmd, err := tx.Exec(ctx, `
		update asset_locations
		set content_id=$1, hash_status='hashed', size_bytes=$2, asset_id=$3
		where asset_id=$4
	`, contentID, bytes, targetAssetID, assetID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	if targetAssetID != assetID {
		_, err = tx.Exec(ctx, `
			update assets target
			set taken_at=coalesce(target.taken_at, source.taken_at),
				metadata_json=source.metadata_json || target.metadata_json,
				updated_at=now()
			from assets source
			where target.id=$1 and source.id=$2
		`, targetAssetID, assetID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			delete from assets a
			where a.id=$1 and not exists(select 1 from asset_locations l where l.asset_id=a.id)
		`, assetID)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.Exec(ctx, `update assets set updated_at=now() where id=$1`, assetID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (db *DB) UpdateAssetMetadata(ctx context.Context, assetID string, takenAt *time.Time, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	data, err := json.Marshal(metadataOrEmpty(metadata))
	if err != nil {
		return err
	}
	cmd, err := db.pool.Exec(ctx, `
		update assets
		set taken_at=coalesce($2, taken_at),
			metadata_json=metadata_json || $3::jsonb,
			updated_at=now()
		where id=$1
	`, assetID, takenAt, data)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) Stats(ctx context.Context) (catalog.Stats, error) {
	var stats catalog.Stats
	err := db.pool.QueryRow(ctx, `
		select
			(select count(*) from assets),
			(select count(*) from asset_locations),
			coalesce(sum(case when media_kind='photo' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='video' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='audio' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='document' then 1 else 0 end), 0),
			coalesce(sum(case when media_kind='track' then 1 else 0 end), 0),
			coalesce(sum(case when hash_status='hashed' then 1 else 0 end), 0),
			coalesce(sum(case when hash_status <> 'hashed' then 1 else 0 end), 0),
			coalesce(sum(size_bytes), 0)
		from asset_locations
	`).Scan(&stats.Assets, &stats.Locations, &stats.Photos, &stats.Videos, &stats.Audio, &stats.Documents, &stats.Tracks, &stats.Hashed, &stats.Unhashed, &stats.TotalBytes)
	if err != nil {
		return stats, err
	}
	err = db.pool.QueryRow(ctx, `
		select count(*)::int, coalesce(sum(location_count), 0)::int
		from (
			select content_id, count(*)::int as location_count
			from asset_locations
			where content_id is not null
			group by content_id
			having count(*) > 1
		) duplicates
	`).Scan(&stats.DuplicateGroups, &stats.DuplicateLocations)
	return stats, err
}

type trackRenderLevel struct {
	Name      string
	MaxPoints int
}

func trackRenderLevels() []trackRenderLevel {
	return []trackRenderLevel{
		{Name: "overview", MaxPoints: 2},
		{Name: "z0", MaxPoints: 4},
		{Name: "z6", MaxPoints: 16},
		{Name: "z10", MaxPoints: 64},
		{Name: "z13", MaxPoints: 256},
		{Name: "z16", MaxPoints: 1024},
	}
}

func trackRenderDetailLevelForMaxPoints(maxPoints int) string {
	switch {
	case maxPoints <= 4:
		return "overview"
	case maxPoints <= 16:
		return "z6"
	case maxPoints <= 64:
		return "z10"
	case maxPoints <= 256:
		return "z13"
	case maxPoints <= 1024:
		return "z16"
	default:
		return ""
	}
}

func insertTrackRenderPoints(ctx context.Context, tx pgx.Tx, trackAssetID string, points []catalog.TrackPoint) error {
	if len(points) == 0 {
		return nil
	}
	for _, level := range trackRenderLevels() {
		sampled := downsampleTrackPoints(points, level.MaxPoints)
		for idx, point := range sampled {
			source := point.Source
			if source == "" {
				source = "gpx"
			}
			if _, err := tx.Exec(ctx, `
				insert into gps_track_render_points(track_asset_id, detail_level, ordinal, recorded_at, lat, lon, elevation_m, speed_mps, source)
				values($1, $2, $3, $4, $5, $6, $7, $8, $9)
				on conflict(track_asset_id, detail_level, ordinal) do update set
					recorded_at=excluded.recorded_at,
					lat=excluded.lat,
					lon=excluded.lon,
					elevation_m=excluded.elevation_m,
					speed_mps=excluded.speed_mps,
					source=excluded.source,
					updated_at=now()
			`, trackAssetID, level.Name, idx+1, point.RecordedAt, point.Lat, point.Lon, point.ElevationM, point.SpeedMPS, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *DB) UpsertTrackPoints(ctx context.Context, trackAssetID string, points []catalog.TrackPoint) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if _, err := tx.Exec(ctx, `delete from track_points where track_asset_id=$1`, trackAssetID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from gps_track_render_points where track_asset_id=$1`, trackAssetID); err != nil {
		return err
	}
	for _, point := range points {
		source := point.Source
		if source == "" {
			source = "gpx"
		}
		if _, err := tx.Exec(ctx, `
			insert into track_points(track_asset_id, recorded_at, lat, lon, elevation_m, speed_mps, source)
			values($1, $2, $3, $4, $5, $6, $7)
		`, trackAssetID, point.RecordedAt, point.Lat, point.Lon, point.ElevationM, point.SpeedMPS, source); err != nil {
			return err
		}
	}
	if err := insertTrackRenderPoints(ctx, tx, trackAssetID, points); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db *DB) ListTracks(ctx context.Context) ([]catalog.TrackSummary, error) {
	rows, err := db.pool.Query(ctx, `
		select a.id::text, a.display_name, count(tp.id)::int,
			min(tp.recorded_at), max(tp.recorded_at), min(tp.lat), min(tp.lon), max(tp.lat), max(tp.lon), a.metadata_json::text
		from assets a
		join track_points tp on tp.track_asset_id=a.id
		group by a.id, a.display_name, a.metadata_json
		order by min(tp.recorded_at), a.display_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.TrackSummary
	for rows.Next() {
		var summary catalog.TrackSummary
		var metadataText string
		if err := rows.Scan(&summary.TrackAssetID, &summary.Name, &summary.PointCount, &summary.StartTime, &summary.EndTime, &summary.MinLat, &summary.MinLon, &summary.MaxLat, &summary.MaxLon, &metadataText); err != nil {
			return nil, err
		}
		applyTrackMetadata(&summary, metadataText)
		out = append(out, summary)
	}
	return out, rows.Err()
}

func applyTrackMetadata(summary *catalog.TrackSummary, metadataText string) {
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataText), &metadata); err != nil {
		return
	}
	if value, ok := metadataFloat(metadata, "distance_m"); ok {
		summary.DistanceM = value
	}
	if value, ok := metadataFloat(metadata, "duration_seconds"); ok {
		summary.DurationSec = &value
	}
	if value, ok := metadataFloat(metadata, "elevation_min_m"); ok {
		summary.ElevationMin = &value
	}
	if value, ok := metadataFloat(metadata, "elevation_max_m"); ok {
		summary.ElevationMax = &value
	}
	if value, ok := metadata["source"].(string); ok {
		summary.SourceFormat = value
	}
	if value, ok := metadata["track_source_format"].(string); ok && summary.SourceFormat == "" {
		summary.SourceFormat = value
	}
}

func metadataFloat(metadata map[string]any, key string) (float64, bool) {
	switch value := metadata[key].(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (db *DB) GetTrack(ctx context.Context, trackAssetID string) (catalog.TrackDetail, error) {
	summaries, err := db.ListTracks(ctx)
	if err != nil {
		return catalog.TrackDetail{}, err
	}
	var summary catalog.TrackSummary
	found := false
	for _, candidate := range summaries {
		if candidate.TrackAssetID == trackAssetID {
			summary = candidate
			found = true
			break
		}
	}
	if !found {
		return catalog.TrackDetail{}, catalog.ErrNotFound
	}
	rows, err := db.pool.Query(ctx, `
		select id, track_asset_id::text, recorded_at, lat, lon, elevation_m, speed_mps, source
		from track_points where track_asset_id=$1 order by recorded_at, id
	`, trackAssetID)
	if err != nil {
		return catalog.TrackDetail{}, err
	}
	defer rows.Close()
	var points []catalog.TrackPoint
	for rows.Next() {
		var point catalog.TrackPoint
		if err := rows.Scan(&point.ID, &point.TrackAssetID, &point.RecordedAt, &point.Lat, &point.Lon, &point.ElevationM, &point.SpeedMPS, &point.Source); err != nil {
			return catalog.TrackDetail{}, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return catalog.TrackDetail{}, err
	}
	return catalog.TrackDetail{Summary: summary, Points: points}, nil
}

func (db *DB) TrackCandidates(ctx context.Context, assetID string) ([]catalog.TrackCandidate, error) {
	asset, err := db.GetAsset(ctx, assetID)
	if err != nil {
		return nil, err
	}
	start, end, ok := assetIntervalForDB(asset)
	if !ok {
		return nil, nil
	}
	rows, err := db.pool.Query(ctx, `
		with summaries as (
			select a.id::text as track_asset_id, a.display_name, count(tp.id)::int as point_count,
				min(tp.recorded_at) as start_time, max(tp.recorded_at) as end_time,
				min(tp.lat) as min_lat, min(tp.lon) as min_lon, max(tp.lat) as max_lat, max(tp.lon) as max_lon
			from assets a
			join track_points tp on tp.track_asset_id=a.id
			group by a.id, a.display_name
		)
		select track_asset_id, display_name, point_count, start_time, end_time, min_lat, min_lon, max_lat, max_lon,
			greatest(start_time, $1::timestamptz), least(end_time, $2::timestamptz)
		from summaries
		where start_time <= $2 and end_time >= $1
		order by start_time
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.TrackCandidate
	for rows.Next() {
		var candidate catalog.TrackCandidate
		if err := rows.Scan(&candidate.Track.TrackAssetID, &candidate.Track.Name, &candidate.Track.PointCount, &candidate.Track.StartTime, &candidate.Track.EndTime,
			&candidate.Track.MinLat, &candidate.Track.MinLon, &candidate.Track.MaxLat, &candidate.Track.MaxLon, &candidate.OverlapStart, &candidate.OverlapEnd); err != nil {
			return nil, err
		}
		if candidate.OverlapStart != nil && candidate.OverlapEnd != nil {
			duration := end.Sub(start).Seconds()
			if duration <= 0 {
				duration = 1
			}
			candidate.OverlapSeconds = candidate.OverlapEnd.Sub(*candidate.OverlapStart).Seconds()
			candidate.Confidence = candidate.OverlapEnd.Sub(*candidate.OverlapStart).Seconds() / duration
			if candidate.Confidence > 1 {
				candidate.Confidence = 1
			}
			candidate.Reason = "overlapping asset and track time intervals"
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func (db *DB) SaveTrackLink(ctx context.Context, link catalog.TrackLink) (catalog.TrackLink, error) {
	now := time.Now().UTC()
	if link.ID == "" {
		link.ID = id.NewUUID()
	}
	if link.MatchStatus == "" {
		link.MatchStatus = "manual"
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now
	_, err := db.pool.Exec(ctx, `
		insert into asset_track_links(id, asset_id, track_asset_id, match_status, overlap_start, overlap_end, time_offset_ms, confidence, created_at, updated_at)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		on conflict(id) do update set
			match_status=excluded.match_status,
			overlap_start=excluded.overlap_start,
			overlap_end=excluded.overlap_end,
			time_offset_ms=excluded.time_offset_ms,
			confidence=excluded.confidence,
			updated_at=excluded.updated_at
	`, link.ID, link.AssetID, link.TrackAssetID, link.MatchStatus, link.OverlapStart, link.OverlapEnd, link.TimeOffsetMS, link.Confidence, link.CreatedAt, link.UpdatedAt)
	return link, err
}

func (db *DB) ListTrackLinks(ctx context.Context, assetID string) ([]catalog.TrackLink, error) {
	query := `
		select id::text, asset_id::text, track_asset_id::text, match_status, overlap_start, overlap_end, time_offset_ms, confidence, created_at, updated_at
		from asset_track_links`
	args := []any{}
	if assetID != "" {
		args = append(args, assetID)
		query += ` where asset_id=$1`
	}
	query += ` order by created_at desc`
	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.TrackLink
	for rows.Next() {
		var link catalog.TrackLink
		if err := rows.Scan(&link.ID, &link.AssetID, &link.TrackAssetID, &link.MatchStatus, &link.OverlapStart, &link.OverlapEnd, &link.TimeOffsetMS, &link.Confidence, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (db *DB) DeleteTrackLink(ctx context.Context, linkID string) error {
	cmd, err := db.pool.Exec(ctx, `delete from asset_track_links where id=$1`, linkID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrNotFound
	}
	return nil
}

func (db *DB) EnqueueJob(ctx context.Context, job jobs.Job) (jobs.Job, error) {
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	payload, counters, err := marshalJobJSON(job)
	if err != nil {
		return jobs.Job{}, err
	}
	_, err = db.pool.Exec(ctx, `
		insert into jobs(id, kind, status, payload_json, counters_json, progress_current, progress_total, attempts, max_attempts,
			worker_id, lease_expires_at, cancel_requested_at, next_run_at, created_at, started_at, finished_at, error)
		values($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, job.ID, job.Kind, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts,
		nullString(job.WorkerID), job.LeaseExpiresAt, job.CancelRequestedAt, job.NextRunAt, job.CreatedAt, job.StartedAt, job.FinishedAt, nullString(job.Error))
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
			worker_id=$9, lease_expires_at=$10, cancel_requested_at=$11, next_run_at=$12, started_at=$13, finished_at=$14, error=$15
		where id=$1
	`, job.ID, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts,
		nullString(job.WorkerID), job.LeaseExpiresAt, job.CancelRequestedAt, job.NextRunAt, job.StartedAt, job.FinishedAt, nullString(job.Error))
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
		select `+jobColumns+`
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
		select `+jobColumns+`
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

func (db *DB) LeaseNextJob(ctx context.Context, workerID string, kinds []string, leaseDuration time.Duration) (jobs.Job, error) {
	if workerID == "" {
		return jobs.Job{}, fmt.Errorf("worker id is required")
	}
	_, _ = db.ReleaseExpiredLeases(ctx, time.Now().UTC())
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return jobs.Job{}, err
	}
	defer rollback(ctx, tx)

	query := `select id::text from jobs
		where status='queued' and (next_run_at is null or next_run_at <= now())`
	args := []any{}
	if len(kinds) > 0 {
		args = append(args, kinds)
		query += fmt.Sprintf(" and kind=any($%d)", len(args))
	}
	query += ` order by created_at, id for update skip locked limit 1`
	var jobID string
	if err := tx.QueryRow(ctx, query, args...).Scan(&jobID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.Job{}, catalog.ErrNotFound
		}
		return jobs.Job{}, err
	}
	leaseUntil := time.Now().UTC().Add(leaseDuration)
	rows, err := tx.Query(ctx, `
		update jobs
		set status='running',
			worker_id=$2,
			lease_expires_at=$3,
			started_at=coalesce(started_at, now()),
			attempts=attempts + 1,
			next_run_at=null,
			error=null
		where id=$1
		returning `+jobColumns+`
	`, jobID, workerID, leaseUntil)
	if err != nil {
		return jobs.Job{}, err
	}
	out, err := scanJobs(rows)
	rows.Close()
	if err != nil {
		return jobs.Job{}, err
	}
	if len(out) != 1 {
		return jobs.Job{}, catalog.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return jobs.Job{}, err
	}
	out[0].Logs, err = db.jobLogs(ctx, out[0].ID)
	return out[0], err
}

func (db *DB) HeartbeatJob(ctx context.Context, jobID, workerID string, leaseDuration time.Duration) error {
	leaseUntil := time.Now().UTC().Add(leaseDuration)
	cmd, err := db.pool.Exec(ctx, `
		update jobs
		set lease_expires_at=$3
		where id=$1 and worker_id=$2 and status in ('running', 'cancel_requested')
	`, jobID, workerID, leaseUntil)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrJobLeaseLost
	}
	return nil
}

func (db *DB) UpdateLeasedJob(ctx context.Context, job jobs.Job, workerID string) error {
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
		set status=case when status='cancel_requested' and $2='running' then status else $2 end,
			payload_json=$3,
			counters_json=$4,
			progress_current=$5,
			progress_total=$6,
			attempts=$7,
			max_attempts=$8,
			cancel_requested_at=coalesce(cancel_requested_at, $9),
			next_run_at=$10,
			started_at=$11,
			finished_at=$12,
			error=$13
		where id=$1 and worker_id=$14 and status in ('running', 'cancel_requested')
	`, job.ID, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts,
		job.CancelRequestedAt, job.NextRunAt, job.StartedAt, job.FinishedAt, nullString(job.Error), workerID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrJobLeaseLost
	}
	if err := replaceJobLogs(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db *DB) CompleteLeasedJob(ctx context.Context, job jobs.Job, workerID string) error {
	if err := jobs.Complete(&job); err != nil {
		return err
	}
	return db.finishLeasedJob(ctx, job, workerID, "running")
}

func (db *DB) FailLeasedJob(ctx context.Context, job jobs.Job, workerID string, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("job failed")
	}
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if job.Attempts < maxAttempts && job.Status != jobs.StatusCancelRequested && jobs.ShouldRetry(cause) {
		delay := time.Duration(job.Attempts)
		if delay <= 0 {
			delay = 1
		}
		if delay > 5 {
			delay = 5
		}
		if err := jobs.Retry(&job, delay*time.Second, cause); err != nil {
			return err
		}
		jobs.AddLog(&job, "warn", fmt.Sprintf("will retry after %s: %v", delay*time.Second, cause))
		return db.finishLeasedJob(ctx, job, workerID, "running")
	}
	if err := jobs.Fail(&job, cause); err != nil {
		return err
	}
	jobs.AddLog(&job, "error", cause.Error())
	return db.finishLeasedJob(ctx, job, workerID, "running", "cancel_requested")
}

func (db *DB) CancelLeasedJob(ctx context.Context, job jobs.Job, workerID string) error {
	if err := jobs.Cancel(&job); err != nil {
		return err
	}
	jobs.AddLog(&job, "info", "job canceled")
	return db.finishLeasedJob(ctx, job, workerID, "running", "cancel_requested")
}

func (db *DB) RequestCancelJob(ctx context.Context, jobID string) (jobs.Job, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return jobs.Job{}, err
	}
	defer rollback(ctx, tx)
	rows, err := tx.Query(ctx, `select `+jobColumns+` from jobs where id=$1 for update`, jobID)
	if err != nil {
		return jobs.Job{}, err
	}
	out, err := scanJobs(rows)
	rows.Close()
	if err != nil {
		return jobs.Job{}, err
	}
	if len(out) == 0 {
		return jobs.Job{}, catalog.ErrNotFound
	}
	job := out[0]
	if err := jobs.RequestCancel(&job); err != nil {
		return jobs.Job{}, err
	}
	jobs.AddLog(&job, "info", "cancellation requested")
	payload, counters, err := marshalJobJSON(job)
	if err != nil {
		return jobs.Job{}, err
	}
	cmd, err := tx.Exec(ctx, `
		update jobs
		set status=$2,
			payload_json=$3,
			counters_json=$4,
			cancel_requested_at=$5,
			finished_at=$6,
			error=$7
		where id=$1
	`, job.ID, string(job.Status), payload, counters, job.CancelRequestedAt, job.FinishedAt, nullString(job.Error))
	if err != nil {
		return jobs.Job{}, err
	}
	if cmd.RowsAffected() == 0 {
		return jobs.Job{}, catalog.ErrNotFound
	}
	if err := replaceJobLogs(ctx, tx, job); err != nil {
		return jobs.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return jobs.Job{}, err
	}
	job.Logs, err = db.jobLogs(ctx, job.ID)
	return job, err
}

func (db *DB) ReleaseExpiredLeases(ctx context.Context, now time.Time) (int64, error) {
	cmd, err := db.pool.Exec(ctx, `
		update jobs
		set status=case
				when status='cancel_requested' then 'canceled'
				else 'queued'
			end,
			worker_id=null,
			lease_expires_at=null,
			finished_at=case
				when status='cancel_requested' then $1
				else finished_at
			end,
			next_run_at=case
				when status='cancel_requested' then next_run_at
				else $1
			end,
			error=case
				when status='cancel_requested' then error
				else coalesce(error, 'job lease expired; retry queued')
			end
		where status in ('running', 'cancel_requested') and lease_expires_at < $1
	`, now)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (db *DB) ensureStorage(ctx context.Context, name, kind, root, mode string, configJSON []byte) (string, error) {
	var storageID string
	err := db.pool.QueryRow(ctx, `select id::text from storage_backends where name=$1`, name).Scan(&storageID)
	if err == nil {
		if root != "" {
			_, _ = db.pool.Exec(ctx, `update storage_backends set kind=$2, root=$3, mode=$4, config_json=$5, updated_at=now() where id=$1`, storageID, kind, root, mode, configJSON)
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
	_, err = db.pool.Exec(ctx, `insert into storage_backends(id, name, kind, root, mode, config_json) values($1, $2, $3, $4, $5, $6)`, storageID, name, kind, root, mode, configJSON)
	return storageID, err
}

func scanAssets(rows pgx.Rows) ([]catalog.Asset, error) {
	byID := map[string]int{}
	var assets []catalog.Asset
	for rows.Next() {
		var asset catalog.Asset
		var loc catalog.Location
		var storageID string
		var metadataBytes []byte
		if err := rows.Scan(&asset.ID, &asset.MediaKind, &asset.DisplayName, &asset.TakenAt, &metadataBytes, &asset.FirstSeenAt, &asset.UpdatedAt,
			&loc.ID, &storageID, &loc.StorageName, &loc.StorageURL, &loc.RelativePath, &loc.FileName, &loc.Extension, &loc.MIME, &loc.MediaKind,
			&loc.SizeBytes, &loc.MTime, &loc.HashStatus, &loc.SHA512Hex, &loc.ContentID, &loc.LastSeenAt); err != nil {
			return nil, err
		}
		if len(metadataBytes) > 0 {
			_ = json.Unmarshal(metadataBytes, &asset.Metadata)
		}
		if asset.Metadata == nil {
			asset.Metadata = map[string]any{}
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

func assetIntervalForDB(asset catalog.Asset) (time.Time, time.Time, bool) {
	if asset.TakenAt == nil {
		return time.Time{}, time.Time{}, false
	}
	start := asset.TakenAt.UTC()
	durationSeconds := 1.0
	if asset.Metadata != nil {
		switch value := asset.Metadata["duration_seconds"].(type) {
		case float64:
			durationSeconds = value
		case int:
			durationSeconds = float64(value)
		case int64:
			durationSeconds = float64(value)
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				durationSeconds = parsed
			}
		}
	}
	if durationSeconds <= 0 {
		durationSeconds = 1
	}
	return start, start.Add(time.Duration(durationSeconds * float64(time.Second))), true
}

func scanJobs(rows pgx.Rows) ([]jobs.Job, error) {
	var out []jobs.Job
	for rows.Next() {
		var job jobs.Job
		var status string
		var payloadBytes []byte
		var countersBytes []byte
		if err := rows.Scan(&job.ID, &job.Kind, &status, &payloadBytes, &countersBytes, &job.ProgressCurrent, &job.ProgressTotal, &job.Attempts, &job.MaxAttempts,
			&job.WorkerID, &job.LeaseExpiresAt, &job.CancelRequestedAt, &job.NextRunAt, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.Error); err != nil {
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

func (db *DB) finishLeasedJob(ctx context.Context, job jobs.Job, workerID string, allowedCurrent ...string) error {
	payload, counters, err := marshalJobJSON(job)
	if err != nil {
		return err
	}
	if len(allowedCurrent) == 0 {
		allowedCurrent = []string{"running"}
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	cmd, err := tx.Exec(ctx, `
		update jobs
		set status=$2,
			payload_json=$3,
			counters_json=$4,
			progress_current=$5,
			progress_total=$6,
			attempts=$7,
			max_attempts=$8,
			worker_id=null,
			lease_expires_at=null,
			cancel_requested_at=$9,
			next_run_at=$10,
			started_at=$11,
			finished_at=$12,
			error=$13
		where id=$1 and worker_id=$14 and status=any($15)
	`, job.ID, string(job.Status), payload, counters, job.ProgressCurrent, job.ProgressTotal, job.Attempts, job.MaxAttempts,
		job.CancelRequestedAt, job.NextRunAt, job.StartedAt, job.FinishedAt, nullString(job.Error), workerID, allowedCurrent)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return catalog.ErrJobLeaseLost
	}
	if err := replaceJobLogs(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceJobLogs(ctx context.Context, tx pgx.Tx, job jobs.Job) error {
	if _, err := tx.Exec(ctx, `delete from job_logs where job_id=$1`, job.ID); err != nil {
		return err
	}
	for _, line := range job.Logs {
		if _, err := tx.Exec(ctx, `insert into job_logs(job_id, level, message, created_at) values($1, $2, $3, $4)`, job.ID, line.Level, line.Message, line.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) jobLogs(ctx context.Context, jobID string) ([]jobs.LogLine, error) {
	rows, err := db.pool.Query(ctx, `select id, level, message, created_at from job_logs where job_id=$1 order by id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []jobs.LogLine
	for rows.Next() {
		var line jobs.LogLine
		if err := rows.Scan(&line.ID, &line.Level, &line.Message, &line.CreatedAt); err != nil {
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

const jobColumns = `id::text, kind, status, payload_json, counters_json, progress_current, progress_total, attempts, max_attempts,
	coalesce(worker_id, ''), lease_expires_at, cancel_requested_at, next_run_at, created_at, started_at, finished_at, coalesce(error, '')`

func IsConnectError(err error) bool {
	var pgErr *pgconn.ConnectError
	return errors.As(err, &pgErr)
}

var _ catalog.Store = (*DB)(nil)
var _ auth.Store = (*DB)(nil)

func NowUTC() time.Time { return time.Now().UTC() }
