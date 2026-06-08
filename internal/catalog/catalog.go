package catalog

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AxisAlexNT/Cartolensia/internal/id"
	"github.com/AxisAlexNT/Cartolensia/internal/jobs"
	"github.com/AxisAlexNT/Cartolensia/internal/storage"
)

const (
	HashStatusUnhashed = "unhashed"
	HashStatusHashed   = "hashed"
)

type Asset struct {
	ID          string         `json:"id"`
	MediaKind   string         `json:"media_kind"`
	DisplayName string         `json:"display_name"`
	TakenAt     *time.Time     `json:"taken_at,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	FirstSeenAt time.Time      `json:"first_seen_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Locations   []Location     `json:"locations"`
}

type Location struct {
	ID           string    `json:"id"`
	AssetID      string    `json:"asset_id"`
	StorageName  string    `json:"storage_name"`
	StorageURL   string    `json:"storage_url"`
	RelativePath string    `json:"relative_path"`
	FileName     string    `json:"file_name"`
	Extension    string    `json:"extension"`
	MIME         string    `json:"mime"`
	MediaKind    string    `json:"media_kind"`
	SizeBytes    int64     `json:"size_bytes"`
	MTime        time.Time `json:"mtime"`
	HashStatus   string    `json:"hash_status"`
	SHA512Hex    string    `json:"sha512_hex,omitempty"`
	ContentID    string    `json:"content_id,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

type Stats struct {
	Assets             int   `json:"assets"`
	Locations          int   `json:"locations"`
	Photos             int   `json:"photos"`
	Videos             int   `json:"videos"`
	Audio              int   `json:"audio"`
	Documents          int   `json:"documents"`
	Tracks             int   `json:"tracks"`
	Unhashed           int   `json:"unhashed"`
	Hashed             int   `json:"hashed"`
	DuplicateGroups    int   `json:"duplicate_groups"`
	DuplicateLocations int   `json:"duplicate_locations"`
	TotalBytes         int64 `json:"total_bytes"`
}

type UpsertResult struct {
	Asset   Asset `json:"asset"`
	Created bool  `json:"created"`
}

type TrackPoint struct {
	ID           int64     `json:"id,omitempty"`
	TrackAssetID string    `json:"track_asset_id,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
	ElevationM   *float64  `json:"elevation_m,omitempty"`
	SpeedMPS     *float64  `json:"speed_mps,omitempty"`
	Source       string    `json:"source"`
}

type TrackSummary struct {
	TrackAssetID string     `json:"track_asset_id"`
	Name         string     `json:"name"`
	PointCount   int        `json:"point_count"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	MinLat       *float64   `json:"min_lat,omitempty"`
	MinLon       *float64   `json:"min_lon,omitempty"`
	MaxLat       *float64   `json:"max_lat,omitempty"`
	MaxLon       *float64   `json:"max_lon,omitempty"`
	DistanceM    float64    `json:"distance_m,omitempty"`
	DurationSec  *float64   `json:"duration_seconds,omitempty"`
	ElevationMin *float64   `json:"elevation_min_m,omitempty"`
	ElevationMax *float64   `json:"elevation_max_m,omitempty"`
	SourceFormat string     `json:"source_format,omitempty"`
}

type TrackDetail struct {
	Summary TrackSummary `json:"summary"`
	Points  []TrackPoint `json:"points"`
}

type TrackLink struct {
	ID           string     `json:"id"`
	AssetID      string     `json:"asset_id"`
	TrackAssetID string     `json:"track_asset_id"`
	MatchStatus  string     `json:"match_status"`
	OverlapStart *time.Time `json:"overlap_start,omitempty"`
	OverlapEnd   *time.Time `json:"overlap_end,omitempty"`
	TimeOffsetMS int64      `json:"time_offset_ms"`
	Confidence   *float64   `json:"confidence,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type TrackCandidate struct {
	Track          TrackSummary `json:"track"`
	OverlapStart   *time.Time   `json:"overlap_start,omitempty"`
	OverlapEnd     *time.Time   `json:"overlap_end,omitempty"`
	OverlapSeconds float64      `json:"overlap_seconds"`
	Confidence     float64      `json:"confidence"`
	Reason         string       `json:"reason,omitempty"`
}

type Transcript struct {
	ID         string              `json:"id"`
	AssetID    string              `json:"asset_id"`
	SourceKind string              `json:"source_kind"`
	Language   string              `json:"language,omitempty"`
	Model      string              `json:"model,omitempty"`
	FullText   string              `json:"full_text"`
	CreatedAt  time.Time           `json:"created_at"`
	Metadata   map[string]any      `json:"metadata,omitempty"`
	Segments   []TranscriptSegment `json:"segments,omitempty"`
}

type TranscriptSegment struct {
	ID           string         `json:"id"`
	TranscriptID string         `json:"transcript_id"`
	AssetID      string         `json:"asset_id"`
	StartMS      int64          `json:"start_ms"`
	EndMS        int64          `json:"end_ms"`
	Text         string         `json:"text"`
	Confidence   *float64       `json:"confidence,omitempty"`
	Speaker      string         `json:"speaker,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type AudioFeatures struct {
	AssetID          string         `json:"asset_id"`
	DurationSeconds  *float64       `json:"duration_seconds,omitempty"`
	TempoBPM         *float64       `json:"tempo_bpm,omitempty"`
	Key              string         `json:"key,omitempty"`
	Mode             string         `json:"mode,omitempty"`
	Loudness         *float64       `json:"loudness,omitempty"`
	SpeechMusicRatio *float64       `json:"speech_music_ratio,omitempty"`
	GenreLabels      []string       `json:"genre_labels,omitempty"`
	Model            string         `json:"model,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type VideoFrameCaption struct {
	ID          string         `json:"id"`
	AssetID     string         `json:"asset_id"`
	TimestampMS int64          `json:"timestamp_ms"`
	Fraction    float64        `json:"fraction"`
	Caption     string         `json:"caption"`
	Model       string         `json:"model,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type DocumentText struct {
	AssetID   string         `json:"asset_id"`
	PageCount int            `json:"page_count,omitempty"`
	Title     string         `json:"title,omitempty"`
	Author    string         `json:"author,omitempty"`
	Text      string         `json:"text,omitempty"`
	Markdown  string         `json:"markdown,omitempty"`
	Engine    string         `json:"engine,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type AssetQuery struct {
	Q          string
	MediaKind  string
	HashStatus string
	Storage    string
	Extension  string
	TakenFrom  *time.Time
	TakenTo    *time.Time
	AlbumID    string
	TrackID    string
	GeoSource  string
	Limit      int
	Offset     int
	Sort       string
	WithTotal  bool
}

type AssetPage struct {
	Assets []Asset `json:"assets"`
	Page   Page    `json:"page"`
}

type TimestampCandidate struct {
	Source     string    `json:"source"`
	Raw        string    `json:"raw,omitempty"`
	Time       time.Time `json:"time"`
	Confidence float64   `json:"confidence"`
}

