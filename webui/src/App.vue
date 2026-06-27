<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import OLMap from "ol/Map";
import View from "ol/View";
import GeoJSON from "ol/format/GeoJSON";
import { defaults as defaultControls } from "ol/control/defaults";
import ScaleLine from "ol/control/ScaleLine";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import Overlay from "ol/Overlay";
import { defaults as defaultInteractions } from "ol/interaction/defaults";
import VectorSource from "ol/source/Vector";
import XYZ from "ol/source/XYZ";
import { fromLonLat, toLonLat } from "ol/proj";
import Feature, { type FeatureLike } from "ol/Feature";
import Point from "ol/geom/Point";
import LineString from "ol/geom/LineString";
import MultiLineString from "ol/geom/MultiLineString";
import { Circle as CircleStyle, Fill, Icon, RegularShape, Stroke, Style, Text } from "ol/style";
import "ol/ol.css";
import {
  api,
  type Album,
  type AlbumItemPage,
  type APIToken,
  type Asset,
  type AssetDetail,
  type AssetRelated,
  type BackendStatus,
  type ComponentEvent,
  type ComponentRecord,
  type DuplicatePage,
  type ExplorerRow,
  type ExplorerView,
  type FileBrowseResponse,
  type FaceCluster,
  type FaceDetection,
  type GeoAlignMarker,
  type GeoAlignSession,
  type Job,
  type JobStats,
  type IndexingStatus,
  type KnowledgeChatResponse,
  type KnowledgeExtractionResult,
  type KnowledgeFact,
  type KnowledgeRelation,
  type MonthBucket,
  type OCRBlock,
  type PlaceCacheEntry,
  type PluginManifest,
  type PreviewCacheEntry,
  type PreviewCacheStats,
  type Principal,
  type ScanRun,
  type SearchResult,
  type SearchPlan,
  type SearchPlace,
  type SearchPlacesResponse,
  type SettingsPayload,
  type ReadinessPayload,
  type Stats,
  type StreamOptions,
  type TranscodeSession,
  type StorageConfig,
  type TileSource,
  type TrackPointInfo,
  type TrackDetail,
  type TrackProfile,
  type TrackSummary,
  type TranscriptRecord,
  type TranscodingCapabilities,
  type TranscodingPreset,
  type VideoTrackPlayerSession,
  type DBExport,
  type ReadOnlySQLResult
} from "./api";
import MonthFilterBar from "./components/MonthFilterBar.vue";
import PagedFileControls from "./components/PagedFileControls.vue";
import TypeaheadSearch, { type TypeaheadResult } from "./components/TypeaheadSearch.vue";

const nav = [
  "Explorer",
  "Discovery",
  "Jobs",
  "Metadata",
  "Storages",
  "Plugins",
  "Stats",
  "Duplicates",
  "Settings",
  "Albums",
  "Map",
  "GPS/KML Tracks",
  "Transcoding",
  "Base AI",
  "AI Classification",
  "OCR",
  "Transcripts",
  "Captions",
  "Face Gallery",
  "Safety Review",
  "Search",
  "Knowledge Base",
  "Knowledge Graph",
  "Geo Align",
  "Video Track Player"
];

const navPageAliases: Record<string, string> = {
  explorer: "Explorer",
  discovery: "Discovery",
  jobs: "Jobs",
  metadata: "Metadata",
  storages: "Storages",
  plugins: "Plugins",
  stats: "Stats",
  duplicates: "Duplicates",
  settings: "Settings",
  albums: "Albums",
  map: "Map",
  "gps-tracks": "GPS/KML Tracks",
  tracks: "GPS/KML Tracks",
  transcoding: "Transcoding",
  "base-ai": "Base AI",
  "asset-detail": "Asset Detail",
  ai: "Base AI",
  "ai-classification": "AI Classification",
  ocr: "OCR",
  transcripts: "Transcripts",
  transcript: "Transcripts",
  captions: "Captions",
  faces: "Face Gallery",
  "face-gallery": "Face Gallery",
  "safety-review": "Safety Review",
  search: "Search",
  "universal-search": "Search",
  "knowledge-base": "Knowledge Base",
  knowledge: "Knowledge Base",
  "knowledge-graph": "Knowledge Graph",
  graph: "Knowledge Graph",
  "geo-align": "Geo Align",
  "video-track-player": "Video Track Player"
};

const routeLabels = new Set([...nav, "Asset Detail"]);
const supportedDiscoveryExtensions = [
  "jpg", "jpeg", "png", "heif", "heic",
  "mp4", "mov", "webm", "mkv", "avi", "m4v",
  "gpx", "kml", "kmz", "gpz",
  "wav", "mp3", "3gp", "3gpp", "aac", "m4a", "flac", "ogg", "oga", "opus", "amr",
  "pdf", "djvu", "txt", "md", "markdown"
].join(",");

function pageFromQuery(): string | null {
  // Explicit ?page=... URLs must win over localStorage so shared/typed links open the requested page.
  const value = new URLSearchParams(window.location.search).get("page");
  if (value === null) return null;
  const mapped = navPageAliases[value.trim().toLowerCase()];
  return mapped ?? "Explorer";
}

function safeRoute(value: string | null | undefined): string {
  return value && routeLabels.has(value) ? value : "Explorer";
}

function pageSlug(label: string): string {
  if (label === "GPS/KML Tracks") return "gps-tracks";
  return label.toLowerCase().replaceAll(" ", "-");
}

function navIcon(label: string): string {
  const icons: Record<string, string> = {
    Explorer: "bi-folder2-open",
    Discovery: "bi-search",
    Jobs: "bi-list-task",
    Metadata: "bi-card-list",
    Storages: "bi-hdd-network",
    Plugins: "bi-puzzle",
    Stats: "bi-bar-chart",
    Duplicates: "bi-intersect",
    Settings: "bi-gear",
    Albums: "bi-collection",
    Map: "bi-map",
    "GPS/KML Tracks": "bi-signpost-split",
    Transcoding: "bi-film",
    "Base AI": "bi-cpu",
    "AI Classification": "bi-tags",
    OCR: "bi-body-text",
    Transcripts: "bi-file-earmark-text",
    Captions: "bi-chat-square-text",
    "Face Gallery": "bi-person-bounding-box",
    "Safety Review": "bi-shield-exclamation",
    Search: "bi-search-heart",
    "Knowledge Base": "bi-journal-richtext",
    "Knowledge Graph": "bi-diagram-3",
    "Geo Align": "bi-crosshair",
    "Video Track Player": "bi-play-btn"
  };
  return icons[label] ?? "bi-circle";
}

const active = ref(pageFromQuery() ?? safeRoute(localStorage.getItem("cartolensia.route")));
const loading = ref(false);
const error = ref("");
const rows = ref<ExplorerRow[]>([]);
const explorer = ref<ExplorerView | null>(null);
const explorerPageLimit = 200;
const explorerBulkPageLimit = 500;
const explorerLoadingMore = ref(false);
const explorerLoadingAll = ref(false);
const explorerPath = ref("");
const explorerQ = ref("");
const explorerMediaKind = ref("");
const explorerHashStatus = ref("");
const explorerExtension = ref("");
const explorerSort = ref("name");
const searchResults = ref<SearchResult[]>([]);
const searchWarnings = ref<string[]>([]);
const universalSearchQ = ref("");
const universalSearchResults = ref<SearchResult[]>([]);
const universalSearchTrackResults = ref<Array<{ track: TrackSummary; matched: string[]; explanation: string }>>([]);
const universalSearchPlaceResults = ref<SearchPlace[]>([]);
const universalSearchWarnings = ref<string[]>([]);
const universalSearchMessage = ref("");
const universalSearchBackend = ref("");
const universalSearchPlan = ref<SearchPlan | null>(null);
const naturalSearchQ = ref("");
const naturalSearchMessage = ref("");
const sqlSearchQ = ref("select asset_id, display_name, media_kind, extension, taken_at from cartolensia_search_assets where extension = 'mp4' order by taken_at desc nulls last");
const sqlSearchResult = ref<ReadOnlySQLResult | null>(null);
const sqlSearchMessage = ref("");
const knowledgeFacts = ref<KnowledgeFact[]>([]);
const knowledgeRelations = ref<KnowledgeRelation[]>([]);
const knowledgeQ = ref("");
const knowledgePredicate = ref("");
const knowledgeRelationFilter = ref("");
const knowledgeFactsTotal = ref(0);
const knowledgeRelationsTotal = ref(0);
const knowledgeLoading = ref(false);
const knowledgeMessage = ref("");
const knowledgeExtraction = ref<KnowledgeExtractionResult | null>(null);
const knowledgeChatInput = ref("");
const knowledgeChatConversationID = ref("");
const knowledgeChat = ref<KnowledgeChatResponse | null>(null);
const knowledgeChatBusy = ref(false);
const searchPlaceCache = ref<SearchPlacesResponse | null>(null);
const editablePlaces = ref<PlaceCacheEntry[]>([]);
const placeCacheQuery = ref("");
const placeCacheMessage = ref("");
const placeDraft = ref<PlaceCacheEntry>({
  name: "",
  display_name: "",
  provider: "local",
  country: "",
  region: "",
  city: "",
  road: "",
  aliases: [],
  lat: 40.1872,
  lon: 44.5152,
  bbox: { min_lon: 44.35, min_lat: 40.05, max_lon: 44.68, max_lat: 40.28 },
  source: "operator_cache"
});
const placeDraftAliases = ref("");
const assetDetail = ref<AssetDetail | null>(null);
const jobs = ref<Job[]>([]);
const jobStats = ref<JobStats | null>(null);
const selectedJob = ref<Job | null>(null);
const storages = ref<StorageConfig[]>([]);
const storageDraft = ref<StorageConfig>({ name: "", kind: "fs", root: "", mode: "strict_read_only", source_url: "", smb: {} });
const storageMessage = ref("");
const plugins = ref<PluginManifest[]>([]);
const stats = ref<Stats | null>(null);
const duplicatePage = ref<DuplicatePage | null>(null);
const backendMonthBuckets = ref<MonthBucket[]>([]);
const backend = ref<BackendStatus | null>(null);
const principal = ref<Principal | null>(null);
const publicAssets = ref<Asset[]>([]);
const loginEmail = ref("");
const loginPassword = ref("");
const oldPassword = ref("");
const newPassword = ref("");
const tokenName = ref("automation");
const tokenScopes = ref("read,jobs:write");
const tokenSecret = ref("");
const apiTokens = ref<APIToken[]>([]);
const settings = ref<SettingsPayload | null>(null);
const settingsTab = ref("general");
const settingsMessage = ref("");
const readiness = ref<ReadinessPayload | null>(null);
const pendingConfig = ref<Record<string, unknown>>({});
const components = ref<ComponentRecord[]>([]);
const componentRoot = ref("");
const componentCounts = ref<Record<string, number>>({});
const componentPathDrafts = ref<Record<string, string>>({});
const componentArchiveDrafts = ref<Record<string, string>>({});
const componentEvents = ref<Record<string, ComponentEvent[]>>({});
const componentMessage = ref("");
const componentBusyKey = ref("");
const pluginSettingText = ref<Record<string, string>>({});
const selectedPluginSettingsId = ref("");
const pluginSettingsMode = ref<"ui" | "yaml">("ui");
const dbExports = ref<DBExport[]>([]);
const dbExportMessage = ref("");
const tracks = ref<TrackSummary[]>([]);
const trackSearchQ = ref("");
const tracksLoadingMore = ref(false);
const tracksHasMore = ref(true);
const trackPageSize = 200;
const selectedTrack = ref<TrackDetail | null>(null);
const selectedTrackAltitude = ref<TrackProfile | null>(null);
const selectedTrackSpeed = ref<TrackProfile | null>(null);
const trackAssets = ref<AssetDetail["asset"][]>([]);
const trackAssetsReason = ref("");
const trackOffsetSeconds = ref(0);
const trackMediaViewMode = ref<"table" | "tile">("tile");
const mapData = ref<Record<string, unknown> | null>(null);
const mapStatus = ref<Record<string, unknown> | null>(null);
const mapMediaKind = ref("");
const mapCluster = ref(true);
const mapAlbumId = ref("");
const mapTrackId = ref("");
const mapPopup = ref<{
  kind: "cluster" | "asset" | "track";
  title: string;
  summary: string;
  assets: Array<{
    id: string;
    name: string;
    media_kind: string;
    preview_url?: string;
    detail_url?: string;
    original_url?: string;
  }>;
  bbox?: Record<string, number>;
  count?: number;
  track_id?: string;
  track_info?: TrackPointInfo | null;
  track_info_loading?: boolean;
  nearby_distance_m?: number;
} | null>(null);
const tileSources = ref<TileSource[]>([]);
const tileStatus = ref("OpenStreetMap tiles load on demand through Cartolensia.");
const mapElement = ref<HTMLDivElement | null>(null);
const mapPopupElement = ref<HTMLDivElement | null>(null);
const galleryTrackElement = ref<HTMLDivElement | null>(null);
const selectedTrackMapElement = ref<HTMLDivElement | null>(null);
const videoTrackMapElement = ref<HTMLDivElement | null>(null);
const geoAlignMapElement = ref<HTMLDivElement | null>(null);
const assetVideoElement = ref<HTMLVideoElement | null>(null);
const assetAudioElement = ref<HTMLAudioElement | null>(null);
const galleryVideoElement = ref<HTMLVideoElement | null>(null);
const galleryAudioElement = ref<HTMLAudioElement | null>(null);
const showMapDebug = ref(localStorage.getItem("cartolensia.map.showDebug") === "true");
const showMapLayerMenu = ref(false);
const showSelectedTrackLayerMenu = ref(false);
const showGalleryTrackLayerMenu = ref(false);
const showGeoAlignLayerMenu = ref(false);
const mapTilesVisible = ref(localStorage.getItem("cartolensia.map.tilesVisible") !== "false");
const mapTracksVisible = ref(localStorage.getItem("cartolensia.map.tracksVisible") !== "false");
const mapAssetsVisible = ref(localStorage.getItem("cartolensia.map.assetsVisible") !== "false");
const trackPreviewTilesEnabled = ref(true);
const selectedTrackLayerVisible = ref(true);
const galleryTrackLayerVisible = ref(true);
const geoAlignTilesVisible = ref(true);
const geoAlignTrackLayerVisible = ref(true);
const geoAlignMarkerLayerVisible = ref(true);
const selectedTrackPreviewStatus = ref("");
const galleryTrackPreviewStatus = ref("");
const selectedTrackFallbackPath = ref("");
const galleryTrackFallbackPath = ref("");
const selectedTrackPointInfo = ref<TrackPointInfo | null>(null);
const selectedTrackPointMessage = ref("");
const showSelectedTrackPointPopup = ref(false);
let olMap: OLMap | null = null;
let galleryTrackMap: OLMap | null = null;
const galleryTrackSource = new VectorSource();
let galleryTrackTileLayer: TileLayer<XYZ> | null = null;
let galleryTrackLayer: VectorLayer<VectorSource> | null = null;
let selectedTrackMap: OLMap | null = null;
const selectedTrackSource = new VectorSource();
let selectedTrackTileLayer: TileLayer<XYZ> | null = null;
let selectedTrackLayer: VectorLayer<VectorSource> | null = null;
let selectedTrackClickBound = false;
let geoAlignMap: OLMap | null = null;
let geoAlignTileLayer: TileLayer<XYZ> | null = null;
let geoAlignTrackLayer: VectorLayer<VectorSource> | null = null;
let geoAlignMarkerLayer: VectorLayer<VectorSource> | null = null;
let geoAlignDragState: { assetId: string; feature: Feature; lastLat: number; lastLon: number; moved: boolean } | null = null;
let geoAlignSuppressNextClick = false;
const geoAlignTrackSource = new VectorSource();
const geoAlignMarkerSource = new VectorSource();
let mapOverlay: Overlay | null = null;
type HlsInstance = import("hls.js").default;
type HlsConstructor = typeof import("hls.js").default;

let activeHls: HlsInstance | null = null;
let hlsConstructorPromise: Promise<HlsConstructor> | null = null;
let mapHasInitialFit = false;
const mapAssetSource = new VectorSource();
const mapTrackSource = new VectorSource();
let mapTileLayer: TileLayer<XYZ> | null = null;
let mapAssetLayer: VectorLayer<VectorSource> | null = null;
let mapTrackLayer: VectorLayer<VectorSource> | null = null;
let videoTrackMap: OLMap | null = null;
let videoTrackTileLayer: TileLayer<XYZ> | null = null;
let videoTrackTrackLayer: VectorLayer<VectorSource> | null = null;
let videoTrackMarkerLayer: VectorLayer<VectorSource> | null = null;
const videoTrackTrackSource = new VectorSource();
const videoTrackMarkerSource = new VectorSource();
let videoTrackMarkerFeature: Feature<Point> | null = null;
let videoTrackRafId = 0;
let videoTrackTicker: number | null = null;
let videoTrackMarkerThrottleAt = 0;
let videoTrackPositionPending = false;
const transcodingCapabilities = ref<TranscodingCapabilities | null>(null);
const transcodePresets = ref<TranscodingPreset[]>([]);
const transcodeMetricsPayload = ref<Record<string, unknown> | null>(null);
const transcodePageTab = ref("capabilities");
const transcodeRuleSourceCodec = ref("");
const transcodeRuleTargetPreset = ref("h264_low_bitrate");
const transcodeTemplate = ref("ffmpeg -i ${input} -c:v ${preset} -f hls ${output}");
const transcodePlannerMessage = ref("Plans write only to the configured Cartolensia transcode cache; originals are never replaced.");
const aiStatus = ref<Record<string, unknown> | null>(null);
const aiWorkers = ref<Record<string, unknown> | null>(null);
type AIJobKind = "classify" | "faces" | "describe" | "safety" | "embed" | "ocr" | "transcribe" | "audio-analyze";

const aiMessage = ref("");
const aiBusyKind = ref<AIJobKind | "">("");
const aiLastResult = ref<Record<string, unknown> | null>(null);
const aiActionHistory = ref<Array<{ id: string; kind: AIJobKind; status: string; summary: string; created_at: string }>>([]);
const assetAIActionStatus = ref<Record<string, { status: string; summary: string; job_id?: string }>>({});
const aiSummary = ref<Record<string, unknown> | null>(null);
const aiTagPayload = ref<Record<string, unknown> | null>(null);
const aiPredictionPayload = ref<Record<string, unknown> | null>(null);
const aiFacePayload = ref<Record<string, unknown> | null>(null);
const aiSafetyPayload = ref<Record<string, unknown> | null>(null);
const aiVectorQuery = ref("brick path");
const aiVectorResults = ref<Record<string, unknown>[]>([]);
const ocrPageQuery = ref("");
const captionsPageQuery = ref("");
const aiPredictionLimit = ref(500);
const aiFaceLimit = ref(500);
const faceClusterLimit = ref(200);
const transcriptRows = ref<TranscriptRecord[]>([]);
const transcriptQuery = ref("");
const transcriptLoading = ref(false);
const transcriptsHasMore = ref(true);
const transcriptSearchResults = ref<SearchResult[]>([]);
const transcriptSearchMessage = ref("");
const vectorConfigHighlight = ref(false);
const vectorStatus = ref<Record<string, unknown> | null>(null);
const faceClustersPayload = ref<{ clusters: FaceCluster[]; total: number; provisional_note?: string } | null>(null);
const selectedFaceCluster = ref<FaceCluster | null>(null);
const faceClusterAssets = ref<Asset[]>([]);
const faceClusterDetections = ref<FaceDetection[]>([]);
const faceGalleryMessage = ref("");
const faceClusterNameDraft = ref("");
const faceSearchQ = ref("");
const showAssetFaceBoxes = ref(localStorage.getItem("cartolensia.asset.showFaceBoxes") !== "false");
const showAssetOCRBoxes = ref(localStorage.getItem("cartolensia.asset.showOCRBoxes") !== "false");
const selectedAssetFaceId = ref("");
const selectedAssetOCRId = ref("");
const faceAddMode = ref(false);
const newFaceName = ref("");
const newFaceConfidence = ref(1);
const newFaceDraftBox = ref<{ x: number; y: number; width: number; height: number } | null>(null);
let faceDraftStart: { x: number; y: number } | null = null;
const geoAlignSession = ref<GeoAlignSession | null>(null);
const geoAlignMessage = ref("");
const geoAlignTrackIds = ref("");
const geoAlignPopupMarker = ref<GeoAlignMarker | null>(null);
const geoAlignPopupTrackInfo = ref<TrackPointInfo | null>(null);
const geoAlignPopupTrackMessage = ref("");
const geoAlignDragActive = ref(false);
const videoTrackSession = ref<VideoTrackPlayerSession | null>(null);
const videoTrackMessage = ref("");
const videoTrackAssetId = ref("");
const videoTrackIds = ref("");
const videoTrackOffsetSeconds = ref(0);
const videoTrackTimestampMode = ref("video_start_time");
const videoTrackPosition = ref<Record<string, unknown> | null>(null);
const videoTrackSyncTimeMs = ref(0);
const videoTrackVideoSearch = ref("");
const videoTrackTrackSearch = ref("");
const videoTrackVideoOptions = ref<Asset[]>([]);
const videoTrackTrackOptions = ref<TrackSummary[]>([]);
const videoTrackSelectedTracks = ref<TrackSummary[]>([]);
const videoTrackCurrentPosition = computed(() => {
  const positions = asArray(videoTrackPosition.value?.positions as unknown[]);
  return (positions[0] as Record<string, unknown> | undefined) ?? null;
});
const assetDetailSeekMs = ref<number | null>(null);
const searchPageTotal = ref(0);
const filePickerOpen = ref(false);
const filePicker = ref<FileBrowseResponse | null>(null);
const filePickerRoot = ref("");
const filePickerPath = ref("");
const filePickerKind = ref<"file" | "folder">("folder");
const filePickerTarget = ref("");
const filePickerMessage = ref("");
const albums = ref<Album[]>([]);
const selectedAlbumId = ref("");
const albumItems = ref<AlbumItemPage | null>(null);
const newAlbumTitle = ref("");
const newAlbumDescription = ref("");
const selectedAssets = ref<Set<string>>(new Set());
const explorerViewMode = ref<"table" | "tile">("tile");
const albumViewMode = ref<"table" | "tile">("tile");
const monthFilter = ref("");
const previewCacheStats = ref<PreviewCacheStats | null>(null);
const previewCache = ref<PreviewCacheEntry[]>([]);
const jobMaxFiles = ref(-1);
const dryRunStorage = ref("");
const dryRunPrefix = ref("Cartolensia-photos");
const dryRunMaxFiles = ref(-1);
const dryRunMaxBytes = ref(-1);
const dryRunExtensions = ref(supportedDiscoveryExtensions);
const hashAfterIndex = ref(true);
const metadataAfterIndex = ref(true);
const previewsAfterIndex = ref(true);
const pipelineIndexFiles = ref(true);
const pipelineParseTracks = ref(true);
const pipelineGeotagExif = ref(true);
const pipelineSnapToTracks = ref(true);
const pipelineRefreshMap = ref(true);
const pipelineRunning = ref(false);
const currentPipelineId = ref("");
const currentPipelineJobId = ref("");
const indexingStatus = ref<IndexingStatus | null>(null);
const pipelineStage = ref("");
const pipelineLog = ref<string[]>([]);
const lastDiscoveryJob = ref<Job | null>(null);
const lastHashJob = ref<Job | null>(null);
const lastMetadataJob = ref<Job | null>(null);
const lastPreviewJob = ref<Job | null>(null);
const dryRunReport = ref<ScanRun | null>(null);
const streamOptions = ref<StreamOptions | null>(null);
const transcodeSession = ref<TranscodeSession | null>(null);
const transcodeMessage = ref("");
const transcodeValidation = ref<Record<string, unknown> | null>(null);
const showAdvancedTranscode = ref(false);
const advancedTranscodeAssetId = ref("");
const customPresetName = ref("Custom LAN preset");
const customPresetHardware = ref("cpu");
const customPresetCodec = ref("h264");
const customPresetEncoder = ref("");
const customPresetMode = ref("quality");
const customPresetParameter = ref("28");
const lastVideoPreset = ref(localStorage.getItem("cartolensia.videoPreset") || "original");
const galleryZoomMode = ref<"fit" | "actual">("fit");
const galleryScale = ref(1);
const galleryPanX = ref(0);
const galleryPanY = ref(0);
const galleryPanning = ref(false);
let galleryPointerLast: { x: number; y: number } | null = null;
const galleryPointers = new Map<number, { x: number; y: number }>();
let galleryPinchStartDistance = 0;
let galleryPinchStartScale = 1;

type GalleryItem = {
  id: string;
  name: string;
  media_kind: string;
  relative_path?: string;
  date?: string;
  size_bytes?: number;
  preview_url: string;
  original_url: string;
};

const galleryItems = ref<GalleryItem[]>([]);
const galleryIndex = ref(0);
const assetRelated = ref<AssetRelated | null>(null);
const failedPreviewIds = ref<Set<string>>(new Set());
const galleryOpen = computed(() => galleryItems.value.length > 0);
const galleryCurrent = computed(() => galleryItems.value[galleryIndex.value] ?? null);
const relatedContextGroups = computed(() =>
  Object.entries(assetRelated.value?.groups ?? {}).filter(([, rows]) => rows.length > 0)
);

const activePlugin = computed(() => {
  const id = active.value.toLowerCase().replaceAll(" ", "-");
  return plugins.value.find((plugin) => plugin.id === id || plugin.name.toLowerCase() === active.value.toLowerCase());
});

const selectedPluginSettings = computed(() => plugins.value.find((plugin) => plugin.id === selectedPluginSettingsId.value) ?? plugins.value[0]);

const breadcrumbs = computed(() => {
  const parts = explorerPath.value.split("/").filter(Boolean);
  const crumbs = [{ name: "Root", path: "" }];
  let current = "";
  for (const part of parts) {
    current = current ? `${current}/${part}` : part;
    crumbs.push({ name: part, path: current });
  }
  return crumbs;
});

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function setActive(next: string, updateURL = true) {
  const route = safeRoute(next);
  active.value = route;
  if (route === "Transcripts" && transcriptRows.value.length === 0 && !transcriptLoading.value) {
    void fetchTranscriptsPage(true);
  }
  if ((route === "Knowledge Base" || route === "Knowledge Graph") && knowledgeFacts.value.length === 0 && knowledgeRelations.value.length === 0) {
    void loadKnowledgeBase();
  }
  if (route !== "Asset Detail") {
    localStorage.setItem("cartolensia.route", route);
  }
  if (updateURL && route !== "Asset Detail") {
    const url = new URL(window.location.href);
    url.searchParams.set("page", pageSlug(route));
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
  }
}

function assetHref(id: string) {
  const url = new URL(window.location.href);
  url.searchParams.set("page", "asset-detail");
  url.searchParams.set("asset_id", id);
  return `${url.pathname}${url.search}${url.hash}`;
}

function firstAssetLocation(asset: Asset) {
  return asset.locations?.[0];
}

function assetName(asset: Asset) {
  return asset.display_name || firstAssetLocation(asset)?.file_name || asset.id;
}

function assetTimestampLabel(asset: Asset) {
  return (
    asset.taken_at ||
    String(asset.metadata?.exif_datetime_original_raw ?? "") ||
    firstAssetLocation(asset)?.mtime ||
    ""
  );
}

function groupLabel(group: string) {
  return group
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function captureCurrentPlaybackTimeMs(): number | null {
  const currentGallery = galleryCurrent.value;
  const galleryMedia =
    currentGallery?.media_kind === "video"
      ? galleryVideoElement.value
      : currentGallery?.media_kind === "audio"
        ? galleryAudioElement.value
        : null;
  const media = galleryMedia ?? assetVideoElement.value ?? assetAudioElement.value;
  if (!media || !Number.isFinite(media.currentTime)) return null;
  return Math.max(0, Math.round(media.currentTime * 1000));
}

function openAssetLink(event: MouseEvent, id: string, options: { closeOverlay?: boolean; preservePlayback?: boolean } = {}) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
    return;
  }
  event.preventDefault();
  const seekMs = options.preservePlayback ? captureCurrentPlaybackTimeMs() : null;
  if (options.closeOverlay) {
    closeGallery();
  }
  void openAsset(id, { seekMs });
}

window.addEventListener("popstate", () => {
  const page = pageFromQuery() ?? safeRoute(localStorage.getItem("cartolensia.route"));
  active.value = page;
  const assetID = new URLSearchParams(window.location.search).get("asset_id");
  if (page === "Asset Detail" && assetID) {
    void (principal.value ? openAsset(assetID) : openPublicAsset(assetID));
  }
});

const visibleExplorerRows = computed(() => {
	const files = asArray(explorer.value?.files);
	const searchRows = searchResults.value.map((result) => assetToExplorerRow(result.asset)).filter((row): row is ExplorerRow => row !== null);
	const base = explorerQ.value.trim() ? searchRows : files;
	if (!monthFilter.value) return base;
	return base.filter((row) => monthKey(row.mtime) === monthFilter.value);
});

const explorerTotalFiles = computed(() => explorer.value?.file_count ?? visibleExplorerRows.value.length);
const explorerLoadedFiles = computed(() => asArray(explorer.value?.files).length);
const explorerHasMoreFiles = computed(() => !explorerQ.value.trim() && explorerLoadedFiles.value < explorerTotalFiles.value);

const searchExplanationByAsset = computed(() => {
	const explanations = new Map<string, string>();
	for (const result of searchResults.value) {
		explanations.set(result.asset.id, result.explanation);
	}
	return explanations;
});

const aiWorkerRows = computed(() => asArray((aiWorkers.value?.workers as unknown[]) ?? []));
const configuredAIWorker = computed(() => {
  return aiWorkerRows.value.find((worker) => Boolean((worker as Record<string, unknown>).configured)) as Record<string, unknown> | undefined;
});
const aiHealth = computed(() => (configuredAIWorker.value?.health ?? {}) as Record<string, unknown>);
const aiCapabilities = computed(() => ((aiHealth.value.capabilities ?? {}) as Record<string, unknown>));
const aiModelStates = computed(() => ((aiCapabilities.value.models ?? {}) as Record<string, Record<string, unknown>>));
const aiCounts = computed(() => ((aiStatus.value?.ai_counts ?? {}) as Record<string, number>));
const aiDevicePolicy = computed(() => ((aiStatus.value?.device_policy ?? {}) as Record<string, unknown>));
const aiNativeWorker = computed(() => ((aiStatus.value?.native_worker ?? {}) as Record<string, unknown>));
const nativeCudaAvailable = computed(() => Boolean(aiDevicePolicy.value.native_cuda_available ?? aiNativeWorker.value.cuda));
const dockerNvidiaRuntime = computed(() => Boolean(aiDevicePolicy.value.docker_nvidia_runtime ?? ((aiStatus.value?.accelerator_hints as Record<string, unknown> | undefined)?.docker_nvidia_runtime)));
const vectorLimits = computed(() => ((vectorStatus.value?.limits ?? {}) as Record<string, unknown>));
const recentAIJobs = computed(() => jobs.value.filter((job) => job.kind.startsWith("ai_")).slice(0, 6));
const activeOrQueuedJobs = computed(() =>
  jobs.value.filter((job) => ["running", "cancel_requested", "queued"].includes(job.status))
);
const recentHistoryJobs = computed(() =>
  jobs.value.filter((job) => !["running", "cancel_requested", "queued"].includes(job.status)).slice(0, 80)
);
const transcodeHardwareStatus = computed(() => {
  const hardware = transcodingCapabilities.value?.hardware ?? {};
  return [
    { label: "CPU", value: true, note: "always available fallback" },
    { label: "NVIDIA NVENC", value: Boolean(hardware.nvidia_smi), note: "requires driver and nvenc encoder" },
    { label: "VAAPI", value: Boolean(hardware.vaapi), note: "requires /dev/dri render device" },
    { label: "Intel QSV", value: Boolean(hardware.qsv), note: "requires /dev/dri render device" }
  ];
});
const transcodeMetricsStatus = computed(() => {
  const filters = (transcodeMetricsPayload.value?.filters ?? {}) as Record<string, unknown>;
  const notes = (transcodeMetricsPayload.value?.notes ?? {}) as Record<string, unknown>;
  return [
    { metric: "SSIM", available: Boolean(filters.ssim), note: String(notes.ssim ?? "ffmpeg ssim filter") },
    { metric: "PSNR", available: Boolean(filters.psnr), note: String(notes.psnr ?? "ffmpeg psnr filter") },
    { metric: "VMAF", available: Boolean(filters.libvmaf), note: String(notes.libvmaf ?? "requires ffmpeg built with libvmaf") }
  ];
});

async function fetchVisibleJobs(): Promise<Job[]> {
  const [runningRows, queuedRows, recentRows] = await Promise.all([
    api.jobs("running_only=true&limit=100"),
    api.jobs("status=queued&sort=created_at&limit=100"),
    api.jobs("limit=200")
  ]);
  const byID = new Map<string, Job>();
  for (const job of [...asArray(runningRows), ...asArray(queuedRows), ...asArray(recentRows)]) {
    byID.set(job.id, job);
  }
  return Array.from(byID.values()).sort((a, b) => {
    const priority = (status: string) => {
      if (status === "running") return 0;
      if (status === "cancel_requested") return 1;
      if (status === "queued") return 2;
      if (status === "failed") return 3;
      if (status === "cancelled") return 4;
      return 5;
    };
    const left = priority(a.status);
    const right = priority(b.status);
    if (left !== right) return left - right;
    const aTime = Date.parse(a.created_at ?? "") || 0;
    const bTime = Date.parse(b.created_at ?? "") || 0;
    if (left <= 2) return aTime - bTime;
    return bTime - aTime;
  });
}

async function fetchTracksPage(reset = false) {
  if (tracksLoadingMore.value) return;
  tracksLoadingMore.value = true;
  try {
    const offset = reset ? 0 : tracks.value.length;
    const rows = await api.gpsTracks({
      limit: trackPageSize,
      offset,
      q: trackSearchQ.value.trim(),
      sort: "time_desc"
    });
    if (reset) {
      tracks.value = rows;
    } else {
      const seen = new Set(tracks.value.map((track) => track.track_asset_id));
      tracks.value = [...tracks.value, ...rows.filter((track) => !seen.has(track.track_asset_id))];
    }
    tracksHasMore.value = rows.length >= trackPageSize;
    videoTrackTrackOptions.value = tracks.value;
  } finally {
    tracksLoadingMore.value = false;
  }
}

async function loadAllTracks() {
  let guard = 0;
  while (tracksHasMore.value && guard < 100) {
    guard += 1;
    await fetchTracksPage(false);
  }
}

function selectTrackSearchResult(item: TypeaheadResult) {
  const track = tracks.value.find((candidate) => candidate.track_asset_id === item.key);
  if (track) {
    trackSearchQ.value = track.name;
    void openTrack(track.track_asset_id);
  }
}

async function goToTrackSearchSelection() {
  if (trackTypeaheadResults.value.length > 0) {
    selectTrackSearchResult(trackTypeaheadResults.value[0]);
    return;
  }
  await fetchTracksPage(true);
}

async function fetchTranscriptsPage(reset = false) {
  if (transcriptLoading.value) return;
  transcriptLoading.value = true;
  try {
    const offset = reset ? 0 : transcriptRows.value.length;
    const page = await api.transcripts(200, offset);
    if (reset) {
      transcriptRows.value = page.transcripts;
    } else {
      const seen = new Set(transcriptRows.value.map((transcript) => transcript.id));
      transcriptRows.value = [
        ...transcriptRows.value,
        ...page.transcripts.filter((transcript) => !seen.has(transcript.id))
      ];
    }
    transcriptsHasMore.value = page.transcripts.length >= page.limit;
  } finally {
    transcriptLoading.value = false;
  }
}

async function loadAllTranscripts() {
  let guard = 0;
  while (transcriptsHasMore.value && guard < 100) {
    guard += 1;
    await fetchTranscriptsPage(false);
  }
}

async function searchTranscripts() {
  const query = transcriptQuery.value.trim();
  if (!query) {
    transcriptSearchResults.value = [];
    transcriptSearchMessage.value = "";
    return;
  }
  const response = await api.search(`transcript:${query}`, 100);
  transcriptSearchResults.value = asArray(response.results);
  transcriptSearchMessage.value = `${response.page.total} transcript search matches`;
}

function selectTranscriptSearchResult(item: TypeaheadResult) {
  const transcript = transcriptRows.value.find((candidate) => candidate.id === item.key);
  if (!transcript) return;
  void openAsset(transcript.asset_id);
}

async function refreshAILists() {
  const predictionTask = activePredictionTaskFilter();
  const predictionQuery = activePredictionQueryFilter();
  const [predictions, faces] = await Promise.all([
    api.aiPredictions(aiPredictionLimit.value, 0, predictionQuery, predictionTask),
    api.aiFaces(aiFaceLimit.value, 0, "")
  ]);
  aiPredictionPayload.value = predictions;
  aiFacePayload.value = faces;
}

async function loadMoreAILists(kind: "predictions" | "faces" | "all" = "all") {
  if (kind === "predictions" || kind === "all") {
    aiPredictionLimit.value = Math.min(5000, aiPredictionLimit.value + 500);
  }
  if (kind === "faces" || kind === "all") {
    aiFaceLimit.value = Math.min(5000, aiFaceLimit.value + 500);
  }
  await refreshAILists();
}

async function loadAllAILists(kind: "predictions" | "faces" | "all" = "all") {
  if (kind === "predictions" || kind === "all") {
    aiPredictionLimit.value = Math.min(5000, Math.max(aiPredictionLimit.value, aiPredictionTotal.value || 5000));
  }
  if (kind === "faces" || kind === "all") {
    aiFaceLimit.value = Math.min(5000, Math.max(aiFaceLimit.value, aiFaceTotal.value || 5000));
  }
  await refreshAILists();
}
const transcodeTemplateSafe = computed(() => {
  const stripped = transcodeTemplate.value
    .replaceAll("${input}", "")
    .replaceAll("${output}", "")
    .replaceAll("${workdir}", "")
    .replaceAll("${preset}", "")
    .replaceAll("${width}", "")
    .replaceAll("${height}", "")
    .replaceAll("${fps}", "")
    .replaceAll("${source_codec}", "");
  return !/[;&|`]/.test(stripped);
});
const aiModelCards = computed(() => [
  { key: "classifier", label: "Classifier", action: "classify", model: aiModelStates.value.classifier },
  { key: "face_detector", label: "Face Detector", action: "faces", model: aiModelStates.value.face_detector },
  { key: "safety", label: "NSFW/Safety", action: "safety", model: aiModelStates.value.safety },
  { key: "openclip", label: "Embeddings", action: "embed", model: aiModelStates.value.openclip },
  { key: "caption", label: "Captioning", action: "describe", model: aiModelStates.value.caption },
  { key: "ocr", label: "OCR Text", action: "ocr", model: aiModelStates.value.ocr ?? { available: false, name: "Tesseract sidecar contract" } }
]);
const aiTags = computed(() => asArray((aiTagPayload.value?.tags as unknown[]) ?? []));
const aiPredictions = computed(() => asArray((aiPredictionPayload.value?.predictions as unknown[]) ?? []));
const aiPredictionTotal = computed(() => Number(aiPredictionPayload.value?.total_predictions ?? aiPredictions.value.length));
const aiFaceTotal = computed(() => Number(aiFacePayload.value?.total ?? aiFaces.value.length));
const aiTagTotal = computed(() => Number(aiPredictionPayload.value?.total_tags ?? aiTags.value.length));
const ocrPredictionRows = computed(() => {
  const query = ocrPageQuery.value.trim().toLowerCase();
  return aiPredictions.value.filter((raw) => {
    const row = raw as Record<string, unknown>;
    const task = String(row.task ?? "");
    if (!["ocr_image", "ocr", "ocr_text"].includes(task)) return false;
    if (!query) return true;
    return [row.label, row.asset_id, row.model_name, (row.metadata as Record<string, unknown> | undefined)?.language]
      .map((value) => String(value ?? "").toLowerCase())
      .some((value) => value.includes(query));
  });
});
const captionPredictionRows = computed(() => {
  const query = captionsPageQuery.value.trim().toLowerCase();
  return aiPredictions.value.filter((raw) => {
    const row = raw as Record<string, unknown>;
    const task = String(row.task ?? "");
    const isCaption = ["describe_image", "caption", "caption_short", "caption_long"].includes(task) || String(row.label ?? "").startsWith("caption:");
    if (!isCaption) return false;
    if (!query) return true;
    return [row.label, row.asset_id, row.model_name, task]
      .map((value) => String(value ?? "").toLowerCase())
      .some((value) => value.includes(query));
  });
});

function activePredictionTaskFilter(): string {
  if (active.value === "OCR") return "ocr_image,ocr,ocr_text";
  if (active.value === "Captions") return "describe_image,caption,caption_short,caption_long";
  return "";
}

function activePredictionQueryFilter(): string {
  if (active.value === "OCR") return ocrPageQuery.value.trim();
  if (active.value === "Captions") return captionsPageQuery.value.trim();
  return "";
}

function recordAssetID(row: unknown): string {
  return String((row as Record<string, unknown> | null | undefined)?.asset_id ?? "").trim();
}

function fuzzyIncludes(value: unknown, query: string): boolean {
  return String(value ?? "").toLowerCase().includes(query);
}

function trackMatchesQuery(track: TrackSummary, query: string): boolean {
  return [
    track.name,
    track.track_asset_id,
    track.source_format,
    track.start_time,
    track.end_time,
    track.distance_m ? `${(track.distance_m / 1000).toFixed(2)} km` : "",
    track.point_count
  ].some((value) => fuzzyIncludes(value, query));
}

function transcriptMatchesQuery(transcript: TranscriptRecord, query: string): boolean {
  return [
    transcript.id,
    transcript.asset_id,
    transcript.source_kind,
    transcript.language,
    transcript.model,
    transcript.full_text,
    transcript.created_at
  ].some((value) => fuzzyIncludes(value, query));
}

const aiFaces = computed(() => asArray((aiFacePayload.value?.faces as unknown[]) ?? []));
const aiSafetyCandidates = computed(() => asArray((aiSafetyPayload.value?.candidates as unknown[]) ?? []));
const faceClusters = computed(() => asArray(faceClustersPayload.value?.clusters));
const filteredFaceClusters = computed(() => {
  const query = faceSearchQ.value.trim().toLowerCase();
  if (!query) return faceClusters.value;
  return faceClusters.value.filter((cluster) => {
    const metadata = cluster.metadata ?? {};
    const text = [
      cluster.label,
      cluster.id,
      metadata.representative_asset_name,
      metadata.representative_asset_id,
      metadata.provisional ? "provisional unassigned" : ""
    ].join(" ").toLowerCase();
    return text.includes(query);
  });
});
const visibleFaceClusters = computed(() => filteredFaceClusters.value.slice(0, faceClusterLimit.value));
const trackTypeaheadResults = computed<TypeaheadResult[]>(() => {
  const query = trackSearchQ.value.trim().toLowerCase();
  if (query.length < 2) return [];
  return tracks.value
    .filter((track) => trackMatchesQuery(track, query))
    .slice(0, 20)
    .map((track) => ({
      key: track.track_asset_id,
      type: track.source_format ?? "track",
      name: track.name,
      detail: [
        track.start_time ? track.start_time.slice(0, 19) : "",
        track.end_time ? `-> ${track.end_time.slice(0, 19)}` : "",
        track.distance_m ? `${(track.distance_m / 1000).toFixed(2)} km` : "",
        `${track.point_count} points`
      ].filter(Boolean).join(" ")
    }));
});
const transcriptTypeaheadResults = computed<TypeaheadResult[]>(() => {
  const query = transcriptQuery.value.trim().toLowerCase();
  if (query.length < 2) return [];
  return transcriptRows.value
    .filter((transcript) => transcriptMatchesQuery(transcript, query))
    .slice(0, 20)
    .map((transcript) => ({
      key: transcript.id,
      type: transcript.source_kind || "transcript",
      name: transcript.full_text.slice(0, 120) || transcript.asset_id,
      originalName: transcript.language,
      detail: `${transcript.asset_id.slice(0, 8)} · ${transcript.model ?? "model unknown"} · ${transcript.created_at ?? ""}`
    }));
});
const visibleTranscriptRows = computed(() => {
  const query = transcriptQuery.value.trim().toLowerCase();
  if (!query) return transcriptRows.value;
  return transcriptRows.value.filter((transcript) => transcriptMatchesQuery(transcript, query));
});
const visibleAssetFaces = computed(() => {
  const faces = asArray(assetDetail.value?.face_detections as unknown[]) as FaceDetection[];
  return faces.filter((face) => !faceIgnored(face));
});
const filePickerRoots = computed(() => Object.values(filePicker.value?.roots ?? {}));
const componentCategories = computed(() => {
  const labels: Record<string, string> = {
    tool: "Media tools",
    metric: "Metrics",
    ocr: "OCR",
    python: "AI runtime",
    model: "AI models",
    database: "Database"
  };
  const groups = new Map<string, { key: string; label: string; items: ComponentRecord[] }>();
  for (const component of components.value) {
    const key = component.category || "other";
    if (!groups.has(key)) groups.set(key, { key, label: labels[key] ?? key, items: [] });
    groups.get(key)?.items.push(component);
  }
  return Array.from(groups.values()).map((group) => ({
    ...group,
    items: group.items.slice().sort((a, b) => a.key.localeCompare(b.key))
  }));
});

const selectedAlbumItems = computed(() => asArray(albumItems.value?.items));
const selectedAlbumAssets = computed(() => selectedAlbumItems.value.map((item) => item.asset));
const indexedVideoRows = computed(() => rows.value.filter((row) => row.media_kind === "video"));

const monthBuckets = computed(() => {
  if (backendMonthBuckets.value.length > 0) {
    return backendMonthBuckets.value.map((bucket) => ({ month: bucket.month, count: bucket.count }));
  }
  const counts: Record<string, number> = {};
  for (const row of rows.value) {
    const key = monthKey(row.mtime);
    if (key) counts[key] = (counts[key] ?? 0) + 1;
  }
  return Object.entries(counts)
    .sort(([a], [b]) => b.localeCompare(a))
    .map(([month, count]) => ({ month, count }));
});

function monthKey(value?: string): string {
  return value && value.length >= 7 ? value.slice(0, 7) : "";
}

function explorerQueryString(extra: Record<string, string | number> = {}): string {
	const params = new URLSearchParams();
  if (explorerQ.value.trim()) params.set("q", explorerQ.value.trim());
  if (explorerMediaKind.value) params.set("media_kind", explorerMediaKind.value);
  if (explorerHashStatus.value) params.set("hash_status", explorerHashStatus.value);
  if (explorerExtension.value.trim()) params.set("extension", explorerExtension.value.trim());
  if (explorerSort.value) params.set("sort", explorerSort.value);
  for (const [key, value] of Object.entries(extra)) {
    params.set(key, String(value));
  }
	return params.toString();
}

function assetToExplorerRow(asset: Asset): ExplorerRow | null {
	const location = asArray(asset.locations)[0];
	if (!location) return null;
	return {
		asset_id: asset.id,
		name: asset.display_name,
		media_kind: asset.media_kind,
		storage_url: location.storage_url,
		relative_path: location.relative_path,
		size_bytes: location.size_bytes,
		mtime: asset.taken_at ?? location.mtime,
		hash_status: location.hash_status,
		sha512_hex: location.sha512_hex
	};
}

function searchExplanation(row: ExplorerRow): string {
	return searchExplanationByAsset.value.get(row.asset_id) ?? "";
}

function rowToGallery(row: ExplorerRow): GalleryItem {
  const mediaEndpoint = row.media_kind === "track" ? "track-thumbnail" : "preview";
  return {
    id: row.asset_id,
    name: row.name,
    media_kind: row.media_kind,
    relative_path: row.relative_path,
    date: row.mtime,
    size_bytes: row.size_bytes,
    preview_url: `/api/v1/media/${encodeURIComponent(row.asset_id)}/${mediaEndpoint}`,
    original_url: `/api/v1/media/${encodeURIComponent(row.asset_id)}/original`
  };
}

function assetToGallery(asset: AssetDetail["asset"]): GalleryItem {
  const location = asArray(asset.locations)[0];
  const mediaEndpoint = asset.media_kind === "track" ? "track-thumbnail" : "preview";
  return {
    id: asset.id,
    name: asset.display_name,
    media_kind: asset.media_kind,
    relative_path: location?.relative_path,
    date: asset.taken_at ?? location?.mtime,
    size_bytes: location?.size_bytes,
    preview_url: `/api/v1/media/${encodeURIComponent(asset.id)}/${mediaEndpoint}`,
    original_url: `/api/v1/media/${encodeURIComponent(asset.id)}/original`
  };
}

function explorerGalleryItems(): GalleryItem[] {
  return visibleExplorerRows.value.map(rowToGallery);
}

function openGallery(items: GalleryItem[], index: number) {
  galleryItems.value = items;
  galleryIndex.value = Math.max(0, Math.min(index, items.length - 1));
  resetGalleryZoom();
  void refreshStreamOptionsForGallery();
  void refreshGalleryTrackPreview();
}

function closeGallery() {
  closeAdvancedTranscode();
  if (transcodeSession.value) void stopActiveTranscode();
  destroyGalleryTrackMap();
  galleryItems.value = [];
  galleryIndex.value = 0;
}

function nextGallery(delta: number) {
  if (!galleryItems.value.length) return;
  if (transcodeSession.value) void stopActiveTranscode();
  galleryIndex.value = (galleryIndex.value + delta + galleryItems.value.length) % galleryItems.value.length;
  resetGalleryZoom();
  void refreshStreamOptionsForGallery();
  void refreshGalleryTrackPreview();
}

function resetGalleryZoom() {
  galleryZoomMode.value = "fit";
  galleryScale.value = 1;
  galleryPanX.value = 0;
  galleryPanY.value = 0;
  galleryPanning.value = false;
  galleryPointerLast = null;
  galleryPointers.clear();
  galleryPinchStartDistance = 0;
  galleryPinchStartScale = 1;
}

function toggleGalleryZoom() {
  if (galleryZoomMode.value === "fit") {
    galleryZoomMode.value = "actual";
    galleryScale.value = 1;
    galleryPanX.value = 0;
    galleryPanY.value = 0;
    return;
  }
  resetGalleryZoom();
}

function galleryImageStyle() {
  if (galleryZoomMode.value !== "actual") return {};
  return {
    transform: `translate(${galleryPanX.value}px, ${galleryPanY.value}px) scale(${galleryScale.value})`
  };
}

function wheelGallery(event: WheelEvent) {
  if (galleryCurrent.value?.media_kind !== "photo") return;
  galleryZoomMode.value = "actual";
  const previous = galleryScale.value;
  const next = Math.max(0.25, Math.min(6, previous * (event.deltaY < 0 ? 1.12 : 0.88)));
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  const dx = event.clientX - (rect.left + rect.width / 2);
  const dy = event.clientY - (rect.top + rect.height / 2);
  const ratio = next / previous;
  galleryPanX.value = dx - (dx - galleryPanX.value) * ratio;
  galleryPanY.value = dy - (dy - galleryPanY.value) * ratio;
  galleryScale.value = next;
}

function panGallery(dx: number, dy: number) {
  if (galleryZoomMode.value !== "actual") return;
  galleryPanX.value += dx;
  galleryPanY.value += dy;
}

function galleryPointerDown(event: PointerEvent) {
  if (galleryCurrent.value?.media_kind !== "photo") return;
  galleryZoomMode.value = "actual";
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  galleryPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
  galleryPointerLast = { x: event.clientX, y: event.clientY };
  galleryPanning.value = true;
  if (galleryPointers.size === 2) {
    const points = Array.from(galleryPointers.values());
    galleryPinchStartDistance = Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
    galleryPinchStartScale = galleryScale.value;
  }
}

function galleryPointerMove(event: PointerEvent) {
  if (galleryCurrent.value?.media_kind !== "photo" || !galleryPointers.has(event.pointerId)) return;
  galleryPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
  if (galleryPointers.size >= 2) {
    const points = Array.from(galleryPointers.values()).slice(0, 2);
    const distance = Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
    if (galleryPinchStartDistance > 0) {
      galleryScale.value = Math.max(0.25, Math.min(6, galleryPinchStartScale * (distance / galleryPinchStartDistance)));
    }
    return;
  }
  if (!galleryPointerLast) {
    galleryPointerLast = { x: event.clientX, y: event.clientY };
    return;
  }
  panGallery(event.clientX - galleryPointerLast.x, event.clientY - galleryPointerLast.y);
  galleryPointerLast = { x: event.clientX, y: event.clientY };
}

function galleryPointerUp(event: PointerEvent) {
  galleryPointers.delete(event.pointerId);
  if (galleryPointers.size === 0) {
    galleryPanning.value = false;
    galleryPointerLast = null;
    galleryPinchStartDistance = 0;
  } else {
    const points = Array.from(galleryPointers.values());
    galleryPointerLast = points[0] ?? null;
  }
}

function createLocalOSMLayer(onError: () => void): TileLayer<XYZ> {
  const layer = new TileLayer({
    source: new XYZ({
      url: "/api/v1/tiles/osm/{z}/{x}/{y}.png",
      maxZoom: 19,
      attributions: "© OpenStreetMap contributors"
    })
  });
  layer.getSource()?.on("tileloaderror", onError);
  return layer;
}

function numericRuntimeSetting(key: string, fallback: number): number {
  const value = settings.value?.runtime_settings?.[key];
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function runtimeTextSetting(key: string, fallback: string): string {
  const value = settings.value?.runtime_settings?.[key];
  if (value === undefined || value === null) return fallback;
  const text = String(value).trim();
  return text || fallback;
}

function boolRuntimeSetting(key: string, fallback: boolean): boolean {
  const value = settings.value?.runtime_settings?.[key];
  if (typeof value === "boolean") return value;
  if (typeof value === "string") {
    const lowered = value.trim().toLowerCase();
    if (["1", "true", "yes", "on"].includes(lowered)) return true;
    if (["0", "false", "no", "off"].includes(lowered)) return false;
  }
  return fallback;
}

function trackArrowIntervalM(): number {
  return Math.max(0, numericRuntimeSetting("gps.track_arrow_interval_m", 500));
}

function trackPreviewStyle(feature: FeatureLike) {
  return [
    new Style({ stroke: new Stroke({ color: "rgba(0,0,0,0.72)", width: 7 }) }),
    new Style({ stroke: new Stroke({ color: "#ffd33d", width: 4 }) })
  ].concat(trackArrowStyles(feature, trackArrowIntervalM(), "#ffd33d", "#0d1117"));
}

function greenTrackStyle(feature: FeatureLike) {
  return [
    new Style({ stroke: new Stroke({ color: "rgba(255,255,255,0.9)", width: 5 }) }),
    new Style({ stroke: new Stroke({ color: "#1a7f37", width: 3 }) })
  ].concat(trackArrowStyles(feature, trackArrowIntervalM(), "#1a7f37", "#ffffff"));
}

function trackArrowStyles(feature: FeatureLike, intervalM: number, fillColor: string, strokeColor: string): Style[] {
  if (intervalM <= 0) return [];
  const geometry = feature.getGeometry();
  if (!geometry) return [];
  const cacheKey = `track_arrows_${Math.round(intervalM)}`;
  const cached = feature.get(cacheKey) as Style[] | undefined;
  if (cached) return cached;
  const lines: number[][][] = [];
  if (geometry instanceof LineString) {
    lines.push(geometry.getCoordinates() as number[][]);
  } else if (geometry instanceof MultiLineString) {
    lines.push(...(geometry.getCoordinates() as number[][][]));
  }
  const styles: Style[] = [];
  for (const line of lines) {
    styles.push(...arrowStylesForLine(line, intervalM, fillColor, strokeColor));
  }
  if ("set" in feature && typeof feature.set === "function") {
    feature.set(cacheKey, styles, true);
  }
  return styles;
}

function arrowStylesForLine(line: number[][], intervalM: number, fillColor: string, strokeColor: string): Style[] {
  const styles: Style[] = [];
  if (line.length < 2) return styles;
  let nextAt = intervalM;
  let walked = 0;
  for (let i = 1; i < line.length; i += 1) {
    const start = line[i - 1];
    const end = line[i];
    const segmentM = projectedSegmentDistanceM(start, end);
    if (!Number.isFinite(segmentM) || segmentM <= 0) continue;
    while (walked + segmentM >= nextAt) {
      const ratio = (nextAt - walked) / segmentM;
      const x = start[0] + (end[0] - start[0]) * ratio;
      const y = start[1] + (end[1] - start[1]) * ratio;
      const rotation = Math.atan2(end[1] - start[1], end[0] - start[0]) - Math.PI / 2;
      styles.push(new Style({
        geometry: new Point([x, y]),
        image: new RegularShape({
          points: 3,
          radius: 8,
          rotation,
          rotateWithView: true,
          fill: new Fill({ color: fillColor }),
          stroke: new Stroke({ color: strokeColor, width: 1.5 })
        })
      }));
      nextAt += intervalM;
    }
    walked += segmentM;
  }
  return styles;
}

function projectedSegmentDistanceM(start: number[], end: number[]): number {
  const [lon1, lat1] = toLonLat(start);
  const [lon2, lat2] = toLonLat(end);
  return haversineDistanceM(lat1, lon1, lat2, lon2);
}

function haversineDistanceM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const radiusM = 6371008.8;
  const toRad = Math.PI / 180;
  const phi1 = lat1 * toRad;
  const phi2 = lat2 * toRad;
  const dLat = (lat2 - lat1) * toRad;
  const dLon = (lon2 - lon1) * toRad;
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(phi1) * Math.cos(phi2) * Math.sin(dLon / 2) ** 2;
  return 2 * radiusM * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

function fitTrackMap(target: OLMap | null, source: VectorSource, maxZoom = 16) {
  if (!target || source.getFeatures().length === 0) return;
  target.updateSize();
  const extent = source.getExtent();
  if (extent) {
    target.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom, duration: 120 });
  }
}

function refitTrackMapWhenStable(target: OLMap | null, source: VectorSource, fit: () => void) {
  if (!target || source.getFeatures().length === 0) return;
  [0, 80, 240, 520].forEach((delay) => {
    window.setTimeout(() => {
      target.updateSize();
      fit();
    }, delay);
  });
}

function fitGalleryTrack() {
  fitTrackMap(galleryTrackMap, galleryTrackSource, 16);
}

function destroyGalleryTrackMap() {
  galleryTrackMap?.setTarget(undefined);
  galleryTrackMap?.dispose();
  galleryTrackMap = null;
  galleryTrackTileLayer = null;
  galleryTrackLayer = null;
  galleryTrackSource.clear();
  galleryTrackFallbackPath.value = "";
}

function fitSelectedTrack() {
  fitTrackMap(selectedTrackMap, selectedTrackSource, 16);
}

function fitMainMap() {
  if (!olMap) return;
  const source = mapAssetSource.getFeatures().length > 0 ? mapAssetSource : mapTrackSource;
  if (source.getFeatures().length === 0) return;
  const extent = source.getExtent();
  if (extent) {
    olMap.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom: 14, duration: 150 });
  }
}

function persistLayerPreference(key: string, value: boolean) {
  localStorage.setItem(key, value ? "true" : "false");
}

async function refreshGalleryTrackPreview() {
  const current = galleryCurrent.value;
  if (!current || current.media_kind !== "track") {
    destroyGalleryTrackMap();
    galleryTrackPreviewStatus.value = "";
    return;
  }
  await nextTick();
  if (!galleryTrackElement.value) return;
  const preview = await api.trackPreview(current.id).catch(() => null);
  if (!preview || galleryCurrent.value?.id !== current.id) {
    galleryTrackPreviewStatus.value = "Track preview data could not be loaded.";
    return;
  }
  renderGalleryTrackMap(preview);
}

function renderGalleryTrackMap(preview: Record<string, unknown>) {
  if (!galleryTrackElement.value) return;
  destroyGalleryTrackMap();
  galleryTrackTileLayer = createLocalOSMLayer(() => {
    tileStatus.value = "Track preview tiles are unavailable; dark vector fallback remains active.";
  });
  galleryTrackTileLayer.setVisible(trackPreviewTilesEnabled.value);
  galleryTrackLayer = new VectorLayer({
    source: galleryTrackSource,
    style: trackPreviewStyle
  });
  galleryTrackLayer.setVisible(galleryTrackLayerVisible.value);
  galleryTrackMap = new OLMap({
    target: galleryTrackElement.value,
    layers: [galleryTrackTileLayer, galleryTrackLayer],
    view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 10 })
  });
  const features = new GeoJSON().readFeatures(preview, { featureProjection: "EPSG:3857" });
  galleryTrackSource.clear();
  galleryTrackSource.addFeatures(features);
  galleryTrackFallbackPath.value = "";
  galleryTrackPreviewStatus.value = trackPreviewStatus(preview, features.length);
  galleryTrackTileLayer?.setVisible(trackPreviewTilesEnabled.value);
  galleryTrackLayer?.setVisible(galleryTrackLayerVisible.value);
  refitTrackMapWhenStable(galleryTrackMap, galleryTrackSource, fitGalleryTrack);
}

async function refreshSelectedTrackMap() {
  const detail = selectedTrack.value;
  if (!detail) {
    selectedTrackSource.clear();
    selectedTrackPreviewStatus.value = "";
    selectedTrackFallbackPath.value = "";
    return;
  }
  await nextTick();
  if (!selectedTrackMapElement.value) return;
  const preview = await api.trackPreview(detail.summary.track_asset_id, 4000).catch(() => null);
  if (!preview || selectedTrack.value?.summary.track_asset_id !== detail.summary.track_asset_id) {
    selectedTrackPreviewStatus.value = "Track preview data could not be loaded.";
    return;
  }
  renderSelectedTrackMap(preview);
}

function renderSelectedTrackMap(preview: Record<string, unknown>) {
  if (!selectedTrackMapElement.value) return;
  if (!selectedTrackMap) {
    selectedTrackTileLayer = createLocalOSMLayer(() => {
      tileStatus.value = "Track detail tiles are unavailable; dark vector fallback remains active.";
    });
    selectedTrackTileLayer.setZIndex(0);
    selectedTrackTileLayer.setVisible(trackPreviewTilesEnabled.value);
    selectedTrackLayer = new VectorLayer({
      source: selectedTrackSource,
      style: trackPreviewStyle
    });
    selectedTrackLayer.setZIndex(5);
    selectedTrackLayer.setVisible(selectedTrackLayerVisible.value);
    selectedTrackMap = new OLMap({
      target: selectedTrackMapElement.value,
      layers: [selectedTrackTileLayer, selectedTrackLayer],
      view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 10 })
    });
    if (!selectedTrackClickBound) {
      selectedTrackClickBound = true;
      selectedTrackMap.on("singleclick", (event) => {
        void loadSelectedTrackPointInfo(event.coordinate);
      });
    }
  } else {
    selectedTrackMap.setTarget(selectedTrackMapElement.value);
  }
  const features = new GeoJSON().readFeatures(preview, { featureProjection: "EPSG:3857" });
  selectedTrackSource.clear();
  selectedTrackSource.addFeatures(features);
  selectedTrackFallbackPath.value = features.length === 0 ? trackFallbackPathFromPreview(preview) : "";
  selectedTrackPreviewStatus.value = trackPreviewStatus(preview, features.length);
  selectedTrackTileLayer?.setVisible(trackPreviewTilesEnabled.value);
  selectedTrackLayer?.setVisible(selectedTrackLayerVisible.value);
  refitTrackMapWhenStable(selectedTrackMap, selectedTrackSource, fitSelectedTrack);
}

async function loadSelectedTrackPointInfo(coordinate: number[]) {
  if (!selectedTrack.value) return;
  const lonLat = toLonLat(coordinate);
  selectedTrackPointMessage.value = "Loading track point details...";
  showSelectedTrackPointPopup.value = true;
  selectedTrackPointInfo.value = null;
  try {
    selectedTrackPointInfo.value = await api.gpsTrackPointInfo(
      selectedTrack.value.summary.track_asset_id,
      Number(lonLat[1]),
      Number(lonLat[0])
    );
    selectedTrackPointMessage.value = "";
  } catch (err) {
    selectedTrackPointMessage.value = err instanceof Error ? err.message : String(err);
  }
}

function destroyVideoTrackMap() {
  stopVideoTrackPlaybackLoop();
  videoTrackMarkerFeature = null;
  videoTrackMarkerSource.clear();
  videoTrackTrackSource.clear();
  if (videoTrackMap) {
    videoTrackMap.setTarget(undefined);
    videoTrackMap.dispose();
  }
  videoTrackMap = null;
  videoTrackTileLayer = null;
  videoTrackTrackLayer = null;
  videoTrackMarkerLayer = null;
}

function videoTrackMarkerStyle(feature: FeatureLike) {
  const label = String(feature.get("name") ?? feature.get("track_name") ?? "");
  return new Style({
    image: new CircleStyle({
      radius: 8,
      fill: new Fill({ color: "#0969da" }),
      stroke: new Stroke({ color: "#ffffff", width: 2 })
    }),
    text: label
      ? new Text({
          text: label.slice(0, 10),
          offsetY: -18,
          fill: new Fill({ color: "#ffffff" }),
          stroke: new Stroke({ color: "#0d1117", width: 3 }),
          font: "700 11px system-ui, sans-serif"
        })
      : undefined
  });
}

function videoTrackPositionNumber(position: Record<string, unknown> | null, key: string): number | undefined {
  if (!position) return undefined;
  const value = Number(position[key]);
  return Number.isFinite(value) ? value : undefined;
}

function videoTrackPositionText(position: Record<string, unknown> | null, key: string): string {
  if (!position) return "";
  const value = position[key];
  return value === undefined || value === null || value === "" ? "" : String(value);
}

function videoTrackPayloadText(key: string): string {
  return videoTrackPositionText(videoTrackPosition.value, key);
}

function videoTrackPointSummary(position: Record<string, unknown> | null): string {
  const lat = videoTrackPositionNumber(position, "lat");
  const lon = videoTrackPositionNumber(position, "lon");
  if (lat === undefined || lon === undefined) return "n/a";
  return `${lat.toFixed(6)}, ${lon.toFixed(6)}`;
}

function updateVideoTrackMarkerLayer() {
  const positions = asArray(videoTrackPosition.value?.positions as unknown[]);
  videoTrackMarkerSource.clear();
  videoTrackMarkerFeature = null;
  if (positions.length === 0) return;
  for (const position of positions) {
    const lat = Number((position as Record<string, unknown>).lat);
    const lon = Number((position as Record<string, unknown>).lon);
    if (!Number.isFinite(lat) || !Number.isFinite(lon)) continue;
    const feature = new Feature({
      geometry: new Point(fromLonLat([lon, lat])),
      kind: "video-track-marker",
      name: String((position as Record<string, unknown>).name ?? (position as Record<string, unknown>).track_id ?? "track")
    });
    feature.set("track_id", String((position as Record<string, unknown>).track_id ?? ""));
    feature.set("time", String((position as Record<string, unknown>).time ?? ""));
    videoTrackMarkerSource.addFeature(feature);
    if (!videoTrackMarkerFeature) {
      videoTrackMarkerFeature = feature as Feature<Point>;
    }
  }
  videoTrackMap?.updateSize();
}

function fitVideoTrackMap() {
  fitTrackMap(videoTrackMap, videoTrackTrackSource, 16);
}

async function renderVideoTrackMap() {
  const session = videoTrackSession.value;
  if (!session || !videoTrackMapElement.value) {
    destroyVideoTrackMap();
    return;
  }
  if (!videoTrackMap) {
    videoTrackTileLayer = createLocalOSMLayer(() => {
      tileStatus.value = "Video track tiles are unavailable; the vector track remains visible.";
    });
    videoTrackTileLayer.setVisible(true);
    videoTrackTrackLayer = new VectorLayer({
      source: videoTrackTrackSource,
      style: (feature) => {
        const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
        return kind === "track" ? greenTrackStyle(feature as Feature) : undefined;
      }
    });
    videoTrackTrackLayer.setZIndex(5);
    videoTrackMarkerLayer = new VectorLayer({
      source: videoTrackMarkerSource,
      style: videoTrackMarkerStyle
    });
    videoTrackMarkerLayer.setZIndex(15);
    videoTrackMap = new OLMap({
      target: videoTrackMapElement.value,
      layers: [videoTrackTileLayer, videoTrackTrackLayer, videoTrackMarkerLayer],
      controls: defaultControls({ attribution: false }).extend([
        new ScaleLine({ units: "metric", bar: true, steps: 4, text: true, minWidth: 120 })
      ]),
      view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 10 })
    });
  } else {
    videoTrackMap.setTarget(videoTrackMapElement.value);
  }
  videoTrackTrackSource.clear();
  const trackSelection = videoTrackSelectedTracks.value.length
    ? videoTrackSelectedTracks.value
    : tracks.value.filter((track) => session.track_ids.includes(track.track_asset_id));
  for (const track of trackSelection) {
    const preview = await api.trackPreview(track.track_asset_id, 2000).catch(() => null);
    if (!preview || videoTrackSession.value?.id !== session.id) continue;
    const features = new GeoJSON().readFeatures(preview, { featureProjection: "EPSG:3857" });
    for (const feature of features) {
      feature.set("track_id", track.track_asset_id);
      feature.set("name", track.name);
      videoTrackTrackSource.addFeature(feature);
    }
  }
  updateVideoTrackMarkerLayer();
  if (videoTrackTrackSource.getFeatures().length > 0) {
    fitVideoTrackMap();
  }
  videoTrackMap?.updateSize();
}

function stopVideoTrackPlaybackLoop() {
  if (videoTrackRafId) {
    window.cancelAnimationFrame(videoTrackRafId);
    videoTrackRafId = 0;
  }
  if (videoTrackTicker !== null) {
    window.clearInterval(videoTrackTicker);
    videoTrackTicker = null;
  }
}

function startVideoTrackPlaybackLoop(video: HTMLVideoElement) {
  const mode = runtimeTextSetting("video_track_player.sync_mode", "interval");
  stopVideoTrackPlaybackLoop();
  const throttleMs = Math.max(0, numericRuntimeSetting("video_track_player.marker_throttle_ms", 250));
  const tick = () => {
    if (!videoTrackSession.value || video.paused || video.ended) {
      stopVideoTrackPlaybackLoop();
      return;
    }
    const now = performance.now();
    if (now - videoTrackMarkerThrottleAt < throttleMs || videoTrackPositionPending) {
      videoTrackRafId = window.requestAnimationFrame(tick);
      return;
    }
    void updateVideoTrackPosition(video.currentTime * 1000);
    videoTrackRafId = window.requestAnimationFrame(tick);
  };
  if (mode === "smooth") {
    videoTrackRafId = window.requestAnimationFrame(tick);
    return;
  }
  const intervalMs = Math.max(500, numericRuntimeSetting("video_track_player.interval_seconds", 3) * 1000);
  videoTrackTicker = window.setInterval(() => {
    if (!videoTrackSession.value || video.paused || video.ended) return;
    const now = performance.now();
    if (now - videoTrackMarkerThrottleAt < throttleMs || videoTrackPositionPending) return;
    void updateVideoTrackPosition(video.currentTime * 1000);
  }, intervalMs);
}

function handleVideoTrackPlaybackEvent(event: Event) {
  const video = event.currentTarget as HTMLVideoElement | null;
  if (!video || !videoTrackSession.value) return;
  if (event.type === "pause" || event.type === "ended") {
    stopVideoTrackPlaybackLoop();
    return;
  }
  if (event.type === "play") {
    startVideoTrackPlaybackLoop(video);
    return;
  }
  if (event.type === "loadedmetadata" || event.type === "seeked" || event.type === "timeupdate") {
    const now = performance.now();
    const throttleMs = Math.max(0, numericRuntimeSetting("video_track_player.marker_throttle_ms", 250));
    if (videoTrackPositionPending) return;
    if (event.type === "timeupdate" && now - videoTrackMarkerThrottleAt < throttleMs) {
      return;
    }
    void updateVideoTrackPosition(video.currentTime * 1000, event.type !== "timeupdate");
  }
}

function trackFallbackPathFromPreview(preview: Record<string, unknown>): string {
  const coordinates = trackPreviewCoordinates(preview);
  if (coordinates.length === 0) return "";
  const lons = coordinates.map((point) => point[0]);
  const lats = coordinates.map((point) => point[1]);
  const minLon = Math.min(...lons);
  const maxLon = Math.max(...lons);
  const minLat = Math.min(...lats);
  const maxLat = Math.max(...lats);
  const width = 1000;
  const height = 420;
  const pad = 26;
  const lonRange = Math.max(0.000001, maxLon - minLon);
  const latRange = Math.max(0.000001, maxLat - minLat);
  return coordinates
    .map(([lon, lat], index) => {
      const x = pad + ((lon - minLon) / lonRange) * (width - pad * 2);
      const y = height - pad - ((lat - minLat) / latRange) * (height - pad * 2);
      return `${index === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

function trackPreviewCoordinates(preview: Record<string, unknown>): Array<[number, number]> {
  const out: Array<[number, number]> = [];
  const features = asArray(preview.features as unknown[]);
  for (const feature of features) {
    const geometry = ((feature as Record<string, unknown>).geometry ?? {}) as Record<string, unknown>;
    collectGeometryCoordinates(String(geometry.type ?? ""), geometry.coordinates, out);
  }
  return out;
}

function collectGeometryCoordinates(type: string, coordinates: unknown, out: Array<[number, number]>) {
  if (type === "Point" && Array.isArray(coordinates) && coordinates.length >= 2) {
    out.push([Number(coordinates[0]), Number(coordinates[1])]);
    return;
  }
  if (type === "LineString" && Array.isArray(coordinates)) {
    for (const point of coordinates) {
      if (Array.isArray(point) && point.length >= 2) out.push([Number(point[0]), Number(point[1])]);
    }
    return;
  }
  if ((type === "MultiLineString" || type === "Polygon") && Array.isArray(coordinates)) {
    for (const line of coordinates) collectGeometryCoordinates("LineString", line, out);
  }
}

function ensureGeoAlignMap() {
  if (!geoAlignMapElement.value) return;
  if (!geoAlignMap) {
    geoAlignTileLayer = createLocalOSMLayer(() => {
      tileStatus.value = "Geo Align OSM tiles are unavailable; vector markers remain active.";
    });
    geoAlignTileLayer.setZIndex(0);
    geoAlignTileLayer.setVisible(geoAlignTilesVisible.value);
    geoAlignTrackLayer = new VectorLayer({
      source: geoAlignTrackSource,
      style: trackPreviewStyle
    });
    geoAlignTrackLayer.setZIndex(4);
    geoAlignMarkerLayer = new VectorLayer({
      source: geoAlignMarkerSource,
      style: (feature) => {
        const status = String(feature.get("status") ?? "");
        const modified = Boolean(feature.get("modified"));
        const thumbnail = String(feature.get("thumbnail_url") ?? "");
        const colors: Record<string, string> = {
          own_geotag: "#1a7f37",
          track_candidate: "#bf8700",
          ungeotagged: "#cf222e"
        };
        const ring = new Style({
          image: new CircleStyle({
            radius: thumbnail ? (modified ? 18 : 15) : (modified ? 12 : 9),
            fill: new Fill({ color: colors[status] ?? "#0969da" }),
            stroke: new Stroke({ color: modified ? "#ffd33d" : "#ffffff", width: modified ? 4 : 2 })
          })
        });
        if (!thumbnail) return ring;
        return [
          ring,
          new Style({
            image: new Icon({
              src: thumbnail,
              crossOrigin: "anonymous",
              scale: 0.18
            })
          })
        ];
      }
    });
    geoAlignMarkerLayer.setZIndex(8);
    geoAlignMap = new OLMap({
      target: geoAlignMapElement.value,
      layers: [geoAlignTileLayer, geoAlignTrackLayer, geoAlignMarkerLayer],
      interactions: defaultInteractions({ shiftDragZoom: false }),
      view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 10 })
    });
    bindGeoAlignViewportDragHandlers();
    geoAlignMap.on("singleclick", (event) => {
      if (!geoAlignMap) return;
      if (geoAlignSuppressNextClick) {
        geoAlignSuppressNextClick = false;
        return;
      }
      const markerFeature = geoAlignMarkerLayer ? geoAlignFeatureAtPixel(event.pixel, geoAlignMarkerLayer) : null;
      const assetID = String(markerFeature?.get("asset_id") ?? "");
      if (assetID) {
        showGeoAlignMarkerPopup(assetID);
        return;
      }
      const trackFeature = geoAlignTrackLayer ? geoAlignFeatureAtPixel(event.pixel, geoAlignTrackLayer) : null;
      const trackID = String(trackFeature?.get("track_id") ?? trackFeature?.get("id") ?? "");
      if (trackID) {
        void showGeoAlignTrackPopup(trackID, event.coordinate);
      } else {
        closeGeoAlignPopup();
      }
    });
  } else {
    geoAlignMap.setTarget(geoAlignMapElement.value);
  }
}

function bindGeoAlignViewportDragHandlers() {
  if (!geoAlignMap) return;
  const viewport = geoAlignMap.getViewport();
  viewport.addEventListener("pointerdown", onGeoAlignPointerDown, { capture: true });
  viewport.addEventListener("pointermove", onGeoAlignPointerMove, { capture: true });
  viewport.addEventListener("pointerup", onGeoAlignPointerUp, { capture: true });
  viewport.addEventListener("pointerleave", onGeoAlignPointerUp, { capture: true });
}

function onGeoAlignPointerDown(pointer: PointerEvent) {
  if (!geoAlignMap || !geoAlignMarkerLayer || !pointer.shiftKey || pointer.button !== 0) return;
  const pixel = geoAlignMap.getEventPixel(pointer);
  const feature = geoAlignFeatureAtPixel(pixel, geoAlignMarkerLayer);
  const assetId = String(feature?.get("asset_id") ?? "");
  if (!feature || !assetId) return;
  pointer.preventDefault();
  pointer.stopPropagation();
  const marker = findGeoAlignMarker(assetId);
  geoAlignDragState = { assetId, feature, lastLat: marker?.staged_lat ?? 0, lastLon: marker?.staged_lon ?? 0, moved: false };
  geoAlignDragActive.value = true;
  geoAlignSuppressNextClick = true;
  geoAlignMap.getViewport().style.cursor = "grabbing";
  showGeoAlignMarkerPopup(assetId);
}

function onGeoAlignPointerMove(pointer: PointerEvent) {
  if (!geoAlignDragState || !geoAlignMap) return;
  pointer.preventDefault();
  pointer.stopPropagation();
  const coordinate = geoAlignMap.getEventCoordinate(pointer);
  const geometry = geoAlignDragState.feature.getGeometry();
  if (geometry instanceof Point) {
    geometry.setCoordinates(coordinate);
  }
  const lonLat = toLonLat(coordinate);
  geoAlignDragState.lastLon = Number(lonLat[0]);
  geoAlignDragState.lastLat = Number(lonLat[1]);
  geoAlignDragState.moved = true;
  const marker = findGeoAlignMarker(geoAlignDragState.assetId);
  if (marker) {
    marker.staged_lat = geoAlignDragState.lastLat;
    marker.staged_lon = geoAlignDragState.lastLon;
    marker.manual_lat = geoAlignDragState.lastLat;
    marker.manual_lon = geoAlignDragState.lastLon;
    marker.modified = true;
    if (geoAlignPopupMarker.value?.asset_id === marker.asset_id) {
      geoAlignPopupMarker.value = { ...marker };
    }
  }
  geoAlignDragState.feature.set("modified", true);
}

function onGeoAlignPointerUp(pointer: PointerEvent) {
  if (!geoAlignDragState) return;
  pointer.preventDefault();
  pointer.stopPropagation();
  void finishGeoAlignMarkerDrag();
}

function geoAlignFeatureAtPixel(pixel: number[], layer: VectorLayer<VectorSource>): Feature | null {
  if (!geoAlignMap) return null;
  let hit: Feature | null = null;
  geoAlignMap.forEachFeatureAtPixel(
    pixel,
    (feature) => {
      hit = feature as Feature;
      return true;
    },
    { hitTolerance: 10, layerFilter: (candidate) => candidate === layer }
  );
  return hit;
}

function findGeoAlignMarker(assetId: string): GeoAlignMarker | null {
  return geoAlignSession.value?.markers.find((marker) => marker.asset_id === assetId) ?? null;
}

function showGeoAlignMarkerPopup(assetId: string, center = false) {
  const marker = findGeoAlignMarker(assetId);
  if (!marker) return;
  geoAlignPopupTrackInfo.value = null;
  geoAlignPopupTrackMessage.value = "";
  geoAlignPopupMarker.value = { ...marker };
  if (center) centerGeoAlignOn(marker.staged_lat, marker.staged_lon, false);
}

async function showGeoAlignTrackPopup(trackId: string, coordinate: number[]) {
  const lonLat = toLonLat(coordinate);
  geoAlignPopupMarker.value = null;
  geoAlignPopupTrackInfo.value = null;
  geoAlignPopupTrackMessage.value = "Loading nearest track point...";
  try {
    geoAlignPopupTrackInfo.value = await api.gpsTrackPointInfo(trackId, Number(lonLat[1]), Number(lonLat[0]));
    geoAlignPopupTrackMessage.value = "";
  } catch (err) {
    geoAlignPopupTrackMessage.value = err instanceof Error ? err.message : String(err);
  }
}

function closeGeoAlignPopup() {
  geoAlignPopupMarker.value = null;
  geoAlignPopupTrackInfo.value = null;
  geoAlignPopupTrackMessage.value = "";
}

function centerGeoAlignOn(lat?: number, lon?: number, animate = true) {
  if (!geoAlignMap || typeof lat !== "number" || typeof lon !== "number") return;
  geoAlignMap.getView().animate({
    center: fromLonLat([lon, lat]),
    zoom: Math.max(geoAlignMap.getView().getZoom() ?? 14, 16),
    duration: animate ? 140 : 0
  });
}

async function finishGeoAlignMarkerDrag() {
  const drag = geoAlignDragState;
  geoAlignDragState = null;
  geoAlignDragActive.value = false;
  if (geoAlignMap) geoAlignMap.getViewport().style.cursor = "";
  if (!drag || !drag.moved || !geoAlignSession.value || !Number.isFinite(drag.lastLat) || !Number.isFinite(drag.lastLon)) return;
  const updated = await api.moveGeoAlignMarker(geoAlignSession.value.id, drag.assetId, drag.lastLat, drag.lastLon);
  const marker = findGeoAlignMarker(drag.assetId);
  if (marker) Object.assign(marker, updated);
  if (geoAlignPopupMarker.value?.asset_id === drag.assetId) {
    geoAlignPopupMarker.value = { ...updated };
  }
  geoAlignMessage.value = `${updated.name} moved. Apply saves the DB-only geotag override; EXIF writeback remains disabled.`;
}

async function refreshGeoAlignMap() {
  await nextTick();
  if (!geoAlignSession.value || !geoAlignMapElement.value) return;
  ensureGeoAlignMap();
  if (!geoAlignMap) return;
  geoAlignTileLayer?.setVisible(geoAlignTilesVisible.value);
  geoAlignTrackLayer?.setVisible(geoAlignTrackLayerVisible.value);
  geoAlignMarkerLayer?.setVisible(geoAlignMarkerLayerVisible.value);
  geoAlignMarkerSource.clear();
  for (const marker of geoAlignSession.value.markers) {
    geoAlignMarkerSource.addFeature(new Feature({
      geometry: new Point(fromLonLat([marker.staged_lon, marker.staged_lat])),
      asset_id: marker.asset_id,
      name: marker.name,
      status: marker.status,
      modified: marker.modified,
      thumbnail_url: marker.thumbnail_url ?? ""
    }));
  }
  geoAlignTrackSource.clear();
  for (const trackID of geoAlignSession.value.track_ids.slice(0, 6)) {
    const preview = await api.trackPreview(trackID, 1200).catch(() => null);
    if (!preview) continue;
    const features = new GeoJSON().readFeatures(preview, { featureProjection: "EPSG:3857" });
    const summary = ((preview.summary ?? {}) as Record<string, unknown>);
    for (const feature of features) {
      feature.set("track_id", trackID);
      feature.set("id", trackID);
      feature.set("name", String(summary.name ?? trackID));
      feature.set("source_format", String(summary.source_format ?? "track"));
      feature.set("point_count", Number(summary.point_count ?? 0));
    }
    geoAlignTrackSource.addFeatures(features);
  }
  window.setTimeout(() => {
    geoAlignMap?.updateSize();
    const source = geoAlignTrackSource.getFeatures().length > 0 ? geoAlignTrackSource : geoAlignMarkerSource;
    if (source.getFeatures().length > 0) {
      const extent = source.getExtent();
      if (extent) {
        geoAlignMap?.getView().fit(extent, { padding: [34, 34, 34, 34], maxZoom: 16, duration: 140 });
      }
    }
  }, 80);
}

function trackPreviewStatus(preview: Record<string, unknown>, renderedFeatures: number): string {
  const summary = (preview.summary ?? {}) as Record<string, unknown>;
  const points = Number(summary.point_count ?? 0);
  const format = String(summary.source_format ?? "track");
  if (renderedFeatures === 0) {
    return "No geometry returned for this track preview.";
  }
  return `${renderedFeatures} layer feature${renderedFeatures === 1 ? "" : "s"} · ${points.toLocaleString()} points · ${format.toUpperCase()}`;
}

function trackProfilePath(profile: TrackProfile | null): string {
  const points = asArray(profile?.series).filter((point) => typeof point.value === "number");
  if (points.length === 0) return "";
  const minX = Math.min(...points.map((point) => point.distance_m));
  const maxX = Math.max(...points.map((point) => point.distance_m));
  const values = points.map((point) => Number(point.value));
  const minY = Math.min(...values);
  const maxY = Math.max(...values);
  const width = 520;
  const height = 140;
  const pad = 14;
  const xRange = Math.max(1, maxX - minX);
  const yRange = Math.max(1e-9, maxY - minY);
  return points
    .map((point, index) => {
      const x = pad + ((point.distance_m - minX) / xRange) * (width - pad * 2);
      const y = height - pad - ((Number(point.value) - minY) / yRange) * (height - pad * 2);
      return `${index === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(" ");
}

function trackProfileRange(profile: TrackProfile | null): string {
  if (!profile?.has_values || typeof profile.min !== "number" || typeof profile.max !== "number") {
    return "No values";
  }
  return `${profile.min.toFixed(profile.metric === "speed" ? 2 : 1)}-${profile.max.toFixed(profile.metric === "speed" ? 2 : 1)} ${profile.unit}`;
}

function trackDurationLabel(track: TrackSummary): string {
  const duration = track.duration_seconds;
  const source = (track.source_format ?? "").toLowerCase();
  if (!duration || duration <= 0) return "No timestamps";
  if ((source === "kml" || source === "kmz") && duration < 60 && track.point_count > 1000) {
    return "Synthetic timestamps for display only";
  }
  if (duration < 120) return `${Math.round(duration)} s`;
  return `${Math.round(duration / 60)} min`;
}

function trackHasRealTime(track: TrackSummary): boolean {
  return !trackDurationLabel(track).toLowerCase().includes("synthetic") && !trackDurationLabel(track).toLowerCase().includes("no timestamps");
}

function markPreviewFailed(id: string) {
  const next = new Set(failedPreviewIds.value);
  next.add(id);
  failedPreviewIds.value = next;
}

function adapterRelativePrefixes(): string[] {
  return dryRunPrefix.value
    .split(",")
    .map((item) => item.trim().replace(/^\/+/, ""))
    .map((item) => item.replace(/^mnt\/Models\/rclone\/?/, ""))
    .map((item) => item.replace(/^\/+|\/+$/g, ""))
    .filter(Boolean);
}

function validateAdapterRelativePrefixes(prefixes: string[]): string {
  const rawPrefixes = dryRunPrefix.value.split(",").map((item) => item.trim()).filter(Boolean);
  for (const raw of rawPrefixes) {
    if (raw.startsWith("/") && raw !== "/mnt/Models/rclone" && !raw.startsWith("/mnt/Models/rclone/")) {
      return `Absolute prefix is not allowed here: ${raw}`;
    }
  }
  for (const prefix of prefixes) {
    if (prefix === "." || prefix === ".." || prefix.includes("../") || prefix.includes("..\\")) {
      return `Unsafe prefix rejected: ${prefix}`;
    }
  }
  return "";
}

async function refreshStreamOptionsForGallery() {
  const current = galleryCurrent.value;
  if (!current || current.media_kind !== "video") {
    streamOptions.value = null;
    return;
  }
  streamOptions.value = await api.streamOptions(current.id).catch(() => null);
}

function shortHash(value?: string): string {
  return value ? `${value.slice(0, 12)}...${value.slice(-8)}` : "";
}

function assetHashStatus(asset: AssetDetail["asset"]): string {
  return asArray(asset.locations)[0]?.hash_status ?? "unknown";
}

function assetHashValue(asset: AssetDetail["asset"]): string {
  return asArray(asset.locations).find((location) => location.sha512_hex)?.sha512_hex ?? "";
}

function assetHasGeo(asset: AssetDetail["asset"]): boolean {
  const metadata = asset.metadata ?? {};
  return typeof metadata.lat === "number" && typeof metadata.lon === "number";
}

function assetPrimaryExtension(asset?: Asset | null): string {
  const locationExt = asArray(asset?.locations).find((location) => location.extension)?.extension;
  const fileExt = asset?.display_name?.split(".").pop();
  return String(locationExt || fileExt || "").trim().toLowerCase().replace(/^\./, "");
}

function assetDocumentText(detail: AssetDetail | null | undefined): string {
  return detail?.document?.markdown || detail?.document?.text || "";
}

function isPDFDocument(detail: AssetDetail | null | undefined): boolean {
  return assetPrimaryExtension(detail?.asset) === "pdf";
}

function isMarkdownDocument(detail: AssetDetail | null | undefined): boolean {
  const extension = assetPrimaryExtension(detail?.asset);
  return extension === "md" || extension === "markdown" || Boolean(detail?.document?.markdown);
}

function isTextDocument(detail: AssetDetail | null | undefined): boolean {
  const extension = assetPrimaryExtension(detail?.asset);
  return extension === "txt" || isMarkdownDocument(detail);
}

type MarkdownPreviewBlock = {
  kind: "heading" | "list" | "code" | "quote" | "paragraph" | "blank";
  text: string;
  level?: number;
};

function markdownPreviewBlocks(text: string): MarkdownPreviewBlock[] {
  const blocks: MarkdownPreviewBlock[] = [];
  let inCode = false;
  let codeBuffer: string[] = [];
  for (const line of text.split(/\r?\n/)) {
    if (line.trim().startsWith("```")) {
      if (inCode) {
        blocks.push({ kind: "code", text: codeBuffer.join("\n") });
        codeBuffer = [];
      }
      inCode = !inCode;
      continue;
    }
    if (inCode) {
      codeBuffer.push(line);
      continue;
    }
    const trimmed = line.trim();
    if (!trimmed) {
      blocks.push({ kind: "blank", text: "" });
      continue;
    }
    const heading = /^(#{1,6})\s+(.+)$/.exec(trimmed);
    if (heading) {
      blocks.push({ kind: "heading", text: heading[2], level: heading[1].length });
      continue;
    }
    if (/^[-*+]\s+/.test(trimmed)) {
      blocks.push({ kind: "list", text: trimmed.replace(/^[-*+]\s+/, "") });
      continue;
    }
    if (/^\d+[.)]\s+/.test(trimmed)) {
      blocks.push({ kind: "list", text: trimmed.replace(/^\d+[.)]\s+/, "") });
      continue;
    }
    if (trimmed.startsWith(">")) {
      blocks.push({ kind: "quote", text: trimmed.replace(/^>\s?/, "") });
      continue;
    }
    blocks.push({ kind: "paragraph", text: line });
  }
  if (inCode || codeBuffer.length > 0) {
    blocks.push({ kind: "code", text: codeBuffer.join("\n") });
  }
  return blocks.slice(0, 240);
}

function videoSource(assetId: string, originalUrl?: string): string {
  if (transcodeSession.value?.asset_id === assetId && transcodeSession.value.playlist_url && !transcodeSession.value.playlist_url.endsWith(".m3u8")) {
    return transcodeSession.value.playlist_url;
  }
  if (transcodeSession.value?.asset_id === assetId && transcodeSession.value.playlist_url && canPlayNativeHLS()) {
    return transcodeSession.value.playlist_url;
  }
  return originalUrl ?? "";
}

async function stopActiveTranscode() {
  if (activeHls) {
    activeHls.destroy();
    activeHls = null;
  }
  const session = transcodeSession.value;
  transcodeSession.value = null;
  if (session?.id) {
    await api.stopTranscodeSession(session.id).catch(() => null);
  }
}

async function selectTranscodeOption(assetId: string, event: Event) {
  const select = event.target as HTMLSelectElement;
  const optionID = select.value;
	transcodeMessage.value = "";
	if (!optionID || optionID === "original") {
		lastVideoPreset.value = "original";
		localStorage.setItem("cartolensia.videoPreset", "original");
		await stopActiveTranscode();
		return;
	}
  const option = asArray(streamOptions.value?.options).find((candidate) => candidate.id === optionID);
	if (!option || !option.available) {
		transcodeMessage.value = option?.disabled_reason ?? "This stream option is not available.";
		select.value = "original";
		return;
	}
	lastVideoPreset.value = optionID;
	localStorage.setItem("cartolensia.videoPreset", optionID);
	if (!option.profile && !option.session_endpoint) {
		await stopActiveTranscode();
		return;
	}
  await stopActiveTranscode();
  transcodeMessage.value = "Starting cache-scoped transcode session...";
  try {
    transcodeSession.value = await api.startTranscodeSession(assetId, option.profile ?? option.id);
    transcodeMessage.value = option.codec === "av1"
      ? "Encoding a cache-scoped AV1/WebM preview. This can take longer on CPU."
      : "Waiting for HLS playlist and first segment in the Cartolensia cache...";
    transcodeSession.value = await waitForTranscodeReady(transcodeSession.value.id);
    await nextTick();
    await attachHLSPlayback();
    transcodeMessage.value = transcodeSession.value.playlist_url.endsWith(".webm")
      ? "Playing AV1/WebM from the Cartolensia transcode cache. Originals remain immutable."
      : "Streaming HLS from the Cartolensia cache. Originals remain immutable.";
  } catch (err) {
    transcodeMessage.value = err instanceof Error ? err.message : String(err);
    await stopActiveTranscode();
    select.value = "original";
	}
}

const supportedTranscodeEncoders = computed(() => asArray(transcodingCapabilities.value?.encoders));

const groupedTranscodeHardware = computed(() => {
	const hardware = transcodingCapabilities.value?.hardware ?? {};
	return [
		{ id: "cpu", label: "CPU", available: true, reason: "" },
		{ id: "nvidia", label: "NVIDIA GPU", available: !!hardware.nvidia_smi, reason: hardware.nvidia_smi ? "" : "Not present" },
		{
			id: "amd",
			label: "AMD GPU",
			available: !!hardware.vaapi || !!hardware.dev_dri,
			reason: hardware.vaapi || hardware.dev_dri ? "" : "No VAAPI/DRI device detected"
		},
		{
			id: "intel",
			label: "Intel GPU",
			available: !!hardware.qsv || !!hardware.dev_dri,
			reason: hardware.qsv || hardware.dev_dri ? "" : "No QSV/DRI device detected"
		}
	];
});

const availableEncodersForCustom = computed(() => {
	const hardware = customPresetHardware.value;
	const codec = customPresetCodec.value;
	return supportedTranscodeEncoders.value.filter((encoder) => {
		const name = encoder.name.toLowerCase();
		const family = (encoder.codec_family ?? "").toLowerCase();
		const hw = (encoder.hardware ?? "").toLowerCase();
		const codecMatches =
			codec === "custom" ||
			family === codec ||
			(codec === "h264" && name.includes("264")) ||
			(codec === "h265" && (name.includes("265") || name.includes("hevc"))) ||
			(codec === "av1" && name.includes("av1"));
		const hardwareMatches =
			hardware === "cpu"
				? !hw || hw === "software"
				: hardware === "nvidia"
					? hw === "nvidia" || name.includes("nvenc")
					: hardware === "amd"
						? hw === "amd" || name.includes("amf") || name.includes("vaapi")
						: hardware === "intel"
							? hw === "intel" || name.includes("qsv") || name.includes("vaapi")
							: true;
		return codecMatches && hardwareMatches;
	});
});

watch([customPresetHardware, customPresetCodec], () => {
	const first = availableEncodersForCustom.value[0]?.name ?? "";
	if (!availableEncodersForCustom.value.some((encoder) => encoder.name === customPresetEncoder.value)) {
		customPresetEncoder.value = first;
	}
});

function currentCustomTranscodePreset(): Partial<TranscodingPreset> {
	return {
		name: customPresetName.value.trim() || "Custom preset",
		hardware: customPresetHardware.value,
		codec: customPresetCodec.value,
		ffmpeg_encoder: customPresetEncoder.value || availableEncodersForCustom.value[0]?.name || "",
		mode: customPresetMode.value,
		parameter_value: customPresetParameter.value,
		container: customPresetCodec.value === "av1" ? "webm" : "hls"
	};
}

async function saveCustomTranscodePreset() {
	const hardware = groupedTranscodeHardware.value.find((item) => item.id === customPresetHardware.value);
	if (!hardware?.available) {
		transcodeMessage.value = hardware?.reason || "Selected hardware is unavailable.";
		return;
	}
	try {
		const preset = currentCustomTranscodePreset();
		const validation = await api.validateTranscodingPreset(preset);
		transcodeValidation.value = validation;
		if (validation.valid === false) {
			transcodeMessage.value = String(validation.error ?? "Preset validation failed.");
			return;
		}
		const saved = await api.saveTranscodingPreset(preset);
		transcodePresets.value = await api.transcodingPresets();
		lastVideoPreset.value = saved.id;
		localStorage.setItem("cartolensia.videoPreset", saved.id);
		transcodeMessage.value = `Saved preset ${saved.name}.`;
	} catch (err) {
		transcodeMessage.value = err instanceof Error ? err.message : String(err);
	}
}

async function applyCustomTranscodePreset(assetId: string) {
	await stopActiveTranscode();
	const preset = currentCustomTranscodePreset();
	transcodeMessage.value = "Validating custom preset and starting a cache-scoped session...";
	try {
		const validation = await api.validateTranscodingPreset(preset);
		transcodeValidation.value = validation;
		if (validation.valid === false) {
			transcodeMessage.value = String(validation.error ?? "Preset validation failed.");
			return;
		}
		transcodeSession.value = await api.startTranscodeSession(assetId, "custom_inline", preset);
		transcodeSession.value = await waitForTranscodeReady(transcodeSession.value.id);
		await nextTick();
		await attachHLSPlayback();
		transcodeMessage.value = "Applied unsaved preset. Streaming from the Cartolensia cache.";
	} catch (err) {
		transcodeMessage.value = err instanceof Error ? err.message : String(err);
		await stopActiveTranscode();
	}
}

async function testCustomTranscodePreset(assetId: string) {
	const preset = currentCustomTranscodePreset();
	transcodeMessage.value = "Running a short hardware validation dry-run...";
	try {
		const result = await api.testTranscodingHardware(preset, assetId);
		transcodeValidation.value = result;
		transcodeMessage.value = result.valid === false
			? `Hardware test failed: ${String(result.dry_run_error ?? result.error ?? "unknown error")}`
			: "Hardware test passed. The preset can be applied or saved.";
	} catch (err) {
		transcodeMessage.value = err instanceof Error ? err.message : String(err);
	}
}

async function removeTranscodePreset(id: string) {
	const preset = transcodePresets.value.find((candidate) => candidate.id === id);
	if (!preset || preset.built_in) return;
	await api.deleteTranscodingPreset(id);
	transcodePresets.value = await api.transcodingPresets();
	if (lastVideoPreset.value === id) {
		lastVideoPreset.value = "original";
		localStorage.setItem("cartolensia.videoPreset", "original");
	}
}

function openAdvancedTranscode(assetId: string) {
  advancedTranscodeAssetId.value = assetId;
  showAdvancedTranscode.value = true;
  void nextTick(() => {
    const first = document.querySelector<HTMLElement>(".transcode-modal [data-autofocus], .transcode-modal input, .transcode-modal select, .transcode-modal button");
    first?.focus();
  });
}

function closeAdvancedTranscode() {
  showAdvancedTranscode.value = false;
  advancedTranscodeAssetId.value = "";
}

function handleAdvancedTranscodeKeydown(event: KeyboardEvent) {
  event.stopPropagation();
  if (event.key === "Escape") {
    event.preventDefault();
    closeAdvancedTranscode();
    return;
  }
  if (event.key !== "Tab") return;
  const container = event.currentTarget as HTMLElement;
  const focusable = Array.from(
    container.querySelectorAll<HTMLElement>("button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])")
  ).filter((element) => !element.hasAttribute("disabled") && element.tabIndex !== -1);
  if (focusable.length === 0) return;
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

async function waitForTranscodeReady(sessionId: string): Promise<TranscodeSession> {
  let current = await api.transcodeSessionStatus(sessionId);
  for (let i = 0; i < 45 && current.status !== "ready" && current.status !== "finished"; i++) {
    if (current.status === "failed") {
      throw new Error(current.error || current.stderr_tail || "Transcode session failed.");
    }
    await delay(1000);
    current = await api.transcodeSessionStatus(sessionId);
  }
  if (current.status !== "ready" && current.status !== "finished") {
    throw new Error("Transcode session did not become ready before the startup timeout.");
  }
  return current;
}

function canPlayNativeHLS(): boolean {
  const video = document.createElement("video");
  return video.canPlayType("application/vnd.apple.mpegurl") !== "" || video.canPlayType("application/x-mpegURL") !== "";
}

async function loadHLSConstructor(): Promise<HlsConstructor> {
  hlsConstructorPromise ??= import("hls.js").then((module) => module.default);
  return hlsConstructorPromise;
}

async function attachHLSPlayback() {
  const session = transcodeSession.value;
  if (!session?.playlist_url) return;
  const video = galleryCurrent.value?.id === session.asset_id ? galleryVideoElement.value : assetVideoElement.value;
  if (!video) return;
  if (!session.playlist_url.endsWith(".m3u8")) {
    if (activeHls) {
      activeHls.destroy();
      activeHls = null;
    }
    video.src = session.playlist_url;
    await video.play().catch(() => undefined);
    return;
  }
  if (canPlayNativeHLS()) {
    video.src = session.playlist_url;
    await video.play().catch(() => undefined);
    return;
  }
  const HlsConstructor = await loadHLSConstructor();
  if (!HlsConstructor.isSupported()) {
    throw new Error("This browser cannot play HLS natively and hls.js is unavailable.");
  }
  if (activeHls) activeHls.destroy();
  activeHls = new HlsConstructor({
    lowLatencyMode: false,
    backBufferLength: 90,
    xhrSetup: (xhr) => {
      xhr.withCredentials = true;
    },
    fetchSetup: (context, initParams) =>
      new Request(context.url, {
        ...initParams,
        credentials: "same-origin"
      })
  });
  activeHls.loadSource(session.playlist_url);
  activeHls.attachMedia(video);
  activeHls.on(HlsConstructor.Events.ERROR, (_event, data) => {
    if (data.fatal) {
      transcodeMessage.value = `HLS playback error: ${data.details || data.type}. Reverting to Original/direct.`;
      void stopActiveTranscode();
    }
  });
  await video.play().catch(() => undefined);
}

function seekAssetMedia(timeMs: number) {
  const media = assetAudioElement.value || assetVideoElement.value;
  if (!media) return;
  media.currentTime = Math.max(0, timeMs / 1000);
  media.play().catch(() => undefined);
}

function seekMediaElement(media: HTMLMediaElement | null, timeMs: number) {
  if (!media) return false;
  const seconds = Math.max(0, timeMs / 1000);
  const applySeek = () => {
    try {
      media.currentTime = seconds;
      media.play().catch(() => undefined);
    } catch {
      // Ignore browsers that briefly reject seeks before metadata is loaded.
    }
  };
  if (media.readyState >= 1) {
    applySeek();
    return true;
  }
  media.addEventListener("loadedmetadata", applySeek, { once: true });
  return false;
}

async function applyAssetDetailSeek() {
  if (assetDetailSeekMs.value === null) return;
  const seekMs = assetDetailSeekMs.value;
  const asset = assetDetail.value?.asset;
  if (!asset) return;
  const media =
    asset.media_kind === "video"
      ? assetVideoElement.value
      : asset.media_kind === "audio"
        ? assetAudioElement.value
        : null;
  if (seekMediaElement(media, seekMs)) {
    assetDetailSeekMs.value = null;
    return;
  }
  if (asset.media_kind === "video" || asset.media_kind === "audio") {
    window.setTimeout(() => {
      if (assetDetailSeekMs.value === seekMs) {
        void applyAssetDetailSeek();
      }
    }, 120);
  }
}

function terminalJobStatus(status: string): boolean {
  return ["succeeded", "failed", "canceled", "cancelled"].includes(status);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function unauthenticatedBackend(auth: BackendStatus["auth"]): BackendStatus {
  const emptyStats: Stats = {
    assets: 0,
    locations: 0,
    photos: 0,
    videos: 0,
    tracks: 0,
    unhashed: 0,
    hashed: 0,
    duplicate_groups: 0,
    duplicate_locations: 0,
    total_bytes: 0
  };
  return {
    store_backend: "locked",
    plugins: 0,
    capabilities: [],
    stats: emptyStats,
    preview_cache: "",
    auth_mode: auth.mode || "local",
    auth,
    http: undefined,
    tools: {}
  };
}

async function waitForJob(id: string, maxPolls = 120): Promise<Job> {
  let current = await api.job(id);
  for (let i = 0; i < maxPolls && !terminalJobStatus(current.status); i++) {
    await delay(1000);
    current = await api.job(id);
  }
  return current;
}

async function refresh() {
  loading.value = true;
  error.value = "";
  try {
    const authState = await refreshAuth();
    if (!authState?.principal && (authState?.auth.mode ?? "local") === "local") {
      backend.value = unauthenticatedBackend(authState?.auth ?? { mode: "local" });
      publicAssets.value = await api.publicAssets("limit=80").catch(() => []);
      loading.value = false;
      return;
    }
    publicAssets.value = [];
    const [
      jobRows,
      jobStatData,
      storageRows,
      pluginRows,
      statData,
      duplicateData,
      monthData,
      backendStatus,
      trackRows,
      geojson,
      mapStatusData,
      albumRows,
      previewStatus,
      previewEntries,
	      transcodeCaps,
	      presetRows,
	      metricsStatus,
	      tileSourceRows,
	      ai,
	      aiWorkerRows,
	      vector,
	      aiSummaryPayload,
	      aiTagsPayload,
	      aiPredictionsPayload,
	      aiFacesPayload,
	      aiSafetyData,
	      faceClusterPayload,
	      settingsPayload,
	      componentStatusPayload,
	      placeCachePayload,
	      editablePlacePayload,
	      readinessPayload,
	      exportRows
    ] = await Promise.all([
      fetchVisibleJobs(),
      api.jobStats(),
      api.storages(),
      api.plugins(),
      api.stats(),
      api.duplicates(),
      api.assetMonths(),
      api.status(),
      api.gpsTracks({ limit: trackPageSize, offset: 0, q: trackSearchQ.value.trim(), sort: "time_desc" }),
      api.map(mapQuery()),
      api.mapStatus(),
      api.albums(),
      api.previewStatus(),
      api.previewCache(),
      api.transcodingCapabilities(),
      api.transcodingPresets(),
	      api.transcodingMetricsStatus(),
	      api.tileSources(),
	      api.aiStatus(),
	      api.aiWorkers(),
	      api.vectorStatus(),
	      api.aiSummary(),
	      api.aiTags(),
	      api.aiPredictions(aiPredictionLimit.value, 0, activePredictionQueryFilter(), activePredictionTaskFilter()),
	      api.aiFaces(aiFaceLimit.value, 0, ""),
	      api.aiSafety(),
	      api.faceClusters(),
	      api.settings(),
	      api.componentStatus(),
	      api.searchPlaces(),
	      api.places(placeCacheQuery.value),
	      api.readiness(),
      api.dbExports()
		]);
		explorer.value = await api.explorerFolders(explorerPath.value, explorerQueryString({ limit: explorerPageLimit, offset: 0 }));
		rows.value = asArray(explorer.value.files);
		if (explorerQ.value.trim()) {
			const search = await api.search(explorerQ.value.trim(), 100);
			searchResults.value = asArray(search.results);
			searchWarnings.value = asArray(search.warnings);
		} else {
			searchResults.value = [];
			searchWarnings.value = [];
		}
		jobs.value = asArray(jobRows);
    jobStats.value = jobStatData;
    storages.value = asArray(storageRows);
    plugins.value = asArray(pluginRows);
    if (!selectedPluginSettingsId.value && plugins.value.length > 0) {
      selectedPluginSettingsId.value = plugins.value[0].id;
    }
    await Promise.all(plugins.value.map(async (plugin) => {
      if (pluginSettingText.value[plugin.id]) return;
      const payload = await api.pluginSettings(plugin.id).catch(() => ({ settings: {} }));
      pluginSettingText.value[plugin.id] = JSON.stringify((payload.settings ?? {}) as Record<string, unknown>, null, 2);
    }));
    stats.value = statData;
    duplicatePage.value = duplicateData;
    backendMonthBuckets.value = asArray(monthData);
    backend.value = backendStatus;
    tracks.value = asArray(trackRows);
    tracksHasMore.value = tracks.value.length >= trackPageSize;
    videoTrackTrackOptions.value = tracks.value;
    if (videoTrackSelectedTracks.value.length === 0 && videoTrackSession.value?.track_ids?.length) {
      videoTrackSelectedTracks.value = tracks.value.filter((track) =>
        videoTrackSession.value?.track_ids.includes(track.track_asset_id)
      );
      videoTrackIds.value = videoTrackSelectedTracks.value.map((track) => track.track_asset_id).join(", ");
    }
    mapData.value = geojson;
    mapStatus.value = mapStatusData;
    albums.value = asArray(albumRows);
    previewCacheStats.value = previewStatus.stats;
    previewCache.value = asArray(previewEntries);
    tileSources.value = asArray(tileSourceRows);
    if (selectedAlbumId.value && !albums.value.some((album) => album.id === selectedAlbumId.value)) {
      selectedAlbumId.value = "";
      albumItems.value = null;
    }
    if (mapAlbumId.value && !albums.value.some((album) => album.id === mapAlbumId.value)) {
      mapAlbumId.value = "";
    }
    if (selectedAlbumId.value) {
      albumItems.value = await api.albumItems(selectedAlbumId.value).catch(() => null);
    }
    await refreshIndexingStatus();
	    transcodingCapabilities.value = transcodeCaps;
	    transcodePresets.value = asArray(presetRows);
	    transcodeMetricsPayload.value = metricsStatus;
	    aiStatus.value = ai;
	    aiWorkers.value = aiWorkerRows;
	    vectorStatus.value = vector;
	    aiSummary.value = aiSummaryPayload;
	    aiTagPayload.value = aiTagsPayload;
	    aiPredictionPayload.value = aiPredictionsPayload;
	    aiFacePayload.value = aiFacesPayload;
	    aiSafetyPayload.value = aiSafetyData;
    faceClustersPayload.value = faceClusterPayload;
    settings.value = settingsPayload;
    if (dryRunExtensions.value === supportedDiscoveryExtensions) {
      const configuredExtensions = String(settingsPayload.runtime_settings?.["indexing.supported_extensions"] ?? "");
      if (configuredExtensions.trim()) dryRunExtensions.value = configuredExtensions;
    }
    components.value = asArray(componentStatusPayload.components);
    componentRoot.value = componentStatusPayload.root;
    componentCounts.value = componentStatusPayload.counts ?? {};
    readiness.value = readinessPayload;
    searchPlaceCache.value = placeCachePayload;
    editablePlaces.value = asArray(editablePlacePayload.places);
    initializePendingConfig(settingsPayload);
    dbExports.value = asArray(exportRows);
    await loadVideoTrackVideoOptions();
    if (principal.value && backendStatus.auth?.mode === "local") {
      apiTokens.value = await api.tokens().catch(() => []);
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

function initializePendingConfig(payload: SettingsPayload) {
  if (Object.keys(pendingConfig.value).length > 0) return;
  const pending = (payload.pending_settings?.pending ?? null) as Record<string, unknown> | null;
  pendingConfig.value = pending ? JSON.parse(JSON.stringify(pending)) : JSON.parse(JSON.stringify(payload.effective ?? {}));
}

async function refreshAuth() {
  try {
    const result = await api.me();
    principal.value = result.principal;
    return result;
  } catch {
    principal.value = null;
    return null;
  }
}

async function login() {
  error.value = "";
  try {
    const result = await api.login(loginEmail.value.trim(), loginPassword.value.replace(/[\r\n]+$/g, ""));
    principal.value = result.principal;
    loginPassword.value = "";
    await refresh();
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  }
}

async function logout() {
  await api.logout();
  principal.value = null;
  apiTokens.value = [];
}

async function startDiscovery() {
  await startIndexingPipeline();
}

function pipelineStorage(): string {
  return dryRunStorage.value || storages.value[0]?.name || "";
}

function pipelineMaxFiles(): number {
  const value = Number(dryRunMaxFiles.value);
  return Number.isFinite(value) && value !== 0 ? Math.trunc(value) : -1;
}

function dryRunPreviewMaxFiles(): number {
  const value = Number(dryRunMaxFiles.value);
  if (!Number.isFinite(value) || value <= 0) return 50;
  return Math.min(Math.trunc(value), 50);
}

async function refreshIndexingStatus() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (!storage || storage === "all") return;
  indexingStatus.value = await api.indexingLatest(storage, prefixes).catch(() => indexingStatus.value);
}

async function runPipelineJob(label: string, job: Job): Promise<Job> {
  pipelineStage.value = label;
  currentPipelineJobId.value = job.id;
  pipelineLog.value = [`${label}: ${job.status}`].concat(pipelineLog.value).slice(0, 12);
  await refresh();
  const finished = await waitForJob(job.id).catch(() => job);
  pipelineLog.value = [`${label}: ${finished.status} (${finished.progress_current}/${finished.progress_total ?? "?"})`]
    .concat(pipelineLog.value)
    .slice(0, 12);
  currentPipelineJobId.value = "";
  await refreshIndexingStatus();
  return finished;
}

async function startIndexingPipeline() {
  const storage = dryRunStorage.value || storages.value[0]?.name || "";
  const prefixes = adapterRelativePrefixes();
  const validation = validateAdapterRelativePrefixes(prefixes);
  if (!storage || validation) {
    error.value = validation || "Indexing pipeline requires a storage.";
    return;
  }
  error.value = "";
  pipelineRunning.value = true;
  pipelineLog.value = [];
  const maxFiles = pipelineMaxFiles();
  try {
    const start = await api.startIndexing({
      storage,
      prefixes,
      max_files: maxFiles,
      max_bytes: dryRunMaxBytes.value,
      include_extensions: dryRunExtensions.value.split(",").map((item) => item.trim()).filter(Boolean),
      index_files: pipelineIndexFiles.value,
      hash: hashAfterIndex.value,
      metadata: metadataAfterIndex.value,
      previews: previewsAfterIndex.value,
      parse_tracks: pipelineParseTracks.value,
      geotag_exif: pipelineGeotagExif.value,
      snap_to_tracks: pipelineSnapToTracks.value,
      refresh_map: pipelineRefreshMap.value
    });
    currentPipelineId.value = start.pipeline_id;
    indexingStatus.value = { scope: start.scope, latest_jobs: {} };
    const discoveryJob = start.queued_jobs.find((job) => job.kind === "discovery");
    if (discoveryJob) {
      lastDiscoveryJob.value = await runPipelineJob("Index files", discoveryJob);
    } else {
      pipelineLog.value = ["Index files: skipped"].concat(pipelineLog.value);
    }
    if (prefixes.length > 0) explorerPath.value = prefixes[0];
    await refreshIndexingStatus();
    const scope = indexingStatus.value?.scope;
    if (hashAfterIndex.value) {
      if ((scope?.unhashed ?? 0) > 0) {
        const hashJob = await api.startHash({ scope: "prefix", storage, prefixes, max_files: maxFiles });
        lastHashJob.value = await runPipelineJob("Hash files", hashJob);
      } else {
        pipelineLog.value = [`Hash files: skipped, ${scope?.hashed ?? 0} already hashed`].concat(pipelineLog.value).slice(0, 12);
      }
    }
    if (metadataAfterIndex.value || pipelineParseTracks.value || pipelineGeotagExif.value) {
      const metadataJob = await api.startMetadataScoped({ storage, prefixes, max_files: maxFiles, only_missing: false });
      lastMetadataJob.value = await runPipelineJob("Extract metadata/EXIF/GPS/KML", metadataJob);
    }
    await refreshIndexingStatus();
    const afterMetadata = indexingStatus.value?.scope;
    if (previewsAfterIndex.value) {
      if ((afterMetadata?.photos ?? 0) > (afterMetadata?.preview_ready ?? 0)) {
        const previewJob = await api.startPreviewsScoped({ storage, prefixes, max_files: maxFiles, only_missing: true });
        lastPreviewJob.value = await runPipelineJob("Generate previews", previewJob);
      } else {
        pipelineLog.value = [`Generate previews: skipped, ${afterMetadata?.preview_ready ?? 0} ready`].concat(pipelineLog.value).slice(0, 12);
      }
    }
    if (pipelineSnapToTracks.value) {
      pipelineLog.value = ["Snap media to tracks: skipped unless a specific parsed track is selected"].concat(pipelineLog.value).slice(0, 12);
    }
    if (pipelineRefreshMap.value) {
      await refreshMap();
      pipelineLog.value = ["Refresh map/clusters: done"].concat(pipelineLog.value).slice(0, 12);
    }
    await refresh();
    await refreshIndexingStatus();
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    pipelineRunning.value = false;
    pipelineStage.value = "";
    currentPipelineJobId.value = "";
  }
}

async function stopIndexingPipeline() {
  if (currentPipelineJobId.value) {
    await api.cancelJob(currentPipelineJobId.value).catch(() => null);
  }
  if (currentPipelineId.value) {
    await api.cancelIndexing(currentPipelineId.value).catch(() => null);
  }
  pipelineRunning.value = false;
  pipelineLog.value = ["Stop requested"].concat(pipelineLog.value).slice(0, 12);
  await refresh();
}

async function startHash() {
  if (selectedAssets.value.size > 0) {
    lastHashJob.value = await api.startHash({ scope: "selected", asset_ids: Array.from(selectedAssets.value), max_files: selectedAssets.value.size });
  } else {
    await startHashForCurrentPrefix();
    return;
  }
  await refresh();
}

async function startHashForCurrentPrefix() {
  const storage = dryRunStorage.value || storages.value[0]?.name || "";
  const prefixes = adapterRelativePrefixes();
  const validation = validateAdapterRelativePrefixes(prefixes);
  if (!storage || validation) {
    error.value = validation || "Hashing current prefix requires a storage.";
    return;
  }
  lastHashJob.value = await api.startHash({
    scope: "prefix",
    storage,
    prefixes,
    max_files: pipelineMaxFiles()
  });
  await refresh();
}

async function startMetadata() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (storage) {
    lastMetadataJob.value = await api.startMetadataScoped({ storage, prefixes, max_files: jobMaxFiles.value, only_missing: false });
  } else {
    lastMetadataJob.value = await api.startMetadata(jobMaxFiles.value);
  }
  await refresh();
}

async function parseTrackFilesForCurrentPrefix() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (!storage) {
    error.value = "Choose a storage before parsing GPS/KML/KMZ/GPZ files.";
    return;
  }
  error.value = "";
  pipelineLog.value = [`Parsing track files under ${prefixes.join(", ") || "whole selected storage"}`].concat(pipelineLog.value).slice(0, 12);
  lastMetadataJob.value = await api.startMetadataScoped({
    storage,
    prefixes,
    max_files: pipelineMaxFiles(),
    only_missing: false
  });
  lastMetadataJob.value = await runPipelineJob("Parse GPS/KML/KMZ/GPZ tracks", lastMetadataJob.value);
  await refresh();
}

async function startPreviews() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (storage) {
    lastPreviewJob.value = await api.startPreviewsScoped({ storage, prefixes, max_files: jobMaxFiles.value, only_missing: true });
  } else {
    lastPreviewJob.value = await api.startPreviews(jobMaxFiles.value);
  }
  await refresh();
}

async function cleanupPreviews(dryRun = true) {
	await api.previewCleanup(dryRun);
	await refresh();
}

async function confirmCleanupPreviews() {
  if (!window.confirm("Clear generated preview cache files under the configured Cartolensia cache root? Originals are not touched.")) {
    return;
  }
  await cleanupPreviews(false);
}

async function requestAIJob(kind: AIJobKind) {
	const actionID = `${kind}-${Date.now()}`;
	aiBusyKind.value = kind;
	aiMessage.value = `Running ${kind} on ${selectedAssets.value.size > 0 ? `${selectedAssets.value.size} selected assets` : "the current indexed scope"}...`;
	try {
		const result = await api.aiJob(kind, {
			scope: selectedAssets.value.size > 0 ? "selected" : "current_indexed",
			asset_ids: Array.from(selectedAssets.value),
			limit: 54
		});
		aiLastResult.value = result;
		const summary = `${String(result.status ?? "completed")} · processed ${String(result.processed ?? 0)} / ${String(result.targets ?? 0)} · stored ${String(result.stored ?? 0)}${result.unsafe ? ` · ${String(result.unsafe)} need review` : ""}`;
		aiMessage.value = summary;
		aiActionHistory.value = [{ id: actionID, kind, status: String(result.status ?? "completed"), summary, created_at: new Date().toLocaleTimeString() }]
			.concat(aiActionHistory.value)
			.slice(0, 8);
    await refresh();
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		aiMessage.value = message;
		aiActionHistory.value = [{ id: actionID, kind, status: "failed", summary: message, created_at: new Date().toLocaleTimeString() }]
			.concat(aiActionHistory.value)
			.slice(0, 8);
	} finally {
		aiBusyKind.value = "";
	}
}

function requestAIModelAction(action: string) {
  if (["classify", "faces", "describe", "safety", "embed", "ocr"].includes(action)) {
    void requestAIJob(action as AIJobKind);
  }
}

function componentByKey(key: string): ComponentRecord | undefined {
  return components.value.find((component) => component.key === key);
}

function missingComponentsForAIAction(kind: AIJobKind): string[] {
  const requirements: Record<AIJobKind, string[]> = {
    classify: ["python-ai-venv", "torch-cuda", "torchvision", "efficientnet-b0"],
    faces: ["python-ai-venv", "opencv-yunet"],
    describe: ["python-ai-venv", "blip-base"],
    safety: ["python-ai-venv", "falconsai-nsfw"],
    embed: ["python-ai-venv", "openclip-vit-b32"],
    ocr: ["tesseract", "tessdata-eng"],
    transcribe: ["python-ai-venv", "asr-faster-whisper", "asr-ctranslate2"],
    "audio-analyze": ["python-ai-venv", "audio-librosa", "audio-soundfile"]
  };
  return requirements[kind].filter((key) => {
    const component = componentByKey(key);
    if (!component) return false;
    if (["installed", "user_provided"].includes(component.status)) return false;
    if (["failed", "disabled"].includes(component.status)) return true;
    return component.status === "missing" && Boolean(component.last_checked_at);
  });
}

async function runAssetAIAction(kind: AIJobKind, label: string, extra: Record<string, unknown> = {}) {
  if (!assetDetail.value) return;
  const mediaKind = assetDetail.value.asset.media_kind;
  const mediaCompatible =
    kind === "transcribe"
      ? mediaKind === "audio" || mediaKind === "video"
      : kind === "audio-analyze"
        ? mediaKind === "audio"
      : mediaKind === "photo";
  if (!mediaCompatible) {
    assetAIActionStatus.value[label] = {
      status: "skipped",
      summary: kind === "transcribe"
        ? "Transcription runs on audio/video assets only."
        : kind === "audio-analyze"
          ? "Audio feature analysis runs on audio assets only."
          : "This action currently runs on photo assets only."
    };
    return;
  }
  const missing = missingComponentsForAIAction(kind);
  if (missing.length > 0) {
    assetAIActionStatus.value[label] = {
      status: "missing",
      summary: `Missing component(s): ${missing.join(", ")}. Open Settings -> Components to check or provide them.`
    };
    settingsTab.value = "components";
    return;
  }
  assetAIActionStatus.value[label] = { status: "running", summary: "Starting bounded asset job..." };
  try {
    const result = await api.aiJob(kind, {
      scope: "selected",
      asset_id: assetDetail.value.asset.id,
      asset_ids: [assetDetail.value.asset.id],
      limit: 1,
      ...extra
    });
    const status = String(result.status ?? "completed");
    const summary = `${status} · processed ${String(result.processed ?? 0)} / ${String(result.targets ?? 1)} · stored ${String(result.stored ?? 0)}`;
    assetAIActionStatus.value[label] = { status, summary, job_id: String(result.job_id ?? "") };
    aiActionHistory.value = [{ id: `${label}-${Date.now()}`, kind, status, summary, created_at: new Date().toLocaleTimeString() }]
      .concat(aiActionHistory.value)
      .slice(0, 8);
    assetDetail.value = await api.asset(assetDetail.value.asset.id);
    jobs.value = await fetchVisibleJobs();
  } catch (err) {
    assetAIActionStatus.value[label] = { status: "failed", summary: err instanceof Error ? err.message : String(err) };
  }
}

async function runAllAssetAIActions() {
  for (const action of [
    { kind: "classify" as AIJobKind, label: "classification" },
    { kind: "faces" as AIJobKind, label: "faces" },
    { kind: "ocr" as AIJobKind, label: "ocr" },
    { kind: "safety" as AIJobKind, label: "safety" },
    { kind: "embed" as AIJobKind, label: "embedding" },
    { kind: "describe" as AIJobKind, label: "short_caption" }
  ]) {
    await runAssetAIAction(action.kind, action.label);
  }
}

function configureVectorStore() {
  settingsTab.value = "ai";
  vectorConfigHighlight.value = true;
  setActive("Settings");
  window.setTimeout(() => {
    vectorConfigHighlight.value = false;
  }, 3000);
}

async function runAIVectorSearch() {
  if (!aiVectorQuery.value.trim()) return;
  const response = await api.vectorSearch(aiVectorQuery.value.trim(), 12);
  aiVectorResults.value = asArray((response as Record<string, unknown>).results as Record<string, unknown>[]);
}

async function runUniversalSearch() {
  const query = universalSearchQ.value.trim();
  if (!query) {
    universalSearchMessage.value = "Enter a word, filename, date, camera, category, caption, tag, hash prefix, album, or track name.";
    universalSearchResults.value = [];
    universalSearchTrackResults.value = [];
    universalSearchPlaceResults.value = [];
    universalSearchBackend.value = "";
    universalSearchPlan.value = null;
    searchPageTotal.value = 0;
    return;
  }
  universalSearchMessage.value = "Searching indexed media and tracks...";
  const response = await api.search(query, 200);
  universalSearchResults.value = asArray(response.results);
  universalSearchTrackResults.value = asArray(response.tracks);
  universalSearchPlaceResults.value = asArray(response.places);
  universalSearchWarnings.value = asArray(response.warnings);
  universalSearchBackend.value = `${response.backend ?? "postgres_local"} · ${response.backend_mode ?? "metadata/local"}`;
  universalSearchPlan.value = response.plan ?? null;
  searchPageTotal.value = response.page?.total ?? universalSearchResults.value.length;
  universalSearchMessage.value = `${searchPageTotal.value} media matches (${universalSearchResults.value.length} shown) · ${universalSearchTrackResults.value.length} track results · ${universalSearchPlaceResults.value.length} place matches`;
}

async function parseUniversalSearch() {
  const query = universalSearchQ.value.trim();
  if (!query) return;
  universalSearchPlan.value = await api.searchParse(query);
}

async function planNaturalLanguageSearch() {
  const query = naturalSearchQ.value.trim();
  if (!query) {
    naturalSearchMessage.value = "Describe what you want to find in English or Russian.";
    return;
  }
  const plan = await api.searchPlan(query);
  universalSearchPlan.value = plan;
  universalSearchQ.value = plan.executable_query;
  naturalSearchMessage.value = `${plan.planner} produced: ${plan.executable_query}`;
}

async function runReadOnlySQLSearch() {
  const sql = sqlSearchQ.value.trim();
  if (!sql) {
    sqlSearchMessage.value = "Enter a SELECT query against cartolensia_search_* views.";
    sqlSearchResult.value = null;
    return;
  }
  sqlSearchMessage.value = "Running read-only query...";
  sqlSearchResult.value = await api.searchSQL(sql, 200);
  sqlSearchMessage.value = `${sqlSearchResult.value.count} rows returned from ${sqlSearchResult.value.views.join(", ")}`;
}

async function loadKnowledgeBase() {
  knowledgeLoading.value = true;
  try {
    const [factPage, relationPage] = await Promise.all([
      api.knowledgeFacts({
        q: knowledgeQ.value.trim(),
        predicate: knowledgePredicate.value.trim(),
        limit: 100,
        offset: 0
      }),
      api.knowledgeRelations({
        q: knowledgeQ.value.trim(),
        relation: knowledgeRelationFilter.value.trim(),
        limit: 100,
        offset: 0
      })
    ]);
    knowledgeFacts.value = factPage.facts;
    knowledgeRelations.value = relationPage.relations;
    knowledgeFactsTotal.value = factPage.page.total;
    knowledgeRelationsTotal.value = relationPage.page.total;
    knowledgeMessage.value = `${knowledgeFactsTotal.value} facts · ${knowledgeRelationsTotal.value} graph relations`;
  } catch (err) {
    knowledgeMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    knowledgeLoading.value = false;
  }
}

async function extractKnowledgeBatch() {
  knowledgeLoading.value = true;
  knowledgeMessage.value = "Extracting a bounded batch of facts and relations from local metadata...";
  try {
    knowledgeExtraction.value = await api.extractKnowledge(1000);
    knowledgeMessage.value = `Extraction upserted ${knowledgeExtraction.value.facts_inserted} facts and ${knowledgeExtraction.value.relations_inserted} relations.`;
    await loadKnowledgeBase();
  } catch (err) {
    knowledgeMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    knowledgeLoading.value = false;
  }
}

async function askKnowledgeBase() {
  const message = knowledgeChatInput.value.trim();
  if (!message) {
    knowledgeMessage.value = "Enter a question about your archive.";
    return;
  }
  knowledgeChatBusy.value = true;
  try {
    knowledgeChat.value = await api.knowledgeChat(message, knowledgeChatConversationID.value, 25);
    knowledgeChatConversationID.value = knowledgeChat.value.conversation_id ?? knowledgeChatConversationID.value;
    knowledgeMessage.value = `Knowledge chat used ${knowledgeChat.value.tool_calls.length} local tool calls.`;
  } catch (err) {
    knowledgeMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    knowledgeChatBusy.value = false;
  }
}

function openKnowledgeAsset(assetID?: string) {
  if (!assetID) return;
  void openAsset(assetID);
}

async function refreshFaceClusters() {
  faceClustersPayload.value = await api.faceClusters();
}

async function openFaceCluster(cluster: FaceCluster) {
  selectedFaceCluster.value = cluster;
  faceClusterNameDraft.value = cluster.label || "";
  faceGalleryMessage.value = "";
  const payload = await api.faceClusterAssets(cluster.id);
  faceClusterAssets.value = asArray(payload.assets);
  faceClusterDetections.value = asArray(payload.faces);
}

async function saveFaceClusterName() {
  if (!selectedFaceCluster.value) return;
  const updated = await api.updateFaceCluster(selectedFaceCluster.value.id, {
    label: faceClusterNameDraft.value.trim(),
    metadata: { local_only: true }
  });
  faceGalleryMessage.value = `Saved local face folder "${updated.label || "Unnamed"}".`;
  await refreshFaceClusters();
  await openFaceCluster(updated);
}

async function ignoreFaceDetection(face: FaceDetection) {
  await api.ignoreFaceDetection(face.id);
  faceGalleryMessage.value = "Face detection ignored locally; originals were not changed.";
  if (selectedFaceCluster.value) {
    await openFaceCluster(selectedFaceCluster.value);
  }
  await refreshFaceClusters();
}

function parsedTrackIds(input: string): string[] {
  const fromInput = input
    .split(/[,\n\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
  return fromInput;
}

async function loadVideoTrackVideoOptions() {
  const params = new URLSearchParams();
  params.set("media_kind", "video");
  params.set("limit", "200");
  params.set("sort", "mtime");
  const q = videoTrackVideoSearch.value.trim();
  if (q) {
    params.set("q", q);
  }
  const rows: Asset[] = await api.assets(params.toString()).catch(() => [] as Asset[]);
  if (videoTrackAssetId.value && !rows.some((asset) => asset.id === videoTrackAssetId.value)) {
    const selected = await api.asset(videoTrackAssetId.value).catch(() => null);
    if (selected?.asset.media_kind === "video") {
      rows.unshift(selected.asset);
    }
  }
  videoTrackVideoOptions.value = rows;
  if (!videoTrackAssetId.value && rows.length > 0) {
    selectVideoTrackVideo(rows[0], false);
  }
}

function selectVideoTrackVideo(asset: Asset, replaceSearch = true) {
  videoTrackAssetId.value = asset.id;
  if (replaceSearch) {
    videoTrackVideoSearch.value = assetName(asset);
  }
}

const selectedVideoTrackAsset = computed(() =>
  videoTrackVideoOptions.value.find((asset) => asset.id === videoTrackAssetId.value) ?? null
);

function videoOptionSummary(asset: Asset) {
  const location = firstAssetLocation(asset);
  const metadata = asset.metadata ?? {};
  const bits = [
    assetTimestampLabel(asset),
    formatDuration(Number(metadata.duration_seconds ?? 0)),
    [metadata.codec, metadata.width && metadata.height ? `${metadata.width}×${metadata.height}` : ""].filter(Boolean).join(" "),
    location?.relative_path ?? ""
  ].filter(Boolean);
  return bits.join(" · ");
}

const filteredVideoTrackTracks = computed(() => {
  const q = videoTrackTrackSearch.value.trim().toLowerCase();
  const source = videoTrackTrackOptions.value.length ? videoTrackTrackOptions.value : tracks.value;
  return source
    .filter((track) => !videoTrackSelectedTracks.value.some((selected) => selected.track_asset_id === track.track_asset_id))
    .filter((track) => !q || `${track.name} ${track.source_format ?? ""}`.toLowerCase().includes(q))
    .slice(0, 20);
});

function trackOptionSummary(track: TrackSummary) {
  const bits = [
    track.source_format,
    track.point_count ? `${track.point_count} points` : "",
    track.start_time && track.end_time ? `${track.start_time} → ${track.end_time}` : "",
    track.distance_m ? formatDistance(track.distance_m) : ""
  ].filter(Boolean);
  return bits.join(" · ");
}

function formatDistance(value: number) {
  return value >= 1000 ? `${(value / 1000).toFixed(2)} km` : `${Math.round(value)} m`;
}

function addVideoTrackTrack(track: TrackSummary) {
  if (!videoTrackSelectedTracks.value.some((selected) => selected.track_asset_id === track.track_asset_id)) {
    videoTrackSelectedTracks.value.push(track);
  }
  videoTrackTrackSearch.value = "";
  videoTrackIds.value = videoTrackSelectedTracks.value.map((item) => item.track_asset_id).join(", ");
}

function removeVideoTrackTrack(trackID: string) {
  videoTrackSelectedTracks.value = videoTrackSelectedTracks.value.filter((track) => track.track_asset_id !== trackID);
  videoTrackIds.value = videoTrackSelectedTracks.value.map((item) => item.track_asset_id).join(", ");
}

async function startGeoAlignSession() {
  geoAlignMessage.value = "Creating DB-only geotag alignment session...";
  const assetIDs = selectedAssets.value.size > 0 ? Array.from(selectedAssets.value) : [];
  geoAlignSession.value = await api.createGeoAlignSession({
    asset_ids: assetIDs,
    track_ids: parsedTrackIds(geoAlignTrackIds.value),
    limit: 54
  });
  geoAlignMessage.value = `${geoAlignSession.value.markers.length} media markers loaded. Write EXIF is disabled for strict read-only storage.`;
  await refreshGeoAlignMap();
}

async function nudgeGeoAlignMarker(assetId: string, dLat: number, dLon: number) {
  if (!geoAlignSession.value) return;
  const marker = geoAlignSession.value.markers.find((item) => item.asset_id === assetId);
  if (!marker) return;
  const updated = await api.moveGeoAlignMarker(
    geoAlignSession.value.id,
    assetId,
    marker.staged_lat + dLat,
    marker.staged_lon + dLon
  );
  marker.staged_lat = updated.staged_lat;
  marker.staged_lon = updated.staged_lon;
  marker.manual_lat = updated.manual_lat;
  marker.manual_lon = updated.manual_lon;
  marker.modified = updated.modified;
  geoAlignMessage.value = `${marker.name} moved locally. Apply saves the override to Cartolensia metadata only.`;
  await refreshGeoAlignMap();
}

async function resetGeoAlignMarker(marker: GeoAlignMarker) {
  if (!geoAlignSession.value) return;
  const updated = await api.resetGeoAlignMarker(geoAlignSession.value.id, marker.asset_id);
  const local = findGeoAlignMarker(marker.asset_id);
  if (local) Object.assign(local, updated);
  if (geoAlignPopupMarker.value?.asset_id === marker.asset_id) {
    geoAlignPopupMarker.value = { ...updated };
  }
  geoAlignMessage.value = `${updated.name} reset to its original or track-derived coordinate.`;
  await refreshGeoAlignMap();
}

function applyGeoAlignMarker(marker: GeoAlignMarker) {
  geoAlignPopupMarker.value = { ...marker };
  void applyGeoAlign();
}

function geoAlignCandidateLabel(candidate: Record<string, unknown>): string {
  const lat = Number(candidate.lat);
  const lon = Number(candidate.lon);
  const mode = String(candidate.mode ?? "track");
  const time = candidate.time ? ` · ${String(candidate.time)}` : "";
  return `${mode} ${Number.isFinite(lat) && Number.isFinite(lon) ? `${lat.toFixed(6)}, ${lon.toFixed(6)}` : ""}${time}`;
}

function showGeoAlignCandidate(candidate: Record<string, unknown>) {
  const lat = Number(candidate.lat);
  const lon = Number(candidate.lon);
  centerGeoAlignOn(lat, lon);
}

function geoAlignCoordinateText(lat?: number, lon?: number): string {
  if (typeof lat !== "number" || typeof lon !== "number") return "n/a";
  return `${lat.toFixed(6)}, ${lon.toFixed(6)}`;
}

function copyText(text: string) {
  void navigator.clipboard?.writeText(text);
}

function downloadTextFile(name: string, text: string) {
  const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function assetReverseCoordinate(): { lat: number; lon: number } | null {
  const detail = assetDetail.value;
  if (!detail) return null;
  const firstPlace = detail.places?.[0];
  if (firstPlace && Number.isFinite(firstPlace.lat) && Number.isFinite(firstPlace.lon)) {
    return { lat: firstPlace.lat, lon: firstPlace.lon };
  }
  const metadata = detail.asset.metadata ?? {};
  const lat = Number(metadata.lat ?? metadata.gps_lat);
  const lon = Number(metadata.lon ?? metadata.gps_lon);
  if (Number.isFinite(lat) && Number.isFinite(lon)) return { lat, lon };
  return null;
}

async function refreshAssetPlaces() {
  const detail = assetDetail.value;
  const coordinate = assetReverseCoordinate();
  if (!detail || !coordinate) {
    error.value = "This asset has no known coordinate to reverse-geocode.";
    return;
  }
  const online = Boolean(settings.value?.runtime_settings?.["search.online_geocoding"]);
  await api.reversePlace(coordinate.lat, coordinate.lon, online);
  assetDetail.value = await api.asset(detail.asset.id);
}

async function resetGeoAlign() {
  if (!geoAlignSession.value) return;
  geoAlignSession.value = await api.resetGeoAlignSession(geoAlignSession.value.id);
  geoAlignMessage.value = "Manual marker moves were reset.";
  closeGeoAlignPopup();
  await refreshGeoAlignMap();
}

async function applyGeoAlign() {
  if (!geoAlignSession.value) return;
  const result = await api.applyGeoAlignSession(geoAlignSession.value.id);
  geoAlignMessage.value = `Applied ${String(result.updated ?? 0)} DB-only geotag overrides. Originals were not modified.`;
  await refresh();
}

async function startVideoTrackPlayerSession() {
  const videoID = videoTrackAssetId.value || videoTrackVideoOptions.value[0]?.id || indexedVideoRows.value[0]?.asset_id || "";
  const selectedTrackIDs = videoTrackSelectedTracks.value.map((track) => track.track_asset_id);
  const trackIDs = selectedTrackIDs.length ? selectedTrackIDs : parsedTrackIds(videoTrackIds.value);
  if (!videoID) {
    videoTrackMessage.value = "Select a video. Tracks can be auto-selected from timestamp overlap when enabled.";
    return;
  }
  videoTrackSession.value = await api.createVideoTrackPlayerSession({
    video_asset_id: videoID,
    track_ids: trackIDs,
    timestamp_mode: videoTrackTimestampMode.value,
    offset_seconds: videoTrackOffsetSeconds.value
  });
  videoTrackAssetId.value = videoID;
  videoTrackIds.value = videoTrackSession.value.track_ids.join(", ");
  if (videoTrackSelectedTracks.value.length === 0 || trackIDs.length === 0) {
    videoTrackSelectedTracks.value = tracks.value.filter((track) =>
      videoTrackSession.value?.track_ids.includes(track.track_asset_id)
    );
  }
  const warnings = asArray(videoTrackSession.value.warnings);
  videoTrackMessage.value = [
    `Session ${videoTrackSession.value.id.slice(0, 8)} ready.`,
    selectedTrackIDs.length === 0 ? "Tracks were auto-selected from overlap." : "Playback positions are computed from video time plus offset.",
    ...warnings
  ].join(" ");
  await updateVideoTrackPosition(0, true);
  await nextTick();
  await renderVideoTrackMap();
}

async function updateVideoTrackPosition(timeMS: number, force = false) {
  if (!videoTrackSession.value) return;
  if (videoTrackPositionPending) return;
  const throttleMs = Math.max(0, numericRuntimeSetting("video_track_player.marker_throttle_ms", 250));
  const now = performance.now();
  if (!force && videoTrackMarkerThrottleAt && now - videoTrackMarkerThrottleAt < throttleMs && timeMS !== 0) return;
  videoTrackMarkerThrottleAt = now;
  videoTrackPositionPending = true;
  videoTrackSyncTimeMs.value = timeMS;
  try {
    videoTrackPosition.value = await api.videoTrackPlayerPosition(videoTrackSession.value.id, timeMS).catch((err) => ({
      error: err instanceof Error ? err.message : String(err)
    }));
    updateVideoTrackMarkerLayer();
    const current = videoTrackCurrentPosition.value;
    const lon = videoTrackPositionNumber(current, "lon");
    const lat = videoTrackPositionNumber(current, "lat");
    if (videoTrackMap && lon !== undefined && lat !== undefined) {
      const view = videoTrackMap.getView();
      if (view) {
        view.animate({ center: fromLonLat([lon, lat]), duration: force ? 220 : 140 });
      }
    }
  } finally {
    videoTrackPositionPending = false;
  }
}

async function startDryRun() {
  const storage = dryRunStorage.value || storages.value[0]?.name || "";
  const prefixes = adapterRelativePrefixes();
  const validation = validateAdapterRelativePrefixes(prefixes);
  if (!storage || validation) {
    error.value = validation || "Preview scan report requires a storage.";
    return;
  }
  const include_extensions = dryRunExtensions.value.split(",").map((item) => item.trim()).filter(Boolean);
  const result = await api.dryRunDiscovery({
    storage,
    prefixes,
    max_files: dryRunPreviewMaxFiles(),
    max_bytes: dryRunMaxBytes.value,
    include_extensions
  });
  dryRunReport.value = result.scan_run;
  await refresh();
  dryRunReport.value = await api.dryRunReport(result.job.id).catch(() => result.scan_run);
}

async function createAlbum() {
  const title = newAlbumTitle.value.trim();
  if (!title) return;
  const album = await api.createAlbum(title, newAlbumDescription.value);
  selectedAlbumId.value = album.id;
  if (selectedAssets.value.size > 0) {
    albumItems.value = await api.addAlbumItems(album.id, Array.from(selectedAssets.value));
    selectedAssets.value = new Set();
  }
  newAlbumTitle.value = "";
  newAlbumDescription.value = "";
  await refresh();
}

async function selectAlbum(id: string) {
  selectedAlbumId.value = id;
  if (!id) {
    albumItems.value = null;
    return;
  }
  albumItems.value = await api.albumItems(id);
}

function clearAlbumSelection() {
  selectedAlbumId.value = "";
  albumItems.value = null;
}

function showSelectedAlbumOnMap() {
  mapAlbumId.value = selectedAlbumId.value;
  setActive("Map");
}

function toggleAssetSelection(id: string) {
  const next = new Set(selectedAssets.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selectedAssets.value = next;
}

async function addSelectedToAlbum() {
  if (!selectedAlbumId.value || selectedAssets.value.size === 0) return;
  albumItems.value = await api.addAlbumItems(selectedAlbumId.value, Array.from(selectedAssets.value));
  selectedAssets.value = new Set();
  await selectAlbum(selectedAlbumId.value);
}

async function addCurrentAssetToAlbum() {
  if (!assetDetail.value || !selectedAlbumId.value) return;
  albumItems.value = await api.addAlbumItems(selectedAlbumId.value, [assetDetail.value.asset.id]);
  await refresh();
}

async function removeAlbumItem(assetId: string) {
  if (!selectedAlbumId.value) return;
  await api.removeAlbumItem(selectedAlbumId.value, assetId);
  await selectAlbum(selectedAlbumId.value);
}

async function openTrack(id: string) {
  selectedTrack.value = await api.gpsTrack(id);
  selectedTrackAltitude.value = await api.gpsTrackProfile(id, "altitude", 1200).catch(() => null);
  selectedTrackSpeed.value = await api.gpsTrackProfile(id, "speed", 1200).catch(() => null);
  mapTrackId.value = id;
  trackAssets.value = [];
  trackAssetsReason.value = "";
  setActive("GPS/KML Tracks");
  await refreshSelectedTrackMap();
}

async function findTrackAssets(id: string) {
  const result = await api.gpsTrackAssets(id, trackOffsetSeconds.value);
  trackAssets.value = asArray(result.assets);
  trackAssetsReason.value = result.reason ?? (trackAssets.value.length === 0 ? "No matching photo/video assets were found for this track." : "");
}

async function findNearbyTrackAssets(id: string) {
  const result = await api.gpsTrackNearbyAssets(id, 100);
  trackAssets.value = asArray(result.assets).map((item) => item.asset);
  trackAssetsReason.value = trackAssets.value.length === 0 ? "No geotagged photo/video assets were found within the default 100 m distance." : "";
}

async function openTrackAndFindAssets(id: string) {
  await openTrack(id);
  await findTrackAssets(id);
}

async function openTrackAndFindNearbyAssets(id: string) {
  await openTrack(id);
  await findNearbyTrackAssets(id);
}

async function snapTrackMedia(id: string) {
  await api.snapTrackMedia(id, trackOffsetSeconds.value);
  await refresh();
}

async function cancelJob(id: string) {
  await api.cancelJob(id);
  await refresh();
}

async function retryJob(id: string, force = false) {
  await api.retryJob(id, force);
  await refresh();
}

async function openJob(id: string) {
  selectedJob.value = await api.job(id);
  setActive("Jobs");
}

async function createToken() {
  const scopes = tokenScopes.value.split(",").map((scope) => scope.trim()).filter(Boolean);
  const result = await api.createToken(tokenName.value, scopes);
  tokenSecret.value = result.secret;
  apiTokens.value = await api.tokens();
}

async function changePassword() {
  await api.changePassword(oldPassword.value, newPassword.value);
  oldPassword.value = "";
  newPassword.value = "";
  await refreshAuth();
}

async function saveRuntimeSettings() {
  if (!settings.value) return;
  const result = await api.patchRuntimeSettings(settings.value.runtime_settings);
  settings.value.runtime_settings = (result.runtime_settings ?? settings.value.runtime_settings) as Record<string, unknown>;
  settingsMessage.value = "Runtime settings saved and applied where supported.";
}

function normalizedPlacePayload(place: PlaceCacheEntry, aliasesText?: string): PlaceCacheEntry {
  const aliases = aliasesText !== undefined
    ? aliasesText.split(",").map((alias) => alias.trim()).filter(Boolean)
    : asArray(place.aliases).map((alias) => String(alias).trim()).filter(Boolean);
  return {
    ...place,
    aliases,
    provider: place.provider || "local",
    display_name: place.display_name || place.name,
    source: place.source || "operator_cache",
    lat: Number(place.lat),
    lon: Number(place.lon),
    bbox: {
      min_lon: Number(place.bbox?.min_lon),
      min_lat: Number(place.bbox?.min_lat),
      max_lon: Number(place.bbox?.max_lon),
      max_lat: Number(place.bbox?.max_lat)
    }
  };
}

async function refreshPlaceCache() {
  try {
    const [summary, entries] = await Promise.all([
      api.searchPlaces(),
      api.places(placeCacheQuery.value)
    ]);
    searchPlaceCache.value = summary;
    editablePlaces.value = asArray(entries.places);
    placeCacheMessage.value = "Place cache refreshed.";
  } catch (err) {
    placeCacheMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function createPlaceFromDraft() {
  try {
    const created = await api.createPlace(normalizedPlacePayload(placeDraft.value, placeDraftAliases.value));
    placeCacheMessage.value = `Created ${created.display_name || created.name}.`;
    placeDraft.value = {
      name: "",
      display_name: "",
      provider: "local",
      country: "",
      region: "",
      city: "",
      road: "",
      aliases: [],
      lat: created.lat,
      lon: created.lon,
      bbox: { ...created.bbox },
      source: "operator_cache"
    };
    placeDraftAliases.value = "";
    await refreshPlaceCache();
  } catch (err) {
    placeCacheMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function savePlace(place: PlaceCacheEntry) {
  if (!place.id) return;
  try {
    const updated = await api.updatePlace(place.id, normalizedPlacePayload(place));
    placeCacheMessage.value = `Saved ${updated.display_name || updated.name}.`;
    await refreshPlaceCache();
  } catch (err) {
    placeCacheMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function deletePlace(place: PlaceCacheEntry) {
  if (!place.id) return;
  if (!window.confirm(`Delete cached place "${place.display_name || place.name}"? This only removes Cartolensia place metadata.`)) {
    return;
  }
  try {
    await api.deletePlace(place.id);
    placeCacheMessage.value = `Deleted ${place.display_name || place.name}.`;
    await refreshPlaceCache();
  } catch (err) {
    placeCacheMessage.value = err instanceof Error ? err.message : String(err);
  }
}

function setStorageDraftSMBField(key: keyof NonNullable<StorageConfig["smb"]>, value: string) {
  storageDraft.value.smb = { ...(storageDraft.value.smb ?? {}), [key]: value };
}

async function validateStorageDraft() {
  storageMessage.value = "Validating storage draft...";
  try {
    const result = await api.createStorage(storageDraft.value, true);
    storageMessage.value = JSON.stringify(result, null, 2);
  } catch (err) {
    storageMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function addRuntimeStorage() {
  storageMessage.value = "Adding storage to the active read-only registry...";
  try {
    const result = await api.createStorage(storageDraft.value, false);
    storageMessage.value = JSON.stringify(result, null, 2);
    storages.value = await api.storages();
  } catch (err) {
    storageMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function validateExistingStorage(name: string) {
  storageMessage.value = `Validating ${name}...`;
  try {
    const result = await api.validateStorage(name);
    storageMessage.value = JSON.stringify(result, null, 2);
  } catch (err) {
    storageMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function openFilePicker(target: string, kind: "file" | "folder" = "folder") {
  filePickerTarget.value = target;
  filePickerKind.value = kind;
  filePickerMessage.value = "";
  filePickerOpen.value = true;
  filePicker.value = await api.browseFiles().catch((err) => {
    filePickerMessage.value = err instanceof Error ? err.message : String(err);
    return null;
  });
  const preferredRoot = target.includes("model") || target.includes("cache") || target.includes("export") || target.startsWith("component:") ? "cartolensia" : "tmp";
  if (filePicker.value?.roots?.[preferredRoot]) {
    await chooseFilePickerRoot(preferredRoot);
  }
}

async function chooseFilePickerRoot(rootID: string) {
  filePickerRoot.value = rootID;
  filePickerPath.value = "";
  await loadFilePickerPath("");
}

async function loadFilePickerPath(path = filePickerPath.value) {
  if (!filePickerRoot.value) return;
  try {
    filePicker.value = await api.browseFiles(filePickerRoot.value, path, filePickerKind.value);
    filePickerPath.value = filePicker.value.current_path ?? "";
    filePickerMessage.value = "";
  } catch (err) {
    filePickerMessage.value = err instanceof Error ? err.message : String(err);
  }
}

function filePickerAbsolute(path = filePickerPath.value): string {
  const root = filePicker.value?.root?.path ?? "";
  if (!root) return path;
  const cleanRoot = root.endsWith("/") ? root.slice(0, -1) : root;
  return path ? `${cleanRoot}/${path}` : cleanRoot;
}

async function openFilePickerEntry(entry: { path: string; kind: string }) {
  if (entry.kind === "folder") {
    await loadFilePickerPath(entry.path);
    return;
  }
  if (filePickerKind.value === "file") {
    selectFilePickerPath(entry.path);
  }
}

function selectFilePickerPath(path = filePickerPath.value) {
  const selected = filePickerAbsolute(path);
  if (filePickerTarget.value === "storageDraft.root") {
    storageDraft.value.root = selected;
  } else if (filePickerTarget.value === "storageDraft.smb.credentials_file") {
    setStorageDraftSMBField("credentials_file", selected);
  } else if (filePickerTarget.value === "pending:storages.0.smb.credentials_file") {
    setPendingStorageSMBField(0, "credentials_file", selected);
  } else if (filePickerTarget.value === "pending:storages.0.root") {
    setPendingStorageField(0, "root", selected);
  } else if (filePickerTarget.value.startsWith("pending:")) {
    setPendingValue(filePickerTarget.value.slice("pending:".length), selected);
  } else if (filePickerTarget.value.startsWith("component:path:")) {
    componentPathDrafts.value[filePickerTarget.value.slice("component:path:".length)] = selected;
  } else if (filePickerTarget.value.startsWith("component:archive:")) {
    componentArchiveDrafts.value[filePickerTarget.value.slice("component:archive:".length)] = selected;
  }
  filePickerOpen.value = false;
}

async function refreshComponents() {
  const payload = await api.componentStatus();
  components.value = asArray(payload.components);
  componentRoot.value = payload.root;
  componentCounts.value = payload.counts ?? {};
}

function componentStatusClass(status: string): string {
  if (["installed", "user_provided"].includes(status)) return "ok";
  if (["downloading", "disabled"].includes(status)) return "warn";
  if (["failed", "missing"].includes(status)) return "bad";
  return "";
}

function componentSourceLabel(component: ComponentRecord): string {
  return [component.source_type, component.version].filter(Boolean).join(" · ") || "not checked";
}

const readinessGroups = computed(() => {
  const grouped = new Map<string, ReadinessPayload["checks"]>();
  for (const check of readiness.value?.checks ?? []) {
    const rows = grouped.get(check.category) ?? [];
    rows.push(check);
    grouped.set(check.category, rows);
  }
  return Array.from(grouped.entries()).map(([category, checks]) => ({ category, checks }));
});

function readinessStatusClass(status: string): string {
  if (status === "ok") return "ok";
  if (status === "warn") return "warn";
  if (status === "error") return "bad";
  return "";
}

function readinessIcon(status: string): string {
  if (status === "ok") return "bi-check-circle";
  if (status === "warn") return "bi-exclamation-triangle";
  if (status === "error") return "bi-x-circle";
  return "bi-info-circle";
}

async function runComponentCheck(key: string) {
  componentBusyKey.value = key;
  componentMessage.value = `Checking ${key}...`;
  try {
    const result = await api.checkComponent(key);
    componentMessage.value = `${key}: ${result.status}${result.error ? ` · ${result.error}` : ""}`;
    await refreshComponents();
    await loadComponentEvents(key);
  } catch (err) {
    componentMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    componentBusyKey.value = "";
  }
}

async function requestComponentDownload(key: string) {
  componentBusyKey.value = key;
  componentMessage.value = `Creating reviewed-download job for ${key}...`;
  try {
    const result = await api.downloadComponent(key);
    componentMessage.value = `${key}: ${result.status}${result.error ? ` · ${result.error}` : ""}`;
    await refreshComponents();
    await loadComponentEvents(key);
  } catch (err) {
    componentMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    componentBusyKey.value = "";
  }
}

async function provideComponentPath(key: string) {
  const path = componentPathDrafts.value[key]?.trim();
  if (!path) {
    componentMessage.value = `Choose or enter a local path for ${key}.`;
    return;
  }
  componentBusyKey.value = key;
  try {
    await api.provideComponentPath(key, path);
    componentMessage.value = `${key}: accepted user-provided path.`;
    await refreshComponents();
    await loadComponentEvents(key);
  } catch (err) {
    componentMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    componentBusyKey.value = "";
  }
}

async function provideComponentArchive(key: string) {
  const path = componentArchiveDrafts.value[key]?.trim();
  if (!path) {
    componentMessage.value = `Choose or enter a local archive for ${key}.`;
    return;
  }
  componentBusyKey.value = key;
  try {
    await api.provideComponentArchive(key, path);
    componentMessage.value = `${key}: archive imported under ${componentRoot.value}.`;
    await refreshComponents();
    await loadComponentEvents(key);
  } catch (err) {
    componentMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    componentBusyKey.value = "";
  }
}

async function setComponentEnabled(key: string, enabled: boolean) {
  componentBusyKey.value = key;
  try {
    await api.setComponentEnabled(key, enabled);
    componentMessage.value = `${key}: ${enabled ? "enabled" : "disabled"}.`;
    await refreshComponents();
    await loadComponentEvents(key);
  } catch (err) {
    componentMessage.value = err instanceof Error ? err.message : String(err);
  } finally {
    componentBusyKey.value = "";
  }
}

async function loadComponentEvents(key: string) {
  const payload = await api.componentEvents(key);
  componentEvents.value[key] = asArray(payload.events);
}

type RuntimeSettingSpec = { key: string; label: string; help: string; kind?: "text" | "number" | "boolean" };

const runtimeSettingTabs: Record<string, RuntimeSettingSpec[]> = {
  indexing: [
    { key: "indexing.default_max_files", label: "Default max files", help: "-1 means no file-count limit for normal indexing. Preview scans stay capped at 50.", kind: "number" },
    { key: "indexing.supported_extensions", label: "Supported discovery extensions", help: "Comma-separated default include list for discovery.", kind: "text" },
    { key: "indexing.hash_after_index", label: "Hash after indexing", help: "Default pipeline stage.", kind: "boolean" },
    { key: "indexing.metadata_after_index", label: "Extract metadata after indexing", help: "Default pipeline stage.", kind: "boolean" },
    { key: "indexing.previews_after_index", label: "Generate previews after indexing", help: "Default pipeline stage.", kind: "boolean" }
  ],
  discovery: [
    { key: "discovery.max_folder_workers", label: "Folder workers", help: "Bounded folder-worker pool for million-file discovery.", kind: "number" },
    { key: "discovery.max_file_workers", label: "File workers", help: "Bounded file-processing worker pool.", kind: "number" },
    { key: "discovery.folder_queue_depth", label: "Folder queue depth", help: "Upper bound for queued folder tasks.", kind: "number" }
  ],
  gps: [
    { key: "gps.track_arrow_interval_m", label: "Track direction arrow interval (m)", help: "Default 500 m. Set 0 to hide direction arrows.", kind: "number" }
  ],
  "video-track-player": [
    { key: "video_track_player.sync_mode", label: "Sync mode", help: "interval or smooth marker updates.", kind: "text" },
    { key: "video_track_player.interval_seconds", label: "Sync interval seconds", help: "Default 3 seconds for interval mode.", kind: "number" },
    { key: "video_track_player.marker_throttle_ms", label: "Marker throttle ms", help: "Limit marker refresh frequency to avoid UI freezes.", kind: "number" },
    { key: "video_track_player.auto_select_overlapping_tracks", label: "Auto-select overlapping tracks", help: "Use timestamp candidates to suggest tracks automatically.", kind: "boolean" },
    { key: "video_track_player.show_debug_overlay", label: "Show debug overlay", help: "Keep JSON debug hidden by default.", kind: "boolean" }
  ],
  preview: [
    { key: "preview.cache_max_bytes", label: "Preview cache max bytes", help: "Cleanup target for generated previews and thumbnails.", kind: "number" },
    { key: "gallery.default_view", label: "Default gallery view", help: "Preferred table/tile gallery mode.", kind: "text" }
  ],
  map: [
    { key: "map.cluster_radius_px", label: "Cluster radius px", help: "Screen distance used for marker clustering.", kind: "number" },
    { key: "map.tiles_enabled", label: "OSM tiles enabled", help: "On-demand tile proxy; no bulk prefetch.", kind: "boolean" }
  ],
  search: [
    { key: "search.default_limit", label: "Default search limit", help: "Bounded result count for broad universal searches.", kind: "number" },
    { key: "search.geocoder_mode", label: "Geocoder mode", help: "cache_only is the current safe default.", kind: "text" },
    { key: "search.online_geocoding", label: "Online geocoding enabled", help: "Currently off by default; provider calls must be user-triggered and cached.", kind: "boolean" },
    { key: "search.geocoder_provider", label: "Geocoder provider", help: "local_place_cache is active now; Nominatim-compatible providers are user-triggered and cached.", kind: "text" },
    { key: "search.geocoder_provider_url", label: "Geocoder provider URL", help: "Nominatim-compatible base URL used only for explicit reverse-geocode requests.", kind: "text" }
  ],
  transcoding: [
    { key: "transcode.session_ttl", label: "Transcode session TTL", help: "Cleanup age for cache-scoped HLS sessions.", kind: "text" }
  ]
};

const pendingSettingTabs: Record<string, RuntimeSettingSpec[]> = {
  metadata: [
    { key: "metadata.exif_enabled", label: "EXIF enabled", help: "Parse JPEG EXIF where supported.", kind: "boolean" },
    { key: "metadata.exif_gps_enabled", label: "EXIF GPS geotagging", help: "Write GPS coordinates into asset_geo.", kind: "boolean" },
    { key: "metadata.ffprobe_enabled", label: "ffprobe video metadata", help: "Best-effort video duration/dimension extraction.", kind: "boolean" },
    { key: "metadata.timezone_policy", label: "Timezone policy", help: "Timezone-less EXIF stays raw unless configured.", kind: "text" }
  ],
  gps: [
    { key: "gps.parse_gpx_enabled", label: "Parse GPX", help: "Enable GPX track parsing.", kind: "boolean" },
    { key: "gps.parse_kml_enabled", label: "Parse KML", help: "Enable KML line/point parsing.", kind: "boolean" },
    { key: "gps.parse_kmz_enabled", label: "Parse KMZ", help: "Enable zipped KML parsing.", kind: "boolean" },
    { key: "gps.parse_gpz_enabled", label: "Parse GPZ", help: "Try zipped GPX/KML track parsing.", kind: "boolean" },
    { key: "gps.synthetic_timestamps_for_notime", label: "Synthetic timestamps for no-time KML", help: "Allows geometry to display when source has no timestamps.", kind: "boolean" },
    { key: "gps.default_simplification_max_points", label: "Simplification max points", help: "Default points in UI track previews.", kind: "number" },
    { key: "gps.track_thumbnail_osm_background", label: "OSM background for track thumbnails", help: "Optional; falls back to dark local renderer.", kind: "boolean" },
    { key: "gps.track_thumbnail_width", label: "Track thumbnail width", help: "Generated thumbnail width.", kind: "number" },
    { key: "gps.track_thumbnail_height", label: "Track thumbnail height", help: "Generated thumbnail height.", kind: "number" },
    { key: "gps.default_nearby_distance_m", label: "Default nearby-media distance", help: "Meters used by track popup nearby query.", kind: "number" },
    { key: "gps.default_time_offset_seconds", label: "Default media time offset", help: "Seconds applied to time-overlap media lookup.", kind: "number" }
  ],
  map: [
    { key: "map.tile_source_template", label: "Tile source template", help: "YAML-bound tile source; requires restart.", kind: "text" },
    { key: "map.tile_attribution", label: "Tile attribution", help: "Attribution shown in the UI.", kind: "text" },
    { key: "map.tile_cache_dir", label: "Tile cache dir", help: "Must stay outside original storage.", kind: "text" },
    { key: "map.popup_gallery_limit", label: "Popup gallery limit", help: "Max sample assets in cluster popup.", kind: "number" }
  ],
  transcoding: [
    { key: "transcoding.ffmpeg_path", label: "ffmpeg path", help: "YAML-bound executable path.", kind: "text" },
    { key: "transcoding.ffprobe_path", label: "ffprobe path", help: "YAML-bound executable path.", kind: "text" },
    { key: "transcoding.hls_segment_duration", label: "HLS segment duration", help: "Seconds per segment.", kind: "number" },
    { key: "transcoding.cache_dir", label: "Transcode cache dir", help: "Must stay outside original storage.", kind: "text" },
    { key: "transcoding.max_concurrent_sessions", label: "Max concurrent sessions", help: "Bound write amplification.", kind: "number" },
    { key: "transcoding.hardware_preference", label: "Hardware preference", help: "cpu, nvidia, amd, intel.", kind: "text" }
  ]
};

function runtimeSpecsForTab(tab: string): RuntimeSettingSpec[] {
  return runtimeSettingTabs[tab] ?? [];
}

function pendingSpecsForTab(tab: string): RuntimeSettingSpec[] {
  return pendingSettingTabs[tab] ?? [];
}

function pluginSpecs(pluginId: string): RuntimeSettingSpec[] {
  const common: RuntimeSettingSpec[] = [
    { key: "enabled", label: "Enabled", help: "Plugin stays loaded when enabled.", kind: "boolean" as const },
    { key: "notes", label: "Operator notes", help: "Free-form local notes for this plugin.", kind: "text" as const }
  ];
  const byPlugin: Record<string, RuntimeSettingSpec[]> = {
    albums: [
      { key: "default_sort", label: "Default album sort", help: "name, sort_order, or created_at.", kind: "text" },
      { key: "show_virtual_warning", label: "Show virtual album warning", help: "Remind users albums never move originals.", kind: "boolean" }
    ],
    mapview: [
      { key: "default_cluster_distance_px", label: "Default cluster distance px", help: "Screen-space clustering distance.", kind: "number" },
      { key: "popup_gallery_limit", label: "Popup gallery limit", help: "Max assets shown in cluster popup.", kind: "number" }
    ],
    gpstracks: [
      { key: "default_nearby_distance_m", label: "Nearby media distance", help: "Default geotag distance for track media lookup.", kind: "number" },
      { key: "thumbnail_osm_background", label: "OSM track thumbnail background", help: "Falls back to dark local renderer.", kind: "boolean" }
    ],
    transcoding: [
      { key: "default_preset", label: "Default preset", help: "Preset selected for new video sessions.", kind: "text" },
      { key: "max_concurrent_sessions", label: "Max concurrent sessions", help: "Bound server-side transcoding load.", kind: "number" }
    ],
    "ai-base": [
      { key: "worker_endpoint", label: "Worker endpoint", help: "HTTP endpoint for local or remote AI sidecar, for example http://ai-node:19090.", kind: "text" },
      { key: "model_cache_dir", label: "Model cache dir", help: "Must stay outside original storage.", kind: "text" }
    ],
    "ai-classification": [
      { key: "taxonomy", label: "Taxonomy", help: "Category taxonomy namespace.", kind: "text" },
      { key: "confidence_threshold", label: "Confidence threshold", help: "Minimum score for automatic tags.", kind: "number" }
    ]
  };
  return common.concat(byPlugin[pluginId] ?? []);
}

function parsedPluginSettings(pluginId: string): Record<string, unknown> {
  try {
    return JSON.parse(pluginSettingText.value[pluginId] || "{}") as Record<string, unknown>;
  } catch {
    return {};
  }
}

function pluginSettingValue(pluginId: string, key: string): string {
  const settings = parsedPluginSettings(pluginId);
  const value = settings[key];
  if (value === undefined || value === null) return "";
  return String(value);
}

function setPluginSettingValue(pluginId: string, key: string, value: string | number | boolean) {
  const settings = parsedPluginSettings(pluginId);
  settings[key] = value;
  pluginSettingText.value[pluginId] = JSON.stringify(settings, null, 2);
}

function setRuntimeSetting(key: string, value: string) {
  if (!settings.value) return;
  const current = settings.value.runtime_settings[key];
  if (typeof current === "number") {
    const parsed = Number(value);
    settings.value.runtime_settings[key] = Number.isFinite(parsed) ? parsed : current;
    return;
  }
  if (typeof current === "boolean") {
    settings.value.runtime_settings[key] = value === "true";
    return;
  }
  settings.value.runtime_settings[key] = value;
}

function pendingObject(path: string): Record<string, unknown> {
  if (!path) return pendingConfig.value;
  const parts = path.split(".");
  let current = pendingConfig.value;
  for (const part of parts) {
    if (!current[part] || typeof current[part] !== "object" || Array.isArray(current[part])) {
      current[part] = {};
    }
    current = current[part] as Record<string, unknown>;
  }
  return current;
}

function pendingValue(path: string, fallback = ""): string {
  const parts = path.split(".");
  let current: unknown = pendingConfig.value;
  for (const part of parts) {
    if (!current || typeof current !== "object") return fallback;
    current = (current as Record<string, unknown>)[part];
  }
  if (Array.isArray(current)) return current.join(",");
  if (current === undefined || current === null) return fallback;
  return String(current);
}

function setPendingValue(path: string, value: string | number | boolean) {
  const parts = path.split(".");
  const key = parts.pop();
  if (!key) return;
  pendingObject(parts.join("."))[key] = value;
}

function setPendingStorageField(index: number, key: string, value: string) {
  const storages = Array.isArray(pendingConfig.value.storages) ? pendingConfig.value.storages as Record<string, unknown>[] : [];
  while (storages.length <= index) storages.push({});
  storages[index] = { ...storages[index], [key]: value };
  pendingConfig.value.storages = storages;
}

function pendingStorageField(index: number, key: string): string {
  const storages = Array.isArray(pendingConfig.value.storages) ? pendingConfig.value.storages as Record<string, unknown>[] : [];
  return String(storages[index]?.[key] ?? "");
}

function setPendingStorageSMBField(index: number, key: string, value: string) {
  const storages = Array.isArray(pendingConfig.value.storages) ? pendingConfig.value.storages as Record<string, unknown>[] : [];
  while (storages.length <= index) storages.push({});
  const smb = storages[index].smb && typeof storages[index].smb === "object" ? storages[index].smb as Record<string, unknown> : {};
  storages[index] = { ...storages[index], smb: { ...smb, [key]: value } };
  pendingConfig.value.storages = storages;
}

function pendingStorageSMBField(index: number, key: string): string {
  const storages = Array.isArray(pendingConfig.value.storages) ? pendingConfig.value.storages as Record<string, unknown>[] : [];
  const smb = storages[index]?.smb;
  if (!smb || typeof smb !== "object" || Array.isArray(smb)) return "";
  return String((smb as Record<string, unknown>)[key] ?? "");
}

async function savePendingSettings() {
  settingsMessage.value = "Saving pending YAML-bound settings...";
  const result = await api.patchPendingSettings(pendingConfig.value);
  if (settings.value) settings.value.pending_settings = result;
  settingsMessage.value = "Pending YAML saved. Restart Cartolensia to apply these YAML-bound changes.";
}

async function clearPendingSettings() {
  const result = await api.clearPendingSettings();
  if (settings.value) settings.value.pending_settings = result;
  pendingConfig.value = settings.value ? JSON.parse(JSON.stringify(settings.value.effective ?? {})) : {};
  settingsMessage.value = "Pending YAML changes cleared.";
}

async function savePluginSettings(pluginID: string) {
  const raw = pluginSettingText.value[pluginID] || "{}";
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(raw) as Record<string, unknown>;
  } catch (err) {
    settingsMessage.value = `Invalid JSON for ${pluginID}: ${err instanceof Error ? err.message : String(err)}`;
    return;
  }
  await api.patchPluginSettings(pluginID, parsed);
  settingsMessage.value = `Saved settings for ${pluginID}.`;
}

async function createDBExport() {
  dbExportMessage.value = "Creating metadata export under the configured Cartolensia cache...";
  try {
    const result = await api.dbExport();
    dbExportMessage.value = `Created ${result.id}`;
    dbExports.value = await api.dbExports();
  } catch (err) {
    dbExportMessage.value = err instanceof Error ? err.message : String(err);
  }
}

async function openFolder(path: string) {
  explorerPath.value = path;
  monthFilter.value = "";
  setActive("Explorer");
  await refresh();
}

async function loadMoreExplorerFiles() {
  if (!explorer.value || explorerLoadingMore.value || explorerLoadingAll.value || !explorerHasMoreFiles.value) return;
  explorerLoadingMore.value = true;
  error.value = "";
  try {
    await appendExplorerFiles(explorerPageLimit);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    explorerLoadingMore.value = false;
  }
}

async function loadAllExplorerFiles() {
  if (!explorer.value || explorerLoadingAll.value || !explorerHasMoreFiles.value) return;
  explorerLoadingAll.value = true;
  error.value = "";
  try {
    while (explorerHasMoreFiles.value && !explorerQ.value.trim()) {
      const before = explorerLoadedFiles.value;
      await appendExplorerFiles(explorerBulkPageLimit);
      if (explorerLoadedFiles.value <= before) break;
      await nextTick();
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    explorerLoadingAll.value = false;
  }
}

async function appendExplorerFiles(limit: number) {
  const current = explorer.value;
  if (!current) return;
  const requestPath = explorerPath.value;
  const requestQuery = explorerQueryString({ limit, offset: asArray(current.files).length });
  const nextPage = await api.explorerFolders(requestPath, requestQuery);
  if (explorerPath.value !== requestPath) return;
  const existing = asArray(explorer.value?.files);
  const seen = new Set(existing.map((file) => `${file.asset_id}\x00${file.relative_path}`));
  const appended = asArray(nextPage.files).filter((file) => {
    const key = `${file.asset_id}\x00${file.relative_path}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  explorer.value = {
    ...nextPage,
    folders: asArray(explorer.value?.folders),
    files: [...existing, ...appended]
  };
  rows.value = asArray(explorer.value.files);
}

async function openAsset(id: string, options: { seekMs?: number | null } = {}) {
  id = id.trim();
  if (!id) {
    error.value = "Cannot open asset: missing asset id.";
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    assetDetailSeekMs.value = options.seekMs ?? null;
    const detail = await api.asset(id);
    assetDetail.value = detail;
    assetRelated.value = null;
    assetAIActionStatus.value = {};
    streamOptions.value = null;
    setActive("Asset Detail", false);
    const url = new URL(window.location.href);
    url.searchParams.set("page", "asset-detail");
    url.searchParams.set("asset_id", id);
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
    await nextTick();
    await applyAssetDetailSeek();
    void hydrateAssetDetailExtras(id, detail.asset.media_kind);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

async function hydrateAssetDetailExtras(id: string, mediaKind: string) {
  const [related, streams] = await Promise.all([
    api.assetRelated(id).catch(() => null),
    mediaKind === "video" ? api.streamOptions(id).catch(() => null) : Promise.resolve(null)
  ]);
  if (assetDetail.value?.asset.id !== id) return;
  assetRelated.value = related;
  streamOptions.value = streams;
}

async function openPublicAsset(id: string, options: { seekMs?: number | null } = {}) {
  loading.value = true;
  error.value = "";
  try {
    assetDetailSeekMs.value = options.seekMs ?? null;
    assetDetail.value = await api.publicAsset(id);
    assetRelated.value = null;
    assetAIActionStatus.value = {};
    streamOptions.value = null;
    setActive("Asset Detail", false);
    const url = new URL(window.location.href);
    url.searchParams.set("page", "asset-detail");
    url.searchParams.set("asset_id", id);
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
    await nextTick();
    await applyAssetDetailSeek();
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

async function setAssetPublic(value: boolean) {
  if (!assetDetail.value || !principal.value) return;
  const assetID = assetDetail.value.asset.id;
  const result = await api.setAssetVisibility(assetID, { public: value });
  assetDetail.value = await api.asset(assetID);
  if (result.public) {
    error.value = "Asset is now visible in the anonymous public gallery.";
  } else {
    error.value = "Asset is no longer public.";
  }
}

async function openAssetOCR(assetID: string, blockID: string) {
  await openAsset(assetID);
  if (blockID) {
    selectedAssetOCRId.value = blockID;
    showAssetOCRBoxes.value = true;
  }
}

function openAssetOCRLink(event: MouseEvent, assetID: string, blockID: string) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
    return;
  }
  event.preventDefault();
  void openAssetOCR(assetID, blockID);
}

async function openGalleryAssetDetail(id: string) {
  const seekMs = captureCurrentPlaybackTimeMs();
  closeGallery();
  await openAsset(id, { seekMs });
}

function canCancel(job: Job): boolean {
  return job.status === "queued" || job.status === "running" || job.status === "cancel_requested";
}

function canRetry(job: Job): boolean {
  return job.status === "failed" || job.status === "canceled";
}

function progressPercent(job: Job): number {
  if (!job.progress_total || job.progress_total <= 0) return 0;
  return Math.min(100, Math.round((job.progress_current / job.progress_total) * 100));
}

function jobCounterSummary(job: Job): string {
  const counters = job.counters ?? {};
  const parts: string[] = [];
  if (counters.folders_scanned || counters.folders_queued) {
    parts.push(`folders ${counters.folders_scanned ?? 0}/${counters.folders_queued ?? "?"}`);
  }
  if (counters.files_seen) {
    parts.push(`files seen ${counters.files_seen}`);
  }
  if (counters.files_returned) {
    parts.push(`matched ${counters.files_returned}`);
  }
  if (counters.files_skipped) {
    parts.push(`skipped ${counters.files_skipped}`);
  }
  if (counters.scanned || counters.created || counters.updated) {
    parts.push(`indexed ${counters.scanned ?? 0}`);
  }
  if (counters.bytes) {
    parts.push(`${formatBytes(counters.bytes)}`);
  }
  return parts.join(" · ");
}

const mapFeatures = computed(() => {
  const features = mapData.value?.features;
  return Array.isArray(features) ? (features as Array<Record<string, unknown>>) : [];
});

const mapWarnings = computed(() => asArray(mapStatus.value?.warnings as string[] | null | undefined));
const selectedPipelineStorage = computed(() => {
  const selected = pipelineStorage();
  if (selected === "all") {
    return { name: "All configured storages", kind: "mixed", root: "all registered roots", mode: "per-storage read-only policy" } as StorageConfig;
  }
  return storages.value.find((storage) => storage.name === selected) ?? storages.value[0];
});
const mapFeatureSummary = computed(() => {
  const clustering = String(mapData.value?.clustering ?? mapStatus.value?.clustering ?? "none");
  const tiles = String(mapStatus.value?.base_tiles_note ?? "Vector map layers are active.");
  return `${mapFeatures.value.length} features · ${clustering} clustering · ${tiles}`;
});

function mapQuery(): Record<string, string | number | boolean> {
  const zoom = Math.round(olMap?.getView().getZoom() ?? 10);
  const width = mapElement.value?.clientWidth ?? 1024;
  const height = mapElement.value?.clientHeight ?? 768;
  const markerPx = 24;
  const query: Record<string, string | number | boolean> = {
    cluster: mapCluster.value,
    zoom,
    width_px: width,
    height_px: height,
    marker_px: markerPx,
    cluster_distance_px: markerPx * 2
  };
  if (mapMediaKind.value) query.media_kind = mapMediaKind.value;
  if (mapAlbumId.value) query.album_id = mapAlbumId.value;
  if (mapTrackId.value) query.track_id = mapTrackId.value;
  return query;
}

async function refreshMap() {
  mapData.value = await api.map(mapQuery());
  mapStatus.value = await api.mapStatus().catch(() => mapStatus.value);
  await nextTick();
  renderOpenLayers();
}

function mapFeatureAsset(feature: { get: (name: string) => unknown }) {
  const id = String(feature.get("asset_id") ?? feature.get("id") ?? "");
  return {
    id,
    name: String(feature.get("name") ?? feature.get("title") ?? id),
    media_kind: String(feature.get("media_kind") ?? feature.get("kind") ?? feature.get("asset_type") ?? "asset"),
    preview_url: String(feature.get("preview_url") ?? ""),
    detail_url: String(feature.get("detail_url") ?? ""),
    original_url: String(feature.get("original_url") ?? "")
  };
}

function normalizeMapAsset(value: unknown) {
  const item = (value ?? {}) as Record<string, unknown>;
  const id = String(item.asset_id ?? item.id ?? "");
  return {
    id,
    name: String(item.name ?? item.title ?? id),
    media_kind: String(item.media_kind ?? item.asset_type ?? "asset"),
    preview_url: String(item.preview_url ?? ""),
    detail_url: String(item.detail_url ?? ""),
    original_url: String(item.original_url ?? "")
  };
}

function assetToMapPopupAsset(asset: AssetDetail["asset"]) {
  const location = asArray(asset.locations)[0];
  return {
    id: asset.id,
    name: asset.display_name,
    media_kind: asset.media_kind,
    preview_url: `/api/v1/media/${asset.id}/${asset.media_kind === "track" ? "track-thumbnail" : "preview"}`,
    detail_url: `/?page=asset-detail&asset_id=${asset.id}`,
    original_url: `/api/v1/media/${asset.id}/original`,
    relative_path: location?.relative_path
  };
}

function faceBoxStyle(face: FaceDetection | Record<string, unknown>, detail: AssetDetail): Record<string, string> {
  const metadata = detail.asset.metadata ?? {};
  const faces = asArray(detail.face_detections as unknown[]) as Array<Record<string, unknown>>;
  const x = Number((face as Record<string, unknown>).x ?? 0);
  const y = Number((face as Record<string, unknown>).y ?? 0);
  const width = Number((face as Record<string, unknown>).width ?? 0);
  const height = Number((face as Record<string, unknown>).height ?? 0);
  const imageWidth = Number(
    metadata.width ??
      metadata.image_width ??
      metadata.ImageWidth ??
      metadata["ExifImageWidth"] ??
      Math.max(...faces.map((item) => Number(item.x ?? 0) + Number(item.width ?? 0)), x + width, 1)
  );
  const imageHeight = Number(
    metadata.height ??
      metadata.image_height ??
      metadata.ImageHeight ??
      metadata["ExifImageHeight"] ??
      Math.max(...faces.map((item) => Number(item.y ?? 0) + Number(item.height ?? 0)), y + height, 1)
  );
  return {
    left: `${Math.max(0, (x / Math.max(1, imageWidth)) * 100)}%`,
    top: `${Math.max(0, (y / Math.max(1, imageHeight)) * 100)}%`,
    width: `${Math.max(0.5, (width / Math.max(1, imageWidth)) * 100)}%`,
    height: `${Math.max(0.5, (height / Math.max(1, imageHeight)) * 100)}%`
  };
}

function ocrBoxStyle(block: OCRBlock | Record<string, unknown>, detail: AssetDetail): Record<string, string> {
  const metadata = detail.asset.metadata ?? {};
  const blocks = asArray(detail.ocr_blocks as unknown[]) as Array<Record<string, unknown>>;
  const x = Number((block as Record<string, unknown>).x ?? 0);
  const y = Number((block as Record<string, unknown>).y ?? 0);
  const width = Number((block as Record<string, unknown>).width ?? 0);
  const height = Number((block as Record<string, unknown>).height ?? 0);
  const imageWidth = Number(
    metadata.width ??
      metadata.image_width ??
      metadata.ImageWidth ??
      metadata["ExifImageWidth"] ??
      Math.max(...blocks.map((item) => Number(item.x ?? 0) + Number(item.width ?? 0)), x + width, 1)
  );
  const imageHeight = Number(
    metadata.height ??
      metadata.image_height ??
      metadata.ImageHeight ??
      metadata["ExifImageHeight"] ??
      Math.max(...blocks.map((item) => Number(item.y ?? 0) + Number(item.height ?? 0)), y + height, 1)
  );
  return {
    left: `${Math.max(0, (x / Math.max(1, imageWidth)) * 100)}%`,
    top: `${Math.max(0, (y / Math.max(1, imageHeight)) * 100)}%`,
    width: `${Math.max(0.5, (width / Math.max(1, imageWidth)) * 100)}%`,
    height: `${Math.max(0.5, (height / Math.max(1, imageHeight)) * 100)}%`
  };
}

function faceMetadata(face: FaceDetection | Record<string, unknown>): Record<string, unknown> {
  return (((face as Record<string, unknown>).metadata ?? {}) as Record<string, unknown>);
}

function faceIgnored(face: FaceDetection | Record<string, unknown>): boolean {
  return Boolean(faceMetadata(face).ignored);
}

function faceDeleted(face: FaceDetection | Record<string, unknown>): boolean {
  return Boolean(faceMetadata(face).deleted);
}

function faceDisplayName(face: FaceDetection | Record<string, unknown>): string {
  const metadata = faceMetadata(face);
  return String(metadata.label ?? metadata.name ?? (face as Record<string, unknown>).cluster_id ?? "Unassigned face");
}

function assetImageDimensions(detail: AssetDetail): { width: number; height: number } {
  const metadata = detail.asset.metadata ?? {};
  const faces = asArray(detail.face_detections as unknown[]) as Array<Record<string, unknown>>;
  const width = Number(
    metadata.width ??
      metadata.image_width ??
      metadata.ImageWidth ??
      metadata.ExifImageWidth ??
      Math.max(...faces.map((item) => Number(item.x ?? 0) + Number(item.width ?? 0)), 1)
  );
  const height = Number(
    metadata.height ??
      metadata.image_height ??
      metadata.ImageHeight ??
      metadata.ExifImageHeight ??
      Math.max(...faces.map((item) => Number(item.y ?? 0) + Number(item.height ?? 0)), 1)
  );
  return { width: Math.max(1, width), height: Math.max(1, height) };
}

function faceDraftBoxStyle(detail: AssetDetail): Record<string, string> {
  if (!newFaceDraftBox.value) return {};
  const dimensions = assetImageDimensions(detail);
  return {
    left: `${(newFaceDraftBox.value.x / dimensions.width) * 100}%`,
    top: `${(newFaceDraftBox.value.y / dimensions.height) * 100}%`,
    width: `${(newFaceDraftBox.value.width / dimensions.width) * 100}%`,
    height: `${(newFaceDraftBox.value.height / dimensions.height) * 100}%`
  };
}

function faceClusterPreviewURL(cluster: FaceCluster): string {
  const faceID = String(cluster.representative_face_id ?? "");
  if (faceID) return `/api/v1/faces/detections/${encodeURIComponent(faceID)}/thumbnail`;
  const assetID = String(cluster.metadata?.representative_asset_id ?? "");
  return assetID ? `/api/v1/media/${assetID}/preview` : "";
}

function faceClusterPreviewTitle(cluster: FaceCluster): string {
  return String(cluster.metadata?.representative_asset_name ?? cluster.label ?? "Face folder preview");
}

function faceCardPreviewURL(face: FaceDetection): string {
  return `/api/v1/faces/detections/${encodeURIComponent(face.id)}/thumbnail`;
}

async function openFaceFromDetection(face: FaceDetection | Record<string, unknown>) {
  const faceRecord = face as FaceDetection;
  const clusterID = String(faceRecord.cluster_id || `asset:${faceRecord.asset_id}`);
  setActive("Face Gallery");
  await refreshFaceClusters();
  const cluster = faceClusters.value.find((candidate) => candidate.id === clusterID) ?? {
    id: clusterID,
    label: faceDisplayName(faceRecord),
    face_count: 0,
    asset_count: 0,
    ignored_count: 0,
    metadata: { provisional: true }
  };
  await openFaceCluster(cluster);
}

function assetImagePoint(event: PointerEvent, detail: AssetDetail): { x: number; y: number } {
  const target = event.currentTarget as HTMLElement;
  const rect = target.getBoundingClientRect();
  const dimensions = assetImageDimensions(detail);
  const x = Math.max(0, Math.min(dimensions.width, ((event.clientX - rect.left) / Math.max(1, rect.width)) * dimensions.width));
  const y = Math.max(0, Math.min(dimensions.height, ((event.clientY - rect.top) / Math.max(1, rect.height)) * dimensions.height));
  return { x, y };
}

function startFaceDraft(event: PointerEvent) {
  if (!faceAddMode.value || !assetDetail.value || assetDetail.value.asset.media_kind !== "photo") return;
  event.preventDefault();
  const point = assetImagePoint(event, assetDetail.value);
  faceDraftStart = point;
  newFaceDraftBox.value = { x: point.x, y: point.y, width: 1, height: 1 };
}

function updateFaceDraft(event: PointerEvent) {
  if (!faceAddMode.value || !assetDetail.value || !faceDraftStart || !newFaceDraftBox.value) return;
  const point = assetImagePoint(event, assetDetail.value);
  newFaceDraftBox.value = {
    x: Math.min(faceDraftStart.x, point.x),
    y: Math.min(faceDraftStart.y, point.y),
    width: Math.max(1, Math.abs(point.x - faceDraftStart.x)),
    height: Math.max(1, Math.abs(point.y - faceDraftStart.y))
  };
}

function finishFaceDraft() {
  faceDraftStart = null;
}

async function saveManualFace() {
  if (!assetDetail.value || !newFaceDraftBox.value) return;
  const box = newFaceDraftBox.value;
  if (box.width < 3 || box.height < 3) {
    error.value = "Draw a larger face rectangle before saving.";
    return;
  }
  const face = await api.createFaceDetection({
    asset_id: assetDetail.value.asset.id,
    x: box.x,
    y: box.y,
    width: box.width,
    height: box.height,
    confidence: newFaceConfidence.value,
    label: newFaceName.value.trim() || "Manual face"
  });
  selectedAssetFaceId.value = face.id;
  faceAddMode.value = false;
  newFaceDraftBox.value = null;
  newFaceName.value = "";
  assetDetail.value = await api.asset(assetDetail.value.asset.id);
  await refreshFaceClusters();
}

async function deleteFaceDetection(face: FaceDetection) {
  await api.deleteFaceDetection(face.id);
  if (selectedAssetFaceId.value === face.id) selectedAssetFaceId.value = "";
  if (assetDetail.value) {
    assetDetail.value = await api.asset(assetDetail.value.asset.id);
  }
  await refreshFaceClusters();
}

async function deleteOCRBlock(block: OCRBlock) {
  await api.deleteOCRBlock(block.asset_id, block.id);
  if (selectedAssetOCRId.value === block.id) selectedAssetOCRId.value = "";
  if (assetDetail.value) {
    assetDetail.value = await api.asset(assetDetail.value.asset.id);
  }
  await refresh();
}

async function mapLoadTrackTimeAssets() {
  const popup = mapPopup.value;
  if (!popup?.track_id) return;
  popup.summary = "Loading media taken during this track...";
  const result = await api.gpsTrackAssets(popup.track_id, trackOffsetSeconds.value);
  if (mapPopup.value?.track_id === popup.track_id) {
    mapPopup.value.assets = asArray(result.assets).map(assetToMapPopupAsset);
    mapPopup.value.summary = mapPopup.value.assets.length > 0
      ? `${mapPopup.value.assets.length} photo/video assets overlap this track by time.`
      : (result.reason ?? "No matching photo/video assets overlap this track by time.");
  }
}

async function mapLoadTrackNearbyAssets() {
  const popup = mapPopup.value;
  if (!popup?.track_id) return;
  const distance = popup.nearby_distance_m && popup.nearby_distance_m > 0 ? popup.nearby_distance_m : 100;
  popup.summary = `Loading geotagged media within ${distance} m of this track...`;
  const result = await api.gpsTrackNearbyAssets(popup.track_id, distance);
  if (mapPopup.value?.track_id === popup.track_id) {
    mapPopup.value.assets = asArray(result.assets).map((item) => {
      const asset = assetToMapPopupAsset(item.asset);
      asset.name = `${asset.name} · ${Math.round(item.distance_m)} m`;
      return asset;
    });
    mapPopup.value.summary = `${mapPopup.value.assets.length} geotagged media assets are within ${distance} m.`;
  }
}

function showOnlyMapTrack(trackId: string) {
  mapTrackId.value = trackId;
  void refreshMap();
}

function zoomMapToCluster(bbox: Record<string, number> | undefined, centroid?: Record<string, number>) {
  if (!olMap || !bbox) return;
  const minLon = Number(bbox.min_lon);
  const minLat = Number(bbox.min_lat);
  const maxLon = Number(bbox.max_lon);
  const maxLat = Number(bbox.max_lat);
  const hasArea = Number.isFinite(minLon) && Number.isFinite(maxLon) && Number.isFinite(minLat) && Number.isFinite(maxLat)
    && (Math.abs(maxLon - minLon) > 0.00001 || Math.abs(maxLat - minLat) > 0.00001);
  if (!olMap || !hasArea) return;
  const zoom = olMap.getView().getZoom() ?? 10;
  const centerLon = centroid && Number.isFinite(Number(centroid.lon)) ? Number(centroid.lon) : (minLon + maxLon) / 2;
  const centerLat = centroid && Number.isFinite(Number(centroid.lat)) ? Number(centroid.lat) : (minLat + maxLat) / 2;
  olMap.getView().animate({ center: fromLonLat([centerLon, centerLat]), zoom: Math.min(19, zoom + 3), duration: 180 });
  window.setTimeout(() => void refreshMap(), 220);
}

function openMapPopup(feature: { get: (name: string) => unknown }, coordinate?: number[]) {
  const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
  if (kind === "cluster") {
    const count = Number(feature.get("count") ?? 0);
    const samples = asArray(feature.get("sample_assets") as Array<Record<string, unknown>> | null | undefined).map(normalizeMapAsset);
    const centroid = (feature.get("centroid") ?? {}) as Record<string, number>;
    mapPopup.value = {
      kind: "cluster",
      title: `${count} assets near ${Number(centroid.lat ?? 0).toFixed(5)}, ${Number(centroid.lon ?? 0).toFixed(5)}`,
      summary: `${feature.get("photos_count") ?? 0} photos · ${feature.get("videos_count") ?? 0} videos · ${feature.get("tracks_count") ?? 0} tracks`,
      assets: samples,
      bbox: (feature.get("bbox") ?? {}) as Record<string, number>,
      count
    };
    if (coordinate && mapOverlay) mapOverlay.setPosition(coordinate);
    return;
  }
  if (kind === "track") {
    const id = String(feature.get("id") ?? "");
    if (!id) return;
    const lonLat = coordinate ? toLonLat(coordinate) : [0, 0];
    mapPopup.value = {
      kind: "track",
      title: String(feature.get("name") ?? id),
      summary: `${String(feature.get("source_format") ?? "track")} · ${feature.get("point_count") ?? 0} points`,
      assets: [],
      track_id: id,
      track_info: null,
      track_info_loading: true,
      nearby_distance_m: 100
    };
    if (coordinate && mapOverlay) mapOverlay.setPosition(coordinate);
    api.gpsTrackPointInfo(id, Number(lonLat[1]), Number(lonLat[0]))
      .then((info) => {
        if (mapPopup.value?.track_id === id) {
          mapPopup.value.track_info = info;
          mapPopup.value.track_info_loading = false;
        }
      })
      .catch((err) => {
        if (mapPopup.value?.track_id === id) {
          mapPopup.value.summary = err instanceof Error ? err.message : String(err);
          mapPopup.value.track_info_loading = false;
        }
      });
    return;
  }
  const asset = mapFeatureAsset(feature);
  if (!asset.id) return;
  mapPopup.value = {
    kind: "asset",
    title: asset.name,
    summary: `${asset.media_kind} · ${String(feature.get("taken_at") ?? "")}`,
    assets: [asset]
  };
  if (coordinate && mapOverlay) mapOverlay.setPosition(coordinate);
}

function handleKeydown(event: KeyboardEvent) {
  if (!galleryOpen.value) return;
  if (showAdvancedTranscode.value) {
    if (event.key === "Escape") {
      event.preventDefault();
      closeAdvancedTranscode();
    }
    return;
  }
  if (event.key === "Escape") {
    closeGallery();
  } else if (event.key === "ArrowRight") {
    event.preventDefault();
    nextGallery(1);
  } else if (event.key === "ArrowLeft") {
    event.preventDefault();
    nextGallery(-1);
  } else if (event.key.toLowerCase() === "w") {
    event.preventDefault();
    panGallery(0, 36);
  } else if (event.key.toLowerCase() === "s") {
    event.preventDefault();
    panGallery(0, -36);
  } else if (event.key.toLowerCase() === "a") {
    event.preventDefault();
    panGallery(36, 0);
  } else if (event.key.toLowerCase() === "d") {
    event.preventDefault();
    panGallery(-36, 0);
  }
}

function ensureOpenLayersMap() {
  if (!mapElement.value) return;
  if (!olMap) {
    mapTileLayer = createLocalOSMLayer(() => {
      tileStatus.value = "Base tiles are unavailable right now; vector asset and track layers remain active.";
    });
    mapTileLayer.setVisible(mapTilesVisible.value);
    mapTrackLayer = new VectorLayer({
      source: mapTrackSource,
      style: (feature) => {
        const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
        if (kind === "track") {
          return greenTrackStyle(feature as Feature);
        }
        return undefined;
      }
    });
    mapTrackLayer.setVisible(mapTracksVisible.value);
    mapAssetLayer = new VectorLayer({
      source: mapAssetSource,
      style: (feature) => {
        const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
        if (kind === "cluster") {
          const count = Number(feature.get("count") ?? 0);
          return new Style({
            image: new CircleStyle({
              radius: count > 99 ? 18 : count > 9 ? 16 : 14,
              fill: new Fill({ color: "#bf8700" }),
              stroke: new Stroke({ color: "#ffffff", width: 2 })
            }),
            text: new Text({
              text: String(count || ""),
              fill: new Fill({ color: "#1f2328" }),
              stroke: new Stroke({ color: "#ffffff", width: 3 }),
              font: "700 12px system-ui, sans-serif"
            })
          });
        }
        return new Style({
          image: new CircleStyle({
            radius: 7,
            fill: new Fill({ color: "#0969da" }),
            stroke: new Stroke({ color: "#ffffff", width: 2 })
          })
        });
      }
    });
    mapAssetLayer.setVisible(mapAssetsVisible.value);
    olMap = new OLMap({
      target: mapElement.value,
      layers: [mapTileLayer, mapTrackLayer, mapAssetLayer],
      view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 9 })
    });
    if (mapPopupElement.value) {
      mapOverlay = new Overlay({
        element: mapPopupElement.value,
        positioning: "bottom-center",
        stopEvent: true,
        offset: [0, -16]
      });
      olMap.addOverlay(mapOverlay);
    }
    olMap.on("singleclick", (event) => {
      if (!olMap) return;
      let handled = false;
      olMap.forEachFeatureAtPixel(
        event.pixel,
        (feature) => {
          openMapPopup(feature, event.coordinate);
          handled = true;
          return true;
        },
        { layerFilter: (layer) => layer === mapAssetLayer }
      );
      if (handled) return;
      olMap.forEachFeatureAtPixel(
        event.pixel,
        (feature) => {
          openMapPopup(feature, event.coordinate);
          return true;
        },
        { layerFilter: (layer) => layer === mapTrackLayer }
      );
    });
  } else {
    olMap.setTarget(mapElement.value);
    if (mapOverlay && mapPopupElement.value) mapOverlay.setElement(mapPopupElement.value);
  }
}

function renderOpenLayers() {
  ensureOpenLayersMap();
  if (!olMap || !mapData.value) return;
  const features = new GeoJSON().readFeatures(mapData.value, { featureProjection: "EPSG:3857" });
  const assetFeatures = features.filter((feature) => String(feature.get("kind") ?? feature.get("asset_type") ?? "") !== "track");
  const trackFeatures = features.filter((feature) => String(feature.get("kind") ?? feature.get("asset_type") ?? "") === "track");
  mapAssetSource.clear();
  mapTrackSource.clear();
  mapTrackSource.addFeatures(trackFeatures);
  mapAssetSource.addFeatures(assetFeatures);
  if (features.length > 0 && !mapHasInitialFit) {
    const extent = mapAssetSource.getFeatures().length > 0 ? mapAssetSource.getExtent() : mapTrackSource.getExtent();
    if (extent) {
      olMap.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom: 14, duration: 150 });
      mapHasInitialFit = true;
    }
  }
  olMap.updateSize();
}

watch([active, mapData], async () => {
  if (active.value === "Map") {
    await nextTick();
    renderOpenLayers();
  }
  if (active.value === "GPS/KML Tracks" && selectedTrack.value) {
    await nextTick();
    await refreshSelectedTrackMap();
  }
  if (active.value === "Geo Align" && geoAlignSession.value) {
    await nextTick();
    await refreshGeoAlignMap();
  }
});

watch([mapMediaKind, mapAlbumId, mapTrackId, mapCluster], () => {
  if (active.value === "Map") {
    void refreshMap();
  }
});

let aiPredictionFilterRefreshTimer: number | undefined;

function scheduleAIListFilterRefresh() {
  if (aiPredictionFilterRefreshTimer !== undefined) {
    window.clearTimeout(aiPredictionFilterRefreshTimer);
  }
  aiPredictionFilterRefreshTimer = window.setTimeout(() => {
    aiPredictionFilterRefreshTimer = undefined;
    if (active.value === "OCR" || active.value === "Captions") {
      void refreshAILists();
    }
  }, 250);
}

watch([() => active.value, ocrPageQuery, captionsPageQuery], () => {
  if (active.value === "OCR" || active.value === "Captions") {
    scheduleAIListFilterRefresh();
  }
});

watch(showMapDebug, (value) => {
  persistLayerPreference("cartolensia.map.showDebug", value);
});

watch([mapTilesVisible, mapTracksVisible, mapAssetsVisible], () => {
  mapTileLayer?.setVisible(mapTilesVisible.value);
  mapTrackLayer?.setVisible(mapTracksVisible.value);
  mapAssetLayer?.setVisible(mapAssetsVisible.value);
  persistLayerPreference("cartolensia.map.tilesVisible", mapTilesVisible.value);
  persistLayerPreference("cartolensia.map.tracksVisible", mapTracksVisible.value);
  persistLayerPreference("cartolensia.map.assetsVisible", mapAssetsVisible.value);
});

watch([trackPreviewTilesEnabled, selectedTrackLayerVisible, galleryTrackLayerVisible], () => {
  selectedTrackTileLayer?.setVisible(trackPreviewTilesEnabled.value);
  galleryTrackTileLayer?.setVisible(trackPreviewTilesEnabled.value);
  selectedTrackLayer?.setVisible(selectedTrackLayerVisible.value);
  galleryTrackLayer?.setVisible(galleryTrackLayerVisible.value);
  if (selectedTrackMap && selectedTrackSource.getFeatures().length > 0) {
    refitTrackMapWhenStable(selectedTrackMap, selectedTrackSource, fitSelectedTrack);
  }
  if (galleryTrackMap && galleryTrackSource.getFeatures().length > 0) {
    refitTrackMapWhenStable(galleryTrackMap, galleryTrackSource, fitGalleryTrack);
  }
  persistLayerPreference("cartolensia.trackPreview.tilesVisible", trackPreviewTilesEnabled.value);
});

watch(
  [() => active.value, () => videoTrackSession.value?.id ?? "", () => videoTrackSelectedTracks.value.map((track) => track.track_asset_id).join(",")],
  async () => {
    if (active.value === "Video Track Player" && videoTrackSession.value) {
      await nextTick();
      await renderVideoTrackMap();
      return;
    }
    if (active.value !== "Video Track Player") {
      stopVideoTrackPlaybackLoop();
      destroyVideoTrackMap();
    }
  }
);

watch([geoAlignTilesVisible, geoAlignTrackLayerVisible, geoAlignMarkerLayerVisible], () => {
  geoAlignTileLayer?.setVisible(geoAlignTilesVisible.value);
  geoAlignTrackLayer?.setVisible(geoAlignTrackLayerVisible.value);
  geoAlignMarkerLayer?.setVisible(geoAlignMarkerLayerVisible.value);
  persistLayerPreference("cartolensia.geoAlign.tilesVisible", geoAlignTilesVisible.value);
});

watch(showAssetFaceBoxes, () => {
  localStorage.setItem("cartolensia.asset.showFaceBoxes", showAssetFaceBoxes.value ? "true" : "false");
});
watch(showAssetOCRBoxes, () => {
  localStorage.setItem("cartolensia.asset.showOCRBoxes", showAssetOCRBoxes.value ? "true" : "false");
});

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let size = value / 1024;
  let unit = units.shift() ?? "KB";
  while (size >= 1024 && units.length > 0) {
    size /= 1024;
    unit = units.shift() ?? unit;
  }
  return `${size.toFixed(1)} ${unit}`;
}

function formatDuration(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "unknown";
  const total = Math.round(value);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  if (hours > 0) return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

onMounted(async () => {
  window.addEventListener("keydown", handleKeydown);
  await refresh();
  const assetID = new URLSearchParams(window.location.search).get("asset_id");
  if (active.value === "Asset Detail" && assetID) {
    if (principal.value) {
      await openAsset(assetID);
    } else {
      await openPublicAsset(assetID);
    }
  } else if (active.value === "Transcripts") {
    await fetchTranscriptsPage(true);
  } else if (active.value === "Knowledge Base" || active.value === "Knowledge Graph") {
    await loadKnowledgeBase();
  }
  await nextTick();
  renderOpenLayers();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown);
  stopVideoTrackPlaybackLoop();
  destroyVideoTrackMap();
  destroyGalleryTrackMap();
  selectedTrackMap?.setTarget(undefined);
  geoAlignMap?.setTarget(undefined);
  olMap?.setTarget(undefined);
});
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <div>
        <h1>Cartolensia</h1>
        <span>{{ backend?.store_backend ?? "starting" }} store · {{ backend?.auth_mode ?? "dev_no_auth" }}</span>
      </div>
      <form v-if="backend?.auth_mode === 'local' && !principal" class="login-form" @submit.prevent="login">
        <input v-model="loginEmail" type="email" autocomplete="username" placeholder="Email" />
        <input v-model="loginPassword" type="password" autocomplete="current-password" placeholder="Password" />
        <button type="submit">Login</button>
        <small class="muted">Use the configured admin email and the password file value. Trailing pasted newlines are ignored.</small>
      </form>
      <div v-else-if="principal" class="userbox">
        <span>{{ principal.email ?? principal.name }}</span>
        <button v-if="backend?.auth_mode === 'local'" type="button" @click="logout">Logout</button>
      </div>
      <button type="button" @click="refresh">Refresh</button>
    </header>

    <div class="layout">
      <nav class="sidebar" aria-label="Primary">
        <button
          v-for="item in nav"
          :key="item"
          type="button"
          :class="{ active: item === active }"
          @click="setActive(item)"
	        >
	          <i :class="['bi', navIcon(item)]" aria-hidden="true"></i>
	          {{ item }}
	        </button>
      </nav>

      <section class="content">
        <div v-if="error" class="alert">{{ error }}</div>
        <div v-if="backend?.auth?.warning" class="alert">{{ backend.auth.warning }}</div>
        <div v-if="loading" class="muted">Loading...</div>

        <section v-if="backend?.auth_mode === 'local' && !principal && active !== 'Asset Detail'" class="panel">
          <header class="panel-head">
            <div>
              <h2>Public Gallery</h2>
              <p class="muted">Only assets explicitly marked Public by an administrator are visible before login.</p>
            </div>
          </header>
          <div v-if="publicAssets.length > 0" class="asset-grid">
            <article v-for="asset in publicAssets" :key="asset.id" class="asset-card">
              <div class="thumb">
                <img
                  v-if="asset.media_kind === 'photo'"
                  :src="`/api/v1/media/${asset.id}/preview`"
                  alt=""
                  loading="lazy"
                />
                <div v-else class="media-kind-preview">
                  <i :class="['bi', asset.media_kind === 'video' ? 'bi-film' : asset.media_kind === 'audio' ? 'bi-soundwave' : asset.media_kind === 'document' ? 'bi-file-earmark-text' : 'bi-file-earmark']" aria-hidden="true"></i>
                  <span>{{ asset.media_kind }}</span>
                </div>
              </div>
              <strong>{{ asset.display_name }}</strong>
              <span>{{ asset.media_kind }}</span>
              <button type="button" @click="openPublicAsset(asset.id)">Open public detail</button>
            </article>
          </div>
          <div v-else class="empty-state compact-empty">
            <i class="bi bi-lock" aria-hidden="true"></i>
            <strong>No public assets are available.</strong>
            <span>Log in as an administrator to browse the private archive or mark selected assets as Public.</span>
          </div>
        </section>

        <section v-else-if="active === 'Explorer'" class="panel">
          <header class="panel-head">
            <h2>Explorer</h2>
            <div class="actions">
              <span>{{ explorer?.folder_count ?? 0 }} folders · {{ visibleExplorerRows.length }} / {{ explorerTotalFiles }} files</span>
              <button type="button" @click="explorerViewMode = explorerViewMode === 'table' ? 'tile' : 'table'">
                {{ explorerViewMode === "table" ? "Tile view" : "Table view" }}
              </button>
              <select v-if="albums.length > 0" v-model="selectedAlbumId" aria-label="Target album">
                <option value="">Choose album...</option>
                <option v-for="album in albums" :key="album.id" :value="album.id">{{ album.title }}</option>
              </select>
              <button v-if="selectedAssets.size > 0 && selectedAlbumId" type="button" @click="addSelectedToAlbum">
                Add {{ selectedAssets.size }} to album
              </button>
            </div>
          </header>
          <div class="breadcrumbs">
            <button v-for="crumb in breadcrumbs" :key="crumb.path" type="button" @click="openFolder(crumb.path)">
              {{ crumb.name }}
            </button>
          </div>
          <form class="control-grid" @submit.prevent="refresh">
            <label>
              Search
              <input v-model="explorerQ" type="search" placeholder="name or path" />
            </label>
            <label>
              Media kind
              <select v-model="explorerMediaKind">
                <option value="">All</option>
                <option value="photo">Photos</option>
                <option value="video">Videos</option>
                <option value="audio">Audio</option>
                <option value="document">Documents</option>
                <option value="track">Tracks</option>
              </select>
            </label>
            <label>
              Hash status
              <select v-model="explorerHashStatus">
                <option value="">All</option>
                <option value="hashed">Hashed</option>
                <option value="unhashed">Unhashed</option>
              </select>
            </label>
            <label>
              Extension
              <input v-model="explorerExtension" type="text" placeholder="jpg" />
            </label>
            <label>
              Sort
              <select v-model="explorerSort">
                <option value="name">Name</option>
                <option value="mtime">Modified time</option>
                <option value="size">Size</option>
                <option value="media_kind">Media kind</option>
              </select>
            </label>
            <button type="submit">Apply filters</button>
	            <button type="button" @click="explorerQ = ''; explorerMediaKind = ''; explorerHashStatus = ''; explorerExtension = ''; explorerSort = 'name'; refresh()">
	              Clear filters
	            </button>
	          </form>
	          <div v-if="explorerQ.trim()" class="search-summary">
	            <strong>Universal search:</strong>
	            <span>{{ visibleExplorerRows.length }} results for "{{ explorerQ.trim() }}"</span>
	            <span v-for="warning in searchWarnings" :key="warning" class="status-badge warn">{{ warning }}</span>
	          </div>
          <MonthFilterBar v-if="monthBuckets.length > 0" v-model="monthFilter" :buckets="monthBuckets" />
          <p v-if="visibleExplorerRows.length === 0 && asArray(explorer?.folders).length === 0" class="empty-state">No assets indexed yet.</p>
          <table v-if="explorerViewMode === 'table'">
            <thead>
              <tr>
                <th></th>
                <th>Name</th>
                <th>Kind</th>
                <th>Size</th>
                <th>Hash</th>
                <th>Path</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="folder in explorer?.folders ?? []" :key="folder.path" class="folder-row">
                <td></td>
                <td>
                  <button type="button" class="link-button" @click="openFolder(folder.path)">
                    {{ folder.name }}
                  </button>
                </td>
                <td>folder</td>
                <td>{{ formatBytes(folder.total_bytes) }}</td>
                <td>{{ folder.file_count }} files</td>
                <td>{{ folder.path }}</td>
              </tr>
              <tr v-for="row in visibleExplorerRows" :key="row.asset_id">
                <td>
                  <input
                    type="checkbox"
                    :checked="selectedAssets.has(row.asset_id)"
                    @change="toggleAssetSelection(row.asset_id)"
                  />
                </td>
                <td>
                  <a class="link-button" :href="assetHref(row.asset_id)" @click="openAssetLink($event, row.asset_id)">
                    {{ row.name }}
                  </a>
                </td>
                <td>{{ row.media_kind }}</td>
                <td>{{ formatBytes(row.size_bytes) }}</td>
                <td>{{ row.hash_status }}</td>
                <td>{{ row.relative_path }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="tile-grid">
	            <article
	              v-if="!explorerQ.trim()"
	              v-for="folder in explorer?.folders ?? []"
	              :key="folder.path"
	              class="asset-tile folder-tile"
	            >
	              <button type="button" class="tile-media folder-media" @click="openFolder(folder.path)">
	                <i class="bi bi-folder2-open" aria-hidden="true"></i>
	              </button>
	              <button type="button" class="tile-title link-button" @click="openFolder(folder.path)">{{ folder.name }}</button>
	              <small>{{ folder.file_count }} files · {{ formatBytes(folder.total_bytes) }}</small>
	              <small>{{ folder.path }}</small>
	            </article>
	            <article
	              v-for="(row, index) in visibleExplorerRows"
	              :key="row.asset_id"
	              class="asset-tile"
              @dblclick="openGallery(explorerGalleryItems(), index)"
            >
              <label class="tile-check">
                <input
                  type="checkbox"
                  :checked="selectedAssets.has(row.asset_id)"
                  @change="toggleAssetSelection(row.asset_id)"
                />
              </label>
              <button type="button" class="tile-media" @click="openGallery(explorerGalleryItems(), index)">
                <img
                  v-if="(row.media_kind === 'photo' || row.media_kind === 'track') && !failedPreviewIds.has(row.asset_id)"
                  :src="`/api/v1/media/${row.asset_id}/${row.media_kind === 'track' ? 'track-thumbnail' : 'preview'}`"
                  alt=""
                  loading="lazy"
                  @error="markPreviewFailed(row.asset_id)"
                />
                <span v-else-if="row.media_kind === 'audio'" class="audio-tile-preview">
                  <i class="bi bi-soundwave" aria-hidden="true"></i>
                  <audio :src="`/api/v1/media/${row.asset_id}/original`" controls preload="metadata" @click.stop></audio>
                  <small>{{ row.name }}</small>
                </span>
                <span v-else class="media-fallback">{{ row.media_kind }}</span>
              </button>
              <a class="tile-title link-button" :href="assetHref(row.asset_id)" @click="openAssetLink($event, row.asset_id)">{{ row.name }}</a>
	              <small>{{ row.media_kind }} · {{ formatBytes(row.size_bytes) }}</small>
	              <small v-if="searchExplanation(row)" class="search-match">{{ searchExplanation(row) }}</small>
	              <small class="tile-badges">
                <span :class="['status-badge', row.hash_status === 'hashed' ? 'ok' : 'warn']">{{ row.hash_status }}</span>
                <span v-if="shortHash(row.sha512_hex)" class="status-badge">{{ shortHash(row.sha512_hex) }}</span>
              </small>
              <small>{{ row.mtime }}</small>
            </article>
          </div>
          <PagedFileControls
            v-if="!explorerQ.trim()"
            :shown="explorerLoadedFiles"
            :total="explorerTotalFiles"
            :loading="explorerLoadingMore"
            :loading-all="explorerLoadingAll"
            @load-more="loadMoreExplorerFiles"
            @load-all="loadAllExplorerFiles"
          />
        </section>

        <section v-else-if="active === 'Discovery'" class="panel">
          <header class="panel-head">
            <h2>Discovery</h2>
            <div class="actions">
              <button type="button" :disabled="pipelineRunning" @click="startIndexingPipeline">Start indexing pipeline</button>
              <button type="button" :disabled="!pipelineRunning" @click="stopIndexingPipeline">Stop current pipeline</button>
              <label class="inline-label">
                Job max files
                <input v-model.number="jobMaxFiles" type="number" min="-1" />
              </label>
            </div>
          </header>
          <div class="safety-card">
            <strong>{{ selectedPipelineStorage?.name ?? "No storage selected" }}</strong>
            <span>{{ selectedPipelineStorage?.root ?? "unknown root" }}</span>
            <span>{{ selectedPipelineStorage?.mode ?? "unknown mode" }}</span>
          </div>
          <p class="form-note discovery-note">
            Max files = <strong>-1</strong> means unlimited for normal indexing. Preview and dry-run reports stay conservatively capped at 50 files unless the API explicitly overrides that safeguard.
          </p>
          <div class="settings-grid discovery-grid">
            <article class="settings-form settings-wide">
              <h3>Scope</h3>
              <label>
                Storage
                <select v-model="dryRunStorage">
                  <option value="">Configured default</option>
                  <option value="all">All configured storages</option>
                  <option v-for="storage in storages" :key="storage.name" :value="storage.name">
                    {{ storage.name }} · {{ storage.root }} · {{ storage.mode }}
                  </option>
                </select>
              </label>
              <label>
                Scope / subpath (optional)
                <input v-model="dryRunPrefix" type="text" />
              </label>
              <label>
                Max files
                <input v-model.number="dryRunMaxFiles" type="number" min="-1" />
              </label>
              <label>
                Max bytes
                <input v-model.number="dryRunMaxBytes" type="number" min="-1" />
              </label>
              <p class="muted">Leave blank to scan the selected storage root. Prefixes are storage-relative, for example <code>Cartolensia-photos</code>. Missing marking is disabled.</p>
            </article>
            <article class="settings-form">
              <h3>Extensions</h3>
              <label>
                Include extensions
                <input v-model="dryRunExtensions" type="text" />
              </label>
              <p class="form-note">Default list includes photos, videos, GPS tracks, audio, PDFs, and text/Markdown documents.</p>
              <div class="actions">
                <button type="button" @click="dryRunExtensions = supportedDiscoveryExtensions">Reset supported defaults</button>
              </div>
            </article>
            <article class="settings-form">
              <h3>Pipeline stages</h3>
              <label class="checkbox-label">
                <input v-model="pipelineIndexFiles" type="checkbox" />
                Discover/index files
              </label>
              <label class="checkbox-label">
                <input v-model="hashAfterIndex" type="checkbox" />
                Hash after indexing
              </label>
              <label class="checkbox-label">
                <input v-model="metadataAfterIndex" type="checkbox" />
                Extract metadata/EXIF
              </label>
              <label class="checkbox-label">
                <input v-model="previewsAfterIndex" type="checkbox" />
                Generate previews
              </label>
              <label class="checkbox-label">
                <input v-model="pipelineParseTracks" type="checkbox" />
                Parse GPS/KML/KMZ tracks
              </label>
              <label class="checkbox-label">
                <input v-model="pipelineGeotagExif" type="checkbox" />
                Geotag from EXIF
              </label>
              <label class="checkbox-label">
                <input v-model="pipelineSnapToTracks" type="checkbox" />
                Snap media to tracks if safe
              </label>
              <label class="checkbox-label">
                <input v-model="pipelineRefreshMap" type="checkbox" />
                Refresh map/clusters
              </label>
            </article>
            <article class="settings-form">
              <h3>Safety</h3>
              <p class="muted">Discovery is read-only and missing marking is disabled. Blank scope scans the storage root; dry-run output stays capped unless explicitly over-limited by the API.</p>
            </article>
            <article class="settings-form">
              <h3>Actions</h3>
              <div class="actions">
                <button type="button" @click="startDryRun">Preview scan report (does not index)</button>
                <button type="button" :disabled="pipelineRunning" @click="startIndexingPipeline">Start indexing pipeline</button>
                <button type="button" :disabled="!pipelineRunning" @click="stopIndexingPipeline">Stop current pipeline</button>
              </div>
            </article>
            <article class="settings-form">
              <h3>Stats</h3>
              <div class="metrics">
                <article><strong>{{ indexingStatus?.scope.assets ?? stats?.assets ?? 0 }}</strong><span>Scoped assets</span></article>
                <article><strong>{{ indexingStatus?.scope.hashed ?? stats?.hashed ?? 0 }}</strong><span>Hashed</span></article>
                <article><strong>{{ indexingStatus?.scope.unhashed ?? stats?.unhashed ?? 0 }}</strong><span>Unhashed</span></article>
                <article><strong>{{ indexingStatus?.scope.geotagged ?? 0 }}</strong><span>Geotagged</span></article>
                <article><strong>{{ indexingStatus?.scope.preview_ready ?? previewCacheStats?.ready ?? 0 }}</strong><span>Previews ready</span></article>
                <article><strong>{{ indexingStatus?.scope.tracks ?? tracks.length }}</strong><span>GPS/KML tracks</span></article>
              </div>
            </article>
          </div>
          <div v-if="pipelineRunning || pipelineLog.length > 0" class="pipeline-panel">
            <strong>{{ pipelineRunning ? `Running: ${pipelineStage}` : "Pipeline status" }}</strong>
            <ol>
              <li v-for="line in pipelineLog" :key="line">{{ line }}</li>
            </ol>
          </div>
          <div v-if="lastDiscoveryJob || lastHashJob || lastMetadataJob || lastPreviewJob" class="job-list">
            <article v-if="lastDiscoveryJob" class="job">
              <strong>Last index job: {{ lastDiscoveryJob.status }}</strong>
              <span>{{ lastDiscoveryJob.id }}</span>
              <span>
                {{ lastDiscoveryJob.counters?.scanned ?? 0 }} scanned ·
                {{ lastDiscoveryJob.counters?.created ?? 0 }} created ·
                {{ lastDiscoveryJob.counters?.updated ?? 0 }} updated
              </span>
              <button type="button" @click="openFolder(adapterRelativePrefixes()[0] ?? '')">Open indexed results</button>
            </article>
            <article v-if="lastHashJob" class="job">
              <strong>Last hash job: {{ lastHashJob.status }}</strong>
              <span>{{ lastHashJob.id }}</span>
              <span>{{ lastHashJob.counters?.hashed ?? 0 }} hashed · {{ lastHashJob.counters?.errors ?? 0 }} errors</span>
              <small>{{ lastHashJob.logs?.at(-1)?.message }}</small>
            </article>
            <article v-if="lastMetadataJob" class="job">
              <strong>Last metadata job: {{ lastMetadataJob.status }}</strong>
              <span>{{ lastMetadataJob.id }}</span>
              <span>{{ lastMetadataJob.counters?.updated ?? 0 }} enriched · {{ lastMetadataJob.counters?.errors ?? 0 }} errors</span>
            </article>
            <article v-if="lastPreviewJob" class="job">
              <strong>Last preview job: {{ lastPreviewJob.status }}</strong>
              <span>{{ lastPreviewJob.id }}</span>
              <span>{{ lastPreviewJob.counters?.created ?? 0 }} generated · {{ lastPreviewJob.counters?.updated ?? 0 }} skipped/updated</span>
              <small>{{ lastPreviewJob.logs?.at(-1)?.message }}</small>
            </article>
          </div>
          <pre v-if="dryRunReport" class="geojson">{{ JSON.stringify(dryRunReport.report, null, 2) }}</pre>
          <div class="job-list">
            <article v-for="job in jobs" :key="job.id" class="job">
              <div class="job-row">
                <strong>{{ job.kind }}</strong>
                <div class="actions">
                  <button type="button" @click="openJob(job.id)">Open</button>
                  <button v-if="canRetry(job)" type="button" @click="retryJob(job.id)">Retry</button>
                  <button v-if="canCancel(job)" type="button" @click="cancelJob(job.id)">Cancel</button>
                </div>
              </div>
              <progress :value="job.progress_current" :max="job.progress_total ?? 100"></progress>
              <span>{{ job.status }} · attempt {{ job.attempts ?? 0 }} / {{ job.max_attempts ?? 0 }}</span>
              <span>{{ job.progress_current }} / {{ job.progress_total ?? "?" }} · {{ progressPercent(job) }}%</span>
              <small v-if="jobCounterSummary(job)">{{ jobCounterSummary(job) }}</small>
              <small>{{ job.logs?.at(-1)?.message ?? job.error }}</small>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Jobs'" class="panel">
          <header class="panel-head">
            <h2>Jobs</h2>
            <span>
              {{ jobStats?.queued ?? 0 }} queued · {{ jobStats?.running ?? 0 }} running · {{ jobStats?.failed ?? 0 }} failed
            </span>
          </header>
          <p class="muted">
            Active and queued work is pinned first. Rapid AI micro-jobs can still appear below in recent history.
          </p>
          <div class="job-list">
            <article v-if="selectedJob" class="job job-detail">
              <div class="job-row">
                <strong>{{ selectedJob.kind }} · {{ selectedJob.status }}</strong>
                <div class="actions">
                  <button v-if="canRetry(selectedJob)" type="button" @click="retryJob(selectedJob.id)">Retry</button>
                  <button v-if="canCancel(selectedJob)" type="button" @click="cancelJob(selectedJob.id)">Cancel</button>
                </div>
              </div>
              <progress :value="selectedJob.progress_current" :max="selectedJob.progress_total ?? 100"></progress>
              <small>{{ selectedJob.id }}</small>
              <small v-if="jobCounterSummary(selectedJob)">{{ jobCounterSummary(selectedJob) }}</small>
              <pre class="logbox">{{ JSON.stringify(selectedJob.logs ?? [], null, 2) }}</pre>
            </article>
            <h3 class="section-subhead">Active / queued</h3>
            <article v-if="activeOrQueuedJobs.length === 0" class="empty-state">
              No active or queued jobs.
            </article>
            <article v-for="job in activeOrQueuedJobs" :key="job.id" class="job job-active">
              <div class="job-row">
                <button type="button" class="link-button" @click="openJob(job.id)">{{ job.kind }}</button>
                <div class="actions">
                  <button v-if="canRetry(job)" type="button" @click="retryJob(job.id)">Retry</button>
                  <button v-if="canCancel(job)" type="button" @click="cancelJob(job.id)">Cancel</button>
                </div>
              </div>
              <progress :value="job.progress_current" :max="job.progress_total ?? 100"></progress>
              <span>{{ job.status }} · {{ job.progress_current }} / {{ job.progress_total ?? "?" }}</span>
              <small v-if="jobCounterSummary(job)">{{ jobCounterSummary(job) }}</small>
              <small>{{ job.logs?.at(-1)?.message ?? job.error }}</small>
            </article>
            <h3 class="section-subhead">Recent history</h3>
            <article v-for="job in recentHistoryJobs" :key="job.id" class="job">
              <div class="job-row">
                <button type="button" class="link-button" @click="openJob(job.id)">{{ job.kind }}</button>
                <div class="actions">
                  <button v-if="canRetry(job)" type="button" @click="retryJob(job.id)">Retry</button>
                </div>
              </div>
              <progress :value="job.progress_current" :max="job.progress_total ?? 100"></progress>
              <span>{{ job.status }} · {{ job.progress_current }} / {{ job.progress_total ?? "?" }}</span>
              <small v-if="jobCounterSummary(job)">{{ jobCounterSummary(job) }}</small>
              <small>{{ job.logs?.at(-1)?.message ?? job.error }}</small>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Metadata'" class="panel">
          <header class="panel-head">
            <h2>Metadata</h2>
            <div class="actions">
              <button type="button" @click="startMetadata">Enrich Metadata</button>
              <button type="button" @click="startPreviews">Generate Previews</button>
              <button type="button" @click="cleanupPreviews(true)">Dry Run Cleanup</button>
              <label class="inline-label">
                Max files
                <input v-model.number="jobMaxFiles" type="number" min="-1" />
              </label>
            </div>
          </header>
          <p class="muted">
            Metadata and preview jobs use -1 for no file-count limit and write only to metadata/cache, never originals.
          </p>
          <div class="metrics">
            <article><strong>{{ stats?.photos ?? 0 }}</strong><span>Images</span></article>
            <article><strong>{{ stats?.videos ?? 0 }}</strong><span>Videos</span></article>
            <article><strong>{{ stats?.tracks ?? 0 }}</strong><span>Tracks</span></article>
            <article><strong>{{ backend?.preview_cache ?? "" }}</strong><span>Preview cache</span></article>
            <article><strong>{{ previewCacheStats?.entries ?? 0 }}</strong><span>Cache entries</span></article>
            <article><strong>{{ formatBytes(previewCacheStats?.bytes ?? 0) }}</strong><span>Cache bytes</span></article>
          </div>
          <table>
            <thead><tr><th>Status</th><th>Asset</th><th>Variant</th><th>Size</th><th>Accessed</th></tr></thead>
            <tbody>
              <tr v-for="entry in previewCache" :key="entry.id">
                <td>{{ entry.status }}</td>
                <td>
                  <button type="button" class="link-button" @click="openAsset(entry.asset_id)">{{ entry.asset_id }}</button>
                </td>
                <td>{{ entry.variant }} · {{ entry.width }}×{{ entry.height }}</td>
                <td>{{ formatBytes(entry.size_bytes) }}</td>
                <td>{{ entry.last_accessed_at ?? "" }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else-if="active === 'Asset Detail'" class="panel">
          <header class="panel-head">
            <h2>{{ assetDetail?.asset.display_name ?? "Asset" }}</h2>
            <div class="actions">
              <select v-if="albums.length > 0" v-model="selectedAlbumId" aria-label="Target album">
                <option value="">Choose album...</option>
                <option v-for="album in albums" :key="album.id" :value="album.id">{{ album.title }}</option>
              </select>
              <button v-if="assetDetail && selectedAlbumId" type="button" @click="addCurrentAssetToAlbum">Add to album</button>
              <button
                v-if="assetDetail"
                type="button"
                @click="openGallery([assetToGallery(assetDetail.asset)], 0)"
              >
                Open viewer
              </button>
              <button type="button" @click="setActive('Explorer')">Back</button>
            </div>
          </header>
          <div v-if="assetDetail" class="media-panel">
            <div v-if="assetDetail.asset.media_kind === 'photo'" class="asset-photo-workbench">
              <div class="face-toolbar">
                <label class="form-check form-switch">
                  <input v-model="showAssetFaceBoxes" class="form-check-input" type="checkbox" />
                  <span>Show face rectangles</span>
                </label>
                <label class="form-check form-switch">
                  <input v-model="showAssetOCRBoxes" class="form-check-input" type="checkbox" />
                  <span>Show OCR boxes</span>
                </label>
                <button type="button" class="btn btn-sm btn-outline-primary" @click="faceAddMode = !faceAddMode; newFaceDraftBox = null">
                  <i class="bi bi-plus-square" aria-hidden="true"></i>
                  {{ faceAddMode ? "Cancel add face" : "Add face" }}
                </button>
                <span v-if="faceAddMode" class="muted">Drag a rectangle on the image, enter a name, then save.</span>
              </div>
              <div
                class="asset-photo-frame"
                :class="{ drawing: faceAddMode }"
                @pointerdown="startFaceDraft"
                @pointermove="updateFaceDraft"
                @pointerup="finishFaceDraft"
                @pointerleave="finishFaceDraft"
              >
                <img
                  :src="assetDetail.original_url || assetDetail.preview_url"
                  alt=""
                  @error="markPreviewFailed(assetDetail.asset.id)"
                />
                <span
                  v-for="face in visibleAssetFaces"
                  v-show="showAssetFaceBoxes"
                  :key="String(face.id)"
                  :class="['face-box', { ignored: faceIgnored(face), selected: selectedAssetFaceId === face.id }]"
                  :style="faceBoxStyle(face, assetDetail)"
                  :title="`Face ${typeof face.confidence === 'number' ? face.confidence.toFixed(3) : ''}`"
                ></span>
                <button
                  v-for="block in assetDetail.ocr_blocks ?? []"
                  v-show="showAssetOCRBoxes"
                  :key="block.id"
                  type="button"
                  :class="['ocr-box', { selected: selectedAssetOCRId === block.id }]"
                  :style="ocrBoxStyle(block, assetDetail)"
                  :title="block.text"
                  @click.stop="selectedAssetOCRId = block.id"
                ></button>
                <span
                  v-if="faceAddMode && newFaceDraftBox"
                  class="face-box draft"
                  :style="faceDraftBoxStyle(assetDetail)"
                  aria-hidden="true"
                ></span>
              </div>
              <div v-if="faceAddMode" class="face-add-panel">
                <label>
                  <span>Face name</span>
                  <input v-model="newFaceName" type="text" placeholder="Local label, e.g. Alice" />
                </label>
                <label>
                  <span>Confidence</span>
                  <input v-model.number="newFaceConfidence" type="number" min="0" max="1" step="0.01" />
                </label>
                <button type="button" class="btn btn-success" :disabled="!newFaceDraftBox" @click="saveManualFace">
                  <i class="bi bi-check2-circle" aria-hidden="true"></i>
                  Save new face
                </button>
              </div>
            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'video'" class="video-panel">
              <video
                v-if="assetDetail.original_url"
                :key="videoSource(assetDetail.asset.id, assetDetail.original_url)"
                ref="assetVideoElement"
                :src="videoSource(assetDetail.asset.id, assetDetail.original_url)"
                controls
                preload="metadata"
              ></video>
              <label>
                Quality
                <select
                  :value="transcodeSession?.asset_id === assetDetail.asset.id ? transcodeSession.profile : 'original'"
                  @change="selectTranscodeOption(assetDetail.asset.id, $event)"
                >
                  <option
                    v-for="option in streamOptions?.options ?? []"
                    :key="option.id"
                    :disabled="!option.available"
                    :value="option.id"
                  >
                    {{ option.label }}{{ option.available ? "" : " — planned" }}
                  </option>
                </select>
              </label>
              <p class="muted">
                {{
                  transcodeMessage ||
                  streamOptions?.options.find((option) => !option.available)?.disabled_reason ||
                  "Original/direct streaming is active."
                }}
              </p>
	              <button v-if="transcodeSession?.asset_id === assetDetail.asset.id" type="button" @click="stopActiveTranscode">
	                Stop transcode session
	              </button>
	              <button type="button" class="icon-button" @click="openAdvancedTranscode(assetDetail.asset.id)">
	                <i class="bi bi-gear" aria-hidden="true"></i>
	                Advanced
	              </button>
	            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'audio'" class="audio-panel">
              <audio
                v-if="assetDetail.original_url"
                ref="assetAudioElement"
                :src="assetDetail.original_url"
                controls
                preload="metadata"
              ></audio>
              <div class="detail-grid compact-detail-grid">
                <article>
                  <strong>Duration</strong>
                  <span>{{ formatDuration(Number(assetDetail.asset.metadata?.duration_seconds ?? 0)) }}</span>
                </article>
                <article>
                  <strong>Codec</strong>
                  <span>{{ assetDetail.asset.metadata?.audio_codec || assetDetail.asset.metadata?.codec || "unknown" }}</span>
                </article>
                <article>
                  <strong>Sample rate</strong>
                  <span>{{ assetDetail.asset.metadata?.sample_rate_hz || "unknown" }}</span>
                </article>
                <article>
                  <strong>Channels</strong>
                  <span>{{ assetDetail.asset.metadata?.channels || "unknown" }}</span>
                </article>
              </div>
              <p class="muted">Audio analysis and transcript actions store metadata in Cartolensia only; originals stay read-only.</p>
            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'document'" class="document-panel">
              <div class="document-panel-head">
                <div>
                  <strong>{{ assetDetail.document?.title || assetDetail.asset.display_name }}</strong>
                  <span>{{ assetPrimaryExtension(assetDetail.asset).toUpperCase() || "DOCUMENT" }}</span>
                </div>
                <div class="inline-actions">
                  <a v-if="assetDetail.original_url" class="btn btn-sm btn-outline-secondary" :href="assetDetail.original_url" target="_blank" rel="noreferrer">
                    <i class="bi bi-box-arrow-up-right" aria-hidden="true"></i>
                    Open original
                  </a>
                  <button
                    v-if="assetDocumentText(assetDetail)"
                    type="button"
                    class="btn btn-sm btn-outline-secondary"
                    @click="copyText(assetDocumentText(assetDetail))"
                  >
                    <i class="bi bi-clipboard" aria-hidden="true"></i>
                    Copy text
                  </button>
                  <button
                    v-if="assetDocumentText(assetDetail)"
                    type="button"
                    class="btn btn-sm btn-outline-secondary"
                    @click="downloadTextFile(`${assetDetail.asset.display_name}.txt`, assetDocumentText(assetDetail))"
                  >
                    <i class="bi bi-download" aria-hidden="true"></i>
                    Download text
                  </button>
                </div>
              </div>

              <iframe
                v-if="isPDFDocument(assetDetail) && assetDetail.original_url"
                class="document-frame"
                :src="assetDetail.original_url"
                :title="`PDF preview for ${assetDetail.asset.display_name}`"
              ></iframe>

              <div v-else-if="isMarkdownDocument(assetDetail) && assetDocumentText(assetDetail)" class="document-preview markdown-preview">
                <template v-for="(block, index) in markdownPreviewBlocks(assetDocumentText(assetDetail))" :key="`${index}-${block.kind}`">
                  <h3 v-if="block.kind === 'heading' && (block.level ?? 1) <= 2">{{ block.text }}</h3>
                  <h4 v-else-if="block.kind === 'heading'">{{ block.text }}</h4>
                  <li v-else-if="block.kind === 'list'">{{ block.text }}</li>
                  <pre v-else-if="block.kind === 'code'"><code>{{ block.text }}</code></pre>
                  <blockquote v-else-if="block.kind === 'quote'">{{ block.text }}</blockquote>
                  <br v-else-if="block.kind === 'blank'" />
                  <p v-else>{{ block.text }}</p>
                </template>
              </div>

              <pre v-else-if="isTextDocument(assetDetail) && assetDocumentText(assetDetail)" class="document-preview text-document-preview">{{ assetDocumentText(assetDetail) }}</pre>

              <div v-else class="empty-state compact-empty">
                <i class="bi bi-file-earmark-text" aria-hidden="true"></i>
                <strong>Document preview unavailable</strong>
                <span>Run document extraction/OCR when the Marker, PyMuPDF, or Tesseract component is available. Cartolensia stores extracted text in PostgreSQL/cache only.</span>
              </div>
            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'track'" class="track-detail-preview">
              <img :src="`/api/v1/media/${assetDetail.asset.id}/track-thumbnail?width=720&height=360`" alt="" />
              <button type="button" @click="openGallery([assetToGallery(assetDetail.asset)], 0)">Open interactive track preview</button>
            </div>
            <div v-else class="media-fallback">Unsupported media preview</div>
          </div>
          <div v-if="assetDetail" class="detail-grid">
            <article>
              <strong>Media</strong>
              <span>{{ assetDetail.asset.media_kind }}</span>
            </article>
            <article>
              <strong>Hash</strong>
              <span>{{ assetHashStatus(assetDetail.asset) }}</span>
              <code v-if="assetHashValue(assetDetail.asset)">{{ shortHash(assetHashValue(assetDetail.asset)) }}</code>
            </article>
            <article>
              <strong>Geotag</strong>
              <span>{{ assetHasGeo(assetDetail.asset) ? "EXIF/GPS available" : "not available" }}</span>
            </article>
            <article>
              <strong>Preview</strong>
              <span>{{ assetDetail.preview.status }}</span>
            </article>
            <article>
              <strong>Original</strong>
              <a v-if="assetDetail.original_url" :href="assetDetail.original_url" target="_blank" rel="noreferrer">Open</a>
            </article>
            <article>
              <strong>Public</strong>
              <label v-if="principal" class="form-check form-switch wide-switch">
                <input
                  class="form-check-input"
                  type="checkbox"
                  :checked="Boolean(assetDetail.visibility?.public || assetDetail.asset.metadata?.public)"
                  @change="setAssetPublic(($event.target as HTMLInputElement).checked)"
                />
                <span>Visible before login</span>
              </label>
              <span v-else>{{ assetDetail.visibility?.public || assetDetail.asset.metadata?.public ? "public" : "private" }}</span>
            </article>
          </div>
          <section v-if="assetDetail && assetRelated" class="settings-form settings-wide related-context-panel">
            <div class="section-title">
              <div>
                <h3><i class="bi bi-diagram-3" aria-hidden="true"></i> Related Context</h3>
                <p class="muted">{{ assetRelated.note }}</p>
              </div>
              <button type="button" class="btn btn-sm btn-outline-secondary" @click="universalSearchQ = assetDetail.asset.display_name; setActive('Search'); runUniversalSearch()">
                Open search
              </button>
            </div>
            <div class="detail-grid compact-detail-grid">
              <article v-if="assetRelated.device">
                <strong>{{ assetRelated.device }}</strong>
                <span>Device</span>
              </article>
              <article v-if="assetRelated.folder">
                <strong>{{ assetRelated.folder }}</strong>
                <span>Folder</span>
              </article>
              <article v-if="assetRelated.timestamp_candidates?.length">
                <strong>{{ String(assetRelated.timestamp_candidates[0]?.time ?? "") }}</strong>
                <span>Best timestamp candidate · {{ String(assetRelated.timestamp_candidates[0]?.source ?? "") }}</span>
              </article>
            </div>
            <div class="related-context-grid">
              <article
                v-for="entry in relatedContextGroups"
                :key="entry[0]"
                class="related-context-group"
              >
                <h4>{{ groupLabel(entry[0]) }}</h4>
                <div class="related-context-list">
                  <a
                    v-for="item in entry[1].slice(0, 8)"
                    :key="item.asset.id"
                    class="related-context-row"
                    :href="assetHref(item.asset.id)"
                    @click="openAssetLink($event, item.asset.id)"
                  >
                    <span>{{ assetName(item.asset) }}</span>
                    <small>{{ item.asset.media_kind }} · {{ item.reason }}</small>
                  </a>
                </div>
              </article>
            </div>
          </section>
          <section v-if="assetDetail" class="settings-form settings-wide ai-asset-actions">
            <div class="section-title">
              <div>
                <h3><i class="bi bi-stars" aria-hidden="true"></i> AI Actions</h3>
                <p class="muted">
                  Actions run only on this asset and are recorded in Jobs. Originals stay read-only; AI outputs are local metadata.
                </p>
              </div>
              <button type="button" class="btn btn-outline-secondary btn-sm" @click="setActive('Jobs')">
                <i class="bi bi-list-task" aria-hidden="true"></i>
                Jobs
              </button>
            </div>
            <div v-if="assetDetail.asset.media_kind === 'photo'" class="ai-action-grid">
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('classify', 'classification')">
                <i class="bi bi-tags" aria-hidden="true"></i>
                Run classification
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('faces', 'faces')">
                <i class="bi bi-person-bounding-box" aria-hidden="true"></i>
                Run face detection
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('ocr', 'ocr')">
                <i class="bi bi-body-text" aria-hidden="true"></i>
                Run OCR
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('safety', 'safety')">
                <i class="bi bi-shield-check" aria-hidden="true"></i>
                Run NSFW/safety check
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('embed', 'embedding')">
                <i class="bi bi-diagram-3" aria-hidden="true"></i>
                Generate embedding
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('describe', 'short_caption', { caption_type: 'short_caption' })">
                <i class="bi bi-chat-square-text" aria-hidden="true"></i>
                Generate short caption
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('describe', 'long_caption', { caption_type: 'long_caption' })">
                <i class="bi bi-text-paragraph" aria-hidden="true"></i>
                Generate long caption
              </button>
              <button type="button" class="btn btn-primary" @click="runAllAssetAIActions">
                <i class="bi bi-play-circle" aria-hidden="true"></i>
                Run all enabled AI functions
              </button>
            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'video'" class="empty-state compact-empty">
              <p>Frame AI for videos is planned. Audio transcription can run now against the selected video audio stream.</p>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('transcribe', 'video_transcript', { model: 'small' })">
                <i class="bi bi-soundwave" aria-hidden="true"></i>
                Run audio transcription
              </button>
            </div>
            <div v-else-if="assetDetail.asset.media_kind === 'audio'" class="ai-action-grid">
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('transcribe', 'audio_transcript', { model: 'small' })">
                <i class="bi bi-soundwave" aria-hidden="true"></i>
                Run transcription
              </button>
              <button type="button" class="btn btn-outline-primary" @click="runAssetAIAction('audio-analyze', 'audio_features')">
                <i class="bi bi-sliders" aria-hidden="true"></i>
                Run audio analysis
              </button>
              <button type="button" class="btn btn-outline-secondary" @click="setActive('Jobs')">
                <i class="bi bi-list-task" aria-hidden="true"></i>
                View audio jobs
              </button>
            </div>
            <div v-else class="empty-state compact-empty">
              AI image actions are not relevant for GPS/KML track assets.
            </div>
            <div v-if="Object.keys(assetAIActionStatus).length" class="ai-action-status-list">
              <article v-for="(state, key) in assetAIActionStatus" :key="key">
                <span :class="['status-badge', state.status === 'completed' ? 'ok' : state.status === 'running' ? 'warn' : state.status === 'missing' || state.status === 'failed' ? 'bad' : '']">
                  <span v-if="state.status === 'running'" class="spinner-border spinner-border-sm" aria-hidden="true"></span>
                  {{ state.status }}
                </span>
                <strong>{{ key }}</strong>
                <span>{{ state.summary }}</span>
                <button v-if="state.status === 'missing'" type="button" class="btn btn-sm btn-outline-primary" @click="settingsTab = 'components'; setActive('Settings')">Open Components</button>
                <button v-if="state.job_id" type="button" class="btn btn-sm btn-outline-secondary" @click="setActive('Jobs')">Job {{ state.job_id.slice(0, 8) }}</button>
              </article>
            </div>
          </section>
          <section v-if="assetDetail && ((assetDetail.ai_tags?.length ?? 0) > 0 || (assetDetail.ai_predictions?.length ?? 0) > 0 || (assetDetail.face_detections?.length ?? 0) > 0 || (assetDetail.ocr_blocks?.length ?? 0) > 0 || (assetDetail.embeddings?.length ?? 0) > 0 || (assetDetail.transcripts?.length ?? 0) > 0 || !!assetDetail.audio_features || (assetDetail.frame_captions?.length ?? 0) > 0 || !!assetDetail.document)" class="settings-form settings-wide">
            <h3>AI Results</h3>
            <div v-if="assetDetail.ai_tags?.length" class="chip-row">
              <span v-for="tag in assetDetail.ai_tags" :key="`${tag.tag}-${tag.source}`" class="chip">
                {{ tag.tag }} <small>{{ tag.source }}</small>
              </span>
            </div>
            <table v-if="assetDetail.ai_predictions?.length">
              <thead><tr><th>Task</th><th>Label</th><th>Confidence</th><th>Model</th></tr></thead>
              <tbody>
                <tr v-for="prediction in assetDetail.ai_predictions" :key="String(prediction.id)">
                  <td>{{ prediction.task }}</td>
                  <td>{{ prediction.label }}</td>
                  <td>{{ prediction.confidence ? Number(prediction.confidence).toFixed(3) : "" }}</td>
                  <td>{{ prediction.model_name }}</td>
                </tr>
              </tbody>
            </table>
            <section v-if="assetDetail.face_detections?.length" class="face-record-list">
              <h4>Faces</h4>
              <p class="muted">Click a record to highlight it. Click the face name/photo to open all photos in that local face folder.</p>
              <article
                v-for="face in visibleAssetFaces"
                :key="face.id"
                :class="['face-record-card', { selected: selectedAssetFaceId === face.id }]"
                @click="selectedAssetFaceId = face.id"
              >
                <button type="button" class="face-record-preview" @click.stop="openFaceFromDetection(face)">
                  <img :src="faceCardPreviewURL(face)" alt="" loading="lazy" />
                </button>
                <button type="button" class="link-button face-record-name" @click.stop="openFaceFromDetection(face)">
                  {{ faceDisplayName(face) }}
                </button>
                <span>Confidence {{ typeof face.confidence === 'number' ? face.confidence.toFixed(3) : 'n/a' }}</span>
                <span>{{ Math.round(face.width) }} × {{ Math.round(face.height) }} px</span>
                <button type="button" class="btn btn-sm btn-outline-danger" @click.stop="deleteFaceDetection(face)">
                  <i class="bi bi-trash" aria-hidden="true"></i>
                  Delete
                </button>
              </article>
            </section>
            <section v-if="assetDetail.ocr_blocks?.length" class="ocr-record-list">
              <h4>OCR Text</h4>
              <p class="muted">OCR is stored as local metadata with bounding boxes. Click text to highlight it on the image.</p>
              <div v-if="assetDetail.ocr_full_text || assetDetail.ocr_summary?.full_text" class="ocr-full-text-panel">
                <div class="section-title-row">
                  <strong>Full text</strong>
                  <span class="status-badge">{{ assetDetail.ocr_summary?.block_count ?? assetDetail.ocr_blocks.length }} blocks</span>
                  <span v-if="assetDetail.ocr_summary?.languages?.length" class="status-badge ok">
                    {{ assetDetail.ocr_summary.languages.join(", ") }}
                  </span>
                </div>
                <textarea
                  readonly
                  :value="assetDetail.ocr_full_text || assetDetail.ocr_summary?.full_text"
                  rows="6"
                  aria-label="Full OCR text"
                ></textarea>
                <div class="inline-actions">
                  <button type="button" class="btn btn-sm btn-outline-secondary" @click="copyText(assetDetail.ocr_full_text || assetDetail.ocr_summary?.full_text || '')">
                    <i class="bi bi-clipboard" aria-hidden="true"></i>
                    Copy full text
                  </button>
                  <button
                    type="button"
                    class="btn btn-sm btn-outline-secondary"
                    @click="downloadTextFile(`${assetDetail.asset.display_name || 'ocr'}.txt`, assetDetail.ocr_full_text || assetDetail.ocr_summary?.full_text || '')"
                  >
                    <i class="bi bi-download" aria-hidden="true"></i>
                    Download .txt
                  </button>
                </div>
              </div>
              <article
                v-for="block in assetDetail.ocr_blocks"
                :key="block.id"
                :class="['ocr-record-card', { selected: selectedAssetOCRId === block.id }]"
                @click="selectedAssetOCRId = block.id"
              >
                <div>
                  <strong>{{ block.text }}</strong>
                  <small>{{ block.language || 'unknown language' }} · {{ block.engine || block.model_name || 'OCR engine' }}</small>
                </div>
                <span>{{ typeof block.confidence === 'number' ? block.confidence.toFixed(2) : 'n/a' }}</span>
                <button type="button" class="btn btn-sm btn-outline-secondary" @click.stop="copyText(block.text)">
                  <i class="bi bi-clipboard" aria-hidden="true"></i>
                  Copy
                </button>
                <button type="button" class="btn btn-sm btn-outline-danger" @click.stop="deleteOCRBlock(block)">
                  <i class="bi bi-trash" aria-hidden="true"></i>
                  Delete
                </button>
              </article>
            </section>
            <section v-if="assetDetail.transcripts?.length" class="ocr-record-list">
              <h4>Transcripts</h4>
              <article v-for="transcript in assetDetail.transcripts" :key="transcript.id" class="ocr-record-card wide-record-card">
                <div>
                  <strong>{{ transcript.language || 'auto language' }} · {{ transcript.model || 'ASR model' }}</strong>
                  <small>{{ transcript.source_kind || 'media audio' }} · {{ transcript.segments?.length ?? 0 }} segments</small>
                  <p>{{ transcript.full_text }}</p>
                  <div v-if="transcript.segments?.length" class="segment-list">
                    <button
                      v-for="segment in transcript.segments"
                      :key="segment.id"
                      type="button"
                      class="segment-row"
                      @click="seekAssetMedia(segment.start_ms)"
                    >
                      <span>{{ formatDuration(segment.start_ms / 1000) }}</span>
                      <strong>{{ segment.text }}</strong>
                      <small>{{ typeof segment.confidence === 'number' ? segment.confidence.toFixed(2) : '' }}</small>
                    </button>
                  </div>
                </div>
                <button type="button" class="btn btn-sm btn-outline-secondary" @click.stop="copyText(transcript.full_text)">
                  <i class="bi bi-clipboard" aria-hidden="true"></i>
                  Copy
                </button>
              </article>
            </section>
            <section v-if="assetDetail.audio_features" class="place-record-grid">
              <article class="place-record-card">
                <strong>Audio features</strong>
                <p>
                  Tempo {{ assetDetail.audio_features.tempo_bpm ?? 'n/a' }} BPM ·
                  Key {{ assetDetail.audio_features.key || 'n/a' }}{{ assetDetail.audio_features.mode ? ` ${assetDetail.audio_features.mode}` : '' }} ·
                  Genres {{ assetDetail.audio_features.genre_labels?.join(', ') || 'not classified' }}
                </p>
                <small>{{ assetDetail.audio_features.model || 'local audio analyzer' }}</small>
              </article>
            </section>
            <section v-if="assetDetail.frame_captions?.length" class="ocr-record-list">
              <h4>Video Frame Captions</h4>
              <article v-for="caption in assetDetail.frame_captions" :key="caption.id" class="ocr-record-card wide-record-card">
                <div>
                  <strong>{{ caption.caption }}</strong>
                  <small>{{ Math.round(caption.timestamp_ms / 1000) }}s · {{ caption.model || 'frame caption model' }}</small>
                </div>
                <button type="button" class="btn btn-sm btn-outline-secondary" @click.stop="copyText(caption.caption)">
                  <i class="bi bi-clipboard" aria-hidden="true"></i>
                  Copy
                </button>
              </article>
            </section>
            <section v-if="assetDetail.document" class="ocr-record-list">
              <h4>Document Text</h4>
              <div class="ocr-full-text-panel">
                <div class="section-title-row">
                  <strong>{{ assetDetail.document.title || assetDetail.asset.display_name }}</strong>
                  <span class="status-badge">{{ assetDetail.document.engine || 'document extractor' }}</span>
                </div>
                <textarea readonly :value="assetDetail.document.markdown || assetDetail.document.text || ''" rows="8"></textarea>
                <div class="inline-actions">
                  <button type="button" class="btn btn-sm btn-outline-secondary" @click="copyText(assetDetail.document?.markdown || assetDetail.document?.text || '')">
                    <i class="bi bi-clipboard" aria-hidden="true"></i>
                    Copy document text
                  </button>
                </div>
              </div>
            </section>
            <p v-if="assetDetail.embeddings?.length">Embeddings stored: {{ assetDetail.embeddings.length }} vector record(s).</p>
          </section>
          <section v-if="assetDetail" class="settings-form settings-wide">
            <div class="section-title-row">
              <h3><i class="bi bi-geo-alt" aria-hidden="true"></i> Places and Coordinates</h3>
              <button type="button" class="btn btn-sm btn-outline-primary" @click="refreshAssetPlaces">
                <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
                Refresh place
              </button>
            </div>
            <p class="muted">
              Reverse geocoding uses the local cache first. Online lookup runs only when enabled in Settings and this button is clicked.
            </p>
            <div v-if="assetDetail.places?.length" class="place-record-grid">
              <article v-for="place in assetDetail.places" :key="`${place.coordinate_source}-${place.display_name}-${place.lat}-${place.lon}`" class="place-record-card">
                <div>
                  <strong>{{ place.display_name }}</strong>
                  <span class="status-badge ok">{{ place.coordinate_source }}</span>
                  <span v-if="place.geo_source" class="status-badge">{{ place.geo_source }}</span>
                </div>
                <p>{{ place.lat.toFixed(6) }}, {{ place.lon.toFixed(6) }}</p>
                <small>{{ place.match }} · {{ place.provider }}/{{ place.source }}</small>
              </article>
            </div>
            <div v-else class="empty-state compact-empty">
              No cached place match for this asset's known coordinates yet.
            </div>
          </section>
          <table v-if="assetDetail">
            <thead><tr><th>Storage</th><th>Path</th><th>Size</th><th>MIME</th></tr></thead>
            <tbody>
              <tr v-for="location in assetDetail.locations" :key="location.id">
                <td>{{ location.storage_name }}</td>
                <td>{{ location.relative_path }}</td>
                <td>{{ formatBytes(location.size_bytes) }}</td>
                <td>{{ location.mime }}</td>
              </tr>
            </tbody>
          </table>
          <pre v-if="assetDetail" class="geojson">{{ JSON.stringify(assetDetail.asset.metadata ?? {}, null, 2) }}</pre>
        </section>

        <section v-else-if="active === 'Storages'" class="panel">
          <header class="panel-head"><h2>Storages</h2></header>
          <table>
            <thead><tr><th>Name</th><th>Kind</th><th>Mode</th><th>Health</th><th>SMB / Source</th><th>Root</th></tr></thead>
            <tbody>
              <tr v-for="storage in storages" :key="storage.name">
                <td>{{ storage.name }}</td>
                <td>{{ storage.kind }}</td>
                <td>{{ storage.mode }}</td>
                <td>
                  <span :class="['status-badge', storage.health === 'available' ? 'ok' : 'warn']">{{ storage.health || 'unknown' }}</span>
                  <code v-if="storage.health_code" class="d-block">{{ storage.health_code }}</code>
                  <small v-if="storage.health_message" class="muted d-block">{{ storage.health_message }}</small>
                </td>
                <td>
                  <small v-if="storage.source_url" class="d-block">{{ storage.source_url }}</small>
                  <small v-if="storage.smb?.host" class="d-block">SMB {{ storage.smb.host }}/{{ storage.smb.share || '?' }}</small>
                </td>
                <td>{{ storage.root }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else-if="active === 'Plugins'" class="panel">
          <header class="panel-head"><h2>Plugins</h2></header>
          <div class="cards">
            <article v-for="plugin in plugins" :key="plugin.id" class="card">
              <h3>{{ plugin.name }}</h3>
              <p>{{ plugin.description }}</p>
              <span>{{ plugin.id }} · {{ plugin.status }} · {{ plugin.runtime }}</span>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Stats'" class="panel">
          <header class="panel-head"><h2>Stats</h2></header>
          <div v-if="stats" class="metrics">
            <article><strong>{{ stats.assets }}</strong><span>Assets</span></article>
            <article><strong>{{ stats.photos }}</strong><span>Photos</span></article>
            <article><strong>{{ stats.videos }}</strong><span>Videos</span></article>
            <article><strong>{{ stats.tracks }}</strong><span>Tracks</span></article>
            <article><strong>{{ stats.hashed }}</strong><span>Hashed</span></article>
            <article><strong>{{ stats.unhashed }}</strong><span>Unhashed</span></article>
            <article><strong>{{ stats.duplicate_groups ?? 0 }}</strong><span>Duplicate groups</span></article>
            <article><strong>{{ stats.duplicate_locations ?? 0 }}</strong><span>Duplicate locations</span></article>
            <article><strong>{{ formatBytes(stats.total_bytes) }}</strong><span>Total</span></article>
          </div>
        </section>

        <section v-else-if="active === 'Duplicates'" class="panel">
          <header class="panel-head">
            <h2>Duplicates</h2>
            <div class="actions">
              <span>{{ duplicatePage?.page.total ?? 0 }} duplicate content groups</span>
              <button type="button" @click="refresh">Refresh</button>
            </div>
          </header>
          <p class="empty-state">
            Duplicate detection is report-only. It groups hashed assets by SHA-512 and size and never deletes, moves, or rewrites originals.
          </p>
          <p v-if="(duplicatePage?.groups.length ?? 0) === 0" class="empty-state">
            No duplicate hashed content groups found.
          </p>
          <div v-else class="cards">
            <article v-for="group in duplicatePage?.groups ?? []" :key="`${group.sha512_hex}-${group.size_bytes}`" class="card duplicate-card">
              <h3>{{ group.asset_count }} assets · {{ formatBytes(group.total_bytes) }}</h3>
              <p>{{ formatBytes(group.size_bytes) }} each · {{ group.sha512_hex.slice(0, 24) }}...</p>
              <table>
                <thead><tr><th>Asset</th><th>Kind</th><th>Path</th></tr></thead>
                <tbody>
                  <tr v-for="asset in group.assets" :key="asset.asset_id">
                    <td><button type="button" class="link-button" @click="openAsset(asset.asset_id)">{{ asset.display_name }}</button></td>
                    <td>{{ asset.media_kind }}</td>
                    <td>{{ asset.relative_path }}</td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Albums'" class="panel">
          <header class="panel-head">
            <h2>Albums</h2>
            <div class="actions">
              <span>Virtual albums never move or delete originals.</span>
              <button v-if="selectedAlbumId" type="button" @click="showSelectedAlbumOnMap">Show on map</button>
              <button v-if="selectedAlbumId" type="button" @click="clearAlbumSelection">Clear album</button>
              <button type="button" @click="albumViewMode = albumViewMode === 'table' ? 'tile' : 'table'">
                {{ albumViewMode === "table" ? "Tile view" : "Table view" }}
              </button>
            </div>
          </header>
          <form class="control-grid" @submit.prevent="createAlbum">
            <label>
              Title
              <input v-model="newAlbumTitle" type="text" />
            </label>
            <label>
              Description
              <input v-model="newAlbumDescription" type="text" />
            </label>
            <button type="submit">Create Album</button>
          </form>
          <p v-if="albums.length === 0" class="empty-state">No albums yet.</p>
          <div class="split-view">
            <aside>
              <button
                v-for="album in albums"
                :key="album.id"
                type="button"
                :class="{ active: album.id === selectedAlbumId }"
                @click="selectAlbum(album.id)"
              >
                {{ album.title }} · {{ album.item_count }}
              </button>
            </aside>
            <section>
              <div class="actions">
                <button v-if="selectedAssets.size > 0 && selectedAlbumId" type="button" @click="addSelectedToAlbum">
                  Add selected assets
                </button>
                <button v-if="selectedAlbumId" type="button" @click="startMetadata">Enrich album assets</button>
                <button v-if="selectedAlbumId" type="button" @click="startPreviews">Generate previews</button>
              </div>
              <p v-if="selectedAlbumId && selectedAlbumItems.length === 0" class="empty-state">
                This album has no assets yet. Select assets in Explorer, then add them to this album.
              </p>
              <table v-if="albumViewMode === 'table'">
                <thead><tr><th>Name</th><th>Kind</th><th>Size</th><th>Actions</th></tr></thead>
                <tbody>
                  <tr v-for="item in selectedAlbumItems" :key="item.asset.id">
                    <td>
                      <button type="button" class="link-button" @click="openAsset(item.asset.id)">
                        {{ item.asset.display_name }}
                      </button>
                    </td>
                    <td>{{ item.asset.media_kind }}</td>
                    <td>{{ formatBytes(item.asset.locations[0]?.size_bytes ?? 0) }}</td>
                    <td><button type="button" @click="removeAlbumItem(item.asset.id)">Remove</button></td>
                  </tr>
                </tbody>
              </table>
              <div v-else class="tile-grid">
                <article v-for="(asset, index) in selectedAlbumAssets" :key="asset.id" class="asset-tile">
                  <button type="button" class="tile-media" @click="openGallery(selectedAlbumAssets.map(assetToGallery), index)">
                    <img
                      v-if="asset.media_kind === 'photo' && !failedPreviewIds.has(asset.id)"
                      :src="`/api/v1/media/${asset.id}/preview`"
                      alt=""
                      loading="lazy"
                      @error="markPreviewFailed(asset.id)"
                    />
                    <span v-else-if="asset.media_kind === 'audio'" class="audio-tile-preview">
                      <i class="bi bi-soundwave" aria-hidden="true"></i>
                      <audio :src="`/api/v1/media/${asset.id}/original`" controls preload="metadata" @click.stop></audio>
                      <small>{{ asset.display_name }}</small>
                    </span>
                    <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                  </button>
                  <a class="tile-title link-button" :href="assetHref(asset.id)" @click="openAssetLink($event, asset.id)">
                    {{ asset.display_name }}
                  </a>
                  <small>{{ asset.media_kind }} · {{ formatBytes(asset.locations[0]?.size_bytes ?? 0) }}</small>
                  <small class="tile-badges">
                    <span :class="['status-badge', assetHashStatus(asset) === 'hashed' ? 'ok' : 'warn']">{{ assetHashStatus(asset) }}</span>
                    <span v-if="assetHasGeo(asset)" class="status-badge ok">geotag</span>
                  </small>
                  <button type="button" @click="removeAlbumItem(asset.id)">Remove from album</button>
                </article>
              </div>
            </section>
          </div>
        </section>

        <section v-else-if="active === 'GPS/KML Tracks'" class="panel">
          <header class="panel-head">
            <h2>GPS/KML Tracks</h2>
            <div class="actions">
              <span>
                {{ tracks.length }} loaded<span v-if="stats?.tracks"> of {{ stats.tracks }}</span> tracks
              </span>
              <button type="button" @click="trackMediaViewMode = trackMediaViewMode === 'table' ? 'tile' : 'table'">
                {{ trackMediaViewMode === "table" ? "Tile media" : "Table media" }}
              </button>
            </div>
          </header>
          <div class="list-toolbar">
            <TypeaheadSearch
              v-model="trackSearchQ"
              :results="trackTypeaheadResults"
              :loading="tracksLoadingMore"
              placeholder="Search track name, date, format, distance..."
              go-label="Find tracks"
              @select="selectTrackSearchResult"
              @go="goToTrackSearchSelection"
            />
            <div class="actions">
              <button type="button" class="btn btn-sm btn-outline-primary" :disabled="tracksLoadingMore" @click="fetchTracksPage(true)">
                Search / refresh
              </button>
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="tracksLoadingMore || !tracksHasMore" @click="fetchTracksPage(false)">
                Load more
              </button>
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="tracksLoadingMore || !tracksHasMore" @click="loadAllTracks">
                Load all
              </button>
            </div>
          </div>
          <p class="muted">Track lists are paged for large archives. Search matches prefixes, suffixes, and middle substrings.</p>
          <p v-if="tracks.length === 0" class="empty-state">
            <span v-if="(stats?.tracks ?? 0) > 0">
              {{ stats?.tracks }} track-like assets are indexed, but no parsed GPS/KML/KMZ/GPZ summaries exist yet.
            </span>
            <span v-else>No GPS/KML track files in this subset.</span>
            <button type="button" @click="parseTrackFilesForCurrentPrefix">Parse track files for current prefix</button>
            <small>Current prefix: {{ adapterRelativePrefixes().join(", ") || "not set" }}</small>
          </p>
          <div v-if="selectedTrack" class="track-detail track-detail-page">
            <header class="track-detail-header">
              <div>
                <h3>{{ selectedTrack.summary.name }}</h3>
                <p class="muted">
                  {{ selectedTrack.summary.source_format ?? "track" }}
                  · {{ selectedTrack.summary.start_time ?? "no start time" }}
                  <span v-if="selectedTrack.summary.end_time">→ {{ selectedTrack.summary.end_time }}</span>
                </p>
              </div>
              <div class="actions">
                <a class="btn btn-outline-secondary" :href="assetHref(selectedTrack.summary.track_asset_id)" @click="openAssetLink($event, selectedTrack.summary.track_asset_id)">Open source asset</a>
                <button type="button" @click="setActive('Map')">Show on map</button>
              </div>
            </header>
            <div class="track-detail-grid">
              <div class="map-shell">
                <div ref="selectedTrackMapElement" class="track-detail-map"></div>
                <svg v-if="selectedTrackFallbackPath" class="track-vector-fallback" viewBox="0 0 1000 420" aria-hidden="true">
                  <path :d="selectedTrackFallbackPath" />
                </svg>
                <div class="map-status-overlay">
                  <i class="bi bi-signpost-split" aria-hidden="true"></i>
                  {{ selectedTrackPreviewStatus || "Loading track geometry..." }}
                </div>
                <div v-if="showSelectedTrackPointPopup" class="track-point-popup">
                  <button type="button" class="btn btn-sm btn-outline-secondary danger-close" @click="showSelectedTrackPointPopup = false">
                    <i class="bi bi-x-lg" aria-hidden="true"></i>
                    Close
                  </button>
                  <strong>Track point</strong>
                  <p v-if="selectedTrackPointMessage" class="muted">{{ selectedTrackPointMessage }}</p>
                  <template v-if="selectedTrackPointInfo">
                    <span>Clicked: {{ selectedTrackPointInfo.clicked.lat.toFixed(6) }}, {{ selectedTrackPointInfo.clicked.lon.toFixed(6) }}</span>
                    <span>Nearest: {{ selectedTrackPointInfo.nearest.lat.toFixed(6) }}, {{ selectedTrackPointInfo.nearest.lon.toFixed(6) }}</span>
                    <span>Distance: {{ selectedTrackPointInfo.distance_m.toFixed(1) }} m</span>
                    <span v-if="selectedTrackPointInfo.timestamp">Time: {{ selectedTrackPointInfo.timestamp }}</span>
                    <span v-if="selectedTrackPointInfo.relative_time_seconds !== undefined">Relative: {{ selectedTrackPointInfo.relative_time_seconds.toFixed(1) }} s</span>
                    <span v-if="selectedTrackPointInfo.speed_mps !== undefined">Speed: {{ selectedTrackPointInfo.speed_mps.toFixed(2) }} m/s</span>
                    <span v-if="selectedTrackPointInfo.elevation_m !== undefined">Elevation: {{ selectedTrackPointInfo.elevation_m.toFixed(1) }} m</span>
                  </template>
                </div>
                <div class="map-layer-control">
                  <button type="button" class="icon-button" @click="showSelectedTrackLayerMenu = !showSelectedTrackLayerMenu">
                    <i class="bi bi-layers" aria-hidden="true"></i>
                    Layers
                  </button>
                  <div v-if="showSelectedTrackLayerMenu" class="layer-menu">
                    <label class="form-check form-switch">
                      <input v-model="trackPreviewTilesEnabled" class="form-check-input" type="checkbox" />
                      <span>OSM tiles</span>
                    </label>
                    <label class="form-check form-switch">
                      <input v-model="selectedTrackLayerVisible" class="form-check-input" type="checkbox" />
                      <span>Track layer</span>
                    </label>
                    <button type="button" class="btn btn-sm btn-outline-primary" @click="fitSelectedTrack">
                      <i class="bi bi-aspect-ratio" aria-hidden="true"></i>
                      Fit to track
                    </button>
                    <small>Tiles load on demand through Cartolensia only; no bulk prefetch.</small>
                  </div>
                </div>
              </div>
              <div class="track-profiles">
                <article class="profile-card">
                  <header>
                    <strong>Altitude</strong>
                    <span>{{ trackProfileRange(selectedTrackAltitude) }}</span>
                  </header>
                  <svg viewBox="0 0 520 140" role="img" aria-label="Altitude profile">
                    <path v-if="trackProfilePath(selectedTrackAltitude)" :d="trackProfilePath(selectedTrackAltitude)" />
                    <text v-else x="20" y="76">No altitude values</text>
                  </svg>
                </article>
                <article class="profile-card">
                  <header>
                    <strong>Speed</strong>
                    <span>{{ trackProfileRange(selectedTrackSpeed) }}</span>
                  </header>
                  <svg viewBox="0 0 520 140" role="img" aria-label="Speed profile">
                    <path v-if="trackProfilePath(selectedTrackSpeed)" :d="trackProfilePath(selectedTrackSpeed)" />
                    <text v-else x="20" y="76">No speed/timestamp values</text>
                  </svg>
                </article>
              </div>
            </div>
            <div class="metrics">
              <article><strong>{{ selectedTrack.summary.point_count }}</strong><span>Points</span></article>
              <article><strong>{{ selectedTrack.summary.distance_m ? `${(selectedTrack.summary.distance_m / 1000).toFixed(2)} km` : "n/a" }}</strong><span>Distance</span></article>
              <article><strong>{{ trackDurationLabel(selectedTrack.summary) }}</strong><span>Duration</span></article>
              <article><strong>{{ selectedTrack.summary.elevation_min_m !== undefined ? `${selectedTrack.summary.elevation_min_m.toFixed(1)}-${selectedTrack.summary.elevation_max_m?.toFixed(1) ?? "?"} m` : "n/a" }}</strong><span>Elevation</span></article>
              <article><strong>{{ selectedTrack.summary.min_lat?.toFixed(5) ?? "n/a" }}</strong><span>Min lat</span></article>
              <article><strong>{{ selectedTrack.summary.max_lon?.toFixed(5) ?? "n/a" }}</strong><span>Max lon</span></article>
            </div>
            <div class="actions">
              <label class="inline-label">
                Offset seconds
                <input v-model.number="trackOffsetSeconds" type="number" />
              </label>
              <button type="button" :disabled="!trackHasRealTime(selectedTrack.summary)" @click="findTrackAssets(selectedTrack.summary.track_asset_id)">Show media during track</button>
              <button type="button" @click="findNearbyTrackAssets(selectedTrack.summary.track_asset_id)">Show geotagged media within 100 m</button>
              <button type="button" @click="snapTrackMedia(selectedTrack.summary.track_asset_id)">Snap media</button>
            </div>
            <p v-if="!trackHasRealTime(selectedTrack.summary)" class="muted">
              This track is geometry-only or uses synthetic timestamps. Use nearby geotagged media for reliable matching.
            </p>
            <p v-if="trackAssets.length === 0" class="empty-state">
              {{ trackAssetsReason || "No media results loaded for this track yet." }}
            </p>
            <table v-if="trackAssets.length > 0 && trackMediaViewMode === 'table'">
              <thead><tr><th>Asset</th><th>Kind</th><th>Taken</th></tr></thead>
              <tbody>
                <tr v-for="asset in trackAssets" :key="asset.id">
                  <td><a class="link-button" :href="assetHref(asset.id)" @click="openAssetLink($event, asset.id)">{{ asset.display_name }}</a></td>
                  <td>{{ asset.media_kind }}</td>
                  <td>{{ asset.taken_at ?? "" }}</td>
                </tr>
              </tbody>
            </table>
            <div v-else-if="trackAssets.length > 0" class="tile-grid">
              <article v-for="(asset, index) in trackAssets" :key="asset.id" class="asset-tile">
                <button type="button" class="tile-media" @click="openGallery(trackAssets.map(assetToGallery), index)">
                  <img
                    v-if="asset.media_kind === 'photo' && !failedPreviewIds.has(asset.id)"
                    :src="`/api/v1/media/${asset.id}/preview`"
                    alt=""
                    loading="lazy"
                    @error="markPreviewFailed(asset.id)"
                  />
                  <span v-else-if="asset.media_kind === 'audio'" class="audio-tile-preview">
                    <i class="bi bi-soundwave" aria-hidden="true"></i>
                    <audio :src="`/api/v1/media/${asset.id}/original`" controls preload="metadata" @click.stop></audio>
                    <small>{{ asset.display_name }}</small>
                  </span>
                  <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                </button>
                <a class="tile-title link-button" :href="assetHref(asset.id)" @click="openAssetLink($event, asset.id)">
                  {{ asset.display_name }}
                </a>
                <small class="tile-badges">
                  <span :class="['status-badge', assetHashStatus(asset) === 'hashed' ? 'ok' : 'warn']">{{ assetHashStatus(asset) }}</span>
                  <span v-if="assetHasGeo(asset)" class="status-badge ok">geotag</span>
                </small>
              </article>
            </div>
          </div>
          <table v-if="tracks.length > 0">
            <thead><tr><th>Name</th><th>Source</th><th>Points</th><th>Start</th><th>End</th><th>Distance</th></tr></thead>
            <tbody>
              <tr v-for="track in tracks" :key="track.track_asset_id">
                <td><button type="button" class="link-button" @click="openTrack(track.track_asset_id)">{{ track.name }}</button></td>
                <td>{{ track.source_format ?? "" }}</td>
                <td>{{ track.point_count }}</td>
                <td>{{ track.start_time ?? "" }}</td>
                <td>{{ track.end_time ?? "" }}</td>
                <td>{{ track.distance_m ? `${(track.distance_m / 1000).toFixed(2)} km` : "" }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else-if="active === 'Map'" class="panel">
          <header class="panel-head">
            <h2>Map</h2>
            <div class="actions">
              <span>{{ mapFeatureSummary }}</span>
              <span>{{ tileStatus }}</span>
              <button type="button" @click="refreshMap">Refresh map</button>
              <button type="button" @click="startMetadata">Run metadata enrichment for indexed assets</button>
              <button v-if="mapAlbumId || mapMediaKind || mapTrackId" type="button" @click="mapAlbumId = ''; mapMediaKind = ''; mapTrackId = ''; refreshMap()">
                Clear filters
              </button>
            </div>
          </header>
          <div class="control-grid">
            <label>
              Media kind
              <select v-model="mapMediaKind">
                <option value="">All</option>
                <option value="photo">Photos</option>
                <option value="video">Videos</option>
                <option value="audio">Audio</option>
              </select>
            </label>
            <label>
              Album
              <select v-model="mapAlbumId">
                <option value="">All albums</option>
                <option v-for="album in albums" :key="album.id" :value="album.id">{{ album.title }}</option>
              </select>
            </label>
            <label>
              Track
              <select v-model="mapTrackId">
                <option value="">All tracks</option>
                <option v-for="track in tracks" :key="track.track_asset_id" :value="track.track_asset_id">
                  {{ track.name }}
                </option>
              </select>
            </label>
            <label class="checkbox-label">
              <input v-model="mapCluster" type="checkbox" />
              Clusters
            </label>
          </div>
          <div v-if="mapAlbumId" class="filter-pill">
            Album filter:
            {{ albums.find((album) => album.id === mapAlbumId)?.title ?? mapAlbumId }}
            <button type="button" @click="mapAlbumId = ''; refreshMap()">Clear album filter</button>
          </div>
          <div v-if="mapWarnings.length > 0" class="alert">
            <p v-for="warning in mapWarnings" :key="warning">{{ warning }}</p>
          </div>
          <p v-if="mapFeatures.length === 0" class="empty-state">
            No map features match the current filters.
            <span v-if="mapAlbumId">The selected album may have no mapped assets. Clear the album filter to check all geotagged media.</span>
            <span v-else>If assets are indexed but not geotagged, run metadata enrichment or choose a prefix with EXIF/GPS metadata.</span>
          </p>
          <div class="map-shell map-page-shell">
            <div ref="mapElement" class="ol-map" role="img" aria-label="OpenLayers map"></div>
            <div class="map-layer-control">
              <button type="button" class="icon-button" @click="showMapLayerMenu = !showMapLayerMenu">
                <i class="bi bi-layers" aria-hidden="true"></i>
                Layers
              </button>
              <div v-if="showMapLayerMenu" class="layer-menu">
                <label class="form-check form-switch">
                  <input v-model="mapTilesVisible" class="form-check-input" type="checkbox" />
                  <span>OSM tiles</span>
                </label>
                <label class="form-check form-switch">
                  <input v-model="mapTracksVisible" class="form-check-input" type="checkbox" />
                  <span>Tracks</span>
                </label>
                <label class="form-check form-switch">
                  <input v-model="mapAssetsVisible" class="form-check-input" type="checkbox" />
                  <span>Photos/videos</span>
                </label>
                <button type="button" class="btn btn-sm btn-outline-primary" @click="fitMainMap">
                  <i class="bi bi-aspect-ratio" aria-hidden="true"></i>
                  Fit features
                </button>
                <small>OSM tiles are on-demand through the local tile proxy.</small>
              </div>
            </div>
          </div>
          <div v-show="mapPopup" ref="mapPopupElement" class="map-popup">
            <div class="job-row">
              <strong>{{ mapPopup?.title }}</strong>
              <button type="button" class="icon-button danger-close" @click="mapPopup = null">
                <i class="bi bi-x-lg" aria-hidden="true"></i>
                Close
              </button>
            </div>
            <p>{{ mapPopup?.summary }}</p>
            <p v-if="mapPopup?.kind === 'cluster' && mapPopup.count && mapPopup.assets.length < mapPopup.count" class="muted">
              Use Zoom to cluster to split nearby points. If points share identical coordinates, use the gallery below.
            </p>
            <div v-if="mapPopup?.kind === 'cluster'" class="actions">
              <button type="button" class="btn btn-sm btn-outline-primary" @click="zoomMapToCluster(mapPopup?.bbox)">
                <i class="bi bi-zoom-in" aria-hidden="true"></i>
                Zoom to cluster
              </button>
            </div>
            <div v-if="mapPopup?.kind === 'track'" class="track-popup-details">
              <p v-if="mapPopup.track_info_loading" class="muted">Loading nearest track point...</p>
              <template v-if="mapPopup.track_info">
                <p>
                  Clicked {{ mapPopup.track_info.clicked.lat.toFixed(6) }},
                  {{ mapPopup.track_info.clicked.lon.toFixed(6) }}
                </p>
                <p>
                  Nearest {{ mapPopup.track_info.nearest.lat.toFixed(6) }},
                  {{ mapPopup.track_info.nearest.lon.toFixed(6) }} ·
                  {{ Math.round(mapPopup.track_info.distance_m) }} m from click
                </p>
	                <p v-if="mapPopup.track_info.timestamp">
	                  Time {{ mapPopup.track_info.timestamp }}
	                  <span v-if="typeof mapPopup.track_info.relative_time_seconds === 'number'">
	                    · +{{ Math.round(mapPopup.track_info.relative_time_seconds) }} s
	                  </span>
	                </p>
	                <p>
	                  <span v-if="typeof mapPopup.track_info.speed_mps === 'number'">Speed {{ mapPopup.track_info.speed_mps.toFixed(2) }} m/s</span>
	                  <span v-if="typeof mapPopup.track_info.elevation_m === 'number'"> · Elevation {{ Math.round(mapPopup.track_info.elevation_m) }} m</span>
	                </p>
              </template>
              <div class="actions">
                <button type="button" @click="mapPopup?.track_id && openTrack(mapPopup.track_id)">Open in Track Manager</button>
                <button type="button" @click="mapLoadTrackTimeAssets">List media during track</button>
                <label class="inline-field">
                  Within meters
                  <input
                    :value="mapPopup.nearby_distance_m ?? 100"
                    type="number"
                    min="1"
                    max="5000"
                    @input="mapPopup && (mapPopup.nearby_distance_m = Number(($event.target as HTMLInputElement).value))"
                  />
                </label>
                <button type="button" @click="mapLoadTrackNearbyAssets">List nearby media</button>
                <button type="button" @click="mapPopup?.track_id && showOnlyMapTrack(mapPopup.track_id)">Show only this track</button>
              </div>
            </div>
            <div class="mini-gallery">
              <article v-for="asset in mapPopup?.assets ?? []" :key="asset.id" class="mini-card">
                <img
                  v-if="asset.media_kind === 'photo' && asset.preview_url"
                  :src="asset.preview_url"
                  alt=""
                  loading="lazy"
                />
                <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                <strong>{{ asset.name }}</strong>
                <div class="actions">
                  <button type="button" @click="openGallery((mapPopup?.assets ?? []).map((item) => ({
                    id: item.id,
                    name: item.name,
                    media_kind: item.media_kind,
                    preview_url: item.preview_url || `/api/v1/media/${item.id}/preview`,
                    original_url: item.original_url || `/api/v1/media/${item.id}/original`
                  })), (mapPopup?.assets ?? []).findIndex((item) => item.id === asset.id))">
                    Open viewer
                  </button>
                  <a class="btn btn-sm btn-outline-secondary" :href="assetHref(asset.id)" @click="openAssetLink($event, asset.id)">Asset detail</a>
                </div>
              </article>
            </div>
          </div>
          <p class="muted" v-if="tileSources.length > 0">
            Tile source: {{ tileSources[0].name }} · {{ tileSources[0].policy }}
          </p>
          <label class="form-check form-switch map-debug-toggle">
            <input v-model="showMapDebug" class="form-check-input" type="checkbox" />
            <span>Show debug GeoJSON</span>
          </label>
          <pre v-if="showMapDebug" class="geojson">{{ JSON.stringify(mapData, null, 2) }}</pre>
        </section>

        <section v-else-if="active === 'Transcoding'" class="panel">
          <header class="panel-head">
            <h2>Transcoding</h2>
            <span>{{ transcodingCapabilities?.ffmpeg.available ? "ffmpeg detected" : "ffmpeg unavailable" }}</span>
          </header>
          <div class="settings-tabs secondary-tabs">
            <button v-for="tab in ['capabilities', 'presets', 'rules', 'templates', 'planner', 'metrics']" :key="tab" type="button" :class="{ active: transcodePageTab === tab }" @click="transcodePageTab = tab">
              {{ tab }}
            </button>
          </div>
          <div class="metrics">
            <article><strong>{{ transcodingCapabilities?.ffmpeg.available ? "yes" : "no" }}</strong><span>ffmpeg</span></article>
            <article><strong>{{ transcodingCapabilities?.ffprobe.available ? "yes" : "no" }}</strong><span>ffprobe</span></article>
            <article><strong>{{ transcodingCapabilities?.encoders.length ?? 0 }}</strong><span>Video encoders</span></article>
            <article><strong>immutable</strong><span>Originals</span></article>
          </div>
          <div v-if="transcodePageTab === 'capabilities'" class="settings-grid">
            <article class="settings-form">
              <h3>Hardware</h3>
              <div class="detail-grid compact-detail">
                <article v-for="item in transcodeHardwareStatus" :key="item.label">
                  <strong>{{ item.value ? "available" : "unavailable" }}</strong>
                  <span>{{ item.label }}</span>
                  <small>{{ item.note }}</small>
                </article>
              </div>
            </article>
            <article class="settings-form settings-wide">
              <h3>Encoders</h3>
              <table>
                <thead><tr><th>Encoder</th><th>Codec</th><th>Hardware</th></tr></thead>
                <tbody>
                  <tr v-for="encoder in transcodingCapabilities?.encoders ?? []" :key="encoder.name">
                    <td>{{ encoder.name }}</td>
                    <td>{{ encoder.codec_family }}</td>
                    <td>{{ encoder.hardware || "cpu/software" }}</td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
          <div v-else-if="transcodePageTab === 'presets'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>Presets</h3>
              <p class="muted">Built-ins cannot be removed. Custom presets are saved as metadata and used by the video player Apply flow.</p>
              <table>
                <thead><tr><th>Name</th><th>Hardware</th><th>Codec</th><th>Encoder</th><th>Mode</th><th>Value</th><th>Status</th><th></th></tr></thead>
                <tbody>
                  <tr v-for="preset in transcodePresets" :key="preset.id">
                    <td>{{ preset.name }}</td>
                    <td>{{ preset.hardware }}</td>
                    <td>{{ preset.codec }}</td>
                    <td>{{ preset.ffmpeg_encoder }}</td>
                    <td>{{ preset.mode }}</td>
                    <td>{{ preset.parameter_value }}</td>
                    <td>{{ preset.available ? "available" : preset.disabled_reason }}</td>
                    <td>
                      <button v-if="!preset.built_in" type="button" class="btn btn-sm btn-outline-danger" @click="removeTranscodePreset(preset.id)">Remove</button>
                      <span v-else class="status-badge">built-in</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
          <div v-else-if="transcodePageTab === 'rules'" class="settings-grid">
            <article class="settings-form">
              <h3>Auto-Selection Rule Draft</h3>
              <p class="muted">MVP planning surface. Rule evaluation is conservative and does not start transcodes.</p>
              <label>Source codec contains <input v-model="transcodeRuleSourceCodec" type="text" placeholder="hevc, h264, vp9" /></label>
              <label>Target preset
                <select v-model="transcodeRuleTargetPreset">
                  <option v-for="preset in transcodePresets" :key="preset.id" :value="preset.id">{{ preset.name }}</option>
                </select>
              </label>
              <p class="alert">Current draft: if codec matches “{{ transcodeRuleSourceCodec || 'any' }}”, use {{ transcodeRuleTargetPreset }}. Output remains cache-only.</p>
            </article>
          </div>
          <div v-else-if="transcodePageTab === 'templates'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>Command Template Draft</h3>
              <p class="muted">Allowed variables: ${input}, ${output}, ${workdir}, ${preset}, ${width}, ${height}, ${fps}, ${source_codec}. Use argv-style implementation before enabling arbitrary templates.</p>
              <textarea v-model="transcodeTemplate" rows="5"></textarea>
              <p :class="['alert', transcodeTemplateSafe ? 'alert-info' : 'alert-danger']">
                {{ transcodeTemplateSafe ? "Template draft looks free of obvious shell separators." : "Template contains shell metacharacters and must be converted to safe argv form before use." }}
              </p>
            </article>
          </div>
          <div v-else-if="transcodePageTab === 'planner'" class="settings-grid">
            <article class="settings-form">
              <h3>Job Planner</h3>
              <p class="muted">{{ transcodePlannerMessage }}</p>
              <button type="button" class="btn btn-outline-primary" @click="transcodePlannerMessage = `Plan generated for ${selectedAssets.size || 'current video scope'} asset(s); execution is intentionally manual.`">
                Plan selected/current scope
              </button>
              <p class="alert">Original replacement, writes beside originals, and archive writes are disabled.</p>
            </article>
          </div>
          <div v-else-if="transcodePageTab === 'metrics'" class="settings-grid">
            <article class="settings-form">
              <h3>Quality Metrics Foundation</h3>
              <p class="muted">Short sample metrics can be added on top of the existing hardware-test path. Long metrics jobs are intentionally not launched here.</p>
              <div class="detail-grid compact-detail">
                <article v-for="metric in transcodeMetricsStatus" :key="metric.metric">
                  <strong>{{ metric.available ? "available" : "unavailable" }}</strong>
                  <span>{{ metric.metric }}</span>
                  <small>{{ metric.note }}</small>
                </article>
              </div>
            </article>
          </div>
        </section>

	        <section v-else-if="active === 'Base AI'" class="panel">
	          <header class="panel-head">
	            <h2>Base AI</h2>
	            <span>Local sidecar inference; no model downloads run automatically.</span>
	          </header>
	          <div class="metrics">
	            <article><strong>{{ (aiStatus?.accelerator_hints as Record<string, unknown> | undefined)?.cpu ? "yes" : "unknown" }}</strong><span>CPU</span></article>
	            <article><strong>{{ nativeCudaAvailable ? "available" : ((aiStatus?.accelerator_hints as Record<string, unknown> | undefined)?.nvidia_smi ? "present" : "no") }}</strong><span>Native CUDA</span></article>
	            <article><strong>{{ dockerNvidiaRuntime ? "detected" : "not detected" }}</strong><span>Docker NVIDIA</span></article>
	            <article><strong>{{ aiDevicePolicy.active_device ?? "cpu" }}</strong><span>Active AI device</span></article>
	            <article><strong>{{ (aiStatus?.accelerator_hints as Record<string, unknown> | undefined)?.dev_dri ? "yes" : "no" }}</strong><span>/dev/dri</span></article>
	            <article><strong>{{ aiCounts.asset_tags ?? 0 }}</strong><span>AI tags</span></article>
	            <article><strong>{{ aiCounts.predictions ?? 0 }}</strong><span>Predictions</span></article>
	            <article><strong>{{ aiCounts.face_detections ?? 0 }}</strong><span>Faces</span></article>
	            <article><strong>{{ vectorStatus?.embedded_assets ?? vectorLimits.embedded_assets ?? 0 }}</strong><span>Embedded assets</span></article>
	          </div>
	          <div class="ai-dashboard">
	            <article class="settings-form settings-wide ai-status-panel">
	              <div class="section-title">
	                <div>
	                  <h3>Worker Profiles</h3>
	                  <p class="muted">Native sidecar endpoint and optional Docker profiles. Model/cache paths remain outside originals.</p>
	                </div>
	                <span :class="['status-badge', configuredAIWorker ? 'ok' : 'warn']">
	                  {{ configuredAIWorker ? 'worker reachable' : 'no worker configured' }}
	                </span>
	              </div>
	              <div class="cards">
	                <article v-for="worker in aiWorkerRows" :key="String((worker as Record<string, unknown>).id)" class="card ai-card">
	                  <div class="job-row">
	                    <strong>{{ (worker as Record<string, unknown>).id }}</strong>
	                    <span :class="['status-badge', (worker as Record<string, unknown>).status === 'ok' ? 'ok' : 'warn']">
	                      {{ (worker as Record<string, unknown>).status }}
	                    </span>
	                  </div>
	                  <span>{{ (worker as Record<string, unknown>).profile }}</span>
	                  <small>{{ (worker as Record<string, unknown>).endpoint || "profile available when configured" }}</small>
	                  <small v-if="(worker as Record<string, unknown>).device">Device: {{ (worker as Record<string, unknown>).device }}</small>
	                  <small>{{ (worker as Record<string, unknown>).note || ((worker as Record<string, unknown>).available === false ? "hardware not detected" : "hardware/profile available") }}</small>
	                </article>
	              </div>
	            </article>

	            <article class="settings-form settings-wide">
	              <div class="section-title">
	                <div>
	                  <h3>Models And Scoped Actions</h3>
	                  <p class="muted">Buttons run on selected assets or the current 54-asset indexed real-peek scope. Nothing scans new files.</p>
	                </div>
	                <span v-if="aiBusyKind" class="status-badge warn">
	                  <span class="spinner-border spinner-border-sm" aria-hidden="true"></span>
	                  {{ aiBusyKind }}
	                </span>
	              </div>
	              <div class="cards">
	                <article v-for="card in aiModelCards" :key="card.key" class="card ai-card">
	                  <div class="job-row">
	                    <strong>{{ card.label }}</strong>
	                    <span :class="['status-badge', card.model?.loaded || card.model?.available ? 'ok' : 'warn']">
	                      {{ card.model?.loaded ? 'loaded' : card.model?.available ? 'available' : 'lazy/not loaded' }}
	                    </span>
	                  </div>
	                  <span>{{ card.model?.name ?? 'model status unknown' }}</span>
	                  <small v-if="card.model?.threshold">threshold {{ card.model.threshold }}</small>
	                  <button
	                    type="button"
	                    class="btn btn-sm btn-outline-primary"
	                    :disabled="Boolean(aiBusyKind)"
	                    @click="requestAIModelAction(card.action)"
	                  >
	                    <i class="bi bi-play-circle" aria-hidden="true"></i>
	                    Run {{ card.label }}
	                  </button>
	                </article>
	              </div>
	              <div v-if="aiMessage" class="ai-result-panel">
	                <strong>Latest action</strong>
	                <span>{{ aiMessage }}</span>
	                <button type="button" class="btn btn-sm btn-outline-secondary" @click="setActive('Jobs')">Open Jobs</button>
	              </div>
	              <pre v-if="aiLastResult" class="compact-json">{{ JSON.stringify(aiLastResult, null, 2) }}</pre>
	            </article>

	            <article class="settings-form">
	              <h3>Vector Store</h3>
	              <p><strong>{{ vectorStatus?.backend ?? "none" }}</strong></p>
	              <p class="muted">
	                {{ vectorStatus?.contract ?? "Vector contract unavailable." }}
	              </p>
	              <div class="detail-grid compact-detail">
	                <article><strong>{{ vectorStatus?.embedded_assets ?? vectorLimits.embedded_assets ?? 0 }}</strong><span>Embedded assets</span></article>
	                <article><strong>{{ vectorStatus?.dimensions ?? "n/a" }}</strong><span>Dimensions</span></article>
	                <article><strong>{{ vectorStatus?.pgvector ? "yes" : "optional" }}</strong><span>pgvector</span></article>
	              </div>
	              <p class="muted">{{ vectorStatus?.pgvector_note ?? "Local JSON/bruteforce fallback is active for small collections." }}</p>
	              <div class="actions">
	                <button type="button" class="btn btn-primary btn-sm" @click="configureVectorStore">
	                  <i class="bi bi-sliders" aria-hidden="true"></i>
	                  Configure Vector Store
	                </button>
	                <button type="button" class="btn btn-outline-primary btn-sm" :disabled="Boolean(aiBusyKind)" @click="requestAIJob('embed')">
	                  Generate embeddings
	                </button>
	              </div>
	            </article>

	            <article class="settings-form">
	              <h3>Recent AI Activity</h3>
	              <div v-if="aiActionHistory.length === 0 && recentAIJobs.length === 0" class="empty-state">No AI action has been started in this browser session.</div>
	              <table v-else>
	                <thead><tr><th>Kind</th><th>Status</th><th>Summary</th></tr></thead>
	                <tbody>
	                  <tr v-for="item in aiActionHistory" :key="item.id">
	                    <td>{{ item.kind }}</td>
	                    <td>{{ item.status }}</td>
	                    <td>{{ item.summary }}</td>
	                  </tr>
	                  <tr v-for="job in recentAIJobs" :key="job.id">
	                    <td>{{ job.kind }}</td>
	                    <td>{{ job.status }}</td>
	                    <td>{{ job.progress_current }} / {{ job.progress_total ?? 0 }}</td>
	                  </tr>
	                </tbody>
	              </table>
	            </article>
	          </div>
	          <details>
	            <summary>Raw AI status</summary>
	            <pre class="geojson">{{ JSON.stringify({ aiStatus, aiWorkers, vectorStatus }, null, 2) }}</pre>
	          </details>
	        </section>

        <section v-else-if="active === 'AI Classification'" class="panel ai-classification-page">
          <header class="panel-head">
            <h2>AI Classification</h2>
            <span>Local predictions, tags, faces, safety review, and vector search. Predictions are suggestions, not truth.</span>
          </header>
          <div class="metrics">
            <article><strong>{{ aiCounts.asset_tags ?? 0 }}</strong><span>AI tags</span></article>
            <article><strong>{{ aiCounts.predictions ?? 0 }}</strong><span>Predictions</span></article>
            <article><strong>{{ aiCounts.face_detections ?? 0 }}</strong><span>Face detections</span></article>
            <article><strong>{{ vectorStatus?.embedded_assets ?? vectorLimits.embedded_assets ?? 0 }}</strong><span>Embedded assets</span></article>
            <article><strong>{{ aiCounts.safety_candidates ?? 0 }}</strong><span>Safety candidates</span></article>
          </div>
          <div class="settings-grid">
            <article class="settings-form">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-tags" aria-hidden="true"></i> Tags / Categories</h3>
                  <p class="muted">Click a tag to search the Explorer.</p>
                </div>
              </div>
              <div v-if="aiTags.length === 0" class="empty-state">No AI tags stored yet.</div>
              <div v-else class="chip-row">
                <button
                  v-for="tag in aiTags"
                  :key="`${String((tag as Record<string, unknown>).source)}-${String((tag as Record<string, unknown>).tag)}`"
                  type="button"
                  class="chip button-chip"
                  @click="explorerQ = `tag:${String((tag as Record<string, unknown>).tag)}`; setActive('Explorer')"
                >
                  {{ (tag as Record<string, unknown>).tag }}
                  <small>{{ (tag as Record<string, unknown>).count }}</small>
                </button>
              </div>
            </article>

            <article class="settings-form">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-shield-check" aria-hidden="true"></i> Safety Review</h3>
                  <p class="muted">Local NSFW/safety labels remain reviewable metadata only.</p>
                </div>
                <button v-if="(aiSafetyPayload?.review_album as Record<string, unknown> | undefined)?.id" type="button" class="btn btn-sm btn-outline-danger" @click="selectedAlbumId = String((aiSafetyPayload?.review_album as Record<string, unknown>).id); setActive('Albums'); selectAlbum(selectedAlbumId)">
                  Potentially Unsafe Album
                </button>
              </div>
              <div v-if="aiSafetyCandidates.length === 0" class="empty-state">No safety candidates require review.</div>
              <table v-else>
                <thead><tr><th>Asset</th><th>Tag</th><th>Source</th><th>Action</th></tr></thead>
                <tbody>
                  <tr v-for="candidate in aiSafetyCandidates" :key="`${String((candidate as Record<string, unknown>).asset_id)}-${String((candidate as Record<string, unknown>).tag)}`">
                    <td><button type="button" class="link-button" @click="openAsset(String((candidate as Record<string, unknown>).asset_id))">{{ String((candidate as Record<string, unknown>).asset_id).slice(0, 8) }}</button></td>
                    <td>{{ (candidate as Record<string, unknown>).tag }}</td>
                    <td>{{ (candidate as Record<string, unknown>).source }}</td>
                    <td><button type="button" class="btn btn-sm btn-outline-secondary" @click="openAsset(String((candidate as Record<string, unknown>).asset_id))">Review</button></td>
                  </tr>
                </tbody>
              </table>
            </article>

            <article class="settings-form settings-wide">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-table" aria-hidden="true"></i> Recent Predictions</h3>
                  <p class="muted">{{ aiPredictions.length }} loaded<span v-if="aiPredictionTotal"> of {{ aiPredictionTotal }}</span> predictions across classification, captions, safety, and OCR.</p>
                </div>
                <div class="actions">
                  <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadMoreAILists('predictions')">Load more</button>
                  <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadAllAILists('predictions')">Load all</button>
                </div>
              </div>
              <div v-if="aiPredictions.length === 0" class="empty-state">No AI predictions stored yet.</div>
              <table v-else>
                <thead><tr><th>Asset</th><th>Task</th><th>Label</th><th>Confidence</th><th>Model</th><th>Created</th></tr></thead>
                <tbody>
                  <tr v-for="prediction in aiPredictions" :key="String((prediction as Record<string, unknown>).id)">
                    <td><button type="button" class="link-button" @click="openAsset(String((prediction as Record<string, unknown>).asset_id))">{{ String((prediction as Record<string, unknown>).asset_id).slice(0, 8) }}</button></td>
                    <td>{{ (prediction as Record<string, unknown>).task }}</td>
                    <td>{{ (prediction as Record<string, unknown>).label }}</td>
                    <td>{{ typeof (prediction as Record<string, unknown>).confidence === 'number' ? Number((prediction as Record<string, unknown>).confidence).toFixed(3) : '' }}</td>
                    <td>{{ (prediction as Record<string, unknown>).model_name }}</td>
                    <td>{{ (prediction as Record<string, unknown>).created_at }}</td>
                  </tr>
                </tbody>
              </table>
            </article>

            <article class="settings-form">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-person-bounding-box" aria-hidden="true"></i> Face Detections</h3>
                  <p class="muted">{{ aiFaces.length }} loaded<span v-if="aiFaceTotal"> of {{ aiFaceTotal }}</span> local-only face boxes. No real-world identity is inferred.</p>
                </div>
                <div class="actions">
                  <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiFaces.length >= aiFaceTotal" @click="loadMoreAILists('faces')">Load more</button>
                  <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiFaces.length >= aiFaceTotal" @click="loadAllAILists('faces')">Load all</button>
                </div>
              </div>
              <div v-if="aiFaces.length === 0" class="empty-state">No face detections stored.</div>
              <table v-else>
                <thead><tr><th>Asset</th><th>Confidence</th><th>Box</th></tr></thead>
                <tbody>
                  <tr v-for="face in aiFaces" :key="String((face as Record<string, unknown>).id)">
                    <td><button type="button" class="link-button" @click="openAsset(String((face as Record<string, unknown>).asset_id))">{{ String((face as Record<string, unknown>).asset_id).slice(0, 8) }}</button></td>
                    <td>{{ typeof (face as Record<string, unknown>).confidence === 'number' ? Number((face as Record<string, unknown>).confidence).toFixed(3) : '' }}</td>
                    <td>{{ Number((face as Record<string, unknown>).width ?? 0).toFixed(2) }} × {{ Number((face as Record<string, unknown>).height ?? 0).toFixed(2) }}</td>
                  </tr>
                </tbody>
              </table>
            </article>

            <article class="settings-form">
              <h3><i class="bi bi-search" aria-hidden="true"></i> Vector Text Search</h3>
              <p class="muted">Uses the local JSON/brute-force fallback over stored OpenCLIP embeddings.</p>
              <div class="input-group">
                <input v-model="aiVectorQuery" class="form-control" type="search" placeholder="e.g. mountain road" @keyup.enter="runAIVectorSearch" />
                <button type="button" class="btn btn-primary" @click="runAIVectorSearch">Search</button>
              </div>
              <div v-if="aiVectorResults.length === 0" class="empty-state">No vector search has been run in this session.</div>
              <table v-else>
                <thead><tr><th>Asset</th><th>Score</th><th>Match</th></tr></thead>
                <tbody>
                  <tr v-for="result in aiVectorResults" :key="String(((result.asset as Record<string, unknown> | undefined)?.id) ?? result.match)">
                    <td><button type="button" class="link-button" @click="openAsset(String((result.asset as Record<string, unknown>).id))">{{ (result.asset as Record<string, unknown>)?.display_name }}</button></td>
                    <td>{{ Number(result.score ?? 0).toFixed(3) }}</td>
                    <td>{{ result.match }}</td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'OCR'" class="panel">
          <header class="panel-head">
            <div>
              <h2>OCR Text</h2>
              <span>Search and inspect OCR text blocks stored as local metadata with bounding boxes.</span>
            </div>
            <button type="button" class="btn btn-outline-primary" @click="requestAIJob('ocr')">
              <i class="bi bi-body-text" aria-hidden="true"></i>
              OCR current scope
            </button>
          </header>
          <div class="search-hero compact-search">
            <label>
              <span>Filter OCR records</span>
              <input v-model="ocrPageQuery" type="search" placeholder="text, language, model, asset id..." />
            </label>
          </div>
          <article class="settings-form settings-wide">
            <div class="section-title">
              <div>
                <h3>OCR Records</h3>
                <p class="muted">{{ ocrPredictionRows.length }} matching rows from {{ aiPredictions.length }} loaded predictions<span v-if="aiPredictionTotal"> of {{ aiPredictionTotal }}</span>.</p>
              </div>
              <div class="actions">
                <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadMoreAILists('predictions')">Load more</button>
                <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadAllAILists('predictions')">Load all</button>
              </div>
            </div>
            <div v-if="ocrPredictionRows.length === 0" class="empty-state">No OCR records match this filter.</div>
            <table v-else>
              <thead><tr><th>Text</th><th>Language</th><th>Confidence</th><th>Model</th><th>Created</th><th>Asset</th></tr></thead>
              <tbody>
                <tr v-for="row in ocrPredictionRows" :key="String((row as Record<string, unknown>).id)">
                  <td>{{ (row as Record<string, unknown>).label }}</td>
                  <td>{{ ((row as Record<string, unknown>).metadata as Record<string, unknown> | undefined)?.language ?? '' }}</td>
                  <td>{{ typeof (row as Record<string, unknown>).confidence === 'number' ? Number((row as Record<string, unknown>).confidence).toFixed(2) : '' }}</td>
                  <td>{{ (row as Record<string, unknown>).model_name }}</td>
                  <td>{{ (row as Record<string, unknown>).created_at }}</td>
                  <td>
                    <a
                      v-if="recordAssetID(row)"
                      class="btn btn-sm btn-outline-secondary"
                      :href="assetHref(recordAssetID(row))"
                      @click="openAssetOCRLink($event, recordAssetID(row), String((row as Record<string, unknown>).id ?? ''))"
                    >
                      Open + highlight
                    </a>
                    <span v-else class="status-badge warn">missing asset id</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </article>
        </section>

        <section v-else-if="active === 'Transcripts'" class="panel">
          <header class="panel-head">
            <div>
              <h2>Transcripts</h2>
              <span>Search speech transcripts extracted from audio and video. Transcript text is local metadata only.</span>
            </div>
            <button type="button" class="btn btn-outline-primary" @click="requestAIJob('transcribe')">
              <i class="bi bi-soundwave" aria-hidden="true"></i>
              Transcribe current scope
            </button>
          </header>
          <div class="list-toolbar">
            <TypeaheadSearch
              v-model="transcriptQuery"
              :results="transcriptTypeaheadResults"
              :loading="transcriptLoading"
              placeholder="Search transcript text, language, model, or asset id..."
              go-label="Search transcripts"
              @select="selectTranscriptSearchResult"
              @go="searchTranscripts"
            />
            <div class="actions">
              <button type="button" class="btn btn-sm btn-outline-primary" :disabled="transcriptLoading" @click="fetchTranscriptsPage(true)">Refresh list</button>
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="transcriptLoading || !transcriptsHasMore" @click="fetchTranscriptsPage(false)">Load more</button>
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="transcriptLoading || !transcriptsHasMore" @click="loadAllTranscripts">Load all</button>
            </div>
          </div>
          <p v-if="transcriptSearchMessage" class="alert">{{ transcriptSearchMessage }}</p>
          <div v-if="transcriptSearchResults.length" class="search-result-tiles">
            <article v-for="result in transcriptSearchResults" :key="result.asset.id" class="search-result-card">
              <img v-if="result.asset.media_kind === 'photo'" :src="`/api/v1/media/${result.asset.id}/preview`" alt="" loading="lazy" />
              <span v-else class="media-fallback">{{ result.asset.media_kind }}</span>
              <strong>{{ result.asset.display_name }}</strong>
              <small>{{ result.explanation }}</small>
              <a class="btn btn-sm btn-outline-secondary" :href="assetHref(result.asset.id)" @click="openAssetLink($event, result.asset.id)">Open asset</a>
            </article>
          </div>
          <article class="settings-form settings-wide">
            <div class="section-title">
              <div>
                <h3>Stored Transcripts</h3>
                <p class="muted">{{ visibleTranscriptRows.length }} matching rows from {{ transcriptRows.length }} loaded transcripts.</p>
              </div>
            </div>
            <div v-if="visibleTranscriptRows.length === 0" class="empty-state">
              No transcripts match this filter. Run ASR from Base AI or an audio/video asset page, or load more rows.
            </div>
            <table v-else>
              <thead><tr><th>Transcript</th><th>Language</th><th>Source</th><th>Model</th><th>Created</th><th>Asset</th></tr></thead>
              <tbody>
                <tr v-for="transcript in visibleTranscriptRows" :key="transcript.id">
                  <td class="wide-text-cell">{{ transcript.full_text.slice(0, 240) }}</td>
                  <td>{{ transcript.language ?? "" }}</td>
                  <td>{{ transcript.source_kind }}</td>
                  <td>{{ transcript.model ?? "" }}</td>
                  <td>{{ transcript.created_at }}</td>
                  <td>
                    <a class="btn btn-sm btn-outline-secondary" :href="assetHref(transcript.asset_id)" @click="openAssetLink($event, transcript.asset_id)">
                      Open asset
                    </a>
                  </td>
                </tr>
              </tbody>
            </table>
          </article>
        </section>

        <section v-else-if="active === 'Captions'" class="panel">
          <header class="panel-head">
            <div>
              <h2>Captions</h2>
              <span>Browse generated short and long captions. Captions are suggestions and remain user-reviewable.</span>
            </div>
            <button type="button" class="btn btn-outline-primary" @click="requestAIJob('describe')">
              <i class="bi bi-chat-square-text" aria-hidden="true"></i>
              Caption current scope
            </button>
          </header>
          <div class="search-hero compact-search">
            <label>
              <span>Filter captions</span>
              <input v-model="captionsPageQuery" type="search" placeholder="caption text, task, model, asset id..." />
            </label>
          </div>
          <article class="settings-form settings-wide">
            <div class="section-title">
              <div>
                <h3>Caption Records</h3>
                <p class="muted">{{ captionPredictionRows.length }} matching rows from {{ aiPredictions.length }} loaded predictions<span v-if="aiPredictionTotal"> of {{ aiPredictionTotal }}</span>.</p>
              </div>
              <div class="actions">
                <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadMoreAILists('predictions')">Load more</button>
                <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="aiPredictions.length >= aiPredictionTotal" @click="loadAllAILists('predictions')">Load all</button>
              </div>
            </div>
            <div v-if="captionPredictionRows.length === 0" class="empty-state">No captions match this filter.</div>
            <table v-else>
              <thead><tr><th>Caption</th><th>Task</th><th>Model</th><th>Created</th><th>Asset</th></tr></thead>
              <tbody>
                <tr v-for="row in captionPredictionRows" :key="String((row as Record<string, unknown>).id)">
                  <td>{{ (row as Record<string, unknown>).label }}</td>
                  <td>{{ (row as Record<string, unknown>).task }}</td>
                  <td>{{ (row as Record<string, unknown>).model_name }}</td>
                  <td>{{ (row as Record<string, unknown>).created_at }}</td>
                  <td>
                    <a
                      v-if="recordAssetID(row)"
                      class="btn btn-sm btn-outline-secondary"
                      :href="assetHref(recordAssetID(row))"
                      @click="openAssetLink($event, recordAssetID(row))"
                    >
                      Open asset
                    </a>
                    <span v-else class="status-badge warn">missing asset id</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </article>
        </section>

        <section v-else-if="active === 'Safety Review'" class="panel">
          <header class="panel-head">
            <div>
              <h2>Safety Review</h2>
              <span>Review local NSFW/safety predictions. Nothing is moved, hidden, or deleted by this page.</span>
            </div>
            <button type="button" class="btn btn-outline-danger" @click="requestAIJob('safety')">
              <i class="bi bi-shield-exclamation" aria-hidden="true"></i>
              Safety scan current scope
            </button>
          </header>
          <div class="settings-grid">
            <article class="settings-form">
              <h3>Summary</h3>
              <div class="detail-grid compact-detail">
                <article><strong>{{ aiCounts.safety_candidates ?? 0 }}</strong><span>Candidates</span></article>
                <article><strong>{{ aiSafetyCandidates.length }}</strong><span>Loaded rows</span></article>
                <article><strong>{{ (aiSafetyPayload?.review_album as Record<string, unknown> | undefined)?.title ?? 'Potentially Unsafe' }}</strong><span>Virtual album</span></article>
              </div>
            </article>
            <article class="settings-form settings-wide">
              <h3>Needs Review / Unsafe Candidates</h3>
              <div v-if="aiSafetyCandidates.length === 0" class="empty-state">No safety candidates require review.</div>
              <table v-else>
                <thead><tr><th>Asset</th><th>Tag</th><th>Source</th><th>Confidence</th><th>Action</th></tr></thead>
                <tbody>
                  <tr v-for="candidate in aiSafetyCandidates" :key="`${String((candidate as Record<string, unknown>).asset_id)}-${String((candidate as Record<string, unknown>).tag)}`">
                    <td>{{ String((candidate as Record<string, unknown>).asset_id).slice(0, 8) }}</td>
                    <td>{{ (candidate as Record<string, unknown>).tag }}</td>
                    <td>{{ (candidate as Record<string, unknown>).source }}</td>
                    <td>{{ typeof (candidate as Record<string, unknown>).confidence === 'number' ? Number((candidate as Record<string, unknown>).confidence).toFixed(3) : '' }}</td>
                    <td><button type="button" class="btn btn-sm btn-outline-secondary" @click="openAsset(String((candidate as Record<string, unknown>).asset_id))">Review asset</button></td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Search'" class="panel universal-search-page">
          <header class="panel-head">
            <div>
              <h2>Universal Search</h2>
              <span>Search media and GPS/KML tracks by names, paths, dates, hashes, metadata, EXIF, AI tags, classes, captions, albums, or tracks.</span>
            </div>
          </header>
          <form class="search-hero" @submit.prevent="runUniversalSearch">
            <label>
              <span>Search query</span>
              <input v-model="universalSearchQ" type="search" placeholder="jpg, 2026-05, PXL, camera, safety, mountain, track name..." />
            </label>
            <button type="button" class="btn btn-outline-secondary" @click="parseUniversalSearch">
              Parse
            </button>
            <button type="submit" class="btn btn-primary">
              <i class="bi bi-search" aria-hidden="true"></i>
              Search
            </button>
          </form>
          <section class="settings-form search-planner-card">
            <h3><i class="bi bi-magic" aria-hidden="true"></i> Ask Cartolensia</h3>
            <p class="muted">Describe the files you want in English or Russian. Local planning is deterministic unless a local LLM endpoint is configured; no remote API is used.</p>
            <div class="inline-form-row">
              <input v-model="naturalSearchQ" type="search" placeholder="find videos with trains in May 2026 / покажи видео с поездом" @keyup.enter="planNaturalLanguageSearch" />
              <button type="button" class="btn btn-outline-primary" @click="planNaturalLanguageSearch">Plan query</button>
              <button type="button" class="btn btn-primary" :disabled="!universalSearchQ.trim()" @click="runUniversalSearch">Run</button>
            </div>
            <p v-if="naturalSearchMessage" class="muted">{{ naturalSearchMessage }}</p>
          </section>
          <div v-if="universalSearchPlan" class="settings-form search-plan-card">
            <div class="split-row">
              <div>
                <h3>Parsed Query</h3>
                <p class="muted">{{ universalSearchPlan.planner }} · {{ universalSearchPlan.backend }} · {{ universalSearchPlan.llm_status }}</p>
              </div>
              <code>{{ universalSearchPlan.executable_query }}</code>
            </div>
            <div class="chip-row">
              <span v-for="clause in universalSearchPlan.clauses" :key="`${clause.field}-${clause.value}-${clause.token}`" class="status-badge">
                {{ clause.field }} {{ clause.operator }} {{ clause.value }}
              </span>
            </div>
            <p v-for="note in universalSearchPlan.notes" :key="note" class="muted">{{ note }}</p>
          </div>
          <p v-if="universalSearchMessage" class="alert">{{ universalSearchMessage }}</p>
          <p v-if="universalSearchBackend" class="muted">
            Search backend: {{ universalSearchBackend }}. Curated PostgreSQL/pgvector views are used for production-scale local search; external search clusters remain optional future adapters.
          </p>
          <details class="search-help-panel">
            <summary>Search syntax</summary>
            <div class="chip-row">
              <span class="status-badge">space = AND</span>
              <span class="status-badge">comma = OR groups</span>
              <span class="status-badge">* and ? wildcards</span>
              <span class="status-badge">quoted phrases</span>
            </div>
            <p class="muted">
              Examples: <code>ext:mp4</code>, <code>kind:video filename:PXL_20260512*</code>,
              <code>ocr:"station"</code>, <code>caption:train</code>, <code>camera:Pixel</code>,
              <code>place:Armenia</code>, or <code>filename:PXL*, metadata word</code>.
            </p>
            <p class="muted">
              SQL-like clauses are translated safely: <code>kind = video and ext = mp4 and caption contains "train"</code>.
            </p>
          </details>
          <details class="search-help-panel">
            <summary>Read-only SQL over search views</summary>
            <p class="muted">
              Advanced diagnostics only. Queries must be a single <code>SELECT</code> against <code>cartolensia_search_*</code> views.
              Mutating SQL, semicolons, comments, and raw table access are rejected by the backend.
            </p>
            <textarea v-model="sqlSearchQ" rows="4" class="wide-textarea"></textarea>
            <div class="inline-form-row">
              <button type="button" class="btn btn-outline-primary" @click="runReadOnlySQLSearch">Run read-only query</button>
              <span v-if="sqlSearchMessage" class="muted">{{ sqlSearchMessage }}</span>
            </div>
            <div v-if="sqlSearchResult" class="table-scroll">
              <p class="muted">{{ sqlSearchResult.note }}</p>
              <table>
                <thead>
                  <tr><th v-for="column in sqlSearchResult.columns" :key="column">{{ column }}</th></tr>
                </thead>
                <tbody>
                  <tr v-for="(row, idx) in sqlSearchResult.rows" :key="idx">
                    <td v-for="column in sqlSearchResult.columns" :key="column">{{ row[column] }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </details>
          <div v-if="universalSearchWarnings.length" class="chip-row">
            <span v-for="warning in universalSearchWarnings" :key="warning" class="status-badge warn">{{ warning }}</span>
          </div>
          <div v-if="universalSearchPlaceResults.length" class="search-place-row">
            <article v-for="place in universalSearchPlaceResults" :key="`${place.provider}-${place.name}`" class="settings-form search-place-card">
              <div>
                <h3><i class="bi bi-geo-alt" aria-hidden="true"></i> {{ place.display_name || place.name }}</h3>
                <p class="muted">Matched local place cache · {{ place.matched_assets }} geotagged media assets inside bbox.</p>
              </div>
              <div class="detail-grid">
                <article><strong>{{ place.lat.toFixed(4) }}, {{ place.lon.toFixed(4) }}</strong><span>Center</span></article>
                <article><strong>{{ place.bbox.min_lat.toFixed(3) }}..{{ place.bbox.max_lat.toFixed(3) }}</strong><span>Latitude bbox</span></article>
                <article><strong>{{ place.bbox.min_lon.toFixed(3) }}..{{ place.bbox.max_lon.toFixed(3) }}</strong><span>Longitude bbox</span></article>
                <article><strong>{{ place.source }}</strong><span>Source</span></article>
              </div>
            </article>
          </div>
          <div class="search-results-grid">
            <article class="settings-form">
              <h3><i class="bi bi-images" aria-hidden="true"></i> Multimedia</h3>
              <div v-if="universalSearchResults.length === 0" class="empty-state">No media results yet.</div>
              <div v-else class="tile-grid search-result-tiles">
                <article v-for="result in universalSearchResults" :key="result.asset.id" class="asset-tile">
                  <button type="button" class="tile-media" @click="openGallery([assetToGallery(result.asset)], 0)">
                    <img v-if="result.asset.media_kind === 'photo'" :src="`/api/v1/media/${result.asset.id}/preview`" :alt="result.asset.display_name" loading="lazy" />
                    <video v-else-if="result.asset.media_kind === 'video'" :src="`/api/v1/media/${result.asset.id}/original`" preload="metadata" muted></video>
                    <img v-else-if="result.asset.media_kind === 'track'" :src="`/api/v1/media/${result.asset.id}/track-thumbnail`" :alt="result.asset.display_name" loading="lazy" />
                    <span v-else-if="result.asset.media_kind === 'audio'" class="audio-tile-preview">
                      <i class="bi bi-soundwave" aria-hidden="true"></i>
                      <audio :src="`/api/v1/media/${result.asset.id}/original`" controls preload="metadata" @click.stop></audio>
                      <small>{{ result.asset.display_name }}</small>
                    </span>
                    <span v-else class="media-fallback">{{ result.asset.media_kind }}</span>
                  </button>
                  <strong class="tile-title">{{ result.asset.display_name }}</strong>
                  <small>{{ result.asset.media_kind }} · {{ result.asset.taken_at ?? "" }}</small>
                  <small class="search-match">{{ result.explanation }}</small>
                  <a class="btn btn-sm btn-outline-secondary" :href="assetHref(result.asset.id)" @click="openAssetLink($event, result.asset.id)">Asset detail</a>
                </article>
              </div>
            </article>
            <article class="settings-form">
              <h3><i class="bi bi-signpost-split" aria-hidden="true"></i> GPS/KML Tracks</h3>
              <div v-if="universalSearchTrackResults.length === 0" class="empty-state">No track results yet.</div>
              <table v-else>
                <thead><tr><th>Name</th><th>Format</th><th>Points</th><th>Matched</th><th></th></tr></thead>
                <tbody>
                  <tr v-for="result in universalSearchTrackResults" :key="result.track.track_asset_id">
                    <td><button type="button" class="link-button" @click="openTrack(result.track.track_asset_id)">{{ result.track.name }}</button></td>
                    <td>{{ result.track.source_format }}</td>
                    <td>{{ result.track.point_count }}</td>
                    <td>{{ result.explanation }}</td>
                    <td><button type="button" class="btn btn-sm btn-outline-secondary" @click="openTrack(result.track.track_asset_id)">Open</button></td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Knowledge Base'" class="panel knowledge-page">
          <header class="panel-head">
            <div>
              <h2>Knowledge Base</h2>
              <span>Human-readable facts mined from local metadata, OCR, transcripts, captions, documents, audio features, geotags, and tracks.</span>
            </div>
            <button type="button" class="btn btn-outline-primary" :disabled="knowledgeLoading" @click="extractKnowledgeBatch">
              Extract facts
            </button>
          </header>
          <section class="settings-form">
            <h3><i class="bi bi-magic" aria-hidden="true"></i> Ask The Local Knowledge Base</h3>
            <p class="muted">The current runner uses deterministic local tools. It plans the request, searches facts and graph relations, and records the conversation. No remote LLM API is used by default.</p>
            <div class="inline-form-row">
              <input v-model="knowledgeChatInput" type="search" placeholder="What was recorded near Lake Ladoga? / Что снято рядом с поездом?" @keyup.enter="askKnowledgeBase" />
              <button type="button" class="btn btn-primary" :disabled="knowledgeChatBusy" @click="askKnowledgeBase">Ask</button>
            </div>
            <p v-if="knowledgeChat?.note" class="muted">{{ knowledgeChat.note }}</p>
            <article v-if="knowledgeChat" class="knowledge-answer">
              <h4>Answer</h4>
              <pre>{{ knowledgeChat.answer }}</pre>
              <details>
                <summary>Tool calls and parsed plan</summary>
                <div class="chip-row">
                  <span v-for="clause in knowledgeChat.planner.clauses" :key="`${clause.field}-${clause.value}`" class="status-badge">
                    {{ clause.field }} {{ clause.operator }} {{ clause.value }}
                  </span>
                </div>
                <pre class="compact-json">{{ JSON.stringify(knowledgeChat.tool_calls, null, 2) }}</pre>
              </details>
            </article>
          </section>
          <section class="settings-form">
            <h3><i class="bi bi-filter" aria-hidden="true"></i> Browse Facts</h3>
            <div class="inline-form-row">
              <input v-model="knowledgeQ" type="search" placeholder="Filter subject, predicate, object, evidence, asset name..." @keyup.enter="loadKnowledgeBase" />
              <input v-model="knowledgePredicate" type="search" placeholder="predicate, e.g. caption, captured_with, ocr_text" @keyup.enter="loadKnowledgeBase" />
              <button type="button" class="btn btn-outline-primary" :disabled="knowledgeLoading" @click="loadKnowledgeBase">Search</button>
            </div>
            <p v-if="knowledgeMessage" class="alert">{{ knowledgeMessage }}</p>
            <p v-if="knowledgeExtraction" class="muted">
              Last extraction batch: {{ knowledgeExtraction.facts_inserted }} fact upserts · {{ knowledgeExtraction.relations_inserted }} relation upserts.
            </p>
            <div v-if="knowledgeFacts.length === 0" class="empty-state">No facts match this filter yet.</div>
            <div v-else class="knowledge-fact-grid">
              <article v-for="fact in knowledgeFacts" :key="fact.id" class="knowledge-card">
                <div class="knowledge-card-head">
                  <strong>{{ fact.subject }}</strong>
                  <span class="status-badge">{{ fact.predicate }}</span>
                </div>
                <p>{{ fact.object }}</p>
                <small v-if="fact.evidence">{{ fact.evidence }}</small>
                <div class="tile-actions">
                  <span class="muted">{{ fact.source_kind }} · {{ fact.language || 'any language' }}</span>
                  <button v-if="fact.asset_id" type="button" class="btn btn-sm btn-outline-secondary" @click="openKnowledgeAsset(fact.asset_id)">Open asset</button>
                </div>
              </article>
            </div>
          </section>
        </section>

        <section v-else-if="active === 'Knowledge Graph'" class="panel knowledge-page">
          <header class="panel-head">
            <div>
              <h2>Knowledge Graph</h2>
              <span>Relations between assets, devices, folders, tracks, transcripts, document text, tags, and extracted knowledge entities.</span>
            </div>
            <button type="button" class="btn btn-outline-primary" :disabled="knowledgeLoading" @click="loadKnowledgeBase">Refresh</button>
          </header>
          <section class="settings-form">
            <h3><i class="bi bi-search" aria-hidden="true"></i> Relation Search</h3>
            <div class="inline-form-row">
              <input v-model="knowledgeQ" type="search" placeholder="Filter relation endpoints, evidence, asset name..." @keyup.enter="loadKnowledgeBase" />
              <input v-model="knowledgeRelationFilter" type="search" placeholder="relation, e.g. stored_in_folder, linked_to_track" @keyup.enter="loadKnowledgeBase" />
              <button type="button" class="btn btn-outline-primary" :disabled="knowledgeLoading" @click="loadKnowledgeBase">Search graph</button>
            </div>
            <p class="muted">{{ knowledgeRelationsTotal }} relations match. Showing the first {{ knowledgeRelations.length }} for responsiveness.</p>
          </section>
          <section class="knowledge-graph-layout">
            <article class="settings-form">
              <h3>Graph Preview</h3>
              <div class="knowledge-graph-preview">
                <div v-for="relation in knowledgeRelations.slice(0, 24)" :key="`graph-${relation.id}`" class="knowledge-edge">
                  <span>{{ relation.from_entity || relation.from_asset_id || 'entity' }}</span>
                  <strong>{{ relation.relation }}</strong>
                  <span>{{ relation.to_entity || relation.to_asset_id || 'entity' }}</span>
                </div>
              </div>
            </article>
            <article class="settings-form">
              <h3>Relations</h3>
              <div v-if="knowledgeRelations.length === 0" class="empty-state">No graph relations match this filter yet.</div>
              <div v-else class="relation-list">
                <article v-for="relation in knowledgeRelations" :key="relation.id" class="relation-row">
                  <button v-if="relation.from_asset_id" type="button" class="link-button" @click="openKnowledgeAsset(relation.from_asset_id)">
                    {{ relation.from_entity || relation.from_asset_id }}
                  </button>
                  <span v-else>{{ relation.from_entity || 'entity' }}</span>
                  <strong>{{ relation.relation }}</strong>
                  <button v-if="relation.to_asset_id" type="button" class="link-button" @click="openKnowledgeAsset(relation.to_asset_id)">
                    {{ relation.to_entity || relation.to_asset_id }}
                  </button>
                  <span v-else>{{ relation.to_entity || 'entity' }}</span>
                  <small v-if="relation.evidence">{{ relation.evidence }}</small>
                </article>
              </div>
            </article>
          </section>
        </section>

        <section v-else-if="active === 'Face Gallery'" class="panel face-gallery-page">
          <header class="panel-head">
            <div>
              <h2>Face Gallery</h2>
              <span>Local-only face folders from stored detections. Names are user labels, not real-world identity.</span>
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" @click="refreshFaceClusters">
              <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
              Refresh
            </button>
          </header>
          <p v-if="faceGalleryMessage" class="alert">{{ faceGalleryMessage }}</p>
          <p v-if="faceClustersPayload?.provisional_note" class="muted">{{ faceClustersPayload.provisional_note }}</p>
          <div class="search-hero compact-search">
            <label>
              <span>Search faces</span>
              <input v-model="faceSearchQ" type="search" placeholder="name, asset, provisional, unassigned..." />
            </label>
          </div>
          <div class="list-toolbar compact-toolbar">
            <span>{{ visibleFaceClusters.length }} shown<span v-if="filteredFaceClusters.length !== visibleFaceClusters.length"> of {{ filteredFaceClusters.length }} matching</span><span v-if="faceClustersPayload?.total"> · {{ faceClustersPayload.total }} total clusters</span></span>
            <div class="actions">
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="visibleFaceClusters.length >= filteredFaceClusters.length" @click="faceClusterLimit += 200">Load more</button>
              <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="visibleFaceClusters.length >= filteredFaceClusters.length" @click="faceClusterLimit = filteredFaceClusters.length">Load all</button>
            </div>
          </div>
          <div class="face-gallery-layout">
            <aside class="face-cluster-list">
              <div v-if="filteredFaceClusters.length === 0" class="empty-state">No face detections match this search.</div>
              <button
                v-for="cluster in visibleFaceClusters"
                :key="cluster.id"
                type="button"
                :class="['face-cluster-card', { active: selectedFaceCluster?.id === cluster.id }]"
                @click="openFaceCluster(cluster)"
              >
                <span class="face-cluster-preview">
                  <img v-if="faceClusterPreviewURL(cluster)" :src="faceClusterPreviewURL(cluster)" :alt="faceClusterPreviewTitle(cluster)" loading="lazy" />
                  <i v-else class="bi bi-person-bounding-box" aria-hidden="true"></i>
                </span>
                <strong>{{ cluster.label || 'Unassigned faces' }}</strong>
                <small>{{ cluster.face_count }} faces · {{ cluster.asset_count }} photos · {{ cluster.ignored_count }} ignored</small>
                <span v-if="(cluster.metadata?.provisional as boolean | undefined)" class="status-badge warn">provisional</span>
              </button>
            </aside>
            <section class="face-cluster-detail">
              <div v-if="!selectedFaceCluster" class="empty-state">
                Select a face folder to inspect its photos and detections.
              </div>
              <template v-else>
                <div class="section-title">
                  <div>
                    <h3>{{ selectedFaceCluster.label || 'Unassigned faces' }}</h3>
                    <p class="muted">{{ faceClusterDetections.length }} detections across {{ faceClusterAssets.length }} assets.</p>
                  </div>
                  <div class="input-group cluster-name-editor">
                    <input v-model="faceClusterNameDraft" class="form-control" type="text" placeholder="Local folder name" />
                    <button type="button" class="btn btn-primary" @click="saveFaceClusterName">
                      <i class="bi bi-check2" aria-hidden="true"></i>
                      Save name
                    </button>
                  </div>
                </div>
                <div class="tile-grid face-asset-grid">
                  <article v-for="asset in faceClusterAssets" :key="asset.id" class="asset-tile face-asset-tile">
                    <button type="button" class="tile-media" @click="openAsset(asset.id)">
                      <img v-if="asset.media_kind === 'photo'" :src="`/api/v1/media/${asset.id}/preview`" :alt="asset.display_name" loading="lazy" />
                      <span v-else-if="asset.media_kind === 'audio'" class="audio-tile-preview">
                        <i class="bi bi-soundwave" aria-hidden="true"></i>
                        <audio :src="`/api/v1/media/${asset.id}/original`" controls preload="metadata" @click.stop></audio>
                        <small>{{ asset.display_name }}</small>
                      </span>
                      <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                    </button>
                    <div class="tile-meta">
                      <strong class="tile-title">{{ asset.display_name }}</strong>
                      <span>{{ faceClusterDetections.filter((face) => face.asset_id === asset.id && !faceIgnored(face)).length }} faces in this folder</span>
                    </div>
                    <a class="btn btn-sm btn-outline-secondary" :href="assetHref(asset.id)" @click="openAssetLink($event, asset.id)">Asset detail</a>
                  </article>
                </div>
                <details class="mt-3 face-detection-details">
                  <summary>Detections in this folder</summary>
                  <table>
                    <thead><tr><th>Preview</th><th>Asset</th><th>Confidence</th><th>Box</th><th>Status</th><th>Action</th></tr></thead>
                    <tbody>
                      <tr v-for="face in faceClusterDetections" :key="face.id">
                        <td><img class="face-table-thumb" :src="faceCardPreviewURL(face)" alt="" loading="lazy" /></td>
                        <td><button type="button" class="link-button" @click="openAsset(face.asset_id)">{{ face.asset_id.slice(0, 8) }}</button></td>
                        <td>{{ typeof face.confidence === 'number' ? face.confidence.toFixed(3) : '' }}</td>
                        <td>{{ Math.round(face.x) }}, {{ Math.round(face.y) }} · {{ Math.round(face.width) }} × {{ Math.round(face.height) }}</td>
                        <td>{{ faceIgnored(face) ? 'ignored' : (faceMetadata(face).review_status ?? 'active') }}</td>
                        <td>
                          <button type="button" class="btn btn-sm btn-outline-danger" :disabled="faceIgnored(face)" @click="deleteFaceDetection(face)">
                            <i class="bi bi-trash" aria-hidden="true"></i>
                            Delete
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </details>
              </template>
            </section>
          </div>
        </section>

        <section v-else-if="active === 'Geo Align'" class="panel geo-align-page">
          <header class="panel-head">
            <div>
              <h2>Photo/GPS Alignment</h2>
              <span>Manual geotag clarification writes to Cartolensia metadata only in this strict read-only real-peek session.</span>
            </div>
            <button type="button" class="btn btn-primary" @click="startGeoAlignSession">
              <i class="bi bi-crosshair" aria-hidden="true"></i>
              Start alignment session
            </button>
          </header>
          <p v-if="geoAlignMessage" class="alert">{{ geoAlignMessage }}</p>
          <div class="geo-align-layout">
            <aside class="geo-align-sidebar settings-form">
              <h3>Scope</h3>
              <p>{{ selectedAssets.size > 0 ? `${selectedAssets.size} selected media assets` : 'Current indexed photo/video scope, bounded to 54 assets' }}</p>
              <label class="form-label">
                Track IDs
                <textarea v-model="geoAlignTrackIds" class="form-control" rows="3" placeholder="Blank uses current parsed GPS/KML tracks"></textarea>
              </label>
              <p class="muted">Green = own geotag. Orange = track candidate. Red = no geotag. Moved markers get a glow.</p>
              <div v-if="geoAlignSession" class="metrics compact-metrics">
                <article><strong>{{ geoAlignSession.markers.length }}</strong><span>Media</span></article>
                <article><strong>{{ geoAlignSession.track_ids.length }}</strong><span>Tracks</span></article>
                <article><strong>{{ geoAlignSession.markers.filter((marker) => marker.modified).length }}</strong><span>Modified</span></article>
              </div>
              <label class="form-check form-switch">
                <input v-model="geoAlignTilesVisible" class="form-check-input" type="checkbox" />
                <span>OSM tiles</span>
              </label>
              <label class="form-check form-switch">
                <input v-model="geoAlignTrackLayerVisible" class="form-check-input" type="checkbox" />
                <span>Track layer</span>
              </label>
              <label class="form-check form-switch">
                <input v-model="geoAlignMarkerLayerVisible" class="form-check-input" type="checkbox" />
                <span>Marker layer</span>
              </label>
              <div class="actions">
                <button type="button" class="btn btn-outline-secondary" :disabled="!geoAlignSession" @click="resetGeoAlign">Reset</button>
                <button type="button" class="btn btn-success" :disabled="!geoAlignSession" @click="applyGeoAlign">Apply DB overrides</button>
                <button type="button" class="btn btn-outline-danger" disabled title="Disabled because rclone_peek is strict_read_only">Write EXIF disabled</button>
              </div>
              <div v-if="geoAlignSession" class="modified-list">
                <h4>Modified media</h4>
                <p v-if="geoAlignSession.markers.filter((marker) => marker.modified).length === 0" class="muted">No manual changes yet.</p>
                <button
                  v-for="marker in geoAlignSession.markers.filter((item) => item.modified)"
                  :key="marker.asset_id"
                  type="button"
                  class="link-button"
                  @click="showGeoAlignMarkerPopup(marker.asset_id, true)"
                >
                  {{ marker.name }}
                </button>
              </div>
            </aside>
            <article class="settings-form geo-align-main">
              <h3>Alignment Map Preview</h3>
              <div v-if="!geoAlignSession" class="empty-state">Start a session to preview markers.</div>
              <div v-else class="map-shell alignment-map-shell">
                <div ref="geoAlignMapElement" class="alignment-ol-map"></div>
                <div v-if="geoAlignDragActive" class="geo-align-drag-hint">
                  <i class="bi bi-arrows-move" aria-hidden="true"></i>
                  Release to stage this marker.
                </div>
                <div class="map-layer-control">
                  <button type="button" class="icon-button" @click="showGeoAlignLayerMenu = !showGeoAlignLayerMenu">
                    <i class="bi bi-layers" aria-hidden="true"></i>
                    Layers
                  </button>
                  <div v-if="showGeoAlignLayerMenu" class="layer-menu">
                    <label class="form-check form-switch">
                      <input v-model="geoAlignTilesVisible" class="form-check-input" type="checkbox" />
                      <span>OSM tiles</span>
                    </label>
                    <label class="form-check form-switch">
                      <input v-model="geoAlignTrackLayerVisible" class="form-check-input" type="checkbox" />
                      <span>Tracks</span>
                    </label>
                    <label class="form-check form-switch">
                      <input v-model="geoAlignMarkerLayerVisible" class="form-check-input" type="checkbox" />
                      <span>Markers</span>
                    </label>
                    <button type="button" class="btn btn-sm btn-outline-primary" @click="refreshGeoAlignMap">
                      <i class="bi bi-aspect-ratio" aria-hidden="true"></i>
                      Fit
                    </button>
                    <small>Tiles load on demand through Cartolensia; no bulk prefetch.</small>
                  </div>
                </div>
                <aside v-if="geoAlignPopupMarker" class="geo-align-popup">
                  <header>
                    <strong>{{ geoAlignPopupMarker.name }}</strong>
                    <button type="button" class="btn btn-sm btn-outline-secondary danger-close" @click="closeGeoAlignPopup">
                      <i class="bi bi-x-lg" aria-hidden="true"></i>
                      Close
                    </button>
                  </header>
                  <div class="geo-align-popup-body">
                    <img :src="geoAlignPopupMarker.thumbnail_url || `/api/v1/media/${geoAlignPopupMarker.asset_id}/preview`" alt="" loading="lazy" />
                    <div>
                      <span :class="['status-badge', geoAlignPopupMarker.modified ? 'warn' : geoAlignPopupMarker.status === 'ungeotagged' ? 'bad' : 'ok']">
                        {{ geoAlignPopupMarker.modified ? 'manual override' : geoAlignPopupMarker.status }}
                      </span>
                      <p>{{ geoAlignPopupMarker.media_kind }} · {{ geoAlignCoordinateText(geoAlignPopupMarker.staged_lat, geoAlignPopupMarker.staged_lon) }}</p>
                      <p>Original: {{ geoAlignCoordinateText(geoAlignPopupMarker.original_lat, geoAlignPopupMarker.original_lon) }}</p>
                      <p v-if="geoAlignPopupMarker.modified">Manual: {{ geoAlignCoordinateText(geoAlignPopupMarker.manual_lat, geoAlignPopupMarker.manual_lon) }}</p>
                    </div>
                  </div>
                  <details v-if="geoAlignPopupMarker.track_candidates.length" open>
                    <summary>Track candidates</summary>
                    <button
                      v-for="(candidate, index) in geoAlignPopupMarker.track_candidates"
                      :key="index"
                      type="button"
                      class="link-button"
                      @click="showGeoAlignCandidate(candidate)"
                    >
                      {{ geoAlignCandidateLabel(candidate) }}
                    </button>
                  </details>
                  <div class="actions">
                    <button type="button" class="btn btn-sm btn-outline-primary" @click="openAsset(geoAlignPopupMarker.asset_id)">Open asset detail</button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="centerGeoAlignOn(geoAlignPopupMarker.original_lat, geoAlignPopupMarker.original_lon)">Show original</button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="resetGeoAlignMarker(geoAlignPopupMarker)">Reset marker</button>
                    <button type="button" class="btn btn-sm btn-success" :disabled="!geoAlignPopupMarker.modified" @click="applyGeoAlignMarker(geoAlignPopupMarker)">Apply marker</button>
                  </div>
                </aside>
                <aside v-else-if="geoAlignPopupTrackInfo || geoAlignPopupTrackMessage" class="geo-align-popup">
                  <header>
                    <strong>{{ geoAlignPopupTrackInfo?.track.name || 'Track point' }}</strong>
                    <button type="button" class="btn btn-sm btn-outline-secondary danger-close" @click="closeGeoAlignPopup">
                      <i class="bi bi-x-lg" aria-hidden="true"></i>
                      Close
                    </button>
                  </header>
                  <p v-if="geoAlignPopupTrackMessage" class="muted">{{ geoAlignPopupTrackMessage }}</p>
                  <template v-if="geoAlignPopupTrackInfo">
                    <span>Clicked: {{ geoAlignPopupTrackInfo.clicked.lat.toFixed(6) }}, {{ geoAlignPopupTrackInfo.clicked.lon.toFixed(6) }}</span>
                    <span>Nearest: {{ geoAlignPopupTrackInfo.nearest.lat.toFixed(6) }}, {{ geoAlignPopupTrackInfo.nearest.lon.toFixed(6) }}</span>
                    <span>Distance: {{ geoAlignPopupTrackInfo.distance_m.toFixed(1) }} m</span>
                    <span v-if="geoAlignPopupTrackInfo.timestamp">Time: {{ geoAlignPopupTrackInfo.timestamp }}</span>
                    <span v-if="geoAlignPopupTrackInfo.relative_time_seconds !== undefined">Time in track: {{ geoAlignPopupTrackInfo.relative_time_seconds.toFixed(1) }} s</span>
                    <span v-if="geoAlignPopupTrackInfo.speed_mps !== undefined">Speed: {{ geoAlignPopupTrackInfo.speed_mps.toFixed(2) }} m/s</span>
                    <span v-if="geoAlignPopupTrackInfo.elevation_m !== undefined">Altitude: {{ geoAlignPopupTrackInfo.elevation_m.toFixed(1) }} m</span>
                    <div class="actions">
                      <button type="button" class="btn btn-sm btn-outline-primary" @click="openTrack(geoAlignPopupTrackInfo.track.track_asset_id)">Open track detail</button>
                      <button type="button" class="btn btn-sm btn-outline-secondary" @click="openTrackAndFindAssets(geoAlignPopupTrackInfo.track.track_asset_id)">List media during track</button>
                      <button type="button" class="btn btn-sm btn-outline-secondary" @click="openTrackAndFindNearbyAssets(geoAlignPopupTrackInfo.track.track_asset_id)">List nearby media</button>
                    </div>
                  </template>
                </aside>
              </div>
            </article>
          </div>
          <article v-if="geoAlignSession" class="settings-form">
            <h3>Markers</h3>
            <table>
              <thead><tr><th>Asset</th><th>Status</th><th>Staged coordinate</th><th>Track candidates</th><th>Move</th></tr></thead>
              <tbody>
                <tr v-for="marker in geoAlignSession.markers" :key="marker.asset_id">
                  <td><button type="button" class="link-button" @click="openAsset(marker.asset_id)">{{ marker.name }}</button></td>
                  <td><span :class="['status-badge', marker.modified ? 'warn' : marker.status === 'ungeotagged' ? 'bad' : 'ok']">{{ marker.modified ? 'modified' : marker.status }}</span></td>
                  <td>{{ marker.staged_lat.toFixed(6) }}, {{ marker.staged_lon.toFixed(6) }}</td>
                  <td>{{ marker.track_candidates.length }}</td>
                  <td class="actions">
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="nudgeGeoAlignMarker(marker.asset_id, 0.0001, 0)">↑</button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="nudgeGeoAlignMarker(marker.asset_id, -0.0001, 0)">↓</button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="nudgeGeoAlignMarker(marker.asset_id, 0, -0.0001)">←</button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" @click="nudgeGeoAlignMarker(marker.asset_id, 0, 0.0001)">→</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </article>
        </section>

        <section v-else-if="active === 'Video Track Player'" class="panel video-track-page">
          <header class="panel-head">
            <div>
              <h2>Video + GPS/KML Track Player</h2>
              <span>Synchronize a selected video with one or more parsed tracks using inferred timestamps, OpenLayers, and a moving marker.</span>
            </div>
            <button type="button" class="btn btn-primary" @click="startVideoTrackPlayerSession">
              <i class="bi bi-play-btn" aria-hidden="true"></i>
              Open player
            </button>
          </header>
          <p v-if="videoTrackMessage" class="alert">{{ videoTrackMessage }}</p>
          <div class="settings-grid">
            <article class="settings-form">
              <h3>Inputs</h3>
              <label class="form-label">
                Search video
                <input
                  v-model="videoTrackVideoSearch"
                  class="form-control"
                  type="search"
                  placeholder="Type part of a video filename, e.g. 072546"
                  @input="loadVideoTrackVideoOptions"
                  @focus="loadVideoTrackVideoOptions"
                />
              </label>
              <div class="selector-suggestions">
                <button
                  v-for="video in videoTrackVideoOptions.slice(0, 20)"
                  :key="video.id"
                  type="button"
                  :class="{ active: video.id === videoTrackAssetId }"
                  @click="selectVideoTrackVideo(video)"
                >
                  <span>{{ assetName(video) }}</span>
                  <small>{{ videoOptionSummary(video) }}</small>
                </button>
              </div>
              <p v-if="selectedVideoTrackAsset" class="muted">
                Selected video: {{ assetName(selectedVideoTrackAsset) }} · {{ videoOptionSummary(selectedVideoTrackAsset) }}
              </p>
              <label class="form-label">
                Search tracks
                <input
                  v-model="videoTrackTrackSearch"
                  class="form-control"
                  type="search"
                  placeholder="Type a GPX/KML name, e.g. 20260512-072610"
                />
              </label>
              <div class="selector-pill-list" aria-label="Selected tracks">
                <span v-for="track in videoTrackSelectedTracks" :key="track.track_asset_id" class="selector-pill">
                  {{ track.name }}
                  <button type="button" aria-label="Remove track" @click="removeVideoTrackTrack(track.track_asset_id)">×</button>
                </span>
              </div>
              <div class="selector-suggestions">
                <button
                  v-for="track in filteredVideoTrackTracks"
                  :key="track.track_asset_id"
                  type="button"
                  @click="addVideoTrackTrack(track)"
                >
                  <span>{{ track.name }}</span>
                  <small>{{ trackOptionSummary(track) }}</small>
                </button>
              </div>
              <label class="form-label">
                Timestamp mode
                <select v-model="videoTrackTimestampMode" class="form-select">
                  <option value="video_start_time">Video timestamp is recording start</option>
                  <option value="video_end_time">Video timestamp is recording end</option>
                </select>
              </label>
              <label class="form-label">
                Offset seconds
                <input v-model.number="videoTrackOffsetSeconds" class="form-control" type="number" step="0.1" />
              </label>
              <p class="muted">
                Sync mode: <strong>{{ runtimeTextSetting("video_track_player.sync_mode", "interval") }}</strong> ·
                Interval: <strong>{{ runtimeTextSetting("video_track_player.interval_seconds", "3") }} s</strong> ·
                Throttle: <strong>{{ runtimeTextSetting("video_track_player.marker_throttle_ms", "250") }} ms</strong> ·
                Auto-select overlapping tracks: <strong>{{ boolRuntimeSetting("video_track_player.auto_select_overlapping_tracks", true) ? "on" : "off" }}</strong>
              </p>
            </article>
            <article class="settings-form settings-wide">
              <h3>Player</h3>
              <div v-if="!videoTrackSession" class="empty-state">Create a session to open the synchronized player.</div>
              <div v-else class="video-track-split">
                <video
                  class="video-track-video"
                  :src="`/api/v1/media/${videoTrackSession.video_asset_id}/original`"
                  controls
                  preload="metadata"
                  @loadedmetadata="handleVideoTrackPlaybackEvent"
                  @play="handleVideoTrackPlaybackEvent"
                  @pause="handleVideoTrackPlaybackEvent"
                  @seeked="handleVideoTrackPlaybackEvent"
                  @ended="handleVideoTrackPlaybackEvent"
                  @timeupdate="handleVideoTrackPlaybackEvent"
                ></video>
                <div class="video-track-map-shell">
                  <div ref="videoTrackMapElement" class="video-track-map" role="img" aria-label="OpenLayers synchronized video track map"></div>
                  <div v-if="videoTrackCurrentPosition" class="video-track-hud">
                    <strong>Track position</strong>
                    <span>Coordinates: {{ videoTrackPointSummary(videoTrackCurrentPosition) }}</span>
                    <span v-if="videoTrackPositionText(videoTrackCurrentPosition, 'time')">Time: {{ videoTrackPositionText(videoTrackCurrentPosition, 'time') }}</span>
                    <span v-if="videoTrackPayloadText('target_time')">Target: {{ videoTrackPayloadText('target_time') }}</span>
                    <span v-if="videoTrackPayloadText('time_source')">Source: {{ videoTrackPayloadText('time_source') }}</span>
                    <span v-if="videoTrackPositionText(videoTrackCurrentPosition, 'mode')">Mode: {{ videoTrackPositionText(videoTrackCurrentPosition, 'mode') }}</span>
                    <span v-if="videoTrackPositionNumber(videoTrackCurrentPosition, 'speed_mps') !== undefined">Speed: {{ videoTrackPositionNumber(videoTrackCurrentPosition, 'speed_mps')?.toFixed(2) }} m/s</span>
                    <span v-if="videoTrackPositionNumber(videoTrackCurrentPosition, 'elevation_m') !== undefined">Altitude: {{ videoTrackPositionNumber(videoTrackCurrentPosition, 'elevation_m')?.toFixed(1) }} m</span>
                  </div>
                  <div class="map-status-overlay">
                    <i class="bi bi-signpost-split" aria-hidden="true"></i>
                    {{ videoTrackPosition?.warning || videoTrackSession.warnings?.[0] || videoTrackMessage || "Open the player to synchronize the marker." }}
                  </div>
                  <div class="map-layer-control">
                    <small class="muted">OSM tiles are on-demand through the local proxy.</small>
                  </div>
                  <pre v-if="boolRuntimeSetting('video_track_player.show_debug_overlay', false)" class="compact-json">{{ JSON.stringify(videoTrackPosition ?? videoTrackSession, null, 2) }}</pre>
                </div>
              </div>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Settings'" class="panel settings-page">
          <header class="panel-head">
            <h2>Settings</h2>
            <span>Runtime settings apply immediately; YAML-bound settings require restart.</span>
          </header>
          <p v-if="settingsMessage" class="alert">{{ settingsMessage }}</p>
          <div class="settings-tabs">
            <button
              v-for="tab in settings?.tabs ?? []"
              :key="tab.id"
              type="button"
              :class="{ active: settingsTab === tab.id }"
              @click="settingsTab = tab.id"
            >
              {{ tab.label }} <span v-if="!tab.runtime" class="status-badge warn">restart</span>
            </button>
          </div>

          <div v-if="settingsTab === 'general'" class="settings-grid">
            <article class="settings-form">
              <h3>Effective Runtime</h3>
              <p>Store backend: {{ backend?.store_backend }}</p>
              <p>Auth mode: {{ backend?.auth_mode }}</p>
              <p>Cache: {{ backend?.preview_cache }}</p>
              <p>HTTP: {{ backend?.http?.addr }} · TLS {{ backend?.http?.tls_enabled ? "enabled" : "disabled" }}</p>
            </article>
            <article class="settings-form">
              <h3>Restart Required</h3>
              <p>{{ settings?.restart_required.note }}</p>
              <code>{{ (settings?.yaml_bound_fields ?? []).join(", ") }}</code>
            </article>
          </div>

          <div v-else-if="['indexing', 'metadata', 'preview', 'map', 'gps', 'search', 'transcoding'].includes(settingsTab)" class="settings-grid">
            <article v-if="runtimeSpecsForTab(settingsTab).length > 0" class="settings-form settings-wide">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-lightning-charge" aria-hidden="true"></i> Runtime Settings</h3>
                  <p class="muted">Runtime values apply immediately where supported. Existing long-running jobs keep their current payloads.</p>
                </div>
                <span class="status-badge ok">applies now</span>
              </div>
              <div class="form-stack">
                <label v-for="spec in runtimeSpecsForTab(settingsTab)" :key="spec.key">
                  <span>{{ spec.label }}</span>
                  <select
                    v-if="spec.kind === 'boolean'"
                    :value="String(settings?.runtime_settings?.[spec.key] ?? false)"
                    @change="setRuntimeSetting(spec.key, ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                  <input
                    v-else
                    :type="spec.kind === 'number' ? 'number' : 'text'"
                    :value="String(settings?.runtime_settings?.[spec.key] ?? '')"
                    @input="setRuntimeSetting(spec.key, ($event.target as HTMLInputElement).value)"
                  />
                  <small>{{ spec.help }}</small>
                </label>
              </div>
              <div class="settings-actions">
                <button type="button" class="btn btn-primary" @click="saveRuntimeSettings">
                  <i class="bi bi-check2-circle" aria-hidden="true"></i>
                  Save runtime settings
                </button>
              </div>
            </article>
            <article v-if="pendingSpecsForTab(settingsTab).length > 0" class="settings-form settings-wide">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-arrow-repeat" aria-hidden="true"></i> Restart-Required Settings</h3>
                  <p class="muted">These are saved as pending YAML and take effect after restart.</p>
                </div>
                <span class="status-badge warn">restart required</span>
              </div>
              <div class="form-stack">
                <label v-for="spec in pendingSpecsForTab(settingsTab)" :key="spec.key">
                  <span>{{ spec.label }}</span>
                  <select
                    v-if="spec.kind === 'boolean'"
                    :value="pendingValue(spec.key, 'false')"
                    @change="setPendingValue(spec.key, ($event.target as HTMLSelectElement).value === 'true')"
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                  <input
                    v-else
                    :type="spec.kind === 'number' ? 'number' : 'text'"
                    :value="pendingValue(spec.key)"
                    @input="setPendingValue(spec.key, spec.kind === 'number' ? Number(($event.target as HTMLInputElement).value) : ($event.target as HTMLInputElement).value)"
                  />
                  <button
                    v-if="spec.kind !== 'number' && (spec.key.includes('path') || spec.key.includes('dir'))"
                    type="button"
                    class="btn btn-outline-secondary btn-sm align-self-start"
                    @click="openFilePicker(`pending:${spec.key}`, spec.key.includes('ffmpeg') || spec.key.includes('ffprobe') ? 'file' : 'folder')"
                  >
                    <i class="bi bi-folder2-open" aria-hidden="true"></i>
                    Browse
                  </button>
                  <small>{{ spec.help }}</small>
                </label>
              </div>
              <div class="settings-actions">
                <button type="button" class="btn btn-outline-primary" @click="savePendingSettings">
                  <i class="bi bi-save" aria-hidden="true"></i>
                  Save pending YAML
                </button>
                <a class="btn btn-outline-secondary" href="/api/v1/settings/pending/download" target="_blank" rel="noreferrer">
                  <i class="bi bi-download" aria-hidden="true"></i>
                  Download pending YAML
                </a>
              </div>
            </article>
            <article v-if="settingsTab === 'gps'" class="settings-form">
              <h3><i class="bi bi-signpost-split" aria-hidden="true"></i> Current GPS/KML State</h3>
              <p>{{ tracks.length }} parsed tracks · {{ stats?.tracks ?? 0 }} track-like assets</p>
              <label class="form-check form-switch">
                <input v-model="trackPreviewTilesEnabled" class="form-check-input" type="checkbox" />
                <span>Use OSM background in interactive track previews</span>
              </label>
              <p class="muted">Track thumbnails are generated into the Cartolensia cache. Originals stay immutable.</p>
            </article>
            <article v-if="settingsTab === 'map'" class="settings-form">
              <h3><i class="bi bi-map" aria-hidden="true"></i> Tile And Cluster Status</h3>
              <p>{{ mapStatus?.base_tiles ?? 'vector-only' }} · {{ mapStatus?.clustering ?? 'none' }}</p>
              <p>{{ tileSources[0]?.policy ?? 'No tile source configured.' }}</p>
              <label class="form-check form-switch">
                <input v-model="showMapDebug" class="form-check-input" type="checkbox" />
                <span>Show raw GeoJSON debug by default in this browser</span>
              </label>
              <div class="chip-row">
                <span class="chip">Cluster radius {{ settings?.runtime_settings?.['map.cluster_radius_px'] ?? 64 }} px</span>
                <span class="chip">On-demand cache only</span>
              </div>
            </article>
            <article v-if="settingsTab === 'search'" class="settings-form settings-wide">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-search-heart" aria-hidden="true"></i> Local Place Search</h3>
                  <p class="muted">{{ searchPlaceCache?.note ?? 'Place lookup is cache-only unless an operator explicitly enables an online provider.' }}</p>
                </div>
                <span :class="['status-badge', searchPlaceCache?.online_enabled ? 'warn' : 'ok']">
                  {{ searchPlaceCache?.mode ?? 'cache_only' }}
                </span>
              </div>
              <div class="detail-grid compact-detail">
                <article><strong>{{ searchPlaceCache?.backend ?? 'postgres_local' }}</strong><span>Search backend</span></article>
                <article><strong>{{ searchPlaceCache?.provider ?? 'local_place_cache' }}</strong><span>Geocoder provider</span></article>
                <article><strong>{{ searchPlaceCache?.places?.length ?? 0 }}</strong><span>Cached places</span></article>
              </div>
              <div class="settings-subsection">
                <div class="section-title compact-title">
                  <div>
                    <h4><i class="bi bi-database-gear" aria-hidden="true"></i> Operator Place Cache</h4>
                    <p class="muted">Editable local entries power offline place search and asset-detail reverse geocoding. No public geocoder is called here.</p>
                  </div>
                  <button type="button" class="btn btn-outline-primary btn-sm" @click="refreshPlaceCache">
                    <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
                    Refresh
                  </button>
                </div>
                <div class="row g-2 align-items-end">
                  <label class="col-md-5">
                    <span>Filter cached places</span>
                    <input v-model="placeCacheQuery" class="form-control" placeholder="Yerevan, Lori, road name" @keyup.enter="refreshPlaceCache" />
                  </label>
                  <div class="col-md-auto">
                    <button type="button" class="btn btn-outline-secondary" @click="refreshPlaceCache">
                      <i class="bi bi-search" aria-hidden="true"></i>
                      Filter
                    </button>
                  </div>
                  <p v-if="placeCacheMessage" class="col-12 muted mb-0">{{ placeCacheMessage }}</p>
                </div>
                <div class="place-editor-grid">
                  <article class="place-editor-card">
                    <h5>Add Place</h5>
                    <div class="row g-2">
                      <label class="col-md-4"><span>Name</span><input v-model="placeDraft.name" class="form-control" placeholder="Place name" /></label>
                      <label class="col-md-4"><span>Display name</span><input v-model="placeDraft.display_name" class="form-control" placeholder="Display name" /></label>
                      <label class="col-md-4"><span>Aliases</span><input v-model="placeDraftAliases" class="form-control" placeholder="comma separated" /></label>
                      <label class="col-md-3"><span>Country</span><input v-model="placeDraft.country" class="form-control" /></label>
                      <label class="col-md-3"><span>Region</span><input v-model="placeDraft.region" class="form-control" /></label>
                      <label class="col-md-3"><span>City</span><input v-model="placeDraft.city" class="form-control" /></label>
                      <label class="col-md-3"><span>Road</span><input v-model="placeDraft.road" class="form-control" /></label>
                      <label class="col-md-2"><span>Lat</span><input v-model.number="placeDraft.lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Lon</span><input v-model.number="placeDraft.lon" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Min lat</span><input v-model.number="placeDraft.bbox.min_lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Max lat</span><input v-model.number="placeDraft.bbox.max_lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Min lon</span><input v-model.number="placeDraft.bbox.min_lon" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Max lon</span><input v-model.number="placeDraft.bbox.max_lon" type="number" step="0.000001" class="form-control" /></label>
                    </div>
                    <button type="button" class="btn btn-primary mt-2" @click="createPlaceFromDraft">
                      <i class="bi bi-plus-lg" aria-hidden="true"></i>
                      Add cached place
                    </button>
                  </article>
                  <article v-for="place in editablePlaces" :key="place.id || `${place.provider}-${place.name}`" class="place-editor-card">
                    <div class="d-flex justify-content-between gap-2 align-items-start">
                      <div>
                        <h5>{{ place.display_name || place.name }}</h5>
                        <p class="muted mb-0">{{ place.provider || 'local' }} · {{ place.source || 'operator_cache' }}</p>
                      </div>
                      <div class="btn-group btn-group-sm">
                        <button type="button" class="btn btn-outline-primary" @click="savePlace(place)">
                          <i class="bi bi-save" aria-hidden="true"></i>
                          Save
                        </button>
                        <button type="button" class="btn btn-outline-danger" @click="deletePlace(place)">
                          <i class="bi bi-trash" aria-hidden="true"></i>
                          Delete
                        </button>
                      </div>
                    </div>
                    <div class="row g-2 mt-1">
                      <label class="col-md-4"><span>Name</span><input v-model="place.name" class="form-control" /></label>
                      <label class="col-md-4"><span>Display name</span><input v-model="place.display_name" class="form-control" /></label>
                      <label class="col-md-4"><span>Aliases</span><input :value="(place.aliases || []).join(', ')" class="form-control" @input="place.aliases = ($event.target as HTMLInputElement).value.split(',').map((alias) => alias.trim()).filter(Boolean)" /></label>
                      <label class="col-md-3"><span>Country</span><input v-model="place.country" class="form-control" /></label>
                      <label class="col-md-3"><span>Region</span><input v-model="place.region" class="form-control" /></label>
                      <label class="col-md-3"><span>City</span><input v-model="place.city" class="form-control" /></label>
                      <label class="col-md-3"><span>Road</span><input v-model="place.road" class="form-control" /></label>
                      <label class="col-md-2"><span>Lat</span><input v-model.number="place.lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Lon</span><input v-model.number="place.lon" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Min lat</span><input v-model.number="place.bbox.min_lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Max lat</span><input v-model.number="place.bbox.max_lat" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Min lon</span><input v-model.number="place.bbox.min_lon" type="number" step="0.000001" class="form-control" /></label>
                      <label class="col-md-2"><span>Max lon</span><input v-model.number="place.bbox.max_lon" type="number" step="0.000001" class="form-control" /></label>
                    </div>
                    <button type="button" class="btn btn-sm btn-outline-secondary mt-2" @click="universalSearchQ = place.name; setActive('Search'); runUniversalSearch()">
                      <i class="bi bi-search" aria-hidden="true"></i>
                      Search this place
                    </button>
                  </article>
                </div>
              </div>
              <div v-if="searchPlaceCache?.places?.length" class="search-place-row">
                <article v-for="place in searchPlaceCache.places" :key="`${place.provider}-${place.name}`" class="search-place-card compact-place-card">
                  <h4><i class="bi bi-geo-alt" aria-hidden="true"></i> {{ place.display_name || place.name }}</h4>
                  <p>{{ place.matched_assets }} current geotagged media assets match this bbox.</p>
                  <small>
                    {{ place.lat.toFixed(4) }}, {{ place.lon.toFixed(4) }} ·
                    {{ place.bbox.min_lat.toFixed(3) }}..{{ place.bbox.max_lat.toFixed(3) }},
                    {{ place.bbox.min_lon.toFixed(3) }}..{{ place.bbox.max_lon.toFixed(3) }}
                  </small>
                  <button type="button" class="btn btn-sm btn-outline-primary" @click="universalSearchQ = place.name; setActive('Search'); runUniversalSearch()">
                    Search this place
                  </button>
                </article>
              </div>
              <p class="muted">No public geocoder is called automatically. Future Nominatim-compatible lookups must be user-triggered and cached before being reused offline.</p>
            </article>
            <article v-if="settingsTab === 'preview'" class="settings-form">
              <h3><i class="bi bi-images" aria-hidden="true"></i> Preview Cache</h3>
              <p>{{ previewCacheStats?.entries ?? 0 }} entries · {{ formatBytes(previewCacheStats?.bytes ?? 0) }}</p>
              <p class="muted">Default workflow is on-demand viewing. Persistent preview generation remains opt-in to avoid write amplification.</p>
              <div class="settings-actions">
                <button type="button" class="btn btn-outline-primary" @click="cleanupPreviews(true)">Preview cleanup report</button>
                <button type="button" class="btn btn-outline-danger" @click="confirmCleanupPreviews">Clear cache</button>
              </div>
            </article>
            <article v-if="settingsTab === 'transcoding'" class="settings-form">
              <h3><i class="bi bi-film" aria-hidden="true"></i> Detected Encoders</h3>
              <p>ffmpeg {{ transcodingCapabilities?.ffmpeg.available ? 'available' : 'missing' }}</p>
              <p>{{ transcodingCapabilities?.encoders.length ?? 0 }} encoders detected.</p>
              <p class="muted">HLS sessions are written only under Cartolensia cache directories and never beside originals.</p>
            </article>
          </div>

          <div v-else-if="settingsTab === 'server'" class="settings-grid">
            <article class="settings-form">
              <h3>Server / HTTP</h3>
              <label>Bind address <input :value="pendingValue('http.addr')" type="text" @input="setPendingValue('http.addr', ($event.target as HTMLInputElement).value)" /></label>
              <label class="checkbox-label">
                <input
                  :checked="pendingValue('http.tls_auto_self_signed') === 'true'"
                  type="checkbox"
                  @change="setPendingValue('http.tls_auto_self_signed', ($event.target as HTMLInputElement).checked)"
                />
                TLS auto self-signed
              </label>
              <label>TLS cert path <input :value="pendingValue('http.tls_cert_file')" type="text" @input="setPendingValue('http.tls_cert_file', ($event.target as HTMLInputElement).value)" /></label>
              <label>TLS key path <input :value="pendingValue('http.tls_key_file')" type="text" @input="setPendingValue('http.tls_key_file', ($event.target as HTMLInputElement).value)" /></label>
              <p class="muted">Server settings are YAML-bound and require restart.</p>
            </article>
            <article class="settings-form">
              <h3>Pending YAML</h3>
              <button type="button" @click="savePendingSettings">Save pending YAML</button>
              <button type="button" @click="clearPendingSettings">Clear pending changes</button>
              <a href="/api/v1/settings/pending/download" target="_blank" rel="noreferrer">Download pending YAML</a>
            </article>
          </div>

          <div v-else-if="settingsTab === 'storage'" class="settings-grid">
            <article class="settings-form">
              <h3>Add Runtime Storage</h3>
              <p class="muted">Adds a filesystem adapter to the active process. Only non-destructive modes are enabled.</p>
              <label>Name <input v-model="storageDraft.name" type="text" placeholder="synthetic_fixture" /></label>
              <label>Kind <input v-model="storageDraft.kind" type="text" /></label>
              <label>Root
                <span class="input-with-button">
                  <input v-model="storageDraft.root" type="text" placeholder="/tmp/cartolensia_synthetic_media" />
                  <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker('storageDraft.root', 'folder')">
                    <i class="bi bi-folder2-open" aria-hidden="true"></i>
                    Browse
                  </button>
                </span>
              </label>
              <label>Source URL
                <input v-model="storageDraft.source_url" type="text" placeholder="smb://tnsmmi.local/share/path" />
                <small class="muted">Optional diagnostic URL. For SMB/CIFS, Cartolensia uses this to distinguish host, share, auth, and missing-file failures.</small>
              </label>
              <div class="settings-subgrid">
                <label>SMB host <input :value="storageDraft.smb?.host || ''" type="text" placeholder="tnsmmi.local" @input="setStorageDraftSMBField('host', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB share/export <input :value="storageDraft.smb?.share || ''" type="text" placeholder="multimedia" @input="setStorageDraftSMBField('share', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB subpath <input :value="storageDraft.smb?.path || ''" type="text" placeholder="optional/path" @input="setStorageDraftSMBField('path', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB domain <input :value="storageDraft.smb?.domain || ''" type="text" placeholder="WORKGROUP" @input="setStorageDraftSMBField('domain', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB username <input :value="storageDraft.smb?.username || ''" type="text" autocomplete="username" @input="setStorageDraftSMBField('username', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB credentials file
                  <span class="input-with-button">
                    <input :value="storageDraft.smb?.credentials_file || ''" type="text" placeholder="/etc/cartolensia/smb-storage.credentials" @input="setStorageDraftSMBField('credentials_file', ($event.target as HTMLInputElement).value)" />
                    <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker('storageDraft.smb.credentials_file', 'file')">
                      <i class="bi bi-folder2-open" aria-hidden="true"></i>
                      Browse
                    </button>
                  </span>
                </label>
                <label>Password env name <input :value="storageDraft.smb?.password_env || ''" type="text" placeholder="CARTOLENSIA_SMB_PASSWORD" @input="setStorageDraftSMBField('password_env', ($event.target as HTMLInputElement).value)" /></label>
              </div>
              <p class="muted">Secrets are not displayed here. Prefer a root-owned credentials file or an environment variable name; do not paste Samba passwords into the UI.</p>
              <label>Mode
                <select v-model="storageDraft.mode">
                  <option value="strict_read_only">strict_read_only</option>
                  <option value="read_only">read_only</option>
                  <option value="journaled_deferred" disabled>journaled_deferred (disabled)</option>
                  <option value="read_write" disabled>read_write (disabled)</option>
                </select>
              </label>
              <div class="actions">
                <button type="button" @click="validateStorageDraft">Validate</button>
                <button type="button" @click="addRuntimeStorage">Add runtime storage</button>
              </div>
              <p class="muted">Roots under /mnt/Models/rclone are locked to strict_read_only.</p>
            </article>
            <article class="settings-form">
              <h3>Pending Restart Storage</h3>
              <p class="muted">Use this for YAML-bound storage changes that should survive a restart.</p>
              <label>Name <input :value="pendingStorageField(0, 'name')" type="text" @input="setPendingStorageField(0, 'name', ($event.target as HTMLInputElement).value)" /></label>
              <label>Root
                <span class="input-with-button">
                  <input :value="pendingStorageField(0, 'root')" type="text" @input="setPendingStorageField(0, 'root', ($event.target as HTMLInputElement).value)" />
                  <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker('pending:storages.0.root', 'folder')">
                    <i class="bi bi-folder2-open" aria-hidden="true"></i>
                    Browse
                  </button>
                </span>
              </label>
              <label>Source URL <input :value="pendingStorageField(0, 'source_url')" type="text" placeholder="smb://tnsmmi.local/share/path" @input="setPendingStorageField(0, 'source_url', ($event.target as HTMLInputElement).value)" /></label>
              <div class="settings-subgrid">
                <label>SMB host <input :value="pendingStorageSMBField(0, 'host')" type="text" @input="setPendingStorageSMBField(0, 'host', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB share/export <input :value="pendingStorageSMBField(0, 'share')" type="text" @input="setPendingStorageSMBField(0, 'share', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB subpath <input :value="pendingStorageSMBField(0, 'path')" type="text" @input="setPendingStorageSMBField(0, 'path', ($event.target as HTMLInputElement).value)" /></label>
                <label>SMB credentials file
                  <span class="input-with-button">
                    <input :value="pendingStorageSMBField(0, 'credentials_file')" type="text" @input="setPendingStorageSMBField(0, 'credentials_file', ($event.target as HTMLInputElement).value)" />
                    <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker('pending:storages.0.smb.credentials_file', 'file')">
                      <i class="bi bi-folder2-open" aria-hidden="true"></i>
                      Browse
                    </button>
                  </span>
                </label>
              </div>
              <label>Mode
                <select :value="pendingStorageField(0, 'mode')" @change="setPendingStorageField(0, 'mode', ($event.target as HTMLSelectElement).value)">
                  <option value="strict_read_only">strict_read_only</option>
                  <option value="read_only">read_only</option>
                </select>
              </label>
              <button type="button" @click="savePendingSettings">Save pending YAML</button>
            </article>
            <article class="settings-form settings-wide">
              <h3>Configured Storages</h3>
              <div v-if="storageMessage" class="alert"><pre>{{ storageMessage }}</pre></div>
              <table>
                <thead><tr><th>Name</th><th>Root</th><th>Source / SMB</th><th>Mode</th><th>Health</th><th>Action</th></tr></thead>
                <tbody>
                  <tr v-for="storage in storages" :key="storage.name">
                    <td>{{ storage.name }}</td>
                    <td>{{ storage.root }}</td>
                    <td>
                      <small v-if="storage.source_url" class="d-block">{{ storage.source_url }}</small>
                      <small v-if="storage.smb?.host" class="d-block">
                        {{ storage.smb.host }}/{{ storage.smb.share || '?' }}<span v-if="storage.smb.path">/{{ storage.smb.path }}</span>
                      </small>
                      <small v-if="storage.smb?.credentials_file" class="muted d-block">credentials file configured</small>
                      <small v-else-if="storage.smb?.password_env" class="muted d-block">password env configured</small>
                    </td>
                    <td>{{ storage.mode }}</td>
                    <td>
                      <span :class="['status-badge', storage.health === 'available' ? 'ok' : 'warn']">{{ storage.health || 'unknown' }}</span>
                      <code v-if="storage.health_code" class="d-block">{{ storage.health_code }}</code>
                      <small v-if="storage.health_message" class="muted d-block">{{ storage.health_message }}</small>
                      <details v-if="storage.details" class="storage-details">
                        <summary>Probe details</summary>
                        <pre>{{ JSON.stringify(storage.details, null, 2) }}</pre>
                      </details>
                    </td>
                    <td><button type="button" @click="validateExistingStorage(storage.name)">Validate</button></td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>

          <div v-else-if="settingsTab === 'auth'" class="settings-grid">
            <article class="settings-form">
              <h3>Auth / Security</h3>
              <label>Mode
                <select :value="pendingValue('auth.mode')" @change="setPendingValue('auth.mode', ($event.target as HTMLSelectElement).value)">
                  <option value="dev_no_auth">dev_no_auth</option>
                  <option value="local">local</option>
                </select>
              </label>
              <label>Admin password env <input :value="pendingValue('auth.admin_password_env')" type="text" @input="setPendingValue('auth.admin_password_env', ($event.target as HTMLInputElement).value)" /></label>
              <label>Session TTL <input :value="pendingValue('auth.session_ttl')" type="text" @input="setPendingValue('auth.session_ttl', ($event.target as HTMLInputElement).value)" /></label>
              <p class="muted">Secrets are referenced by env var or file path only; do not paste passwords into pending YAML.</p>
              <button type="button" @click="savePendingSettings">Save pending YAML</button>
            </article>
          </div>

          <div v-else-if="settingsTab === 'ai'" class="settings-grid">
            <article :class="['settings-form', 'settings-wide', { 'highlight-card': vectorConfigHighlight }]">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-diagram-3" aria-hidden="true"></i> AI / Vector</h3>
                  <p class="muted">Local JSON/bruteforce vector search is active now. pgvector remains optional for larger collections.</p>
                </div>
                <span :class="['status-badge', vectorStatus?.available ? 'ok' : 'warn']">{{ vectorStatus?.backend ?? 'not configured' }}</span>
              </div>
              <div class="detail-grid compact-detail">
                <article><strong>{{ vectorStatus?.embedded_assets ?? vectorLimits.embedded_assets ?? 0 }}</strong><span>Embedded assets</span></article>
                <article><strong>{{ vectorStatus?.dimensions ?? "n/a" }}</strong><span>Dimensions</span></article>
                <article><strong>{{ vectorStatus?.pgvector ? "available" : "optional" }}</strong><span>pgvector</span></article>
              </div>
              <div class="form-stack">
                <label>
                  <span>Vector backend</span>
                  <select :value="pendingValue('ai.vector_backend', 'local_json_bruteforce')" @change="setPendingValue('ai.vector_backend', ($event.target as HTMLSelectElement).value)">
                    <option value="local_json_bruteforce">local_json_bruteforce</option>
                    <option value="pgvector" disabled>pgvector (future/optional)</option>
                  </select>
                  <small>Local fallback is production-safe for small personal/lab collections and requires no extension.</small>
                </label>
                <label>
                  <span>Preferred AI device</span>
                  <select :value="pendingValue('ai.device_preference', 'auto')" @change="setPendingValue('ai.device_preference', ($event.target as HTMLSelectElement).value)">
                    <option value="auto">auto - prefer largest available GPU</option>
                    <option value="cpu">CPU fallback</option>
                    <option value="nvidia" :disabled="!nativeCudaAvailable && !dockerNvidiaRuntime">NVIDIA CUDA</option>
                    <option value="amd" :disabled="!(aiStatus?.accelerator_hints as Record<string, unknown> | undefined)?.dev_dri">AMD/ROCm or VAAPI - unverified</option>
                    <option value="intel" :disabled="!(aiStatus?.accelerator_hints as Record<string, unknown> | undefined)?.dev_dri">Intel/XPU or QSV - unverified</option>
                  </select>
                  <small>Active now: {{ aiDevicePolicy.active_device ?? "cpu" }}. CPU fallback remains available.</small>
                </label>
                <label>
                  <span>Worker endpoint</span>
                  <input :value="pendingValue('ai.worker_endpoint')" type="text" @input="setPendingValue('ai.worker_endpoint', ($event.target as HTMLInputElement).value)" />
                  <small>Native sidecar default: http://127.0.0.1:19090. Use a remote host:port for a dedicated AI executor node.</small>
                </label>
                <label>
                  <span>Model cache directory</span>
                  <span class="input-with-button">
                    <input :value="pendingValue('ai.model_cache_dir', '.cartolensia/models')" type="text" @input="setPendingValue('ai.model_cache_dir', ($event.target as HTMLInputElement).value)" />
                    <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker('pending:ai.model_cache_dir', 'folder')">
                      <i class="bi bi-folder2-open" aria-hidden="true"></i>
                      Browse
                    </button>
                  </span>
                  <small>Must stay outside original storage. Default is repo-local.</small>
                </label>
              </div>
              <div class="settings-actions">
                <button type="button" class="btn btn-outline-primary" @click="savePendingSettings">
                  <i class="bi bi-save" aria-hidden="true"></i>
                  Save pending YAML
                </button>
                <button type="button" class="btn btn-outline-secondary" @click="setActive('Base AI')">
                  Back to AI dashboard
                </button>
              </div>
            </article>
          </div>

          <div v-else-if="settingsTab === 'components'" class="settings-grid">
            <article class="settings-form settings-wide">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-box-seam" aria-hidden="true"></i> Component Manager</h3>
                  <p class="muted">Tools, OCR language packs, Python runtime, and AI models are tracked with source and license provenance. No component is installed at startup.</p>
                </div>
                <button type="button" class="btn btn-outline-primary" :disabled="Boolean(componentBusyKey)" @click="refreshComponents">
                  <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
                  Refresh
                </button>
              </div>
              <div class="detail-grid compact-detail">
                <article><strong>{{ componentCounts.installed ?? 0 }}</strong><span>Installed</span></article>
                <article><strong>{{ componentCounts.user_provided ?? 0 }}</strong><span>User provided</span></article>
                <article><strong>{{ componentCounts.missing ?? 0 }}</strong><span>Missing</span></article>
                <article><strong>{{ componentCounts.failed ?? 0 }}</strong><span>Failed</span></article>
              </div>
              <p class="muted">Component root: <code>{{ componentRoot || '.cartolensia/components' }}</code>. Archives are extracted only there after traversal and expected-file validation.</p>
              <div v-if="componentMessage" class="alert">{{ componentMessage }}</div>
            </article>

            <article v-for="group in componentCategories" :key="group.key" class="settings-form settings-wide">
              <h3>{{ group.label }}</h3>
              <div class="component-grid">
                <article v-for="component in group.items" :key="component.key" class="component-card">
                  <div class="section-title compact-title">
                    <div>
                      <h4>{{ component.name }}</h4>
                      <code>{{ component.key }}</code>
                    </div>
                    <span :class="['status-badge', componentStatusClass(component.status)]">
                      <span v-if="componentBusyKey === component.key" class="spinner-border spinner-border-sm" aria-hidden="true"></span>
                      {{ component.status }}
                    </span>
                  </div>
                  <p>{{ componentSourceLabel(component) }}</p>
                  <dl class="component-facts">
                    <div><dt>License</dt><dd>{{ component.license_name || 'review required' }}</dd></div>
                    <div><dt>Source</dt><dd>{{ component.provenance_url || component.source_url || 'operator supplied' }}</dd></div>
                    <div><dt>Install path</dt><dd><code>{{ component.install_path || component.executable_path || 'not configured' }}</code></dd></div>
                    <div v-if="component.checksum"><dt>Checksum</dt><dd><code>{{ component.checksum.slice(0, 16) }}...</code></dd></div>
                  </dl>
                  <p v-if="component.error" class="alert alert-warning">{{ component.error }}</p>
                  <div class="component-actions">
                    <button type="button" class="btn btn-sm btn-outline-primary" :disabled="Boolean(componentBusyKey)" @click="runComponentCheck(component.key)">
                      <i class="bi bi-search" aria-hidden="true"></i>
                      Check
                    </button>
                    <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="Boolean(componentBusyKey)" @click="requestComponentDownload(component.key)">
                      <i class="bi bi-cloud-download" aria-hidden="true"></i>
                      Download/Install
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm"
                      :class="component.status === 'disabled' ? 'btn-outline-success' : 'btn-outline-warning'"
                      :disabled="Boolean(componentBusyKey)"
                      @click="setComponentEnabled(component.key, component.status === 'disabled')"
                    >
                      <i class="bi" :class="component.status === 'disabled' ? 'bi-toggle-on' : 'bi-toggle-off'" aria-hidden="true"></i>
                      {{ component.status === 'disabled' ? 'Enable' : 'Disable' }}
                    </button>
                  </div>
                  <div class="component-provide">
                    <label>
                      <span>Provide path</span>
                      <span class="input-with-button">
                        <input v-model="componentPathDrafts[component.key]" type="text" placeholder="/usr/bin/ffmpeg or extracted directory" />
                        <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker(`component:path:${component.key}`, 'folder')">
                          <i class="bi bi-folder2-open" aria-hidden="true"></i>
                          Browse
                        </button>
                      </span>
                    </label>
                    <button type="button" class="btn btn-sm btn-primary" :disabled="Boolean(componentBusyKey)" @click="provideComponentPath(component.key)">Use path</button>
                    <label>
                      <span>Provide archive</span>
                      <span class="input-with-button">
                        <input v-model="componentArchiveDrafts[component.key]" type="text" placeholder="/tmp/component.zip or .tar.gz" />
                        <button type="button" class="btn btn-outline-secondary btn-sm" @click="openFilePicker(`component:archive:${component.key}`, 'file')">
                          <i class="bi bi-file-earmark-zip" aria-hidden="true"></i>
                          Browse
                        </button>
                      </span>
                    </label>
                    <button type="button" class="btn btn-sm btn-primary" :disabled="Boolean(componentBusyKey)" @click="provideComponentArchive(component.key)">Import archive</button>
                  </div>
                  <details @toggle="($event.target as HTMLDetailsElement).open && loadComponentEvents(component.key)">
                    <summary>Events</summary>
                    <ul class="component-events">
                      <li v-for="event in componentEvents[component.key] ?? []" :key="event.id">
                        <span :class="['status-badge', event.level === 'error' ? 'bad' : event.level === 'warn' ? 'warn' : 'ok']">{{ event.level }}</span>
                        <span>{{ event.created_at }}</span>
                        <span>{{ event.message }}</span>
                      </li>
                    </ul>
                  </details>
                </article>
              </div>
            </article>
          </div>

          <div v-else-if="settingsTab === 'readiness'" class="settings-grid">
            <article class="settings-form settings-wide readiness-summary">
              <div class="section-title">
                <div>
                  <h3><i class="bi bi-clipboard2-check" aria-hidden="true"></i> Deployment Readiness</h3>
                  <p class="muted">{{ readiness?.note ?? 'Checks run against the current process and configured paths.' }}</p>
                </div>
                <span :class="['status-badge', readinessStatusClass(readiness?.status ?? 'warn')]">
                  {{ readiness?.status ?? 'loading' }}
                </span>
              </div>
              <div class="detail-grid compact-detail">
                <article><strong>{{ readiness?.counts?.ok ?? 0 }}</strong><span>OK</span></article>
                <article><strong>{{ readiness?.counts?.warn ?? 0 }}</strong><span>Warnings</span></article>
                <article><strong>{{ readiness?.counts?.error ?? 0 }}</strong><span>Errors</span></article>
                <article><strong>{{ readiness?.generated_at ? new Date(readiness.generated_at).toLocaleTimeString() : 'n/a' }}</strong><span>Last check</span></article>
              </div>
              <div class="settings-actions">
                <button type="button" class="btn btn-outline-primary" @click="refresh">
                  <i class="bi bi-arrow-clockwise" aria-hidden="true"></i>
                  Recheck
                </button>
                <button type="button" class="btn btn-outline-secondary" @click="settingsTab = 'components'">
                  Components
                </button>
                <button type="button" class="btn btn-outline-secondary" @click="settingsTab = 'storage'">
                  Storage
                </button>
                <button type="button" class="btn btn-outline-secondary" @click="settingsTab = 'auth'">
                  Auth
                </button>
              </div>
            </article>

            <article v-for="group in readinessGroups" :key="group.category" class="settings-form settings-wide">
              <h3>{{ group.category }}</h3>
              <div class="readiness-list">
                <article v-for="check in group.checks" :key="check.id" class="readiness-check">
                  <i :class="['bi', readinessIcon(check.status), readinessStatusClass(check.status)]" aria-hidden="true"></i>
                  <div>
                    <div class="section-title compact-title">
                      <h4>{{ check.label }}</h4>
                      <span :class="['status-badge', readinessStatusClass(check.status)]">{{ check.status }}</span>
                    </div>
                    <p>{{ check.summary }}</p>
                    <details v-if="check.details">
                      <summary>Details</summary>
                      <pre class="compact-json">{{ JSON.stringify(check.details, null, 2) }}</pre>
                    </details>
                  </div>
                </article>
              </div>
            </article>
          </div>

          <div v-else-if="settingsTab === 'raw'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>Raw YAML / Effective Config</h3>
              <p class="muted">Raw view is for audit/debug. Use the tabs for editable controls.</p>
              <pre class="geojson">{{ JSON.stringify({ effective: settings?.effective ?? {}, pending: pendingConfig }, null, 2) }}</pre>
            </article>
          </div>

          <div v-else-if="settingsTab === 'backups'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>DB Export</h3>
              <p class="muted">
                Export writes a metadata/config JSON file under the Cartolensia cache export directory. It never writes into original storage.
              </p>
              <button type="button" @click="createDBExport">Create metadata export</button>
              <p>{{ dbExportMessage }}</p>
              <table v-if="dbExports.length > 0">
                <thead><tr><th>Export</th><th>Size</th><th>Created</th><th>Download</th></tr></thead>
                <tbody>
                  <tr v-for="item in dbExports" :key="item.id">
                    <td>{{ item.id }}</td>
                    <td>{{ formatBytes(item.size_bytes) }}</td>
                    <td>{{ item.created_at }}</td>
                    <td><a :href="item.download_url" target="_blank" rel="noreferrer">Download</a></td>
                  </tr>
                </tbody>
              </table>
            </article>
          </div>

          <div v-else-if="settingsTab === 'plugins'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>Plugin Settings</h3>
              <div class="settings-tabs secondary-tabs">
                <button
                  v-for="plugin in plugins"
                  :key="plugin.id"
                  type="button"
                  :class="{ active: selectedPluginSettingsId === plugin.id }"
                  @click="selectedPluginSettingsId = plugin.id"
                >
                  {{ plugin.name }}
                </button>
              </div>
              <div class="actions">
                <button type="button" :class="{ active: pluginSettingsMode === 'ui' }" @click="pluginSettingsMode = 'ui'">UI config</button>
                <button type="button" :class="{ active: pluginSettingsMode === 'yaml' }" @click="pluginSettingsMode = 'yaml'">YAML/JSON config</button>
              </div>
              <template v-if="selectedPluginSettings">
                <p>{{ selectedPluginSettings.id }} · {{ selectedPluginSettings.status }} · {{ selectedPluginSettings.runtime }}</p>
                <div v-if="pluginSettingsMode === 'ui'" class="form-stack">
                  <label v-for="spec in pluginSpecs(selectedPluginSettings.id)" :key="spec.key">
                    {{ spec.label }}
                    <select
                      v-if="spec.kind === 'boolean'"
                      :value="pluginSettingValue(selectedPluginSettings.id, spec.key) || 'false'"
                      @change="setPluginSettingValue(selectedPluginSettings.id, spec.key, ($event.target as HTMLSelectElement).value === 'true')"
                    >
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                    <input
                      v-else
                      :type="spec.kind === 'number' ? 'number' : 'text'"
                      :value="pluginSettingValue(selectedPluginSettings.id, spec.key)"
                      @input="setPluginSettingValue(selectedPluginSettings.id, spec.key, spec.kind === 'number' ? Number(($event.target as HTMLInputElement).value) : ($event.target as HTMLInputElement).value)"
                    />
                    <small>{{ spec.help }}</small>
                  </label>
                </div>
                <textarea
                  v-else
                  :value="pluginSettingText[selectedPluginSettings.id] ?? '{}'"
                  rows="12"
                  @input="pluginSettingText[selectedPluginSettings.id] = ($event.target as HTMLTextAreaElement).value"
                ></textarea>
                <div class="actions">
                  <button type="button" @click="savePluginSettings(selectedPluginSettings.id)">Save plugin settings</button>
                  <button type="button" @click="pluginSettingText[selectedPluginSettings.id] = '{}'">Reset to defaults</button>
                </div>
              </template>
              <p class="muted">Settings persist through the plugin settings endpoint; plugins opt into using them.</p>
            </article>
          </div>

          <div v-else class="settings-grid">
            <article class="settings-form">
              <h3>{{ settingsTab }}</h3>
              <p class="muted">This tab is currently informational.</p>
            </article>
          </div>

          <div class="settings-grid">
            <form v-if="backend?.auth_mode === 'local' && principal" class="settings-form" @submit.prevent="changePassword">
              <h3>Password</h3>
              <input v-model="oldPassword" type="password" autocomplete="current-password" placeholder="Current password" />
              <input v-model="newPassword" type="password" autocomplete="new-password" placeholder="New password" />
              <button type="submit">Change Password</button>
            </form>
            <form v-if="backend?.auth_mode === 'local' && principal" class="settings-form" @submit.prevent="createToken">
              <h3>API Tokens</h3>
              <input v-model="tokenName" type="text" placeholder="Token name" />
              <input v-model="tokenScopes" type="text" placeholder="Scopes" />
              <button type="submit">Create Token</button>
              <code v-if="tokenSecret">{{ tokenSecret }}</code>
            </form>
          </div>
          <table v-if="apiTokens.length > 0">
            <thead><tr><th>Name</th><th>Scopes</th><th>Expires</th><th>Last Used</th></tr></thead>
            <tbody>
              <tr v-for="token in apiTokens" :key="token.id">
                <td>{{ token.name }}</td>
                <td>{{ token.scopes.join(", ") }}</td>
                <td>{{ token.expires_at ?? "" }}</td>
                <td>{{ token.last_used_at ?? "" }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else class="panel">
          <header class="panel-head"><h2>{{ active }}</h2></header>
          <p>{{ activePlugin?.description ?? "Plugin surface reserved for the next MVP phase." }}</p>
          <p class="muted">Backend manifest is loaded; feature execution is intentionally stubbed.</p>
        </section>
      </section>
    </div>
    <div v-if="galleryOpen && galleryCurrent" class="gallery-overlay" role="dialog" aria-modal="true">
      <button type="button" class="gallery-close danger-close" aria-label="Close viewer" @click="closeGallery">
        <i class="bi bi-x-lg" aria-hidden="true"></i>
        Close
      </button>
      <button type="button" class="gallery-nav gallery-prev" aria-label="Previous asset" @click="nextGallery(-1)">‹</button>
      <figure :class="['gallery-stage', { 'track-gallery-stage': galleryCurrent.media_kind === 'track' }]">
        <img
          v-if="galleryCurrent.media_kind === 'photo'"
          :class="{ actual: galleryZoomMode === 'actual' }"
          :style="galleryImageStyle()"
          :src="galleryCurrent.original_url"
          :alt="galleryCurrent.name"
          @wheel.prevent="wheelGallery"
          @dblclick="toggleGalleryZoom"
          @pointerdown.prevent="galleryPointerDown"
          @pointermove.prevent="galleryPointerMove"
          @pointerup="galleryPointerUp"
          @pointercancel="galleryPointerUp"
        />
        <video
          v-else-if="galleryCurrent.media_kind === 'video'"
          :key="videoSource(galleryCurrent.id, galleryCurrent.original_url)"
          ref="galleryVideoElement"
          :src="videoSource(galleryCurrent.id, galleryCurrent.original_url)"
          controls
          preload="metadata"
        ></video>
        <div v-else-if="galleryCurrent.media_kind === 'audio'" class="gallery-audio-player">
          <i class="bi bi-soundwave" aria-hidden="true"></i>
          <audio ref="galleryAudioElement" :src="galleryCurrent.original_url" controls preload="metadata"></audio>
          <strong>{{ galleryCurrent.name }}</strong>
          <span>{{ galleryCurrent.relative_path }}</span>
        </div>
        <div v-else-if="galleryCurrent.media_kind === 'track'" class="map-shell gallery-track-shell">
          <div
            ref="galleryTrackElement"
            class="gallery-track-map"
            role="img"
            aria-label="Interactive track preview"
          ></div>
          <svg v-if="galleryTrackFallbackPath" class="track-vector-fallback" viewBox="0 0 1000 420" aria-hidden="true">
            <path :d="galleryTrackFallbackPath" />
          </svg>
          <div class="map-status-overlay">
            <i class="bi bi-signpost-split" aria-hidden="true"></i>
            {{ galleryTrackPreviewStatus || "Loading track geometry..." }}
          </div>
          <div class="map-layer-control">
            <button type="button" class="icon-button" @click="showGalleryTrackLayerMenu = !showGalleryTrackLayerMenu">
              <i class="bi bi-layers" aria-hidden="true"></i>
              Layers
            </button>
            <div v-if="showGalleryTrackLayerMenu" class="layer-menu">
              <label class="form-check form-switch">
                <input v-model="trackPreviewTilesEnabled" class="form-check-input" type="checkbox" />
                <span>OSM tiles</span>
              </label>
              <label class="form-check form-switch">
                <input v-model="galleryTrackLayerVisible" class="form-check-input" type="checkbox" />
                <span>Track layer</span>
              </label>
              <button type="button" class="btn btn-sm btn-outline-primary" @click="fitGalleryTrack">
                <i class="bi bi-aspect-ratio" aria-hidden="true"></i>
                Fit to track
              </button>
              <small>Tile background uses Cartolensia's on-demand proxy only.</small>
            </div>
          </div>
        </div>
        <div v-else class="gallery-fallback">
          <strong>{{ galleryCurrent.media_kind }}</strong>
          <span>Inline preview is unavailable for this asset type.</span>
        </div>
        <figcaption>
          <strong>{{ galleryCurrent.name }}</strong>
          <span>{{ galleryIndex + 1 }} / {{ galleryItems.length }}</span>
          <span>{{ galleryCurrent.date ?? "" }}</span>
          <span>{{ galleryCurrent.relative_path ?? "" }}</span>
          <div v-if="galleryCurrent.media_kind === 'video'" class="quality-row">
            <label>
              Quality
              <select
                :value="transcodeSession?.asset_id === galleryCurrent.id ? transcodeSession.profile : 'original'"
                @change="selectTranscodeOption(galleryCurrent.id, $event)"
              >
                <option
                  v-for="option in streamOptions?.options ?? []"
                  :key="option.id"
                  :disabled="!option.available"
                  :value="option.id"
                >
                  {{ option.label }}{{ option.available ? "" : " — planned" }}
                </option>
              </select>
            </label>
            <span>{{ transcodeMessage || "Original streaming is active; safe transcode sessions use the Cartolensia cache only." }}</span>
	            <button v-if="transcodeSession?.asset_id === galleryCurrent.id" type="button" @click="stopActiveTranscode">
	              Stop session
	            </button>
	            <button type="button" class="icon-button" @click="openAdvancedTranscode(galleryCurrent.id)">
	              <i class="bi bi-gear" aria-hidden="true"></i>
	              Advanced
	            </button>
	          </div>
          <div class="actions">
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="toggleGalleryZoom">
              {{ galleryZoomMode === "fit" ? "100% zoom" : "Fit to screen" }}
            </button>
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="resetGalleryZoom">Fit</button>
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="galleryZoomMode = 'actual'; galleryScale = 1; galleryPanX = 0; galleryPanY = 0">100%</button>
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="resetGalleryZoom">Reset zoom</button>
            <span v-if="galleryCurrent.media_kind === 'photo'">{{ Math.round(galleryScale * 100) }}%</span>
            <a :href="galleryCurrent.original_url" target="_blank" rel="noreferrer">Open Original</a>
            <a
              class="btn btn-outline-secondary"
              :href="assetHref(galleryCurrent.id)"
              @click="openAssetLink($event, galleryCurrent.id, { closeOverlay: true, preservePlayback: true })"
            >
              Asset Detail
            </a>
          </div>
        </figcaption>
      </figure>
      <button type="button" class="gallery-nav gallery-next" aria-label="Next asset" @click="nextGallery(1)">›</button>
    </div>

    <div
      v-if="filePickerOpen"
      class="modal fade show d-block file-picker-modal"
      tabindex="-1"
      role="dialog"
      aria-modal="true"
      aria-label="Browse server paths"
    >
      <div class="modal-dialog modal-lg modal-dialog-centered">
        <div class="modal-content bg-dark text-light">
          <div class="modal-header">
            <h2 class="modal-title h5">
              <i class="bi bi-folder2-open" aria-hidden="true"></i>
              Browse Server Paths
            </h2>
            <button type="button" class="btn-close btn-close-white" aria-label="Close" @click="filePickerOpen = false"></button>
          </div>
          <div class="modal-body">
            <p class="muted">Read-only path picker. It lists only allowlisted roots and never writes to storage.</p>
            <div v-if="filePickerMessage" class="alert">{{ filePickerMessage }}</div>
            <div class="settings-grid">
              <label class="form-label">
                Root
                <select class="form-select" :value="filePickerRoot" @change="chooseFilePickerRoot(($event.target as HTMLSelectElement).value)">
                  <option value="">Choose root...</option>
                  <option v-for="root in filePickerRoots" :key="root.id" :value="root.id">
                    {{ root.label }} · {{ root.path }}
                  </option>
                </select>
              </label>
              <label class="form-label">
                Current path
                <span class="input-with-button">
                  <input v-model="filePickerPath" class="form-control" type="text" @keyup.enter="loadFilePickerPath()" />
                  <button type="button" class="btn btn-outline-light btn-sm" @click="loadFilePickerPath()">Open</button>
                </span>
              </label>
            </div>
            <div class="breadcrumb-line">
              <button v-if="filePicker?.parent !== undefined" type="button" class="btn btn-sm btn-outline-light" @click="loadFilePickerPath(filePicker?.parent || '')">
                <i class="bi bi-arrow-up" aria-hidden="true"></i>
                Parent
              </button>
              <span>{{ filePickerAbsolute() }}</span>
            </div>
            <div v-for="warning in filePicker?.warnings ?? []" :key="warning" class="alert alert-warning">{{ warning }}</div>
            <div class="file-picker-list">
              <button
                v-for="entry in filePicker?.entries ?? []"
                :key="entry.path"
                type="button"
                :class="['file-picker-entry', { folder: entry.kind === 'folder' }]"
                @dblclick="openFilePickerEntry(entry)"
                @click="entry.kind === 'folder' ? loadFilePickerPath(entry.path) : filePickerKind === 'file' && selectFilePickerPath(entry.path)"
              >
                <i :class="['bi', entry.kind === 'folder' ? 'bi-folder2' : 'bi-file-earmark']" aria-hidden="true"></i>
                <span>{{ entry.name }}</span>
                <small>{{ entry.kind === 'folder' ? 'folder' : formatBytes(entry.size_bytes ?? 0) }}</small>
              </button>
            </div>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-outline-light danger-close" @click="filePickerOpen = false">
              <i class="bi bi-x-lg" aria-hidden="true"></i>
              Close
            </button>
            <button type="button" class="btn btn-primary" @click="selectFilePickerPath()">
              Select current {{ filePickerKind }}
            </button>
          </div>
        </div>
      </div>
    </div>
    <div v-if="filePickerOpen" class="modal-backdrop fade show cartolensia-modal-backdrop"></div>

    <div
      v-if="showAdvancedTranscode"
      class="modal fade show d-block transcode-modal"
      tabindex="-1"
      role="dialog"
      aria-modal="true"
      aria-label="Advanced transcode settings"
      @keydown="handleAdvancedTranscodeKeydown"
    >
      <div class="modal-dialog modal-lg modal-dialog-centered">
        <div class="modal-content bg-dark text-light">
          <div class="modal-header">
            <h2 class="modal-title h5">Advanced Transcode Settings</h2>
            <button type="button" class="btn-close btn-close-white" aria-label="Close" @click="closeAdvancedTranscode"></button>
          </div>
          <div class="modal-body">
            <div class="settings-grid">
              <label class="form-label">
                Preset name
                <input v-model="customPresetName" data-autofocus class="form-control" type="text" />
              </label>
              <label class="form-label">
                Hardware
                <select v-model="customPresetHardware" class="form-select">
                  <option
                    v-for="hardware in groupedTranscodeHardware"
                    :key="hardware.id"
                    :disabled="!hardware.available"
                    :value="hardware.id"
                  >
                    {{ hardware.label }}{{ hardware.available ? "" : ` — ${hardware.reason}` }}
                  </option>
                </select>
              </label>
              <label class="form-label">
                Codec
                <select v-model="customPresetCodec" class="form-select">
                  <option value="h264">H.264</option>
                  <option value="h265">H.265 / HEVC</option>
                  <option value="av1">AV1</option>
                  <option value="custom">Custom encoder</option>
                </select>
              </label>
              <label class="form-label">
                Encoder
                <select v-model="customPresetEncoder" class="form-select">
                  <option value="">Auto</option>
                  <option v-for="encoder in availableEncodersForCustom" :key="encoder.name" :value="encoder.name">
                    {{ encoder.name }}{{ encoder.description ? ` · ${encoder.description}` : "" }}
                  </option>
                </select>
              </label>
              <label class="form-label">
                Mode
                <select v-model="customPresetMode" class="form-select">
                  <option value="quality">Quality / CRF</option>
                  <option value="quantizer">Quantizer / CQ</option>
                  <option value="bitrate">Bitrate</option>
                </select>
              </label>
              <label class="form-label">
                Value
                <input v-model="customPresetParameter" class="form-control" type="text" placeholder="28 or 1500k" />
              </label>
            </div>
            <p class="muted mt-3">
              HLS output is written only into the Cartolensia transcode cache. Originals remain immutable.
            </p>
            <pre v-if="transcodeValidation" class="logbox">{{ JSON.stringify(transcodeValidation, null, 2) }}</pre>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-outline-light danger-close" @click="closeAdvancedTranscode">
              <i class="bi bi-x-lg" aria-hidden="true"></i>
              Close
            </button>
            <button type="button" class="btn btn-outline-info" :disabled="!advancedTranscodeAssetId" @click="testCustomTranscodePreset(advancedTranscodeAssetId)">
              Test current hardware configuration
            </button>
            <button type="button" class="btn btn-primary" :disabled="!advancedTranscodeAssetId" @click="applyCustomTranscodePreset(advancedTranscodeAssetId)">
              Apply
            </button>
            <button type="button" class="btn btn-success" @click="saveCustomTranscodePreset">Save preset</button>
            <button
              v-if="lastVideoPreset !== 'original' && transcodePresets.some((preset) => preset.id === lastVideoPreset && !preset.built_in)"
              type="button"
              class="btn btn-outline-danger"
              @click="removeTranscodePreset(lastVideoPreset)"
            >
              Remove selected custom preset
            </button>
          </div>
        </div>
      </div>
    </div>
    <div v-if="showAdvancedTranscode" class="modal-backdrop fade show cartolensia-modal-backdrop"></div>
  </main>
</template>
