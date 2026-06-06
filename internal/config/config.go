package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigPath  = "config/cartolensia.yaml"
	ModeStrictReadOnly = "strict_read_only"
)

type Config struct {
	HTTP     HTTPConfig      `json:"http" yaml:"http"`
	Database DatabaseConfig  `json:"database" yaml:"database"`
	Cache    CacheConfig     `json:"cache" yaml:"cache"`
	Storages []StorageConfig `json:"storages" yaml:"storages"`
	Plugins  PluginConfig    `json:"plugins" yaml:"plugins"`
	Workers  WorkerConfig    `json:"workers" yaml:"workers"`
	Auth     AuthConfig      `json:"auth" yaml:"auth"`
	Source   string          `json:"source"`
}

type HTTPConfig struct {
	Addr string `json:"addr" yaml:"addr"`
}

type DatabaseConfig struct {
	URL           string `json:"url" yaml:"url"`
	MigrationsDir string `json:"migrations_dir,omitempty" yaml:"migrations_dir"`
}

type CacheConfig struct {
	Dir string `json:"dir" yaml:"dir"`
}

type StorageConfig struct {
	Name string `json:"name" yaml:"name"`
	Kind string `json:"kind" yaml:"kind"`
	Root string `json:"root" yaml:"root"`
	Mode string `json:"mode" yaml:"mode"`
}

type PluginConfig struct {
	Enabled []string `json:"enabled" yaml:"enabled"`
}

type WorkerConfig struct {
	Enabled           bool   `json:"enabled" yaml:"enabled"`
	WorkerID          string `json:"worker_id" yaml:"worker_id"`
	PollInterval      string `json:"poll_interval" yaml:"poll_interval"`
	LeaseDuration     string `json:"lease_duration" yaml:"lease_duration"`
	HeartbeatInterval string `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	MaxConcurrency    int    `json:"max_concurrency" yaml:"max_concurrency"`
}

type AuthConfig struct {
	Mode             string `json:"mode" yaml:"mode"`
	AdminEmail       string `json:"admin_email,omitempty" yaml:"admin_email"`
	AdminPasswordEnv string `json:"admin_password_env,omitempty" yaml:"admin_password_env"`
}

func Load(path string) (Config, error) {
	if path == "" {
		path = os.Getenv("CARTOLENSIA_CONFIG")
	}
	if path == "" {
		path = DefaultConfigPath
	}

	cfg := Defaults()
	cfg.Source = path

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
		cfg.Source = path
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	} else {
		cfg.Source = "defaults"
	}

	applyEnv(&cfg)
	if err := Validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Defaults() Config {
	return Config{
		HTTP:  HTTPConfig{Addr: ":8080"},
		Cache: CacheConfig{Dir: ".cartolensia/cache"},
		Storages: []StorageConfig{{
			Name: "fixture",
			Kind: "fs",
			Root: "./testdata/media_fixture",
			Mode: ModeStrictReadOnly,
		}},
		Plugins: PluginConfig{Enabled: []string{
			"albums",
			"mapview",
			"gpstracks",
			"transcoding",
			"ai-base",
			"ai-classification",
		}},
		Workers: WorkerConfig{
			Enabled:           true,
			PollInterval:      "1s",
			LeaseDuration:     "30s",
			HeartbeatInterval: "10s",
			MaxConcurrency:    2,
		},
		Auth: AuthConfig{
			Mode:             "dev_no_auth",
			AdminPasswordEnv: "CARTOLENSIA_ADMIN_PASSWORD",
		},
	}
}

func Validate(cfg *Config) error {
	if strings.TrimSpace(cfg.HTTP.Addr) == "" {
		return errors.New("http.addr is required")
	}
	if strings.TrimSpace(cfg.Cache.Dir) == "" {
		return errors.New("cache.dir is required")
	}
	if len(cfg.Storages) == 0 {
		return errors.New("at least one storage is required")
	}
	if cfg.Workers.PollInterval == "" {
		cfg.Workers.PollInterval = "1s"
	}
	if cfg.Workers.LeaseDuration == "" {
		cfg.Workers.LeaseDuration = "30s"
	}
	if cfg.Workers.HeartbeatInterval == "" {
		cfg.Workers.HeartbeatInterval = "10s"
	}
	if cfg.Workers.MaxConcurrency <= 0 {
		cfg.Workers.MaxConcurrency = 1
	}
	if cfg.Auth.Mode == "" {
		cfg.Auth.Mode = "dev_no_auth"
	}
	if cfg.Auth.Mode != "dev_no_auth" && cfg.Auth.Mode != "local" {
		return fmt.Errorf("unsupported auth mode %q", cfg.Auth.Mode)
	}
	seen := map[string]struct{}{}
	for i := range cfg.Storages {
		st := &cfg.Storages[i]
		st.Name = strings.TrimSpace(st.Name)
		st.Kind = strings.TrimSpace(st.Kind)
		st.Mode = strings.TrimSpace(st.Mode)
		if st.Mode == "" {
			st.Mode = ModeStrictReadOnly
		}
		if st.Name == "" {
			return fmt.Errorf("storages[%d].name is required", i)
		}
		if _, ok := seen[st.Name]; ok {
			return fmt.Errorf("duplicate storage name %q", st.Name)
		}
		seen[st.Name] = struct{}{}
		if st.Kind != "fs" {
			return fmt.Errorf("storage %q uses unsupported kind %q", st.Name, st.Kind)
		}
		if st.Mode != ModeStrictReadOnly {
			return fmt.Errorf("storage %q uses unsupported mode %q", st.Name, st.Mode)
		}
		if strings.TrimSpace(st.Root) == "" {
			return fmt.Errorf("storage %q root is required", st.Name)
		}
	}
	return nil
}

func EffectiveStorageRoots(cfg *Config) (Config, error) {
	next := *cfg
	next.Storages = append([]StorageConfig(nil), cfg.Storages...)
	for i := range next.Storages {
		root, err := filepath.Abs(next.Storages[i].Root)
		if err != nil {
			return Config{}, fmt.Errorf("resolve storage %q root: %w", next.Storages[i].Name, err)
		}
		next.Storages[i].Root = root
	}
	cacheDir, err := filepath.Abs(next.Cache.Dir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve cache dir: %w", err)
	}
	next.Cache.Dir = cacheDir
	return next, nil
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("CARTOLENSIA_HTTP_ADDR"); value != "" {
		cfg.HTTP.Addr = value
	}
	if value := os.Getenv("CARTOLENSIA_DATABASE_URL"); value != "" {
		cfg.Database.URL = value
	}
	if value := os.Getenv("CARTOLENSIA_MIGRATIONS_DIR"); value != "" {
		cfg.Database.MigrationsDir = value
	}
	if value := os.Getenv("CARTOLENSIA_CACHE_DIR"); value != "" {
		cfg.Cache.Dir = value
	}
	if value := os.Getenv("CARTOLENSIA_WORKERS_ENABLED"); value == "0" || strings.EqualFold(value, "false") {
		cfg.Workers.Enabled = false
	}
	if value := os.Getenv("CARTOLENSIA_WORKER_ID"); value != "" {
		cfg.Workers.WorkerID = value
	}
	if value := os.Getenv("CARTOLENSIA_AUTH_MODE"); value != "" {
		cfg.Auth.Mode = value
	}
}