type DuplicateGroup struct {
	ContentID  string           `json:"content_id,omitempty"`
	SHA512Hex  string           `json:"sha512_hex"`
	SizeBytes  int64            `json:"size_bytes"`
	Assets     []DuplicateAsset `json:"assets"`
	AssetCount int              `json:"asset_count"`
	TotalBytes int64            `json:"total_bytes"`
}

type DuplicateAsset struct {
	AssetID      string    `json:"asset_id"`
	DisplayName  string    `json:"display_name"`
	MediaKind    string    `json:"media_kind"`
	StorageName  string    `json:"storage_name"`
	RelativePath string    `json:"relative_path"`
	StorageURL   string    `json:"storage_url"`
	MTime        time.Time `json:"mtime"`
}

type DuplicatePage struct {
	Groups []DuplicateGroup `json:"groups"`
	Page   Page             `json:"page"`
}

type Album struct {
	ID          string    `json:"id"`
	ParentID    string    `json:"parent_id,omitempty"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sort_order"`
	ItemCount   int       `json:"item_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AlbumQuery struct {
	ParentID string
	Tree     bool
	Limit    int
	Offset   int
}

type AlbumItem struct {
	AlbumID   string    `json:"album_id"`
	Asset     Asset     `json:"asset"`
	Note      string    `json:"note"`
	SortOrder int       `json:"sort_order"`
	AddedAt   time.Time `json:"added_at"`
}

type AlbumItemQuery struct {
	AlbumID string
	Limit   int
	Offset  int
}

type AlbumItemPage struct {
	Items []AlbumItem `json:"items"`
	Page  Page        `json:"page"`
}

type AssetGeo struct {
	AssetID      string         `json:"asset_id"`
	Lat          float64        `json:"lat"`
	Lon          float64        `json:"lon"`
	Source       string         `json:"source"`
	Confidence   *float64       `json:"confidence,omitempty"`
	TakenAt      *time.Time     `json:"taken_at,omitempty"`
	TrackAssetID string         `json:"track_asset_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type GeoQuery struct {
	BBox      *BBox
	Source    string
	MediaKind string
	AlbumID   string
	TrackID   string
	TimeFrom  *time.Time
	TimeTo    *time.Time
	Limit     int
	Offset    int
	Clusters  bool
	Zoom      int
}

type GeoAsset struct {
	Asset Asset    `json:"asset"`
	Geo   AssetGeo `json:"geo"`
}

type BBox struct {
	MinLon float64 `json:"min_lon"`
	MinLat float64 `json:"min_lat"`
	MaxLon float64 `json:"max_lon"`
	MaxLat float64 `json:"max_lat"`
}

type PlaceCacheEntry struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	NormalizedName string         `json:"normalized_name"`
	Aliases        []string       `json:"aliases,omitempty"`
	Provider       string         `json:"provider"`
	DisplayName    string         `json:"display_name"`
	Country        string         `json:"country,omitempty"`
	Region         string         `json:"region,omitempty"`
	City           string         `json:"city,omitempty"`
	Road           string         `json:"road,omitempty"`
	Lat            float64        `json:"lat"`
	Lon            float64        `json:"lon"`
	BBox           BBox           `json:"bbox"`
	Source         string         `json:"source"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
}

type PlaceQuery struct {
	Q      string
	Limit  int
	Offset int
}

type GPSTrackQuery struct {
	Q        string
	BBox     *BBox
	TimeFrom *time.Time
	TimeTo   *time.Time
	Limit    int
	Offset   int
	Sort     string
}

type TrackPointQuery struct {
	TrackAssetID string
	TimeFrom     *time.Time
	TimeTo       *time.Time
	Simplify     bool
	MaxPoints    int
}

type TrackAssetQuery struct {
	TrackAssetID       string
	OffsetSeconds      int64
	MediaKind          string
	ExcludeTrackAssets bool
	IncludeGeotagged   bool
	IncludeUngeotagged bool
	Limit              int
	Offset             int
}

type ScanRun struct {
	ID                string         `json:"id"`
	JobID             string         `json:"job_id,omitempty"`
	StorageName       string         `json:"storage_name"`
	Mode              string         `json:"mode"`
	Prefixes          []string       `json:"prefixes"`
	MaxFiles          int            `json:"max_files"`
	MaxBytes          int64          `json:"max_bytes"`
	HashRequested     bool           `json:"hash_requested"`
	MetadataRequested bool           `json:"metadata_requested"`
	PreviewsRequested bool           `json:"previews_requested"`
	MarkMissing       bool           `json:"mark_missing"`
	DryRun            bool           `json:"dry_run"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	FinishedAt        *time.Time     `json:"finished_at,omitempty"`
	Report            map[string]any `json:"report"`
	CreatedAt         time.Time      `json:"created_at"`
}

type ScanRunQuery struct {
	StorageName string
	Limit       int
	Offset      int
}

type PreviewCacheEntry struct {
	ID             string     `json:"id"`
	AssetID        string     `json:"asset_id"`
	ContentID      string     `json:"content_id,omitempty"`
	Variant        string     `json:"variant"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	Format         string     `json:"format"`
	CachePath      string     `json:"cache_path"`
	Status         string     `json:"status"`
	SizeBytes      int64      `json:"size_bytes"`
	CreatedAt      time.Time  `json:"created_at"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	Error          string     `json:"error,omitempty"`
}

type PreviewCacheQuery struct {
	AssetID string
	Status  string
	Limit   int
	Offset  int
}

type PreviewCacheStats struct {
	Entries    int   `json:"entries"`
	Ready      int   `json:"ready"`
	Failed     int   `json:"failed"`
	Bytes      int64 `json:"bytes"`
	OldestUnix int64 `json:"oldest_unix,omitempty"`
}

