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
	"sync"
	"syscall"
	"time"
)

var (
	ErrReadOnly       = errors.New("storage is strict read-only")
	ErrTraversal      = errors.New("storage path escapes root")
	ErrUnknownStorage = errors.New("unknown storage")
)

type Config struct {
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Root      string     `json:"root"`
	Mode      string     `json:"mode"`
	SourceURL string     `json:"source_url,omitempty"`
	SMB       *SMBConfig `json:"smb,omitempty"`
}

type SMBConfig struct {
	Host            string `json:"host,omitempty"`
	Share           string `json:"share,omitempty"`
	Path            string `json:"path,omitempty"`
	Domain          string `json:"domain,omitempty"`
	Username        string `json:"username,omitempty"`
	CredentialsFile string `json:"credentials_file,omitempty"`
	PasswordEnv     string `json:"password_env,omitempty"`
}

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]*FSAdapter
	configs  map[string]Config
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

type WalkOptions struct {
	Prefixes          []string
	MaxFiles          int
	MaxBytes          int64
	MaxFolderWorkers  int
	MaxFileWorkers    int
	FolderQueueDepth  int
	IncludeExtensions []string
	ExcludePatterns   []string
	Progress          func(WalkReport)
}

var supportedExtensions = []string{
	"jpg", "jpeg", "png", "heif", "heic",
	"mp4", "mov", "webm", "mkv", "avi", "m4v",
	"gpx", "kml", "kmz", "gpz",
	"wav", "mp3", "3gp", "3gpp", "aac", "m4a", "flac", "ogg", "oga", "opus", "amr",
	"pdf", "djvu", "txt", "md", "markdown",
}

func SupportedExtensions() []string {
	out := append([]string{}, supportedExtensions...)
	sort.Strings(out)
	return out
}

type WalkReport struct {
	FoldersQueued  int            `json:"folders_queued"`
	FoldersScanned int            `json:"folders_scanned"`
	FilesSeen      int            `json:"files_seen"`
	FilesReturned  int            `json:"files_returned"`
	FilesSkipped   int            `json:"files_skipped"`
	BytesSeen      int64          `json:"bytes_seen"`
	Complete       bool           `json:"complete"`
	SkippedReasons map[string]int `json:"skipped_reasons"`
}

func NewRegistry(configs []Config) (*Registry, error) {
	reg := &Registry{adapters: map[string]*FSAdapter{}, configs: map[string]Config{}}
	for _, cfg := range configs {
		if cfg.Kind != "fs" {
			return nil, fmt.Errorf("unsupported storage kind %q", cfg.Kind)
		}
		if cfg.Mode == "" {
			cfg.Mode = "strict_read_only"
		}
		adapter, err := NewFSAdapter(cfg.Name, cfg.Root)
		if err != nil {
			return nil, err
		}
		if _, exists := reg.adapters[cfg.Name]; exists {
			return nil, fmt.Errorf("duplicate storage %q", cfg.Name)
		}
		reg.adapters[cfg.Name] = adapter
		reg.configs[cfg.Name] = Config{Name: cfg.Name, Kind: cfg.Kind, Root: adapter.Root(), Mode: cfg.Mode, SourceURL: cfg.SourceURL, SMB: cloneSMBConfig(cfg.SMB)}
	}
	return reg, nil
}

func (r *Registry) ListStorages() []Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Config, 0, len(names))
	for _, name := range names {
		out = append(out, cloneConfig(r.configs[name]))
	}
	return out
}

func (r *Registry) GetStorage(name string) (Config, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[name]
	if !ok {
		return Config{}, fmt.Errorf("%w: %s", ErrUnknownStorage, name)
	}
	return cloneConfig(cfg), nil
}

func (r *Registry) AddStorage(cfg Config) (Config, error) {
	normalized, adapter, err := normalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[normalized.Name]; exists {
		return Config{}, fmt.Errorf("storage %q already exists", normalized.Name)
	}
	r.adapters[normalized.Name] = adapter
	r.configs[normalized.Name] = normalized
	return normalized, nil
}

