<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import Map from "ol/Map";
import View from "ol/View";
import GeoJSON from "ol/format/GeoJSON";
import VectorLayer from "ol/layer/Vector";
import VectorSource from "ol/source/Vector";
import { fromLonLat } from "ol/proj";
import { Circle as CircleStyle, Fill, Stroke, Style } from "ol/style";
import "ol/ol.css";
import {
  api,
  type Album,
  type AlbumItemPage,
  type APIToken,
  type AssetDetail,
  type BackendStatus,
  type ExplorerRow,
  type ExplorerView,
  type Job,
  type JobStats,
  type PluginManifest,
  type PreviewCacheEntry,
  type PreviewCacheStats,
  type Principal,
  type ScanRun,
  type Stats,
  type StorageConfig,
  type TrackDetail,
  type TrackSummary,
  type TranscodingCapabilities
} from "./api";

const nav = [
  "Explorer",
  "Discovery",
  "Jobs",
  "Metadata",
  "Storages",
  "Plugins",
  "Stats",
  "Settings",
  "Albums",
  "Map",
  "GPS Tracks",
  "Transcoding",
  "Base AI",
  "AI Classification"
];

const active = ref(localStorage.getItem("cartolensia.route") ?? "Explorer");
const loading = ref(false);
const error = ref("");
const rows = ref<ExplorerRow[]>([]);
const explorer = ref<ExplorerView | null>(null);
const explorerPath = ref("");
const assetDetail = ref<AssetDetail | null>(null);
const jobs = ref<Job[]>([]);
const jobStats = ref<JobStats | null>(null);
const selectedJob = ref<Job | null>(null);
const storages = ref<StorageConfig[]>([]);
const plugins = ref<PluginManifest[]>([]);
const stats = ref<Stats | null>(null);
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
const tracks = ref<TrackSummary[]>([]);
const selectedTrack = ref<TrackDetail | null>(null);
const trackAssets = ref<AssetDetail["asset"][]>([]);
const trackOffsetSeconds = ref(0);
const mapData = ref<Record<string, unknown> | null>(null);
const mapStatus = ref<Record<string, unknown> | null>(null);
const mapMediaKind = ref("");
const mapCluster = ref(true);
const mapAlbumId = ref("");
const mapTrackId = ref("");
const mapElement = ref<HTMLDivElement | null>(null);
let olMap: Map | null = null;
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
const previewCacheStats = ref<PreviewCacheStats | null>(null);
const previewCache = ref<PreviewCacheEntry[]>([]);
const dryRunStorage = ref("");
const dryRunPrefix = ref("photos");
const dryRunMaxFiles = ref(50);
const dryRunMaxBytes = ref(2147483648);
const dryRunExtensions = ref("jpg,jpeg,png,gpx");
const dryRunReport = ref<ScanRun | null>(null);

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

function setActive(next: string) {
  active.value = next;
  localStorage.setItem("cartolensia.route", next);
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
      backendStatus,
      trackRows,
      geojson,
      mapStatusData,
      albumRows,
      previewStatus,
      previewEntries,
      transcodeCaps,
      ai,
      vector
    ] = await Promise.all([
      api.explorer(),
      api.jobs(),
      api.jobStats(),
      api.storages(),
      api.plugins(),
      api.stats(),
      api.status(),
      api.gpsTracks(),
      api.map(mapQuery()),
      api.mapStatus(),
      api.albums(),
      api.previewStatus(),
      api.previewCache(),
      api.transcodingCapabilities(),
      api.aiStatus(),
      api.vectorStatus()
    ]);
    rows.value = explorerRows;
    explorer.value = await api.explorerFolders(explorerPath.value);
    jobs.value = jobRows;
    jobStats.value = jobStatData;
    storages.value = storageRows;
    plugins.value = pluginRows;
    stats.value = statData;
    backend.value = backendStatus;
    tracks.value = trackRows;
    mapData.value = geojson;
    mapStatus.value = mapStatusData;
    albums.value = albumRows;
    previewCacheStats.value = previewStatus.stats;
    previewCache.value = previewEntries;
    if (!selectedAlbumId.value && albumRows.length > 0) {
      selectedAlbumId.value = albumRows[0].id;
    }
    if (selectedAlbumId.value) {
      albumItems.value = await api.albumItems(selectedAlbumId.value).catch(() => null);
    }
    transcodingCapabilities.value = transcodeCaps;
    aiStatus.value = ai;
    vectorStatus.value = vector;
    if (principal.value && backendStatus.auth?.mode === "local") {
      apiTokens.value = await api.tokens().catch(() => []);
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
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
  await api.startDiscovery();
  await refresh();
}

