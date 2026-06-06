<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import OLMap from "ol/Map";
import View from "ol/View";
import GeoJSON from "ol/format/GeoJSON";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import Overlay from "ol/Overlay";
import VectorSource from "ol/source/Vector";
import XYZ from "ol/source/XYZ";
import { fromLonLat } from "ol/proj";
import { Circle as CircleStyle, Fill, Stroke, Style, Text } from "ol/style";
import Hls from "hls.js";
import "ol/ol.css";
import {
  api,
  type Album,
  type AlbumItemPage,
  type APIToken,
  type AssetDetail,
  type BackendStatus,
  type DuplicatePage,
  type ExplorerRow,
  type ExplorerView,
  type Job,
  type JobStats,
  type IndexingStatus,
  type MonthBucket,
  type PluginManifest,
  type PreviewCacheEntry,
  type PreviewCacheStats,
  type Principal,
  type ScanRun,
  type SettingsPayload,
  type Stats,
  type StreamOptions,
  type TranscodeSession,
  type StorageConfig,
  type TileSource,
  type TrackDetail,
  type TrackSummary,
  type TranscodingCapabilities,
  type DBExport
} from "./api";

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
  "AI Classification"
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
  ai: "Base AI",
  "ai-classification": "AI Classification"
};

const routeLabels = new Set([...nav, "Asset Detail"]);

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

const active = ref(pageFromQuery() ?? safeRoute(localStorage.getItem("cartolensia.route")));
const loading = ref(false);
const error = ref("");
const rows = ref<ExplorerRow[]>([]);
const explorer = ref<ExplorerView | null>(null);
const explorerPath = ref("");
const explorerQ = ref("");
const explorerMediaKind = ref("");
const explorerHashStatus = ref("");
const explorerExtension = ref("");
const explorerSort = ref("name");
const assetDetail = ref<AssetDetail | null>(null);
const jobs = ref<Job[]>([]);
const jobStats = ref<JobStats | null>(null);
const selectedJob = ref<Job | null>(null);
const storages = ref<StorageConfig[]>([]);
const plugins = ref<PluginManifest[]>([]);
const stats = ref<Stats | null>(null);
const duplicatePage = ref<DuplicatePage | null>(null);
const backendMonthBuckets = ref<MonthBucket[]>([]);
const backend = ref<BackendStatus | null>(null);
const principal = ref<Principal | null>(null);
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
const pendingConfig = ref<Record<string, unknown>>({});
const pluginSettingText = ref<Record<string, string>>({});
const dbExports = ref<DBExport[]>([]);
const dbExportMessage = ref("");
const tracks = ref<TrackSummary[]>([]);
const selectedTrack = ref<TrackDetail | null>(null);
const trackAssets = ref<AssetDetail["asset"][]>([]);
const trackOffsetSeconds = ref(0);
const trackMediaViewMode = ref<"table" | "tile">("tile");
const mapData = ref<Record<string, unknown> | null>(null);
const mapStatus = ref<Record<string, unknown> | null>(null);
const mapMediaKind = ref("");
const mapCluster = ref(true);
const mapAlbumId = ref("");
const mapTrackId = ref("");
const mapPopup = ref<{
  kind: "cluster" | "asset";
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
} | null>(null);
const tileSources = ref<TileSource[]>([]);
const tileStatus = ref("OpenStreetMap tiles load on demand through Cartolensia.");
const mapElement = ref<HTMLDivElement | null>(null);
const mapPopupElement = ref<HTMLDivElement | null>(null);
const assetVideoElement = ref<HTMLVideoElement | null>(null);
const galleryVideoElement = ref<HTMLVideoElement | null>(null);
let olMap: OLMap | null = null;
let mapOverlay: Overlay | null = null;
let activeHls: Hls | null = null;
let mapHasInitialFit = false;
const mapSource = new VectorSource();
const transcodingCapabilities = ref<TranscodingCapabilities | null>(null);
const aiStatus = ref<Record<string, unknown> | null>(null);
const vectorStatus = ref<Record<string, unknown> | null>(null);
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
const jobMaxFiles = ref(50);
const dryRunStorage = ref("");
const dryRunPrefix = ref("Cartolensia-photos");
const dryRunMaxFiles = ref(50);
const dryRunMaxBytes = ref(2147483648);
const dryRunExtensions = ref("jpg,jpeg,png,gpx,kml,kmz,gpz,heif,heic,mp4,mov");
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
const galleryZoomMode = ref<"fit" | "actual">("fit");
const galleryScale = ref(1);
const galleryPanX = ref(0);
const galleryPanY = ref(0);

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
const failedPreviewIds = ref<Set<string>>(new Set());
const galleryOpen = computed(() => galleryItems.value.length > 0);
const galleryCurrent = computed(() => galleryItems.value[galleryIndex.value] ?? null);