func (r *Registry) UpdateStorage(name string, cfg Config) (Config, error) {
	if strings.TrimSpace(name) == "" {
		return Config{}, errors.New("storage name is required")
	}
	cfg.Name = name
	normalized, adapter, err := normalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[name]; !exists {
		return Config{}, fmt.Errorf("%w: %s", ErrUnknownStorage, name)
	}
	r.adapters[name] = adapter
	r.configs[name] = normalized
	return normalized, nil
}

func ValidateConfig(cfg Config) (Config, error) {
	normalized, _, err := normalizeConfig(cfg)
	return normalized, err
}

func normalizeConfig(cfg Config) (Config, *FSAdapter, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Kind = strings.TrimSpace(cfg.Kind)
	cfg.Mode = strings.TrimSpace(cfg.Mode)
	cfg.SourceURL = strings.TrimSpace(cfg.SourceURL)
	if cfg.Kind == "" {
		cfg.Kind = "fs"
	}
	if cfg.Mode == "" {
		cfg.Mode = "strict_read_only"
	}
	if cfg.Kind != "fs" {
		return Config{}, nil, fmt.Errorf("unsupported storage kind %q", cfg.Kind)
	}
	if cfg.Mode != "strict_read_only" && cfg.Mode != "read_only" {
		return Config{}, nil, fmt.Errorf("storage mode %q is not enabled in this build", cfg.Mode)
	}
	adapter, err := NewFSAdapter(cfg.Name, cfg.Root)
	if err != nil {
		return Config{}, nil, err
	}
	cfg.Root = adapter.Root()
	cfg.SMB = cloneSMBConfig(cfg.SMB)
	return cfg, adapter, nil
}

func cloneSMBConfig(in *SMBConfig) *SMBConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.Host = strings.TrimSpace(out.Host)
	out.Share = strings.Trim(strings.TrimSpace(out.Share), "/")
	out.Path = strings.Trim(strings.TrimSpace(out.Path), "/")
	out.Domain = strings.TrimSpace(out.Domain)
	out.Username = strings.TrimSpace(out.Username)
	out.CredentialsFile = strings.TrimSpace(out.CredentialsFile)
	out.PasswordEnv = strings.TrimSpace(out.PasswordEnv)
	if out.Host == "" && out.Share == "" && out.Path == "" && out.Domain == "" && out.Username == "" && out.CredentialsFile == "" && out.PasswordEnv == "" {
		return nil
	}
	return &out
}

func cloneConfig(in Config) Config {
	in.SMB = cloneSMBConfig(in.SMB)
	return in
}

func (r *Registry) Adapter(name string) (*FSAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	} else if !nonFatalSymlinkResolutionError(err) {
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

func (a *FSAdapter) ListRecursiveBounded(ctx context.Context, opts WalkOptions) ([]FileInfo, WalkReport, error) {
	var files []FileInfo
	var mu sync.Mutex
	report, err := a.WalkRecursiveBounded(ctx, opts, func(info FileInfo) error {
		mu.Lock()
		defer mu.Unlock()
		files = append(files, info)
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].StorageURL < files[j].StorageURL
	})
	return files, report, err
}