async function startHash() {
  await api.startHash();
  await refresh();
}

async function startMetadata() {
  await api.startMetadata();
  await refresh();
}

async function startPreviews() {
  await api.startPreviews();
  await refresh();
}

async function cleanupPreviews(dryRun = true) {
  await api.previewCleanup(dryRun);
  await refresh();
}

async function startDryRun() {
  const storage = dryRunStorage.value || storages.value[0]?.name || "";
  const prefixes = dryRunPrefix.value.split(",").map((item) => item.trim()).filter(Boolean);
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
  newAlbumTitle.value = "";
  newAlbumDescription.value = "";
  await refresh();
}

async function selectAlbum(id: string) {
  selectedAlbumId.value = id;
  mapAlbumId.value = id;
  albumItems.value = await api.albumItems(id);
}

function toggleAssetSelection(id: string) {
  const next = new Set(selectedAssets.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selectedAssets.value = next;
}

async function addSelectedToAlbum() {
  if (!selectedAlbumId.value || selectedAssets.value.size === 0) return;
  await api.addAlbumItems(selectedAlbumId.value, Array.from(selectedAssets.value));
  selectedAssets.value = new Set();
  await selectAlbum(selectedAlbumId.value);
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
  setActive("GPS Tracks");
}

async function findTrackAssets(id: string) {
  const result = await api.gpsTrackAssets(id, trackOffsetSeconds.value);
  trackAssets.value = result.assets;
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

async function openFolder(path: string) {
  explorerPath.value = path;
  await refresh();
}

async function openAsset(id: string) {
  loading.value = true;
  error.value = "";
  try {
    assetDetail.value = await api.asset(id);
    setActive("Asset Detail");
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
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

function mapQuery(): Record<string, string | number | boolean> {
  const query: Record<string, string | number | boolean> = { cluster: mapCluster.value };
  if (mapMediaKind.value) query.media_kind = mapMediaKind.value;
  if (mapAlbumId.value) query.album_id = mapAlbumId.value;
  if (mapTrackId.value) query.track_id = mapTrackId.value;
  return query;
}

async function refreshMap() {
  mapData.value = await api.map(mapQuery());
  await nextTick();
  renderOpenLayers();
}

function ensureOpenLayersMap() {
  if (!mapElement.value) return;
  if (!olMap) {
    const vectorLayer = new VectorLayer({
      source: mapSource,
      style: (feature) => {
        const kind = String(feature.get("kind") ?? feature.get("asset_type") ?? "");
        if (kind === "track") {
          return new Style({ stroke: new Stroke({ color: "#1a7f37", width: 3 }) });
        }
        if (kind === "cluster") {
          return new Style({
            image: new CircleStyle({
              radius: 12,
              fill: new Fill({ color: "#bf8700" }),
              stroke: new Stroke({ color: "#ffffff", width: 2 })
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
    olMap = new Map({
      target: mapElement.value,
      layers: [vectorLayer],
      view: new View({ center: fromLonLat([44.05, 40.05]), zoom: 9 })
    });
    olMap.on("singleclick", (event) => {
      olMap?.forEachFeatureAtPixel(event.pixel, (feature) => {
        const id = feature.get("id");
        const kind = feature.get("kind");
        if (typeof id === "string" && kind !== "track" && kind !== "cluster") void openAsset(id);
        if (typeof id === "string" && kind === "track") void openTrack(id);
      });
    });
  } else {
    olMap.setTarget(mapElement.value);
  }
}

function renderOpenLayers() {
  ensureOpenLayersMap();
  if (!olMap || !mapData.value) return;
  const features = new GeoJSON().readFeatures(mapData.value, { featureProjection: "EPSG:3857" });
  mapSource.clear();
  mapSource.addFeatures(features);
  if (features.length > 0) {
    const extent = mapSource.getExtent();
    if (extent) {
      olMap.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom: 14, duration: 150 });
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
  await refresh();
  await nextTick();
  renderOpenLayers();
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
              <span>{{ explorer?.folder_count ?? 0 }} folders · {{ explorer?.file_count ?? rows.length }} files</span>
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
          <table>
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
              <tr v-for="row in explorer?.files ?? rows" :key="row.asset_id">
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
        </section>

        <section v-else-if="active === 'Discovery'" class="panel">
          <header class="panel-head">
            <h2>Discovery</h2>
            <div class="actions">
              <button type="button" @click="startDiscovery">Scan Fixture</button>
              <button type="button" @click="startHash">Hash Unhashed</button>
              <button type="button" @click="startMetadata">Enrich Metadata</button>
              <button type="button" @click="startPreviews">Generate Previews</button>
            </div>
          </header>
          <form class="control-grid" @submit.prevent="startDryRun">
            <label>
              Storage
              <select v-model="dryRunStorage">
                <option value="">Default storage</option>
                <option v-for="storage in storages" :key="storage.name" :value="storage.name">
                  {{ storage.name }} · {{ storage.mode }}
                </option>
              </select>
            </label>
            <label>
              Prefixes
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
            <button type="submit">Start Dry Run</button>
          </form>
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
            </div>
          </header>
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
            <button type="button" @click="setActive('Explorer')">Back</button>
          </header>
          <div v-if="assetDetail" class="detail-grid">
            <article class="preview-tile">
              <img v-if="assetDetail.preview_url && assetDetail.preview.status !== 'unsupported'" :src="assetDetail.preview_url" alt="" />
              <span v-else>{{ assetDetail.preview.status }}</span>
            </article>
            <article>
              <strong>Media</strong>
              <span>{{ assetDetail.asset.media_kind }}</span>
            </article>
            <article>
              <strong>Hash</strong>
              <span>{{ String(assetDetail.content.hash_status ?? "unknown") }}</span>
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
            <article><strong>{{ formatBytes(stats.total_bytes) }}</strong><span>Total</span></article>
          </div>
        </section>

        <section v-else-if="active === 'Albums'" class="panel">
          <header class="panel-head">
            <h2>Albums</h2>
            <button v-if="selectedAlbumId" type="button" @click="setActive('Map')">Show on map</button>
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
              <table>
                <thead><tr><th>Name</th><th>Kind</th><th>Size</th><th>Actions</th></tr></thead>
                <tbody>
                  <tr v-for="item in albumItems?.items ?? []" :key="item.asset.id">
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
            </section>
          </div>
        </section>

        <section v-else-if="active === 'GPS Tracks'" class="panel">
          <header class="panel-head">
            <h2>GPS Tracks</h2>
            <span>{{ tracks.length }} tracks</span>
          </header>
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
            <table v-if="trackAssets.length > 0">
              <thead><tr><th>Asset</th><th>Kind</th><th>Taken</th></tr></thead>
              <tbody>
                <tr v-for="asset in trackAssets" :key="asset.id">
                  <td><button type="button" class="link-button" @click="openAsset(asset.id)">{{ asset.display_name }}</button></td>
                  <td>{{ asset.media_kind }}</td>
                  <td>{{ asset.taken_at ?? "" }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <table>
            <thead><tr><th>Name</th><th>Points</th><th>Start</th><th>End</th><th>Distance</th></tr></thead>
            <tbody>
              <tr v-for="track in tracks" :key="track.track_asset_id">
                <td><button type="button" class="link-button" @click="openTrack(track.track_asset_id)">{{ track.name }}</button></td>
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
              <span>{{ mapFeatures.length }} features · {{ mapStatus?.base_tiles ?? "no tiles" }}</span>
              <button type="button" @click="refreshMap">Apply</button>
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
          <div ref="mapElement" class="ol-map" role="img" aria-label="OpenLayers map"></div>
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
          <header class="panel-head"><h2>Base AI</h2></header>
          <pre class="geojson">{{ JSON.stringify({ aiStatus, vectorStatus }, null, 2) }}</pre>
        </section>

        <section v-else-if="active === 'Settings'" class="panel">
          <header class="panel-head"><h2>Settings</h2></header>
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
  </main>
</template>
