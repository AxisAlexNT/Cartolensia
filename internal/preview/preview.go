package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

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
	Status   Status `json:"status"`
	URL      string `json:"url,omitempty"`
	CacheKey string `json:"cache_key,omitempty"`
	Message  string `json:"message,omitempty"`
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
	return filepath.Join(cacheDir, "previews", key[:2], key, variant+".jpg")
}

func ForAsset(asset catalog.Asset) Info {
	loc, ok := catalog.FirstLocation(asset)
	if !ok {
		return Info{Status: StatusUnsupported, Message: "asset has no storage location"}
	}
	switch loc.MediaKind {
	case "photo", "video":
		return Info{Status: StatusNotImplemented, CacheKey: CacheKey(asset), Message: "preview generation is not implemented yet"}
	default:
		return Info{Status: StatusUnsupported, CacheKey: CacheKey(asset), Message: "preview is unsupported for this media kind"}
	}
}
