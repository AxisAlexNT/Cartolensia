package preview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/catalog"
)

type Status string

const (
	StatusReady          Status = "ready"
	StatusQueued         Status = "queued"
	StatusUnsupported    Status = "unsupported"
	StatusFailed         Status = "failed"
	StatusNotImplemented Status = "not_implemented"
)

type Info struct {
	Status    Status `json:"status"`
	URL       string `json:"url,omitempty"`
	CacheKey  string `json:"cache_key,omitempty"`
	CachePath string `json:"cache_path,omitempty"`
	Message   string `json:"message,omitempty"`
}

func IndexEntry(asset catalog.Asset, info Info, variant string) catalog.PreviewCacheEntry {
	if variant == "" {
		variant = "default"
	}
	entry := catalog.PreviewCacheEntry{
		AssetID:   asset.ID,
		Variant:   variant,
		Width:     256,
		Height:    256,
		Format:    "jpg",
		CachePath: info.CachePath,
		Status:    string(info.Status),
		Error:     info.Message,
	}
	if loc, ok := catalog.FirstLocation(asset); ok {
		entry.ContentID = loc.ContentID
	}
	if info.CachePath != "" {
		if stat, err := os.Stat(info.CachePath); err == nil {
			entry.SizeBytes = stat.Size()
		}
	}
	return entry
}

func CacheKey(asset catalog.Asset) string {
	seed := asset.ID
	for _, loc := range asset.Locations {
		if loc.ContentID != "" {
			seed = loc.ContentID
			break
		}
	}
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func CachePath(cacheDir string, asset catalog.Asset, variant string) string {
	key := CacheKey(asset)
	if variant == "" {
		variant = "default"
	}
	return filepath.Clean(filepath.Join(cacheDir, "previews", key[:2], key, variant+".jpg"))
}

func ForAsset(asset catalog.Asset) Info {
	return InfoForAsset("", asset)
}

func InfoForAsset(cacheDir string, asset catalog.Asset) Info {
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return Info{Status: StatusUnsupported, Message: "asset has no storage location"}
	}
	switch loc.MediaKind {
	case "photo":
		info := Info{Status: StatusQueued, CacheKey: CacheKey(asset), Message: "preview will be generated on demand"}
		if cacheDir == "" {
			return info
		}
		cachePath := CachePath(cacheDir, asset, "default")
		info.CachePath = cachePath
		if err := ensureInsideCache(cacheDir, cachePath); err != nil {
			info.Status = StatusFailed
			info.Message = err.Error()
			return info
		}
		if _, err := os.Stat(cachePath); err == nil {
			info.Status = StatusReady
			info.Message = ""
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			info.Status = StatusFailed
			info.Message = err.Error()
		}
		return info
	case "video":
		return Info{Status: StatusNotImplemented, CacheKey: CacheKey(asset), Message: "preview generation is not implemented yet"}
	default:
		return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), Message: "preview is unsupported for this media kind"}
	}
}

func GenerateImage(ctx context.Context, cacheDir string, asset catalog.Asset, reader io.Reader) (Info, error) {
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return Info{Status: StatusUnsupported, Message: "asset has no storage location"}, nil
	}
	if loc.MediaKind != "photo" {
		return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), Message: "preview generation supports photos only"}, nil
	}
	cachePath := CachePath(cacheDir, asset, "default")
	if err := ensureInsideCache(cacheDir, cachePath); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	if _, err := os.Stat(cachePath); err == nil {
		return Info{Status: StatusReady, CacheKey: CacheKey(asset), CachePath: cachePath}, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	if err := ctx.Err(); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	src, _, err := image.Decode(reader)
	if err != nil {
		if strings.Contains(err.Error(), "unknown format") {
			return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), CachePath: cachePath, Message: "image format is unsupported by the built-in decoder"}, nil
		}
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	thumb := resizeNearest(src, 256)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), "preview-*.tmp")
	if err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := jpeg.Encode(tmp, thumb, &jpeg.Options{Quality: 82}); err != nil {
		_ = tmp.Close()
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	if err := tmp.Close(); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	if err := os.Rename(tmpName, cachePath); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), CachePath: cachePath, Message: err.Error()}, err
	}
	return Info{Status: StatusReady, CacheKey: CacheKey(asset), CachePath: cachePath}, nil
}

func GenerateImageBytes(ctx context.Context, asset catalog.Asset, reader io.Reader) (Info, []byte, error) {
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return Info{Status: StatusUnsupported, Message: "asset has no storage location"}, nil, nil
	}
	if loc.MediaKind != "photo" {
		return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), Message: "preview generation supports photos only"}, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), Message: err.Error()}, nil, err
	}
	src, _, err := image.Decode(reader)
	if err != nil {
		if strings.Contains(err.Error(), "unknown format") {
			return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), Message: "image format is unsupported by the built-in decoder"}, nil, nil
		}
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), Message: err.Error()}, nil, err
	}
	thumb := resizeNearest(src, 256)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 82}); err != nil {
		return Info{Status: StatusFailed, CacheKey: CacheKey(asset), Message: err.Error()}, nil, err
	}
	return Info{Status: StatusReady, CacheKey: CacheKey(asset)}, buf.Bytes(), nil
}

func ensureInsideCache(cacheDir, target string) error {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, cleanTarget)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return errors.New("preview cache path escapes cache directory")
	}
	return nil
}

func resizeNearest(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}
	scale := math.Min(float64(maxDim)/float64(width), float64(maxDim)/float64(height))
	if scale > 1 {
		scale = 1
	}
	dstW := int(math.Max(1, math.Round(float64(width)*scale)))
	dstH := int(math.Max(1, math.Round(float64(height)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		srcY := bounds.Min.Y + int(float64(y)*float64(height)/float64(dstH))
		for x := 0; x < dstW; x++ {
			srcX := bounds.Min.X + int(float64(x)*float64(width)/float64(dstW))
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

type CacheEntry struct {
	Path      string
	SizeBytes int64
	ModTime   time.Time
}

func Cleanup(cacheDir string, maxBytes int64, ttl time.Duration, now time.Time) ([]CacheEntry, error) {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, err
	}
	var entries []CacheEntry
	previewsDir := filepath.Join(root, "previews")
	if _, err := os.Stat(previewsDir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err := filepath.WalkDir(previewsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if err := ensureInsideCache(root, path); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, CacheEntry{Path: path, SizeBytes: info.Size(), ModTime: info.ModTime()})
		return nil
	}); err != nil {
		return nil, err
	}
	var removed []CacheEntry
	total := int64(0)
	for _, entry := range entries {
		total += entry.SizeBytes
		if ttl > 0 && now.Sub(entry.ModTime) > ttl {
			if err := os.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
			removed = append(removed, entry)
			total -= entry.SizeBytes
		}
	}
	if maxBytes <= 0 || total <= maxBytes {
		return removed, nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModTime.Before(entries[j].ModTime) })
	for _, entry := range entries {
		if total <= maxBytes {
			break
		}
		if err := ensureInsideCache(root, entry.Path); err != nil {
			return removed, err
		}
		if err := os.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed = append(removed, entry)
		total -= entry.SizeBytes
	}
	return removed, nil
}