type TranscodingPreset struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	BuiltIn        bool           `json:"built_in"`
	Available      bool           `json:"available"`
	DisabledReason string         `json:"disabled_reason,omitempty"`
	Hardware       string         `json:"hardware"`
	Codec          string         `json:"codec"`
	FFmpegEncoder  string         `json:"ffmpeg_encoder"`
	Mode           string         `json:"mode"`
	ParameterValue string         `json:"parameter_value"`
	Container      string         `json:"container"`
	ExtraArgs      map[string]any `json:"extra_args,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type AssetTag struct {
	AssetID    string         `json:"asset_id"`
	Tag        string         `json:"tag"`
	Source     string         `json:"source"`
	Confidence *float64       `json:"confidence,omitempty"`
	PluginID   string         `json:"plugin_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AIPrediction struct {
	ID           string         `json:"id"`
	AssetID      string         `json:"asset_id"`
	PluginID     string         `json:"plugin_id,omitempty"`
	WorkerID     string         `json:"worker_id"`
	Task         string         `json:"task"`
	Label        string         `json:"label"`
	Confidence   *float64       `json:"confidence,omitempty"`
	ModelName    string         `json:"model_name"`
	ModelVersion string         `json:"model_version"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type FaceDetection struct {
	ID         string         `json:"id"`
	AssetID    string         `json:"asset_id"`
	PluginID   string         `json:"plugin_id,omitempty"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Width      float64        `json:"width"`
	Height     float64        `json:"height"`
	Confidence *float64       `json:"confidence,omitempty"`
	ClusterID  string         `json:"cluster_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type FaceCluster struct {
	ID                   string         `json:"id"`
	Label                string         `json:"label"`
	RepresentativeFaceID string         `json:"representative_face_id,omitempty"`
	FaceCount            int            `json:"face_count"`
	AssetCount           int            `json:"asset_count"`
	IgnoredCount         int            `json:"ignored_count"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type EmbeddingModel struct {
	ID        string         `json:"id"`
	Modality  string         `json:"modality"`
	ModelName string         `json:"model_name"`
	Version   string         `json:"version"`
	Dimension int            `json:"dimension,omitempty"`
	PluginID  string         `json:"plugin_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type AssetEmbedding struct {
	ID        string         `json:"id"`
	AssetID   string         `json:"asset_id"`
	ModelID   string         `json:"model_id"`
	Modality  string         `json:"modality"`
	SourceRef string         `json:"source_ref"`
	Vector    []float64      `json:"vector"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type VectorSearchResult struct {
	Asset Asset   `json:"asset"`
	Score float64 `json:"score"`
	Match string  `json:"match"`
}

type Component struct {
	ID             string         `json:"id"`
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	Version        string         `json:"version,omitempty"`
	Status         string         `json:"status"`
	SourceType     string         `json:"source_type"`
	SourceURL      string         `json:"source_url,omitempty"`
	LicenseName    string         `json:"license_name,omitempty"`
	ProvenanceURL  string         `json:"provenance_url,omitempty"`
	InstallPath    string         `json:"install_path,omitempty"`
	ExecutablePath string         `json:"executable_path,omitempty"`
	Checksum       string         `json:"checksum,omitempty"`
	SizeBytes      int64          `json:"size_bytes,omitempty"`
	LastCheckedAt  *time.Time     `json:"last_checked_at,omitempty"`
	InstalledAt    *time.Time     `json:"installed_at,omitempty"`
	Error          string         `json:"error,omitempty"`
	Metadata       map[string]any `json:"metadata_json,omitempty"`
}

type ComponentEvent struct {
	ID           string         `json:"id"`
	ComponentKey string         `json:"component_key"`
	Level        string         `json:"level"`
	Message      string         `json:"message"`
	CreatedAt    time.Time      `json:"created_at"`
	Metadata     map[string]any `json:"metadata_json,omitempty"`
}

type ComponentQuery struct {
	Category string
	Status   string
	Q        string
	Limit    int
	Offset   int
}

type Store interface {
	UpsertDiscoveredFile(context.Context, storage.FileInfo) (UpsertResult, error)
	ListAssets(context.Context) ([]Asset, error)
	GetAsset(context.Context, string) (Asset, error)
	QueryAssets(context.Context, AssetQuery) (AssetPage, error)
	UpdateAssetMetadata(context.Context, string, *time.Time, map[string]any) error
	UpdateLocationHash(context.Context, string, string, int64) error
	Stats(context.Context) (Stats, error)
	UpsertTrackPoints(context.Context, string, []TrackPoint) error
	ListTracks(context.Context) ([]TrackSummary, error)
	GetTrack(context.Context, string) (TrackDetail, error)
	UpsertGPSTrackSummary(context.Context, TrackSummary, map[string]any) error
	ListGPSTracks(context.Context, GPSTrackQuery) ([]TrackSummary, error)
	UpdateGPSTrackMetadata(context.Context, string, string, string) error
	QueryTrackPoints(context.Context, TrackPointQuery) ([]TrackPoint, error)
	QueryTrackAssets(context.Context, TrackAssetQuery) (AssetPage, error)
	TrackCandidates(context.Context, string) ([]TrackCandidate, error)
	SaveTrackLink(context.Context, TrackLink) (TrackLink, error)
	ListTrackLinks(context.Context, string) ([]TrackLink, error)
	DeleteTrackLink(context.Context, string) error
	CreateAlbum(context.Context, Album) (Album, error)
	UpdateAlbum(context.Context, Album) (Album, error)
	DeleteAlbum(context.Context, string) error
	GetAlbum(context.Context, string) (Album, error)
	ListAlbums(context.Context, AlbumQuery) ([]Album, error)
	AddAlbumItems(context.Context, string, []string) error
	RemoveAlbumItem(context.Context, string, string) error
	ListAlbumItems(context.Context, AlbumItemQuery) (AlbumItemPage, error)
	UpsertAssetGeo(context.Context, AssetGeo, bool) (AssetGeo, error)
	GetAssetGeo(context.Context, string) (AssetGeo, error)
	QueryAssetGeo(context.Context, GeoQuery) ([]GeoAsset, error)
	CreateScanRun(context.Context, ScanRun) (ScanRun, error)
	UpdateScanRunReport(context.Context, string, map[string]any) error
	FinishScanRun(context.Context, string, map[string]any) error
	GetScanRunByJob(context.Context, string) (ScanRun, error)
	ListScanRuns(context.Context, ScanRunQuery) ([]ScanRun, error)
	UpsertPreviewCacheEntry(context.Context, PreviewCacheEntry) (PreviewCacheEntry, error)
	GetPreviewCacheEntry(context.Context, string, string, int, int, string) (PreviewCacheEntry, error)
	ListPreviewCacheEntries(context.Context, PreviewCacheQuery) ([]PreviewCacheEntry, error)
	MarkPreviewAccessed(context.Context, string) error
	PreviewCacheStats(context.Context) (PreviewCacheStats, error)
	CleanupPreviewCacheEntries(context.Context, time.Time, int64) ([]PreviewCacheEntry, error)
	ListTranscodingPresets(context.Context) ([]TranscodingPreset, error)
	UpsertTranscodingPreset(context.Context, TranscodingPreset) (TranscodingPreset, error)
	DeleteTranscodingPreset(context.Context, string) error
	UpsertAssetTag(context.Context, AssetTag) (AssetTag, error)
	ListAssetTags(context.Context, string) ([]AssetTag, error)
	CreateAIPrediction(context.Context, AIPrediction) (AIPrediction, error)
	ListAIPredictions(context.Context, string) ([]AIPrediction, error)
	DeleteAIPrediction(context.Context, string, string) error
	UpsertPlace(context.Context, PlaceCacheEntry) (PlaceCacheEntry, error)
	ListPlaces(context.Context, PlaceQuery) ([]PlaceCacheEntry, error)
	DeletePlace(context.Context, string) error
	CreateFaceDetection(context.Context, FaceDetection) (FaceDetection, error)
	ListFaceDetections(context.Context, string) ([]FaceDetection, error)
	UpsertFaceCluster(context.Context, FaceCluster) (FaceCluster, error)
	ListFaceClusters(context.Context) ([]FaceCluster, error)
	UpdateFaceDetection(context.Context, FaceDetection) (FaceDetection, error)
	UpsertEmbeddingModel(context.Context, EmbeddingModel) (EmbeddingModel, error)
	UpsertAssetEmbedding(context.Context, AssetEmbedding) (AssetEmbedding, error)
	ListAssetEmbeddings(context.Context, string) ([]AssetEmbedding, error)
	VectorSearch(context.Context, string, []float64, int) ([]VectorSearchResult, error)
	UpsertTranscript(context.Context, Transcript, []TranscriptSegment) (Transcript, error)
	ListTranscripts(context.Context, string, int) ([]Transcript, error)
	ListAllTranscripts(context.Context, int, int) ([]Transcript, error)
	DeleteTranscript(context.Context, string) error
	UpsertAudioFeatures(context.Context, AudioFeatures) (AudioFeatures, error)
	GetAudioFeatures(context.Context, string) (AudioFeatures, error)
	UpsertVideoFrameCaption(context.Context, VideoFrameCaption) (VideoFrameCaption, error)
	ListVideoFrameCaptions(context.Context, string, int) ([]VideoFrameCaption, error)
	UpsertDocumentText(context.Context, DocumentText) (DocumentText, error)
	GetDocumentText(context.Context, string) (DocumentText, error)
	UpsertComponent(context.Context, Component) (Component, error)
	ListComponents(context.Context, ComponentQuery) ([]Component, error)
	GetComponent(context.Context, string) (Component, error)
	AddComponentEvent(context.Context, ComponentEvent) (ComponentEvent, error)
	ListComponentEvents(context.Context, string, int) ([]ComponentEvent, error)
	EnqueueJob(context.Context, jobs.Job) (jobs.Job, error)
	UpdateJob(context.Context, jobs.Job) error
	ListJobs(context.Context) ([]jobs.Job, error)
	GetJob(context.Context, string) (jobs.Job, error)
	LeaseNextJob(context.Context, string, []string, time.Duration) (jobs.Job, error)
	HeartbeatJob(context.Context, string, string, time.Duration) error
	UpdateLeasedJob(context.Context, jobs.Job, string) error
	CompleteLeasedJob(context.Context, jobs.Job, string) error
	FailLeasedJob(context.Context, jobs.Job, string, error) error
	CancelLeasedJob(context.Context, jobs.Job, string) error
	RequestCancelJob(context.Context, string) (jobs.Job, error)
	ReleaseExpiredLeases(context.Context, time.Time) (int64, error)
}

type MemoryStore struct {
	mu               sync.RWMutex
	assets           map[string]Asset
	byURL            map[string]string
	locationByAsset  map[string]string
	trackPoints      map[string][]TrackPoint
	trackLinks       map[string]TrackLink
	albums           map[string]Album
	albumItems       map[string]map[string]AlbumItem
	assetGeo         map[string]AssetGeo
	gpsTracks        map[string]TrackSummary
	scanRuns         map[string]ScanRun
	previewEntries   map[string]PreviewCacheEntry
	transcodePresets map[string]TranscodingPreset
	assetTags        map[string]map[string]AssetTag
	aiPredictions    map[string][]AIPrediction
	places           map[string]PlaceCacheEntry
	faceDetections   map[string][]FaceDetection
	faceClusters     map[string]FaceCluster
	embeddingModels  map[string]EmbeddingModel
	assetEmbeddings  map[string]AssetEmbedding
	transcripts      map[string][]Transcript
	audioFeatures    map[string]AudioFeatures
	frameCaptions    map[string][]VideoFrameCaption
	documentText     map[string]DocumentText
	components       map[string]Component
	componentEvents  map[string][]ComponentEvent
	jobs             map[string]jobs.Job
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		assets:           make(map[string]Asset),
		byURL:            make(map[string]string),
		locationByAsset:  make(map[string]string),
		trackPoints:      make(map[string][]TrackPoint),
		trackLinks:       make(map[string]TrackLink),
		albums:           make(map[string]Album),
		albumItems:       make(map[string]map[string]AlbumItem),
		assetGeo:         make(map[string]AssetGeo),
		gpsTracks:        make(map[string]TrackSummary),
		scanRuns:         make(map[string]ScanRun),
		previewEntries:   make(map[string]PreviewCacheEntry),
		transcodePresets: make(map[string]TranscodingPreset),
		assetTags:        make(map[string]map[string]AssetTag),
		aiPredictions:    make(map[string][]AIPrediction),
		places:           make(map[string]PlaceCacheEntry),
		faceDetections:   make(map[string][]FaceDetection),
		faceClusters:     make(map[string]FaceCluster),
		embeddingModels:  make(map[string]EmbeddingModel),
		assetEmbeddings:  make(map[string]AssetEmbedding),
		transcripts:      make(map[string][]Transcript),
		audioFeatures:    make(map[string]AudioFeatures),
		frameCaptions:    make(map[string][]VideoFrameCaption),
		documentText:     make(map[string]DocumentText),
		components:       make(map[string]Component),
		componentEvents:  make(map[string][]ComponentEvent),
		jobs:             make(map[string]jobs.Job),
	}
}

