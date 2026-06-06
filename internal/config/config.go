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
	Source   string          `json:"source"`
}

type HTTPConfig struct {
	Addr string `json:"addr" yaml:"addr"`
}

type DatabaseConfig struct {
	URL string `json:"url" yaml:"url"`
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
	if value := os.Getenv("CARTOLENSIA_CACHE_DIR"); value != "" {
		cfg.Cache.Dir = value
	}
}