const activePlugin = computed(() => {
  const id = active.value.toLowerCase().replaceAll(" ", "-");
  return plugins.value.find((plugin) => plugin.id === id || plugin.name.toLowerCase() === active.value.toLowerCase());
});

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
  if (route !== "Asset Detail") {
    localStorage.setItem("cartolensia.route", route);
  }
  if (updateURL && route !== "Asset Detail") {
    const url = new URL(window.location.href);
    url.searchParams.set("page", pageSlug(route));
    window.history.pushState({}, "", `${url.pathname}${url.search}${url.hash}`);
  }
}

window.addEventListener("popstate", () => {
  active.value = pageFromQuery() ?? safeRoute(localStorage.getItem("cartolensia.route"));
});

const visibleExplorerRows = computed(() => {
  const files = asArray(explorer.value?.files);
  const base = files.length > 0 || explorerPath.value ? files : rows.value;
  if (!monthFilter.value) return base;
  return base.filter((row) => monthKey(row.mtime) === monthFilter.value);
});

const selectedAlbumItems = computed(() => asArray(albumItems.value?.items));
const selectedAlbumAssets = computed(() => selectedAlbumItems.value.map((item) => item.asset));

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

function explorerQueryString(): string {
  const params = new URLSearchParams();
  if (explorerQ.value.trim()) params.set("q", explorerQ.value.trim());
  if (explorerMediaKind.value) params.set("media_kind", explorerMediaKind.value);
  if (explorerHashStatus.value) params.set("hash_status", explorerHashStatus.value);
  if (explorerExtension.value.trim()) params.set("extension", explorerExtension.value.trim());
  if (explorerSort.value) params.set("sort", explorerSort.value);
  return params.toString();
}

function rowToGallery(row: ExplorerRow): GalleryItem {
  return {
    id: row.asset_id,
    name: row.name,
    media_kind: row.media_kind,
    relative_path: row.relative_path,
    date: row.mtime,
    size_bytes: row.size_bytes,
    preview_url: `/api/v1/media/${encodeURIComponent(row.asset_id)}/preview`,
    original_url: `/api/v1/media/${encodeURIComponent(row.asset_id)}/original`
  };
}

function assetToGallery(asset: AssetDetail["asset"]): GalleryItem {
  const location = asArray(asset.locations)[0];
  return {
    id: asset.id,
    name: asset.display_name,
    media_kind: asset.media_kind,
    relative_path: location?.relative_path,
    date: asset.taken_at ?? location?.mtime,
    size_bytes: location?.size_bytes,
    preview_url: `/api/v1/media/${encodeURIComponent(asset.id)}/preview`,
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
}

function closeGallery() {
  if (transcodeSession.value) void stopActiveTranscode();
  galleryItems.value = [];
  galleryIndex.value = 0;
}

function nextGallery(delta: number) {
  if (!galleryItems.value.length) return;
  if (transcodeSession.value) void stopActiveTranscode();
  galleryIndex.value = (galleryIndex.value + delta + galleryItems.value.length) % galleryItems.value.length;
  resetGalleryZoom();
  void refreshStreamOptionsForGallery();
}

function resetGalleryZoom() {
  galleryZoomMode.value = "fit";
  galleryScale.value = 1;
  galleryPanX.value = 0;
  galleryPanY.value = 0;
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
  const next = galleryScale.value * (event.deltaY < 0 ? 1.12 : 0.88);
  galleryScale.value = Math.max(0.25, Math.min(6, next));
}

function panGallery(dx: number, dy: number) {
  if (galleryZoomMode.value !== "actual") return;
  galleryPanX.value += dx;
  galleryPanY.value += dy;
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
  if (prefixes.length === 0) return "Enter at least one storage-relative subpath, for example Cartolensia-photos.";
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

function videoSource(assetId: string, originalUrl?: string): string {
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
    await stopActiveTranscode();
    return;
  }
  const option = asArray(streamOptions.value?.options).find((candidate) => candidate.id === optionID);
  if (!option || !option.available) {
    transcodeMessage.value = option?.disabled_reason ?? "This stream option is not available.";
    select.value = "original";
    return;
  }
  if (!option.profile && !option.session_endpoint) {
    await stopActiveTranscode();
    return;
  }
  await stopActiveTranscode();
  transcodeMessage.value = "Starting cache-scoped transcode session...";
  try {
    transcodeSession.value = await api.startTranscodeSession(assetId, option.profile ?? option.id);
    transcodeMessage.value = "Waiting for HLS playlist and first segment in the Cartolensia cache...";
    transcodeSession.value = await waitForTranscodeReady(transcodeSession.value.id);
    await nextTick();
    await attachHLSPlayback();
    transcodeMessage.value = "Streaming HLS from the Cartolensia cache. Originals remain immutable.";
  } catch (err) {
    transcodeMessage.value = err instanceof Error ? err.message : String(err);
    await stopActiveTranscode();
    select.value = "original";
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

async function attachHLSPlayback() {
  const session = transcodeSession.value;
  if (!session?.playlist_url) return;
  const video = galleryCurrent.value?.id === session.asset_id ? galleryVideoElement.value : assetVideoElement.value;
  if (!video) return;
  if (canPlayNativeHLS()) {
    video.src = session.playlist_url;
    await video.play().catch(() => undefined);
    return;
  }
  if (!Hls.isSupported()) {
    throw new Error("This browser cannot play HLS natively and hls.js is unavailable.");
  }
  if (activeHls) activeHls.destroy();
  activeHls = new Hls({ lowLatencyMode: false, backBufferLength: 90 });
  activeHls.loadSource(session.playlist_url);
  activeHls.attachMedia(video);
  activeHls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal) {
      transcodeMessage.value = `HLS playback error: ${data.details || data.type}. Reverting to Original/direct.`;
      void stopActiveTranscode();
    }
  });
  await video.play().catch(() => undefined);
}