func (s *MemoryStore) UpsertDiscoveredFile(_ context.Context, info storage.FileInfo) (UpsertResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if assetID, ok := s.byURL[info.StorageURL]; ok {
		asset := s.assets[assetID]
		if len(asset.Locations) > 0 {
			asset.Locations[0].SizeBytes = info.SizeBytes
			asset.Locations[0].MTime = info.MTime
			asset.Locations[0].MIME = info.MIME
			asset.Locations[0].LastSeenAt = now
			asset.Locations[0].MediaKind = info.MediaKind
			asset.Locations[0].Extension = info.Extension
		}
		asset.MediaKind = info.MediaKind
		asset.DisplayName = info.Name
		asset.UpdatedAt = now
		s.assets[assetID] = asset
		return UpsertResult{Asset: asset}, nil
	}
	assetID := id.NewUUID()
	locationID := id.NewUUID()
	asset := Asset{
		ID:          assetID,
		MediaKind:   info.MediaKind,
		DisplayName: info.Name,
		Metadata:    map[string]any{},
		FirstSeenAt: now,
		UpdatedAt:   now,
		Locations: []Location{{
			ID:           locationID,
			AssetID:      assetID,
			StorageName:  info.StorageName,
			StorageURL:   info.StorageURL,
			RelativePath: info.RelativePath,
			FileName:     info.Name,
			Extension:    info.Extension,
			MIME:         info.MIME,
			MediaKind:    info.MediaKind,
			SizeBytes:    info.SizeBytes,
			MTime:        info.MTime,
			HashStatus:   HashStatusUnhashed,
			LastSeenAt:   now,
		}},
	}
	s.assets[assetID] = asset
	s.byURL[info.StorageURL] = assetID
	s.locationByAsset[assetID] = locationID
	return UpsertResult{Asset: asset, Created: true}, nil
}

func (s *MemoryStore) ListAssets(_ context.Context) ([]Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Asset, 0, len(s.assets))
	for _, asset := range s.assets {
		out = append(out, cloneAsset(asset))
	}
	sort.Slice(out, func(i, j int) bool {
		left := ""
		right := ""
		if len(out[i].Locations) > 0 {
			left = out[i].Locations[0].StorageURL
		}
		if len(out[j].Locations) > 0 {
			right = out[j].Locations[0].StorageURL
		}
		return left < right
	})
	return out, nil
}

func (s *MemoryStore) GetAsset(_ context.Context, assetID string) (Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return cloneAsset(asset), nil
}