func (a *FSAdapter) WalkRecursiveBounded(ctx context.Context, opts WalkOptions, visit func(FileInfo) error) (WalkReport, error) {
	report := WalkReport{Complete: true, SkippedReasons: map[string]int{}}
	if opts.MaxFiles == 0 {
		opts.MaxFiles = 50
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = 2 << 30
	}
	if opts.MaxFolderWorkers <= 0 {
		opts.MaxFolderWorkers = 4
	}
	if opts.MaxFileWorkers <= 0 {
		opts.MaxFileWorkers = 8
	}
	if opts.FolderQueueDepth <= 0 {
		opts.FolderQueueDepth = 64
	}
	include := extensionSet(opts.IncludeExtensions)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	folderTasks := make(chan string, opts.FolderQueueDepth)
	fileTasks := make(chan string, max(16, opts.FolderQueueDepth*4))
	progressDone := make(chan struct{})
	var folderWG sync.WaitGroup
	var fileWG sync.WaitGroup
	var mu sync.Mutex
	limitReached := false
	var firstErr error
	snapshot := func() WalkReport {
		mu.Lock()
		defer mu.Unlock()
		out := report
		out.SkippedReasons = map[string]int{}
		for k, v := range report.SkippedReasons {
			out.SkippedReasons[k] = v
		}
		return out
	}
	if opts.Progress != nil {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					opts.Progress(snapshot())
				}
			}
		}()
	}
	defer func() {
		close(progressDone)
		if opts.Progress != nil {
			opts.Progress(snapshot())
		}
	}()
	addSkipped := func(reason string) {
		mu.Lock()
		report.FilesSkipped++
		report.SkippedReasons[reason]++
		mu.Unlock()
	}
	setErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			report.Complete = false
			report.SkippedReasons["callback_error"]++
			cancel()
		}
		mu.Unlock()
	}
	markLimit := func() bool {
		mu.Lock()
		defer mu.Unlock()
		if limitReached {
			return true
		}
		limitReached = true
		report.Complete = false
		report.SkippedReasons["limit"]++
		cancel()
		return true
	}
	shouldStop := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return limitReached
	}
	enqueueFolder := func(rel string) (bool, error) {
		if rel == "" {
			return true, nil
		}
		if excludedDirectoryByPattern(rel, opts.ExcludePatterns) {
			addSkipped("pattern")
			return true, nil
		}
		if shouldStop() {
			return true, nil
		}
		folderWG.Add(1)
		mu.Lock()
		report.FoldersQueued++
		mu.Unlock()
		if err := ctx.Err(); err != nil {
			folderWG.Done()
			if shouldStop() {
				return true, nil
			}
			return false, err
		}
		select {
		case folderTasks <- rel:
			return true, nil
		default:
			// A wide directory can fill the bounded queue while every folder
			// worker is still enumerating child directories. Process the child
			// inline instead of blocking all workers on enqueue.
			return false, nil
		}
	}
	processFile := func(relative string) error {
		defer fileWG.Done()
		if shouldStop() {
			return nil
		}
		info, err := a.Stat(relative)
		if err != nil {
			addSkipped("stat_error")
			return nil
		}
		mu.Lock()
		report.FilesSeen++
		mu.Unlock()
		if !extensionAllowed(info.Extension, include) {
			addSkipped("extension")
			return nil
		}
		if excludedByPattern(info.RelativePath, opts.ExcludePatterns) {
			addSkipped("pattern")
			return nil
		}
		mu.Lock()
		limitHit := (!maxIntUnlimited(opts.MaxFiles) && report.FilesReturned >= opts.MaxFiles) ||
			(!maxInt64Unlimited(opts.MaxBytes) && report.BytesSeen+info.SizeBytes > opts.MaxBytes)
		if limitHit {
			mu.Unlock()
			markLimit()
			return nil
		}
		report.FilesReturned++
		report.BytesSeen += info.SizeBytes
		mu.Unlock()
		if visit != nil {
			if err := visit(info); err != nil {
				setErr(err)
			}
		}
		return nil
	}
	var processFolder func(string) error
	processFolder = func(rel string) error {
		defer folderWG.Done()
		if shouldStop() {
			return nil
		}
		if excludedDirectoryByPattern(rel, opts.ExcludePatterns) {
			addSkipped("pattern")
			return nil
		}
		full := a.root
		var err error
		if strings.TrimSpace(rel) != "" {
			full, err = a.safePath(rel)
			if err != nil {
				addSkipped("folder_error")
				return nil
			}
		}
		stat, err := os.Stat(full)
		if err != nil {
			addSkipped("folder_error")
			return nil
		}
		if !stat.IsDir() {
			fileWG.Add(1)
			select {
			case fileTasks <- rel:
			case <-ctx.Done():
				fileWG.Done()
				if shouldStop() {
					return nil
				}
				return ctx.Err()
			}
			return nil
		}
		entries, err := os.ReadDir(full)
		if err != nil {
			addSkipped("folder_error")
			return nil
		}
		mu.Lock()
		report.FoldersScanned++
		mu.Unlock()
		for _, entry := range entries {
			if shouldStop() {
				return nil
			}
			childRel := filepath.ToSlash(filepath.Join(rel, entry.Name()))
			if entry.Type()&fs.ModeSymlink != 0 {
				addSkipped("symlink")
				continue
			}
			if entry.IsDir() {
				if excludedDirectoryByPattern(childRel, opts.ExcludePatterns) {
					addSkipped("pattern")
					continue
				}
				queued, err := enqueueFolder(childRel)
				if err != nil {
					return err
				}
				if !queued {
					if err := processFolder(childRel); err != nil {
						return err
					}
				}
				continue
			}
			fileWG.Add(1)
			select {
			case fileTasks <- childRel:
			case <-ctx.Done():
				fileWG.Done()
				if shouldStop() {
					return nil
				}
				return ctx.Err()
			}
		}
		return nil
	}
	folderWorkerCount := opts.MaxFolderWorkers
	if folderWorkerCount < 1 {
		folderWorkerCount = 1
	}
	fileWorkerCount := opts.MaxFileWorkers
	if fileWorkerCount < 1 {
		fileWorkerCount = 1
	}
	for i := 0; i < folderWorkerCount; i++ {
		go func() {
			for rel := range folderTasks {
				_ = processFolder(rel)
			}
		}()
	}
	for i := 0; i < fileWorkerCount; i++ {
		go func() {
			for rel := range fileTasks {
				_ = processFile(rel)
			}
		}()
	}
	prefixes := opts.Prefixes
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}
	for _, prefix := range prefixes {
		if strings.TrimSpace(prefix) == "" {
			folderWG.Add(1)
			if err := processFolder(""); err != nil {
				return report, err
			}
			continue
		}
		rel, err := NormalizeRelativePath(prefix)
		if err != nil {
			return report, err
		}
		queued, err := enqueueFolder(rel)
		if err != nil {
			return report, err
		}
		if !queued {
			if err := processFolder(rel); err != nil {
				return report, err
			}
		}
	}
	folderWG.Wait()
	close(folderTasks)
	close(fileTasks)
	fileWG.Wait()
	return report, firstErr
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