function terminalJobStatus(status: string): boolean {
  return ["succeeded", "failed", "canceled", "cancelled"].includes(status);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
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
    await refreshAuth();
    const [
      explorerRows,
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
      tileSourceRows,
      ai,
      vector,
      settingsPayload,
      exportRows
    ] = await Promise.all([
      api.explorer(explorerQueryString()),
      api.jobs(),
      api.jobStats(),
      api.storages(),
      api.plugins(),
      api.stats(),
      api.duplicates(),
      api.assetMonths(),
      api.status(),
      api.gpsTracks(),
      api.map(mapQuery()),
      api.mapStatus(),
      api.albums(),
      api.previewStatus(),
      api.previewCache(),
      api.transcodingCapabilities(),
      api.tileSources(),
      api.aiStatus(),
      api.vectorStatus(),
      api.settings(),
      api.dbExports()
    ]);
    rows.value = asArray(explorerRows);
    explorer.value = await api.explorerFolders(explorerPath.value, explorerQueryString());
    jobs.value = asArray(jobRows);
    jobStats.value = jobStatData;
    storages.value = asArray(storageRows);
    plugins.value = asArray(pluginRows);
    stats.value = statData;
    duplicatePage.value = duplicateData;
    backendMonthBuckets.value = asArray(monthData);
    backend.value = backendStatus;
    tracks.value = asArray(trackRows);
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
    aiStatus.value = ai;
    vectorStatus.value = vector;
    settings.value = settingsPayload;
    initializePendingConfig(settingsPayload);
    dbExports.value = asArray(exportRows);
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
  } catch {
    principal.value = null;
  }
}

async function login() {
  error.value = "";
  try {
    const result = await api.login(loginEmail.value, loginPassword.value);
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
  return Math.min(Math.max(dryRunMaxFiles.value, 1), 50);
}

async function refreshIndexingStatus() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (!storage || prefixes.length === 0) return;
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
    explorerPath.value = prefixes[0] ?? explorerPath.value;
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
    max_files: Math.min(Math.max(dryRunMaxFiles.value, 1), 50)
  });
  await refresh();
}

async function startMetadata() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (storage && prefixes.length > 0) {
    lastMetadataJob.value = await api.startMetadataScoped({ storage, prefixes, max_files: jobMaxFiles.value, only_missing: false });
  } else {
    lastMetadataJob.value = await api.startMetadata(jobMaxFiles.value);
  }
  await refresh();
}