func (s *MemoryStore) UpdateLocationHash(_ context.Context, assetID, sha512Hex string, bytes int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return ErrNotFound
	}
	contentID := id.NewUUID()
	targetAssetID := assetID
	for candidateID, candidate := range s.assets {
		if candidateID == assetID {
			continue
		}
		for _, loc := range candidate.Locations {
			if loc.HashStatus == HashStatusHashed && loc.SHA512Hex == sha512Hex && loc.SizeBytes == bytes {
				contentID = loc.ContentID
				targetAssetID = candidateID
				break
			}
		}
		if targetAssetID != assetID {
			break
		}
	}
	for i := range asset.Locations {
		asset.Locations[i].SHA512Hex = sha512Hex
		asset.Locations[i].ContentID = contentID
		asset.Locations[i].HashStatus = HashStatusHashed
		if bytes >= 0 {
			asset.Locations[i].SizeBytes = bytes
		}
	}
	now := time.Now().UTC()
	if targetAssetID != assetID {
		target := s.assets[targetAssetID]
		for _, loc := range asset.Locations {
			loc.AssetID = targetAssetID
			target.Locations = append(target.Locations, loc)
			s.byURL[loc.StorageURL] = targetAssetID
		}
		if target.TakenAt == nil && asset.TakenAt != nil {
			taken := asset.TakenAt.UTC()
			target.TakenAt = &taken
		}
		if target.Metadata == nil && asset.Metadata != nil {
			target.Metadata = map[string]any{}
		}
		for key, value := range asset.Metadata {
			if _, exists := target.Metadata[key]; !exists {
				target.Metadata[key] = value
			}
		}
		target.UpdatedAt = now
		s.assets[targetAssetID] = target
		delete(s.assets, assetID)
		delete(s.locationByAsset, assetID)
		return nil
	}
	asset.UpdatedAt = now
	s.assets[assetID] = asset
	return nil
}

func (s *MemoryStore) UpdateAssetMetadata(_ context.Context, assetID string, takenAt *time.Time, metadata map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return ErrNotFound
	}
	if takenAt != nil {
		t := takenAt.UTC()
		asset.TakenAt = &t
	}
	if asset.Metadata == nil {
		asset.Metadata = map[string]any{}
	}
	for key, value := range metadata {
		asset.Metadata[key] = value
	}
	asset.UpdatedAt = time.Now().UTC()
	s.assets[assetID] = asset
	return nil
}

func (s *MemoryStore) Stats(_ context.Context) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var stats Stats
	duplicateCounts := map[string]int{}
	stats.Assets = len(s.assets)
	for _, asset := range s.assets {
		for _, loc := range asset.Locations {
			stats.Locations++
			stats.TotalBytes += loc.SizeBytes
			switch loc.MediaKind {
			case "photo":
				stats.Photos++
			case "video":
				stats.Videos++
			case "audio":
				stats.Audio++
			case "document":
				stats.Documents++
			case "track":
				stats.Tracks++
			}
			switch loc.HashStatus {
			case HashStatusHashed:
				stats.Hashed++
				if loc.ContentID != "" {
					duplicateCounts[loc.ContentID]++
				} else if loc.SHA512Hex != "" {
					duplicateCounts[loc.SHA512Hex]++
				}
			default:
				stats.Unhashed++
			}
		}
	}
	for _, count := range duplicateCounts {
		if count > 1 {
			stats.DuplicateGroups++
			stats.DuplicateLocations += count
		}
	}
	return stats, nil
}

func (s *MemoryStore) UpsertTrackPoints(_ context.Context, trackAssetID string, points []TrackPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[trackAssetID]; !ok {
		return ErrNotFound
	}
	out := make([]TrackPoint, 0, len(points))
	for i, point := range points {
		point.ID = int64(i + 1)
		point.TrackAssetID = trackAssetID
		if point.Source == "" {
			point.Source = "gpx"
		}
		out = append(out, point)
	}
	s.trackPoints[trackAssetID] = out
	return nil
}

func (s *MemoryStore) ListTracks(_ context.Context) ([]TrackSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TrackSummary, 0, len(s.trackPoints))
	for trackAssetID, points := range s.trackPoints {
		out = append(out, summarizeTrack(s.assets[trackAssetID], points))
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].StartTime
		right := out[j].StartTime
		if left == nil || right == nil {
			return out[i].Name < out[j].Name
		}
		return left.Before(*right)
	})
	return out, nil
}

func (s *MemoryStore) GetTrack(_ context.Context, trackAssetID string) (TrackDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points, ok := s.trackPoints[trackAssetID]
	if !ok {
		return TrackDetail{}, ErrNotFound
	}
	return TrackDetail{Summary: summarizeTrack(s.assets[trackAssetID], points), Points: append([]TrackPoint(nil), points...)}, nil
}

func (s *MemoryStore) TrackCandidates(_ context.Context, assetID string) ([]TrackCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	asset, ok := s.assets[assetID]
	if !ok {
		return nil, ErrNotFound
	}
	assetStart, assetEnd, ok := assetInterval(asset)
	if !ok {
		return nil, nil
	}
	var out []TrackCandidate
	for trackAssetID, points := range s.trackPoints {
		summary := summarizeTrack(s.assets[trackAssetID], points)
		if summary.StartTime == nil || summary.EndTime == nil {
			continue
		}
		overlapStart, overlapEnd, overlap := overlapInterval(assetStart, assetEnd, *summary.StartTime, *summary.EndTime)
		if !overlap {
			continue
		}
		confidence := overlapEnd.Sub(overlapStart).Seconds() / assetEnd.Sub(assetStart).Seconds()
		if confidence > 1 {
			confidence = 1
		}
		out = append(out, TrackCandidate{Track: summary, OverlapStart: &overlapStart, OverlapEnd: &overlapEnd, OverlapSeconds: overlapEnd.Sub(overlapStart).Seconds(), Confidence: confidence, Reason: "overlapping asset and track time intervals"})
	}
	return out, nil
}

func (s *MemoryStore) SaveTrackLink(_ context.Context, link TrackLink) (TrackLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.assets[link.AssetID]; !ok {
		return TrackLink{}, ErrNotFound
	}
	if _, ok := s.assets[link.TrackAssetID]; !ok {
		return TrackLink{}, ErrNotFound
	}
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
	s.trackLinks[link.ID] = link
	return link, nil
}

func (s *MemoryStore) ListTrackLinks(_ context.Context, assetID string) ([]TrackLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []TrackLink
	for _, link := range s.trackLinks {
		if assetID == "" || link.AssetID == assetID {
			out = append(out, link)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) DeleteTrackLink(_ context.Context, linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.trackLinks[linkID]; !ok {
		return ErrNotFound
	}
	delete(s.trackLinks, linkID)
	return nil
}

func (s *MemoryStore) EnqueueJob(_ context.Context, job jobs.Job) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	s.jobs[job.ID] = job
	return cloneJob(job), nil
}

func (s *MemoryStore) UpdateJob(_ context.Context, job jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return ErrNotFound
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) ListJobs(_ context.Context) ([]jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]jobs.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) GetJob(_ context.Context, jobID string) (jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, ErrNotFound
	}
	return cloneJob(job), nil
}