type stopWalkError struct{}

func (stopWalkError) Error() string { return "bounded walk stopped" }

func extensionSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "."))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func extensionAllowed(ext string, include map[string]struct{}) bool {
	if len(include) == 0 {
		return true
	}
	_, ok := include[strings.ToLower(strings.TrimPrefix(ext, "."))]
	return ok
}

func excludedByPattern(relativePath string, patterns []string) bool {
	relativePath = normalizeGlobPath(relativePath)
	for _, pattern := range patterns {
		pattern = normalizeGlobPath(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := path.Match(pattern, relativePath); ok {
			return true
		}
		if ok, _ := path.Match(pattern, path.Base(relativePath)); ok {
			return true
		}
	}
	return false
}

func excludedDirectoryByPattern(relativePath string, patterns []string) bool {
	relativePath = normalizeGlobPath(relativePath)
	if relativePath == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = normalizeGlobPath(pattern)
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "/**")
			if relativePath == prefix || strings.HasPrefix(relativePath, prefix+"/") {
				return true
			}
		}
		if ok, _ := path.Match(pattern, relativePath); ok {
			return true
		}
		if ok, _ := path.Match(pattern, path.Base(relativePath)); ok {
			return true
		}
	}
	return false
}

func normalizeGlobPath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "/")
	return value
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

func maxIntUnlimited(value int) bool { return value < 0 }

func maxInt64Unlimited(value int64) bool { return value < 0 }

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
	} else if !nonFatalSymlinkResolutionError(err) {
		return "", err
	}
	if !isWithin(a.root, cleanFull) {
		return "", ErrTraversal
	}
	return cleanFull, nil
}

func nonFatalSymlinkResolutionError(err error) bool {
	return err == nil ||
		errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, syscall.ENODEV) ||
		errors.Is(err, syscall.ENOTCONN) ||
		errors.Is(err, syscall.EHOSTDOWN) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EIO)
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
	case "3gp", "3gpp", "aac", "m4a", "mp3", "wav", "flac", "ogg", "oga", "opus", "amr":
		return "audio"
	case "gpx", "tcx", "fit", "kml", "kmz", "gpz":
		return "track"
	case "pdf", "djvu", "txt", "md", "markdown":
		return "document"
	}
	if strings.HasPrefix(mimeType, "image/") {
		return "photo"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "video"
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return "audio"
	}
	if strings.HasPrefix(mimeType, "application/pdf") || strings.HasPrefix(mimeType, "text/") {
		return "document"
	}
	return "other"
}
