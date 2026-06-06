package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Version     string   `json:"version" yaml:"version"`
	Description string   `json:"description" yaml:"description"`
	DependsOn   []string `json:"depends_on" yaml:"depends_on"`
	Runtime     string   `json:"runtime" yaml:"runtime"`
	Status      string   `json:"status" yaml:"status"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

func BuiltIns() []Manifest {
	return []Manifest{
		{ID: "albums", Name: "Albums", Version: "0.1.0", Description: "Database-backed virtual album grouping skeleton.", Runtime: "builtin", Status: "stub"},
		{ID: "mapview", Name: "Map View", Version: "0.1.0", Description: "Map-first media browsing and clustering skeleton.", Runtime: "builtin", Status: "stub"},
		{ID: "gpstracks", Name: "GPS Tracks", Version: "0.1.0", Description: "Track ingestion, linking, and live video-track sync skeleton.", Runtime: "builtin", Status: "stub"},
		{ID: "transcoding", Name: "Transcoding", Version: "0.1.0", Description: "Safe transcoding manager skeleton; never writes into originals.", Runtime: "builtin", Status: "stub"},
		{ID: "ai-base", Name: "Base AI", Version: "0.1.0", Description: "AI runtime and VectorStore abstraction skeleton.", Runtime: "builtin", Status: "stub"},
		{ID: "ai-classification", Name: "AI Classification", Version: "0.1.0", Description: "Transport and place classification workflow skeleton.", DependsOn: []string{"ai-base"}, Runtime: "builtin", Status: "stub"},
	}
}

func Load(dir string, includeBuiltIns bool) ([]Manifest, error) {
	var manifests []Manifest
	if includeBuiltIns {
		manifests = append(manifests, BuiltIns()...)
	}
	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("read plugins dir: %w", err)
			}
		} else {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				path := filepath.Join(dir, entry.Name(), "plugin.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					return nil, fmt.Errorf("read plugin manifest %s: %w", path, err)
				}
				var manifest Manifest
				if err := yaml.Unmarshal(data, &manifest); err != nil {
					return nil, fmt.Errorf("parse plugin manifest %s: %w", path, err)
				}
				manifests = append(manifests, manifest)
			}
		}
	}
	return Sort(manifests)
}

func Sort(manifests []Manifest) ([]Manifest, error) {
	byID := map[string]Manifest{}
	for _, manifest := range manifests {
		if err := Validate(manifest); err != nil {
			return nil, err
		}
		if _, exists := byID[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate plugin id %q", manifest.ID)
		}
		byID[manifest.ID] = manifest
	}

	visiting := map[string]bool{}
	visited := map[string]bool{}
	var ordered []Manifest
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("plugin dependency cycle at %q", id)
		}
		manifest, ok := byID[id]
		if !ok {
			return fmt.Errorf("missing plugin dependency %q", id)
		}
		visiting[id] = true
		deps := append([]string(nil), manifest.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		ordered = append(ordered, manifest)
		return nil
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func Validate(manifest Manifest) error {
	if !idPattern.MatchString(manifest.ID) {
		return fmt.Errorf("invalid plugin id %q", manifest.ID)
	}
	if manifest.Name == "" {
		return fmt.Errorf("plugin %q missing name", manifest.ID)
	}
	if manifest.Version == "" {
		return fmt.Errorf("plugin %q missing version", manifest.ID)
	}
	return nil
}
