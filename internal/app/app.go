package app

import (
	"context"
	"fmt"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
	"github.com/AxisAlexNT/Cartolensia/internal/config"
	"github.com/AxisAlexNT/Cartolensia/internal/database"
	"github.com/AxisAlexNT/Cartolensia/internal/plugins"
	"github.com/AxisAlexNT/Cartolensia/internal/server"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
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
		storageConfigs = append(storageConfigs, storage.Config{Name: st.Name, Kind: st.Kind, Root: st.Root, Mode: st.Mode})
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
		if err := db.ApplyMigrations(ctx, "migrations"); err != nil {
			db.Close()
			return nil, err
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
	return app, nil
}

func (a *App) Close() {
	if a != nil && a.DB != nil {
		a.DB.Close()
	}
}

func (a *App) Handler() *server.Server {
	return server.New(server.Dependencies{
		Version:      Version,
		Config:       a.Config,
		Plugins:      a.Plugins,
		Registry:     a.Registry,
		Store:        a.Store,
		StoreBackend: a.StoreBackend,
		Capabilities: a.Capabilities,
	})
}