func (s *MemoryStore) LeaseNextJob(_ context.Context, workerID string, kinds []string, leaseDuration time.Duration) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	_, _ = s.releaseExpiredLeasesLocked(now)
	var selected *jobs.Job
	for _, job := range s.jobs {
		job := job
		if job.Status != jobs.StatusQueued {
			continue
		}
		if job.NextRunAt != nil && job.NextRunAt.After(now) {
			continue
		}
		if !jobKindAllowed(job.Kind, kinds) {
			continue
		}
		if selected == nil || job.CreatedAt.Before(selected.CreatedAt) || (job.CreatedAt.Equal(selected.CreatedAt) && job.ID < selected.ID) {
			selected = &job
		}
	}
	if selected == nil {
		return jobs.Job{}, ErrNotFound
	}
	job := *selected
	job.Status = jobs.StatusRunning
	job.WorkerID = workerID
	leaseUntil := now.Add(leaseDuration)
	job.LeaseExpiresAt = &leaseUntil
	if job.StartedAt == nil {
		started := now
		job.StartedAt = &started
	}
	job.Attempts++
	job.NextRunAt = nil
	job.Error = ""
	s.jobs[job.ID] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *MemoryStore) HeartbeatJob(_ context.Context, jobID, workerID string, leaseDuration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return ErrNotFound
	}
	if job.WorkerID != workerID || (job.Status != jobs.StatusRunning && job.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	leaseUntil := time.Now().UTC().Add(leaseDuration)
	job.LeaseExpiresAt = &leaseUntil
	s.jobs[jobID] = job
	return nil
}

func (s *MemoryStore) UpdateLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	if current.Status == jobs.StatusCancelRequested && job.Status == jobs.StatusRunning {
		job.Status = jobs.StatusCancelRequested
		job.CancelRequestedAt = current.CancelRequestedAt
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) CompleteLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || current.Status != jobs.StatusRunning {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	if err := jobs.Complete(&job); err != nil {
		return err
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) FailLeasedJob(_ context.Context, job jobs.Job, workerID string, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	return s.failLeasedLocked(job, cause)
}

func (s *MemoryStore) CancelLeasedJob(_ context.Context, job jobs.Job, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok {
		return ErrNotFound
	}
	if current.WorkerID != workerID || (current.Status != jobs.StatusRunning && current.Status != jobs.StatusCancelRequested) {
		return ErrJobLeaseLost
	}
	job.WorkerID = current.WorkerID
	job.LeaseExpiresAt = current.LeaseExpiresAt
	if err := jobs.Cancel(&job); err != nil {
		return err
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *MemoryStore) RequestCancelJob(_ context.Context, jobID string) (jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.Job{}, ErrNotFound
	}
	if err := jobs.RequestCancel(&job); err != nil {
		return jobs.Job{}, err
	}
	jobs.AddLog(&job, "info", "cancellation requested")
	s.jobs[jobID] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *MemoryStore) ReleaseExpiredLeases(_ context.Context, now time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releaseExpiredLeasesLocked(now)
}

func (s *MemoryStore) releaseExpiredLeasesLocked(now time.Time) (int64, error) {
	var released int64
	for id, job := range s.jobs {
		if job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.Before(now) {
			continue
		}
		if job.Status != jobs.StatusRunning && job.Status != jobs.StatusCancelRequested {
			continue
		}
		released++
		if job.Status == jobs.StatusCancelRequested {
			_ = jobs.Cancel(&job)
			jobs.AddLog(&job, "warn", "expired lease cancelled after cancellation request")
			s.jobs[id] = cloneJob(job)
			continue
		}
		_ = s.failLeasedLocked(job, fmt.Errorf("job lease expired"))
	}
	return released, nil
}

func (s *MemoryStore) failLeasedLocked(job jobs.Job, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("job failed")
	}
	maxAttempts := job.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if job.Attempts < maxAttempts && job.Status != jobs.StatusCancelRequested && jobs.ShouldRetry(cause) {
		delay := retryDelay(job.Attempts)
		if err := jobs.Retry(&job, delay, cause); err != nil {
			return err
		}
		jobs.AddLog(&job, "warn", fmt.Sprintf("will retry after %s: %v", delay, cause))
	} else {
		if err := jobs.Fail(&job, cause); err != nil {
			return err
		}
		jobs.AddLog(&job, "error", cause.Error())
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

var ErrNotFound = &notFoundError{}

var ErrInvalid = errors.New("invalid catalog input")

var ErrJobLeaseLost = errors.New("job lease is not owned by worker")

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

type ExplorerOptions struct {
	Storage    string
	Path       string
	Q          string
	MediaKind  string
	HashStatus string
	Extension  string
	Limit      int
	Offset     int
	Sort       string
}

type ExplorerView struct {
	CurrentPath string           `json:"current_path"`
	ParentPath  string           `json:"parent_path,omitempty"`
	Folders     []ExplorerFolder `json:"folders"`
	Files       []ExplorerFile   `json:"files"`
	FileCount   int              `json:"file_count"`
	FolderCount int              `json:"folder_count"`
	TotalBytes  int64            `json:"total_bytes"`
	Offset      int              `json:"offset"`
	Limit       int              `json:"limit"`
}

type ExplorerFolder struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	FileCount   int       `json:"file_count"`
	TotalBytes  int64     `json:"total_bytes"`
	LatestMTime time.Time `json:"latest_mtime"`
}

type ExplorerFile struct {
	AssetID      string    `json:"asset_id"`
	Name         string    `json:"name"`
	MediaKind    string    `json:"media_kind"`
	StorageURL   string    `json:"storage_url"`
	RelativePath string    `json:"relative_path"`
	SizeBytes    int64     `json:"size_bytes"`
	MTime        time.Time `json:"mtime"`
	HashStatus   string    `json:"hash_status"`
	SHA512Hex    string    `json:"sha512_hex,omitempty"`
}

func BuildExplorerView(assets []Asset, opts ExplorerOptions) (ExplorerView, error) {
	current, err := normalizeExplorerPath(opts.Path)
	if err != nil {
		return ExplorerView{}, err
	}
	if opts.Limit <= 0 || opts.Limit > 500 {
		opts.Limit = 200
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	view := ExplorerView{
		CurrentPath: current,
		ParentPath:  parentExplorerPath(current),
		Folders:     []ExplorerFolder{},
		Files:       []ExplorerFile{},
		Limit:       opts.Limit,
		Offset:      opts.Offset,
	}
	folderIndex := map[string]int{}
	for _, asset := range assets {
		if opts.Q != "" && len(SearchAssets([]Asset{asset}, opts.Q)) == 0 {
			continue
		}
		for _, loc := range asset.Locations {
			if opts.Storage != "" && loc.StorageName != opts.Storage {
				continue
			}
			if opts.MediaKind != "" && loc.MediaKind != opts.MediaKind {
				continue
			}
			if opts.HashStatus != "" && loc.HashStatus != opts.HashStatus {
				continue
			}
			if opts.Extension != "" && strings.TrimPrefix(strings.ToLower(loc.Extension), ".") != strings.TrimPrefix(strings.ToLower(opts.Extension), ".") {
				continue
			}
			remaining, ok := pathRemainder(loc.RelativePath, current)
			if !ok || remaining == "" {
				continue
			}
			view.TotalBytes += loc.SizeBytes
			if slash := strings.IndexByte(remaining, '/'); slash >= 0 {
				name := remaining[:slash]
				folderPath := joinExplorerPath(current, name)
				idx, ok := folderIndex[folderPath]
				if !ok {
					view.Folders = append(view.Folders, ExplorerFolder{Name: name, Path: folderPath})
					idx = len(view.Folders) - 1
					folderIndex[folderPath] = idx
				}
				view.Folders[idx].FileCount++
				view.Folders[idx].TotalBytes += loc.SizeBytes
				if loc.MTime.After(view.Folders[idx].LatestMTime) {
					view.Folders[idx].LatestMTime = loc.MTime
				}
				continue
			}
			view.Files = append(view.Files, ExplorerFile{
				AssetID:      asset.ID,
				Name:         loc.FileName,
				MediaKind:    loc.MediaKind,
				StorageURL:   loc.StorageURL,
				RelativePath: loc.RelativePath,
				SizeBytes:    loc.SizeBytes,
				MTime:        loc.MTime,
				HashStatus:   loc.HashStatus,
				SHA512Hex:    loc.SHA512Hex,
			})
		}
	}
	sort.Slice(view.Folders, func(i, j int) bool {
		return view.Folders[i].Name < view.Folders[j].Name
	})
	sortExplorerFiles(view.Files, opts.Sort)
	view.FileCount = len(view.Files)
	view.FolderCount = len(view.Folders)
	if opts.Offset >= len(view.Files) {
		view.Files = []ExplorerFile{}
		return view, nil
	}
	end := opts.Offset + opts.Limit
	if end > len(view.Files) {
		end = len(view.Files)
	}
	view.Files = view.Files[opts.Offset:end]
	return view, nil
}

func BuildDuplicateGroups(assets []Asset, limit, offset int) DuplicatePage {
	type key struct {
		hash string
		size int64
	}
	groupsByKey := map[key]*DuplicateGroup{}
	for _, asset := range assets {
		for _, loc := range asset.Locations {
			if loc.HashStatus != HashStatusHashed || loc.SHA512Hex == "" {
				continue
			}
			k := key{hash: loc.SHA512Hex, size: loc.SizeBytes}
			group := groupsByKey[k]
			if group == nil {
				group = &DuplicateGroup{
					ContentID: loc.ContentID,
					SHA512Hex: loc.SHA512Hex,
					SizeBytes: loc.SizeBytes,
					Assets:    []DuplicateAsset{},
				}
				groupsByKey[k] = group
			}
			if group.ContentID == "" {
				group.ContentID = loc.ContentID
			}
			group.Assets = append(group.Assets, DuplicateAsset{
				AssetID: asset.ID, DisplayName: asset.DisplayName, MediaKind: asset.MediaKind,
				StorageName: loc.StorageName, RelativePath: loc.RelativePath, StorageURL: loc.StorageURL, MTime: loc.MTime,
			})
			group.TotalBytes += loc.SizeBytes
		}
	}
	groups := []DuplicateGroup{}
	for _, group := range groupsByKey {
		if len(group.Assets) < 2 {
			continue
		}
		sort.Slice(group.Assets, func(i, j int) bool {
			if group.Assets[i].DisplayName == group.Assets[j].DisplayName {
				return group.Assets[i].RelativePath < group.Assets[j].RelativePath
			}
			return group.Assets[i].DisplayName < group.Assets[j].DisplayName
		})
		group.AssetCount = len(group.Assets)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].TotalBytes == groups[j].TotalBytes {
			return groups[i].SHA512Hex < groups[j].SHA512Hex
		}
		return groups[i].TotalBytes > groups[j].TotalBytes
	})
	total := len(groups)
	limit, offset = normalizePage(limit, offset)
	if offset >= len(groups) {
		groups = []DuplicateGroup{}
	} else {
		end := offset + limit
		if end > len(groups) {
			end = len(groups)
		}
		groups = groups[offset:end]
	}
	return DuplicatePage{Groups: groups, Page: Page{Limit: limit, Offset: offset, Total: total}}
}