async function parseTrackFilesForCurrentPrefix() {
  const storage = pipelineStorage();
  const prefixes = adapterRelativePrefixes();
  if (!storage || prefixes.length === 0) {
    error.value = "Enter a storage-relative track prefix before parsing GPS/KML/KMZ/GPZ files.";
    return;
  }
  error.value = "";
  pipelineLog.value = [`Parsing track files under ${prefixes.join(", ")}`].concat(pipelineLog.value).slice(0, 12);
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
  if (storage && prefixes.length > 0) {
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
    max_files: dryRunMaxFiles.value,
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
  mapTrackId.value = id;
  trackAssets.value = [];
  setActive("GPS/KML Tracks");
}

async function findTrackAssets(id: string) {
  const result = await api.gpsTrackAssets(id, trackOffsetSeconds.value);
  trackAssets.value = asArray(result.assets);
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
  setActive("Explorer");
  await refresh();
}

async function openAsset(id: string) {
  loading.value = true;
  error.value = "";
  try {
    assetDetail.value = await api.asset(id);
    streamOptions.value =
      assetDetail.value.asset.media_kind === "video" ? await api.streamOptions(id).catch(() => null) : null;
    setActive("Asset Detail", false);
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

async function openGalleryAssetDetail(id: string) {
  closeGallery();
  await openAsset(id);
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

const mapFeatures = computed(() => {
  const features = mapData.value?.features;
  return Array.isArray(features) ? (features as Array<Record<string, unknown>>) : [];
});

const mapWarnings = computed(() => asArray(mapStatus.value?.warnings as string[] | null | undefined));
const selectedPipelineStorage = computed(() => storages.value.find((storage) => storage.name === pipelineStorage()) ?? storages.value[0]);
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
  mapPopup.value = null;
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

function maybeZoomCluster(feature: { get: (name: string) => unknown }) {
  const bbox = (feature.get("bbox") ?? {}) as Record<string, number>;
  const centroid = (feature.get("centroid") ?? {}) as Record<string, number>;
  const minLon = Number(bbox.min_lon);
  const minLat = Number(bbox.min_lat);
  const maxLon = Number(bbox.max_lon);
  const maxLat = Number(bbox.max_lat);
  const hasArea = Number.isFinite(minLon) && Number.isFinite(maxLon) && Number.isFinite(minLat) && Number.isFinite(maxLat)
    && (Math.abs(maxLon - minLon) > 0.00001 || Math.abs(maxLat - minLat) > 0.00001);
  if (!olMap || !hasArea) return;
  const zoom = olMap.getView().getZoom() ?? 10;
  const centerLon = Number.isFinite(Number(centroid.lon)) ? Number(centroid.lon) : (minLon + maxLon) / 2;
  const centerLat = Number.isFinite(Number(centroid.lat)) ? Number(centroid.lat) : (minLat + maxLat) / 2;
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
    maybeZoomCluster(feature);
    return;
  }
  if (kind === "track") {
    const id = String(feature.get("id") ?? "");
    if (id) void openTrack(id);
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
    const tileLayer = new TileLayer({
      source: new XYZ({
        url: "/api/v1/tiles/osm/{z}/{x}/{y}.png",
        maxZoom: 19,
        attributions: "© OpenStreetMap contributors"
      })
    });
    tileLayer.getSource()?.on("tileloaderror", () => {
      tileStatus.value = "Base tiles are unavailable right now; vector asset and track layers remain active.";
    });
    const vectorLayer = new VectorLayer({
      source: mapSource,
      style: (feature) => {
        const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
        if (kind === "track") {
          return new Style({ stroke: new Stroke({ color: "#1a7f37", width: 3 }) });
        }
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
    olMap = new OLMap({
      target: mapElement.value,
      layers: [tileLayer, vectorLayer],
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
      olMap?.forEachFeatureAtPixel(event.pixel, (feature) => {
        openMapPopup(feature, event.coordinate);
      });
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
  mapSource.clear();
  mapSource.addFeatures(features);
  if (features.length > 0 && !mapHasInitialFit) {
    const extent = mapSource.getExtent();
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
});

watch([mapMediaKind, mapAlbumId, mapTrackId, mapCluster], () => {
  if (active.value === "Map") {
    void refreshMap();
  }
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

onMounted(async () => {
  window.addEventListener("keydown", handleKeydown);
  await refresh();
  await nextTick();
  renderOpenLayers();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleKeydown);
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
          {{ item }}
        </button>
      </nav>

      <section class="content">
        <div v-if="error" class="alert">{{ error }}</div>
        <div v-if="backend?.auth?.warning" class="alert">{{ backend.auth.warning }}</div>
        <div v-if="loading" class="muted">Loading...</div>

        <section v-if="active === 'Explorer'" class="panel">
          <header class="panel-head">
            <h2>Explorer</h2>
            <div class="actions">
              <span>{{ explorer?.folder_count ?? 0 }} folders · {{ visibleExplorerRows.length }} files</span>
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
          <div class="month-strip" v-if="monthBuckets.length > 0">
            <button type="button" :class="{ active: monthFilter === '' }" @click="monthFilter = ''">All months</button>
            <button
              v-for="bucket in monthBuckets"
              :key="bucket.month"
              type="button"
              :class="{ active: monthFilter === bucket.month }"
              @click="monthFilter = bucket.month"
            >
              {{ bucket.month }} · {{ bucket.count }}
            </button>
          </div>
          <p v-if="rows.length === 0 && asArray(explorer?.folders).length === 0" class="empty-state">No assets indexed yet.</p>
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
                  <button type="button" class="link-button" @click="openAsset(row.asset_id)">
                    {{ row.name }}
                  </button>
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
                  v-if="row.media_kind === 'photo' && !failedPreviewIds.has(row.asset_id)"
                  :src="`/api/v1/media/${row.asset_id}/preview`"
                  alt=""
                  loading="lazy"
                  @error="markPreviewFailed(row.asset_id)"
                />
                <span v-else class="media-fallback">{{ row.media_kind }}</span>
              </button>
              <button type="button" class="tile-title link-button" @click="openAsset(row.asset_id)">{{ row.name }}</button>
              <small>{{ row.media_kind }} · {{ formatBytes(row.size_bytes) }}</small>
              <small class="tile-badges">
                <span :class="['status-badge', row.hash_status === 'hashed' ? 'ok' : 'warn']">{{ row.hash_status }}</span>
                <span v-if="shortHash(row.sha512_hex)" class="status-badge">{{ shortHash(row.sha512_hex) }}</span>
              </small>
              <small>{{ row.mtime }}</small>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Discovery'" class="panel">
          <header class="panel-head">
            <h2>Discovery</h2>
            <div class="actions">
              <button type="button" :disabled="pipelineRunning" @click="startIndexingPipeline">Start indexing pipeline</button>
              <button type="button" :disabled="!pipelineRunning" @click="stopIndexingPipeline">Stop current pipeline</button>
              <label class="inline-label">
                Job max files
                <input v-model.number="jobMaxFiles" type="number" min="1" max="50" />
              </label>
            </div>
          </header>
          <div class="safety-card">
            <strong>{{ selectedPipelineStorage?.name ?? "No storage selected" }}</strong>
            <span>{{ selectedPipelineStorage?.root ?? "unknown root" }}</span>
            <span>{{ selectedPipelineStorage?.mode ?? "unknown mode" }}</span>
          </div>
          <form class="control-grid pipeline-settings" @submit.prevent="startDryRun">
            <h3>Shared scan scope and pipeline stages</h3>
            <p class="form-note">These settings apply to both Preview scan report and Start indexing pipeline.</p>
            <label>
              Storage
              <select v-model="dryRunStorage">
                <option value="">Configured default</option>
                <option v-for="storage in storages" :key="storage.name" :value="storage.name">
                  {{ storage.name }} · {{ storage.root }} · {{ storage.mode }}
                </option>
              </select>
            </label>
            <label>
              Scan subpath
              <input v-model="dryRunPrefix" type="text" />
            </label>
            <label>
              Max files
              <input v-model.number="dryRunMaxFiles" type="number" min="1" max="50" />
            </label>
            <label>
              Max bytes
              <input v-model.number="dryRunMaxBytes" type="number" min="1" />
            </label>
            <label>
              Include extensions
              <input v-model="dryRunExtensions" type="text" />
            </label>
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
            <p class="form-note">
              Prefix is storage-relative, for example <code>Cartolensia-photos</code>. Missing marking is disabled.
            </p>
            <div class="actions">
              <button type="submit">Preview scan report (does not index)</button>
              <button type="button" :disabled="pipelineRunning" @click="startIndexingPipeline">Start indexing pipeline</button>
              <button type="button" :disabled="!pipelineRunning" @click="stopIndexingPipeline">Stop current pipeline</button>
            </div>
          </form>
          <div class="metrics">
            <article><strong>{{ indexingStatus?.scope.assets ?? stats?.assets ?? 0 }}</strong><span>Scoped assets</span></article>
            <article><strong>{{ indexingStatus?.scope.hashed ?? stats?.hashed ?? 0 }}</strong><span>Hashed</span></article>
            <article><strong>{{ indexingStatus?.scope.unhashed ?? stats?.unhashed ?? 0 }}</strong><span>Unhashed</span></article>
            <article><strong>{{ indexingStatus?.scope.geotagged ?? 0 }}</strong><span>Geotagged</span></article>
            <article><strong>{{ indexingStatus?.scope.preview_ready ?? previewCacheStats?.ready ?? 0 }}</strong><span>Previews ready</span></article>
            <article><strong>{{ indexingStatus?.scope.tracks ?? tracks.length }}</strong><span>GPS/KML tracks</span></article>
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
              <pre class="logbox">{{ JSON.stringify(selectedJob.logs ?? [], null, 2) }}</pre>
            </article>
            <article v-for="job in jobs" :key="job.id" class="job">
              <div class="job-row">
                <button type="button" class="link-button" @click="openJob(job.id)">{{ job.kind }}</button>
                <div class="actions">
                  <button v-if="canRetry(job)" type="button" @click="retryJob(job.id)">Retry</button>
                  <button v-if="canCancel(job)" type="button" @click="cancelJob(job.id)">Cancel</button>
                </div>
              </div>
              <progress :value="job.progress_current" :max="job.progress_total ?? 100"></progress>
              <span>{{ job.status }} · {{ job.progress_current }} / {{ job.progress_total ?? "?" }}</span>
              <small>{{ job.error }}</small>
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
                <input v-model.number="jobMaxFiles" type="number" min="1" max="50" />
              </label>
            </div>
          </header>
          <p class="muted">
            Metadata and preview jobs are bounded by this max-files value and write only to metadata/cache, never originals.
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
            <img
              v-if="assetDetail.asset.media_kind === 'photo'"
              :src="assetDetail.preview_url || assetDetail.original_url"
              alt=""
              @error="markPreviewFailed(assetDetail.asset.id)"
            />
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
          </div>
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
            <thead><tr><th>Name</th><th>Kind</th><th>Mode</th><th>Root</th></tr></thead>
            <tbody>
              <tr v-for="storage in storages" :key="storage.name">
                <td>{{ storage.name }}</td>
                <td>{{ storage.kind }}</td>
                <td>{{ storage.mode }}</td>
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
                    <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                  </button>
                  <button type="button" class="tile-title link-button" @click="openAsset(asset.id)">
                    {{ asset.display_name }}
                  </button>
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
              <span>{{ tracks.length }} tracks</span>
              <button type="button" @click="trackMediaViewMode = trackMediaViewMode === 'table' ? 'tile' : 'table'">
                {{ trackMediaViewMode === "table" ? "Tile media" : "Table media" }}
              </button>
            </div>
          </header>
          <p v-if="tracks.length === 0" class="empty-state">
            <span v-if="(stats?.tracks ?? 0) > 0">
              {{ stats?.tracks }} track-like assets are indexed, but no parsed GPS/KML/KMZ/GPZ summaries exist yet.
            </span>
            <span v-else>No GPS/KML track files in this subset.</span>
            <button type="button" @click="parseTrackFilesForCurrentPrefix">Parse track files for current prefix</button>
            <small>Current prefix: {{ adapterRelativePrefixes().join(", ") || "not set" }}</small>
          </p>
          <div v-if="selectedTrack" class="track-detail">
            <div class="metrics">
              <article><strong>{{ selectedTrack.summary.point_count }}</strong><span>Points</span></article>
              <article><strong>{{ selectedTrack.summary.distance_m ? `${(selectedTrack.summary.distance_m / 1000).toFixed(2)} km` : "" }}</strong><span>Distance</span></article>
              <article><strong>{{ selectedTrack.summary.duration_seconds ? `${Math.round(selectedTrack.summary.duration_seconds / 60)} min` : "" }}</strong><span>Duration</span></article>
            </div>
            <div class="actions">
              <label class="inline-label">
                Offset seconds
                <input v-model.number="trackOffsetSeconds" type="number" />
              </label>
              <button type="button" @click="findTrackAssets(selectedTrack.summary.track_asset_id)">Find media</button>
              <button type="button" @click="snapTrackMedia(selectedTrack.summary.track_asset_id)">Snap media</button>
              <button type="button" @click="setActive('Map')">Show on map</button>
            </div>
            <p v-if="trackAssets.length === 0" class="empty-state">No media results loaded for this track yet.</p>
            <table v-if="trackAssets.length > 0 && trackMediaViewMode === 'table'">
              <thead><tr><th>Asset</th><th>Kind</th><th>Taken</th></tr></thead>
              <tbody>
                <tr v-for="asset in trackAssets" :key="asset.id">
                  <td><button type="button" class="link-button" @click="openAsset(asset.id)">{{ asset.display_name }}</button></td>
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
                  <span v-else class="media-fallback">{{ asset.media_kind }}</span>
                </button>
                <button type="button" class="tile-title link-button" @click="openAsset(asset.id)">
                  {{ asset.display_name }}
                </button>
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
          <div ref="mapElement" class="ol-map" role="img" aria-label="OpenLayers map"></div>
          <div v-show="mapPopup" ref="mapPopupElement" class="map-popup">
            <div class="job-row">
              <strong>{{ mapPopup?.title }}</strong>
              <button type="button" @click="mapPopup = null">Close</button>
            </div>
            <p>{{ mapPopup?.summary }}</p>
            <p v-if="mapPopup?.kind === 'cluster' && mapPopup.count && mapPopup.assets.length < mapPopup.count" class="muted">
              Zooming in tries to split this cluster. If points share identical coordinates, use the gallery below.
            </p>
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
                  <button type="button" @click="openAsset(asset.id)">Asset detail</button>
                </div>
              </article>
            </div>
          </div>
          <p class="muted" v-if="tileSources.length > 0">
            Tile source: {{ tileSources[0].name }} · {{ tileSources[0].policy }}
          </p>
          <pre class="geojson">{{ JSON.stringify(mapData, null, 2) }}</pre>
        </section>

        <section v-else-if="active === 'Transcoding'" class="panel">
          <header class="panel-head">
            <h2>Transcoding</h2>
            <span>{{ transcodingCapabilities?.ffmpeg.available ? "ffmpeg detected" : "ffmpeg unavailable" }}</span>
          </header>
          <div class="metrics">
            <article><strong>{{ transcodingCapabilities?.ffmpeg.available ? "yes" : "no" }}</strong><span>ffmpeg</span></article>
            <article><strong>{{ transcodingCapabilities?.ffprobe.available ? "yes" : "no" }}</strong><span>ffprobe</span></article>
            <article><strong>{{ transcodingCapabilities?.encoders.length ?? 0 }}</strong><span>Video encoders</span></article>
            <article><strong>immutable</strong><span>Originals</span></article>
          </div>
          <table>
            <thead><tr><th>Encoder</th><th>Codec</th><th>Hardware</th></tr></thead>
            <tbody>
              <tr v-for="encoder in transcodingCapabilities?.encoders ?? []" :key="encoder.name">
                <td>{{ encoder.name }}</td>
                <td>{{ encoder.codec_family }}</td>
                <td>{{ encoder.hardware }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else-if="active === 'Base AI'" class="panel">
          <header class="panel-head">
            <h2>Base AI</h2>
            <span>Inference is disabled until an AI sidecar/vector store is configured.</span>
          </header>
          <div class="metrics">
            <article><strong>{{ aiStatus?.accelerators ? "detected" : "basic" }}</strong><span>Accelerators</span></article>
            <article><strong>{{ vectorStatus?.backend ?? "not configured" }}</strong><span>Vector store</span></article>
            <article><strong>{{ vectorStatus?.pgvector_available ? "yes" : "optional" }}</strong><span>pgvector</span></article>
            <article><strong>disabled</strong><span>Model downloads</span></article>
          </div>
          <div class="settings-grid">
            <article class="settings-form">
              <h3>Planned Workers</h3>
              <p>ai-cpu · ai-nvidia · ai-rocm · ai-intel</p>
              <p class="muted">Workers are future sidecars. Cartolensia will not download models or run inference in this mode.</p>
            </article>
            <article class="settings-form">
              <h3>Modalities</h3>
              <p>image · video_frame · audio_segment · text_query</p>
              <button disabled type="button">Configure vector store</button>
              <button disabled type="button">Run embedding job</button>
            </article>
          </div>
          <details>
            <summary>Raw AI status</summary>
            <pre class="geojson">{{ JSON.stringify({ aiStatus, vectorStatus }, null, 2) }}</pre>
          </details>
        </section>

        <section v-else-if="active === 'Settings'" class="panel">
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

          <div v-else-if="settingsTab === 'indexing' || settingsTab === 'metadata' || settingsTab === 'preview' || settingsTab === 'map' || settingsTab === 'transcoding'" class="settings-grid">
            <article class="settings-form settings-wide">
              <h3>Runtime Settings</h3>
              <p class="muted">These preferences are accepted by the runtime settings endpoint. Existing long-running jobs keep their current payloads.</p>
              <table>
                <thead><tr><th>Key</th><th>Value</th></tr></thead>
                <tbody>
                  <tr v-for="(value, key) in settings?.runtime_settings ?? {}" :key="key">
                    <td>{{ key }}</td>
                    <td>
                      <select v-if="typeof value === 'boolean'" :value="String(value)" @change="setRuntimeSetting(String(key), ($event.target as HTMLSelectElement).value)">
                        <option value="true">true</option>
                        <option value="false">false</option>
                      </select>
                      <input v-else :value="String(value)" type="text" @input="setRuntimeSetting(String(key), ($event.target as HTMLInputElement).value)" />
                    </td>
                  </tr>
                </tbody>
              </table>
              <button type="button" @click="saveRuntimeSettings">Save runtime settings</button>
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
              <h3>Primary Storage Draft</h3>
              <label>Name <input :value="pendingStorageField(0, 'name')" type="text" @input="setPendingStorageField(0, 'name', ($event.target as HTMLInputElement).value)" /></label>
              <label>Root <input :value="pendingStorageField(0, 'root')" type="text" @input="setPendingStorageField(0, 'root', ($event.target as HTMLInputElement).value)" /></label>
              <label>Mode
                <select :value="pendingStorageField(0, 'mode')" @change="setPendingStorageField(0, 'mode', ($event.target as HTMLSelectElement).value)">
                  <option value="strict_read_only">strict_read_only</option>
                  <option value="read_only">read_only</option>
                </select>
              </label>
              <p class="muted">Active storage roots are not changed until restart. Real-peek must remain strict_read_only.</p>
              <button type="button" @click="savePendingSettings">Save pending YAML</button>
            </article>
            <article class="settings-form settings-wide">
              <h3>Configured Storages</h3>
              <table>
                <thead><tr><th>Name</th><th>Root</th><th>Mode</th></tr></thead>
                <tbody>
                  <tr v-for="storage in storages" :key="storage.name">
                    <td>{{ storage.name }}</td>
                    <td>{{ storage.root }}</td>
                    <td>{{ storage.mode }}</td>
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
            <article class="settings-form">
              <h3>AI / Vector</h3>
              <label>Vector backend <input :value="pendingValue('ai.vector_backend', 'disabled')" type="text" @input="setPendingValue('ai.vector_backend', ($event.target as HTMLInputElement).value)" /></label>
              <label>Worker endpoint <input :value="pendingValue('ai.worker_endpoint')" type="text" @input="setPendingValue('ai.worker_endpoint', ($event.target as HTMLInputElement).value)" /></label>
              <p class="muted">AI execution remains disabled until sidecar workers and provenance policy are configured.</p>
              <button type="button" @click="savePendingSettings">Save pending YAML</button>
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
            <article v-for="plugin in plugins" :key="plugin.id" class="settings-form">
              <h3>{{ plugin.name }}</h3>
              <p>{{ plugin.id }} · {{ plugin.status }} · {{ plugin.runtime }}</p>
              <textarea
                :value="pluginSettingText[plugin.id] ?? '{}'"
                rows="6"
                @input="pluginSettingText[plugin.id] = ($event.target as HTMLTextAreaElement).value"
              ></textarea>
              <button type="button" @click="savePluginSettings(plugin.id)">Save plugin settings</button>
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
      <button type="button" class="gallery-close" aria-label="Close viewer" @click="closeGallery">Close</button>
      <button type="button" class="gallery-nav gallery-prev" aria-label="Previous asset" @click="nextGallery(-1)">‹</button>
      <figure class="gallery-stage">
        <img
          v-if="galleryCurrent.media_kind === 'photo'"
          :class="{ actual: galleryZoomMode === 'actual' }"
          :style="galleryImageStyle()"
          :src="galleryCurrent.original_url"
          :alt="galleryCurrent.name"
          @wheel.prevent="wheelGallery"
          @dblclick="toggleGalleryZoom"
        />
        <video
          v-else-if="galleryCurrent.media_kind === 'video'"
          :key="videoSource(galleryCurrent.id, galleryCurrent.original_url)"
          ref="galleryVideoElement"
          :src="videoSource(galleryCurrent.id, galleryCurrent.original_url)"
          controls
          preload="metadata"
        ></video>
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
          </div>
          <div class="actions">
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="toggleGalleryZoom">
              {{ galleryZoomMode === "fit" ? "100% zoom" : "Fit to screen" }}
            </button>
            <button v-if="galleryCurrent.media_kind === 'photo'" type="button" @click="resetGalleryZoom">Reset zoom</button>
            <span v-if="galleryCurrent.media_kind === 'photo'">{{ Math.round(galleryScale * 100) }}%</span>
            <a :href="galleryCurrent.original_url" target="_blank" rel="noreferrer">Open Original</a>
            <button type="button" @click="openGalleryAssetDetail(galleryCurrent.id)">Asset Detail</button>
          </div>
        </figcaption>
      </figure>
      <button type="button" class="gallery-nav gallery-next" aria-label="Next asset" @click="nextGallery(1)">›</button>
    </div>
  </main>
</template>
