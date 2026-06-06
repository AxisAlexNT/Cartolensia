package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrReadOnly       = errors.New("storage is strict read-only")
	ErrTraversal      = errors.New("storage path escapes root")
	ErrUnknownStorage = errors.New("unknown storage")
)

type Config struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Root string `json:"root"`
	Mode string `json:"mode"`
}

type Registry struct {
	adapters map[string]*FSAdapter
}

type FileInfo struct {
	StorageName  string    `json:"storage_name"`
	StorageURL   string    `json:"storage_url"`
	RelativePath string    `json:"relative_path"`
	Name         string    `json:"name"`
	Extension    string    `json:"extension"`
	MIME         string    `json:"mime"`
	MediaKind    string    `json:"media_kind"`
	SizeBytes    int64     `json:"size_bytes"`
	MTime        time.Time `json:"mtime"`
}

func NewRegistry(configs []Config) (*Registry, error) {
	reg := &Registry{adapters: map[string]*FSAdapter{}}
	for _, cfg := range configs {
		if cfg.Kind != "fs" {
			return nil, fmt.Errorf("unsupported storage kind %q", cfg.Kind)
		}
		adapter, err := NewFSAdapter(cfg.Name, cfg.Root)
		if err != nil {
			return nil, err
		}
		if _, exists := reg.adapters[cfg.Name]; exists {
			return nil, fmt.Errorf("duplicate storage %q", cfg.Name)
		}
		reg.adapters[cfg.Name] = adapter
	}
	return reg, nil
}

func (r *Registry) ListStorages() []Config {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Config, 0, len(names))
	for _, name := range names {
		adapter := r.adapters[name]
		out = append(out, Config{Name: name, Kind: "fs", Root: adapter.Root(), Mode: "strict_read_only"})
	}
	return out
}

func (r *Registry) Adapter(name string) (*FSAdapter, error) {
	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStorage, name)
	}
	return adapter, nil
}

func (r *Registry) OpenByURL(raw string) (*os.File, FileInfo, error) {
	u, err := ParseURL(raw)
	if err != nil {
		return nil, FileInfo{}, err
	}
	adapter, err := r.Adapter(u.Storage)
	if err != nil {
		return nil, FileInfo{}, err
	}
	return adapter.Open(u.RelativePath)
}

type FSAdapter struct {
	name string
	root string
}

func NewFSAdapter(name, root string) (*FSAdapter, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("storage name is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	evalRoot, err := filepath.EvalSymlinks(absRoot)
	if err == nil {
		absRoot = evalRoot
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("resolve root symlinks: %w", err)
	}
	return &FSAdapter{name: name, root: filepath.Clean(absRoot)}, nil
}

func (a *FSAdapter) Name() string { return a.name }
func (a *FSAdapter) Root() string { return a.root }

func (a *FSAdapter) URL(relativePath string) (string, error) {
	rel, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	return (&URL{Scheme: "fs", Storage: a.name, RelativePath: rel}).String(), nil
}

func (a *FSAdapter) ListRecursive(ctx context.Context) ([]FileInfo, error) {
	var files []FileInfo
	err := filepath.WalkDir(a.root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(a.root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := a.Stat(rel)
		if err != nil {
			return err
		}
		files = append(files, info)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].StorageURL < files[j].StorageURL
	})
	return files, nil
}

func (a *FSAdapter) Open(relativePath string) (*os.File, FileInfo, error) {
	full, err := a.safePath(relativePath)
	if err != nil {
		return nil, FileInfo{}, err
	}
	info, err := a.Stat(relativePath)
	if err != nil {
		return nil, FileInfo{}, err
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, FileInfo{}, err
	}
	return file, info, nil
}

func (a *FSAdapter) Stat(relativePath string) (FileInfo, error) {
	rel, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return FileInfo{}, err
	}
	full, err := a.safePath(rel)
	if err != nil {
		return FileInfo{}, err
	}
	stat, err := os.Stat(full)
	if err != nil {
		return FileInfo{}, err
	}
	if stat.IsDir() {
		return FileInfo{}, fmt.Errorf("path is a directory: %s", rel)
	}
	storageURL, err := a.URL(rel)
	if err != nil {
		return FileInfo{}, err
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(rel), "."))
	mimeType := mime.TypeByExtension("." + ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return FileInfo{
		StorageName:  a.name,
		StorageURL:   storageURL,
		RelativePath: rel,
		Name:         path.Base(rel),
		Extension:    ext,
		MIME:         mimeType,
		MediaKind:    MediaKind(ext, mimeType),
		SizeBytes:    stat.Size(),
		MTime:        stat.ModTime().UTC(),
	}, nil
}

func (a *FSAdapter) Write(_ string, _ io.Reader) error { return ErrReadOnly }
func (a *FSAdapter) Delete(_ string) error             { return ErrReadOnly }
func (a *FSAdapter) Move(_, _ string) error            { return ErrReadOnly }
func (a *FSAdapter) Mkdir(_ string) error              { return ErrReadOnly }

func (a *FSAdapter) safePath(relativePath string) (string, error) {
	rel, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	full := filepath.Join(a.root, filepath.FromSlash(rel))
	cleanFull := filepath.Clean(full)
	evalFull, err := filepath.EvalSymlinks(cleanFull)
	if err == nil {
		cleanFull = evalFull
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if !isWithin(a.root, cleanFull) {
		return "", ErrTraversal
	}
	return cleanFull, nil
}

func isWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type URL struct {
	Scheme       string
	Storage      string
	RelativePath string
}

func ParseURL(raw string) (URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return URL{}, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return URL{}, fmt.Errorf("invalid storage url %q", raw)
	}
	if parsed.Scheme != "fs" {
		return URL{}, fmt.Errorf("unsupported storage url scheme %q", parsed.Scheme)
	}
	rel, err := NormalizeRelativePath(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return URL{}, err
	}
	unescaped, err := url.PathUnescape(rel)
	if err != nil {
		return URL{}, err
	}
	rel, err = NormalizeRelativePath(unescaped)
	if err != nil {
		return URL{}, err
	}
	return URL{Scheme: parsed.Scheme, Storage: parsed.Host, RelativePath: rel}, nil
}

func (u URL) String() string {
	escaped := strings.TrimPrefix((&url.URL{Path: "/" + u.RelativePath}).EscapedPath(), "/")
	return u.Scheme + "://" + u.Storage + "/" + escaped
}

func NormalizeRelativePath(input string) (string, error) {
	input = strings.ReplaceAll(input, "\\", "/")
	input = strings.TrimSpace(input)
	if input == "" || strings.HasPrefix(input, "/") {
		return "", ErrTraversal
	}
	for _, segment := range strings.Split(input, "/") {
		if segment == ".." {
			return "", ErrTraversal
		}
	}
	clean := path.Clean(input)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrTraversal
	}
	return clean, nil
}

func MediaKind(ext, mimeType string) string {
	switch strings.ToLower(ext) {
	case "jpg", "jpeg", "png", "webp", "gif", "heic", "heif", "tif", "tiff":
		return "photo"
	case "mp4", "mov", "mkv", "avi", "webm", "m4v":
		return "video"
	case "gpx", "tcx", "fit", "kml":
		return "track"
	}
	if strings.HasPrefix(mimeType, "image/") {
		return "photo"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "video"
	}
	return "other"
}