func cloneAsset(asset Asset) Asset {
	asset.Locations = append([]Location(nil), asset.Locations...)
	if asset.Metadata != nil {
		metadata := make(map[string]any, len(asset.Metadata))
		for key, value := range asset.Metadata {
			metadata[key] = value
		}
		asset.Metadata = metadata
	}
	return asset
}

func cloneJob(job jobs.Job) jobs.Job {
	job.Logs = append([]jobs.LogLine(nil), job.Logs...)
	return job
}

func FirstLocation(asset Asset) (Location, bool) {
	if len(asset.Locations) == 0 {
		return Location{}, false
	}
	return asset.Locations[0], true
}

func SearchAssets(assets []Asset, query string) []Asset {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return assets
	}
	var out []Asset
	for _, asset := range assets {
		if strings.Contains(strings.ToLower(asset.DisplayName), query) {
			out = append(out, asset)
			continue
		}
		for _, loc := range asset.Locations {
			if strings.Contains(strings.ToLower(loc.RelativePath), query) {
				out = append(out, asset)
				break
			}
		}
	}
	return out
}

func AssetTimestampCandidates(asset Asset, loc *time.Location) []TimestampCandidate {
	if loc == nil {
		loc = time.Local
	}
	var out []TimestampCandidate
	add := func(source, raw string, ts time.Time, confidence float64) {
		if ts.IsZero() {
			return
		}
		for _, existing := range out {
			if existing.Source == source && existing.Time.Equal(ts) {
				return
			}
		}
		out = append(out, TimestampCandidate{Source: source, Raw: raw, Time: ts, Confidence: confidence})
	}
	if asset.TakenAt != nil {
		add("taken_at", asset.TakenAt.Format(time.RFC3339Nano), *asset.TakenAt, 1.0)
	}
	if raw, ok := metadataString(asset.Metadata, "exif_datetime_original_raw"); ok {
		if ts, ok := parseLooseLocalTime(raw, loc); ok {
			add("exif_datetime_original_raw", raw, ts, 0.82)
		}
	}
	names := []string{asset.DisplayName}
	for _, location := range asset.Locations {
		names = append(names, location.FileName, location.RelativePath)
		if !location.MTime.IsZero() {
			add("file_mtime", location.MTime.Format(time.RFC3339Nano), location.MTime, 0.62)
		}
	}
	for _, name := range names {
		if ts, raw, ok := parsePixelFilenameTime(name, loc); ok {
			add("filename_timestamp", raw, ts, 0.76)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence == out[j].Confidence {
			return out[i].Time.Before(out[j].Time)
		}
		return out[i].Confidence > out[j].Confidence
	})
	return out
}

