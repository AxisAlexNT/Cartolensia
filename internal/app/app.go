package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/auth"
	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/discovery"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/metadata"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/preview"
	"github.com/AxisAlexNT/Cartolensia/internal/server"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
	"github.com/AxisAlexNT/Cartolensia/internal/workers"
)

const Version = "dev"

type App struct {
	Config       config.Config
	Plugins      []plugins.Manifest
	Store        catalog.Store
	Registry     *storage.Registry
	DB           *database.DB
	StoreBackend string
	Capabilities []database.Capability
	Workers      *workers.Manager
	authn        auth.Authenticator
	authz        auth.Authorizer
	authService  *auth.LocalService
	server       *server.Server
}

func New(ctx context.Context, configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	cfg, err = config.EffectiveStorageRoots(&cfg)
	if err != nil {
		return nil, err
	}
	pluginManifests, err := plugins.Load("plugins", true)
	if err != nil {
		return nil, err
	}
	storageConfigs := make([]storage.Config, 0, len(cfg.Storages))
	for _, st := range cfg.Storages {
		storageConfigs = append(storageConfigs, storage.Config{
			Name:      st.Name,
			Kind:      st.Kind,
			Root:      st.Root,
			Mode:      st.Mode,
			SourceURL: st.SourceURL,
			SMB:       smbConfigFromConfig(st.SMB),
		})
	}
	registry, err := storage.NewRegistry(storageConfigs)
	if err != nil {
		return nil, err
	}
	app := &App{
		Config:       cfg,
		Plugins:      pluginManifests,
		Registry:     registry,
		Store:        catalog.NewMemoryStore(),
		StoreBackend: "memory",
	}
	if cfg.Database.URL != "" {
		db, err := database.Open(ctx, cfg.Database.URL)
		if err != nil {
			return nil, err
		}
		if cfg.Database.MigrationsDir != "" {
			err = db.ApplyMigrationsFromDir(ctx, cfg.Database.MigrationsDir)
		} else {
			err = db.ApplyEmbeddedMigrations(ctx)
		}
		if err != nil {
			db.Close()
			return nil, err
		}
		if err := db.EnsureOptionalVectorSchema(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure optional vector schema: %w", err)
		}
		if err := db.SnapshotConfig(ctx, cfg); err != nil {
			db.Close()
			return nil, fmt.Errorf("snapshot config: %w", err)
		}
		if err := db.UpsertStorages(ctx, cfg.Storages); err != nil {
			db.Close()
			return nil, fmt.Errorf("upsert storages: %w", err)
		}
		if err := db.UpsertPlugins(ctx, pluginManifests); err != nil {
			db.Close()
			return nil, fmt.Errorf("upsert plugins: %w", err)
		}
		caps, err := db.Capabilities(ctx)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("detect database capabilities: %w", err)
		}
		app.DB = db
		app.Store = db
		app.StoreBackend = "postgres"
		app.Capabilities = caps
	}
	switch cfg.Auth.Mode {
	case "local":
		authStore := auth.Store(auth.NewMemoryStore())
		if app.DB != nil {
			authStore = app.DB
		}
		authCfg, err := authConfig(cfg.Auth)
		if err != nil {
			app.Close()
			return nil, err
		}
		service := auth.NewLocalService(authStore, authCfg)
		password, err := bootstrapPassword(cfg.Auth, authCfg.AdminPasswordEnv)
		if err != nil {
			app.Close()
			return nil, err
		}
		if _, _, err := service.Bootstrap(ctx, password); err != nil {
			app.Close()
			return nil, fmt.Errorf("bootstrap local auth: %w", err)
		}
		app.authn = service
		app.authz = service
		app.authService = service
	default:
		app.authn = auth.DevNoAuth{}
		app.authz = auth.DevNoAuth{}
	}
	app.server = app.buildServer()
	if cfg.Workers.Enabled {
		workerCfg, err := workerConfig(cfg.Workers)
		if err != nil {
			app.Close()
			return nil, err
		}
		manager := workers.New(app.Store, workerCfg)
		manager.Register("discovery", func(ctx context.Context, job *jobs.Job) error {
			runner := discovery.Runner{Registry: app.Registry, Store: app.Store, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.Scan(ctx, job)
		})
		manager.Register("discovery_dry_run", func(ctx context.Context, job *jobs.Job) error {
			runner := discovery.Runner{Registry: app.Registry, Store: app.Store, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.DryRun(ctx, job)
		})
		manager.Register("hash", func(ctx context.Context, job *jobs.Job) error {
			runner := discovery.Runner{Registry: app.Registry, Store: app.Store, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.HashUnhashed(ctx, job)
		})
		manager.Register("metadata_enrich", func(ctx context.Context, job *jobs.Job) error {
			runner := metadata.Runner{Registry: app.Registry, Store: app.Store, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.Enrich(ctx, job)
		})
		manager.Register("geo_snap", func(ctx context.Context, job *jobs.Job) error {
			runner := metadata.Runner{Registry: app.Registry, Store: app.Store, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.SnapToTrack(ctx, job)
		})
		manager.Register("preview_generate", func(ctx context.Context, job *jobs.Job) error {
			runner := preview.Runner{Registry: app.Registry, Store: app.Store, CacheDir: app.Config.Cache.Dir, WorkerID: manager.WorkerID(), LeaseDuration: manager.LeaseDuration()}
			return runner.Generate(ctx, job)
		})
		manager.Register("ai_backfill", func(ctx context.Context, job *jobs.Job) error {
			return app.server.RunAIBackfillJob(ctx, job, manager.WorkerID(), manager.LeaseDuration())
		})
		manager.Start()
		app.Workers = manager
	}
	return app, nil
}

func smbConfigFromConfig(in *config.SMBStorageConfig) *storage.SMBConfig {
	if in == nil {
		return nil
	}
	return &storage.SMBConfig{
		Host:            in.Host,
		Share:           in.Share,
		Path:            in.Path,
		Domain:          in.Domain,
		Username:        in.Username,
		CredentialsFile: in.CredentialsFile,
		PasswordEnv:     in.PasswordEnv,
	}
}

func (a *App) Close() {
	if a != nil && a.Workers != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.Workers.Stop(ctx)
	}
	if a != nil && a.DB != nil {
		a.DB.Close()
	}
}

func (a *App) Handler() *server.Server {
	if a.server != nil {
		return a.server
	}
	a.server = a.buildServer()
	return a.server
}

func (a *App) buildServer() *server.Server {
	return server.New(server.Dependencies{
		Version:       Version,
		Config:        a.Config,
		Plugins:       a.Plugins,
		Registry:      a.Registry,
		Store:         a.Store,
		StoreBackend:  a.StoreBackend,
		Capabilities:  a.Capabilities,
		Authenticator: a.authn,
		Authorizer:    a.authz,
		AuthService:   a.authService,
	})
}

func workerConfig(cfg config.WorkerConfig) (workers.Config, error) {
	pollInterval, err := time.ParseDuration(cfg.PollInterval)
	if err != nil {
		return workers.Config{}, fmt.Errorf("parse workers.poll_interval: %w", err)
	}
	leaseDuration, err := time.ParseDuration(cfg.LeaseDuration)
	if err != nil {
		return workers.Config{}, fmt.Errorf("parse workers.lease_duration: %w", err)
	}
	heartbeatInterval, err := time.ParseDuration(cfg.HeartbeatInterval)
	if err != nil {
		return workers.Config{}, fmt.Errorf("parse workers.heartbeat_interval: %w", err)
	}
	return workers.Config{
		WorkerID:          cfg.WorkerID,
		PollInterval:      pollInterval,
		LeaseDuration:     leaseDuration,
		HeartbeatInterval: heartbeatInterval,
		MaxConcurrency:    cfg.MaxConcurrency,
	}, nil
}

func authConfig(cfg config.AuthConfig) (auth.Config, error) {
	sessionTTL, err := time.ParseDuration(cfg.SessionTTL)
	if err != nil {
		return auth.Config{}, fmt.Errorf("parse auth.session_ttl: %w", err)
	}
	apiTokenTTL, err := time.ParseDuration(cfg.APITokenTTL)
	if err != nil {
		return auth.Config{}, fmt.Errorf("parse auth.api_token_ttl: %w", err)
	}
	return auth.Config{
		AdminEmail:              cfg.AdminEmail,
		AdminDisplayName:        cfg.AdminDisplayName,
		AdminPasswordEnv:        cfg.AdminPasswordEnv,
		RotateBootstrapPassword: cfg.RotateBootstrapPassword,
		SessionTTL:              sessionTTL,
		APITokenTTL:             apiTokenTTL,
		CookieName:              cfg.CookieName,
		CookieSecure:            cfg.CookieSecure,
		CSRFHeader:              cfg.CSRFHeader,
	}, nil
}

func bootstrapPassword(cfg config.AuthConfig, passwordEnv string) (string, error) {
	if passwordEnv == "" {
		passwordEnv = "CARTOLENSIA_ADMIN_PASSWORD"
	}
	if password := os.Getenv(passwordEnv); password != "" {
		return password, nil
	}
	if cfg.AdminPasswordFile == "" {
		return "", nil
	}
	data, err := os.ReadFile(cfg.AdminPasswordFile)
	if err != nil {
		return "", fmt.Errorf("read auth.admin_password_file: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}