func AssetTimestampInRange(asset Asset, start, end time.Time, tolerance time.Duration, loc *time.Location) (TimestampCandidate, bool) {
	windowStart := start.Add(-tolerance)
	windowEnd := end.Add(tolerance)
	for _, candidate := range AssetTimestampCandidates(asset, loc) {
		if !candidate.Time.Before(windowStart) && !candidate.Time.After(windowEnd) {
			return candidate, true
		}
	}
	return TimestampCandidate{}, false
}

func AssetPrimaryTimestamp(asset Asset, loc *time.Location) (TimestampCandidate, bool) {
	candidates := AssetTimestampCandidates(asset, loc)
	if len(candidates) == 0 {
		return TimestampCandidate{}, false
	}
	return candidates[0], true
}

func metadataString(metadata map[string]any, key string) (string, bool) {
	if metadata == nil {
		return "", false
	}
	value, ok := metadata[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		return typed, typed != ""
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		return text, text != ""
	}
}

func parseLooseLocalTime(raw string, loc *time.Location) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006:01:02 15:04:05",
		"2006-01-02 15:04:05",
		"20060102_150405",
		"20060102-150405",
	} {
		if ts, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return ts, true
		}
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func parsePixelFilenameTime(name string, loc *time.Location) (time.Time, string, bool) {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	for _, prefix := range []string{"pxl_", "vid_", "img_", "dsc_"} {
		idx := strings.Index(lower, prefix)
		if idx < 0 {
			continue
		}
		start := idx + len(prefix)
		if len(name) < start+15 {
			continue
		}
		raw := name[start : start+15]
		if raw[8] != '_' && raw[8] != '-' {
			continue
		}
		datePart := raw[:8]
		timePart := raw[9:15]
		if !allDigits(datePart) || !allDigits(timePart) {
			continue
		}
		parsedRaw := datePart + "_" + timePart
		if ts, err := time.ParseInLocation("20060102_150405", parsedRaw, loc); err == nil {
			return ts, parsedRaw, true
		}
	}
	return time.Time{}, "", false
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func jobKindAllowed(kind string, kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, allowed := range kinds {
		if kind == allowed {
			return true
		}
	}
	return false
}

func retryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	if attempts > 5 {
		attempts = 5
	}
	return time.Duration(attempts) * time.Second
}

func normalizeExplorerPath(input string) (string, error) {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	if input == "" || input == "/" {
		return "", nil
	}
	if strings.HasPrefix(input, "/") {
		return "", fmt.Errorf("explorer path must be relative")
	}
	clean := path.Clean(input)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("explorer path escapes root")
	}
	return clean, nil
}

func parentExplorerPath(current string) string {
	if current == "" {
		return ""
	}
	parent := path.Dir(current)
	if parent == "." {
		return ""
	}
	return parent
}

func joinExplorerPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func pathRemainder(relativePath, current string) (string, bool) {
	if current == "" {
		return relativePath, true
	}
	if relativePath == current {
		return "", false
	}
	prefix := current + "/"
	if !strings.HasPrefix(relativePath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(relativePath, prefix), true
}

func sortExplorerFiles(files []ExplorerFile, key string) {
	switch key {
	case "size":
		sort.Slice(files, func(i, j int) bool {
			if files[i].SizeBytes == files[j].SizeBytes {
				return files[i].Name < files[j].Name
			}
			return files[i].SizeBytes < files[j].SizeBytes
		})
	case "mtime":
		sort.Slice(files, func(i, j int) bool {
			if files[i].MTime.Equal(files[j].MTime) {
				return files[i].Name < files[j].Name
			}
			return files[i].MTime.After(files[j].MTime)
		})
	case "media_kind":
		sort.Slice(files, func(i, j int) bool {
			if files[i].MediaKind == files[j].MediaKind {
				return files[i].Name < files[j].Name
			}
			return files[i].MediaKind < files[j].MediaKind
		})
	default:
		sort.Slice(files, func(i, j int) bool {
			return files[i].Name < files[j].Name
		})
	}
}

func summarizeTrack(asset Asset, points []TrackPoint) TrackSummary {
	summary := TrackSummary{TrackAssetID: asset.ID, Name: asset.DisplayName, PointCount: len(points)}
	if asset.Metadata != nil {
		if value, ok := asset.Metadata["track_source_format"].(string); ok {
			summary.SourceFormat = value
		}
	}
	for i, point := range points {
		if summary.SourceFormat == "" {
			summary.SourceFormat = point.Source
		}
		lat := point.Lat
		lon := point.Lon
		if i == 0 {
			if !point.RecordedAt.IsZero() {
				start := point.RecordedAt
				end := point.RecordedAt
				summary.StartTime = &start
				summary.EndTime = &end
			}
			summary.MinLat = &lat
			summary.MaxLat = &lat
			summary.MinLon = &lon
			summary.MaxLon = &lon
		}
		if !point.RecordedAt.IsZero() && (summary.StartTime == nil || point.RecordedAt.Before(*summary.StartTime)) {
			t := point.RecordedAt
			summary.StartTime = &t
		}
		if !point.RecordedAt.IsZero() && (summary.EndTime == nil || point.RecordedAt.After(*summary.EndTime)) {
			t := point.RecordedAt
			summary.EndTime = &t
		}
		if summary.MinLat == nil || point.Lat < *summary.MinLat {
			v := point.Lat
			summary.MinLat = &v
		}
		if summary.MaxLat == nil || point.Lat > *summary.MaxLat {
			v := point.Lat
			summary.MaxLat = &v
		}
		if summary.MinLon == nil || point.Lon < *summary.MinLon {
			v := point.Lon
			summary.MinLon = &v
		}
		if summary.MaxLon == nil || point.Lon > *summary.MaxLon {
			v := point.Lon
			summary.MaxLon = &v
		}
		if point.ElevationM != nil {
			ele := *point.ElevationM
			if summary.ElevationMin == nil || ele < *summary.ElevationMin {
				summary.ElevationMin = &ele
			}
			if summary.ElevationMax == nil || ele > *summary.ElevationMax {
				summary.ElevationMax = &ele
			}
		}
		if i > 0 {
			summary.DistanceM += haversineMeters(points[i-1].Lat, points[i-1].Lon, point.Lat, point.Lon)
		}
	}
	if summary.StartTime != nil && summary.EndTime != nil && summary.EndTime.After(*summary.StartTime) {
		duration := summary.EndTime.Sub(*summary.StartTime).Seconds()
		summary.DurationSec = &duration
	}
	return summary
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const radiusM = 6371008.8
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLambda := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return 2 * radiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func assetInterval(asset Asset) (time.Time, time.Time, bool) {
	if asset.TakenAt == nil {
		return time.Time{}, time.Time{}, false
	}
	start := asset.TakenAt.UTC()
	durationSeconds := float64(1)
	if asset.Metadata != nil {
		switch value := asset.Metadata["duration_seconds"].(type) {
		case float64:
			durationSeconds = value
		case int:
			durationSeconds = float64(value)
		case int64:
			durationSeconds = float64(value)
		case jsonNumber:
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

func overlapInterval(aStart, aEnd, bStart, bEnd time.Time) (time.Time, time.Time, bool) {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	return start, end, end.After(start) || end.Equal(start)
}

type jsonNumber interface {
	Float64() (float64, error)
}
