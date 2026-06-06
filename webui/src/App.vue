<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  api,
  type APIToken,
  type AssetDetail,
  type BackendStatus,
  type ExplorerRow,
  type ExplorerView,
  type Job,
  type JobStats,
  type PluginManifest,
  type Principal,
  type Stats,
  type StorageConfig,
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
const mapData = ref<Record<string, unknown> | null>(null);
const transcodingCapabilities = ref<TranscodingCapabilities | null>(null);
const aiStatus = ref<Record<string, unknown> | null>(null);
const vectorStatus = ref<Record<string, unknown> | null>(null);

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
    const [explorerRows, jobRows, jobStatData, storageRows, pluginRows, statData, backendStatus, trackRows, geojson, transcodeCaps, ai, vector] = await Promise.all([
      api.explorer(),
      api.jobs(),
      api.jobStats(),
      api.storages(),
      api.plugins(),
      api.stats(),
      api.status(),
      api.tracks(),
      api.map(),
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

onMounted(refresh);
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
            <span>{{ explorer?.folder_count ?? 0 }} folders · {{ explorer?.file_count ?? rows.length }} files</span>
          </header>
          <div class="breadcrumbs">
            <button v-for="crumb in breadcrumbs" :key="crumb.path" type="button" @click="openFolder(crumb.path)">
              {{ crumb.name }}
            </button>
          </div>
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Size</th>
                <th>Hash</th>
                <th>Path</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="folder in explorer?.folders ?? []" :key="folder.path" class="folder-row">
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
            </div>
          </header>
          <div class="metrics">
            <article><strong>{{ stats?.photos ?? 0 }}</strong><span>Images</span></article>
            <article><strong>{{ stats?.videos ?? 0 }}</strong><span>Videos</span></article>
            <article><strong>{{ stats?.tracks ?? 0 }}</strong><span>Tracks</span></article>
            <article><strong>{{ backend?.preview_cache ?? "" }}</strong><span>Preview cache</span></article>
          </div>
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

        <section v-else-if="active === 'GPS Tracks'" class="panel">
          <header class="panel-head">
            <h2>GPS Tracks</h2>
            <span>{{ tracks.length }} tracks</span>
          </header>
          <table>
            <thead><tr><th>Name</th><th>Points</th><th>Start</th><th>End</th><th>Distance</th></tr></thead>
            <tbody>
              <tr v-for="track in tracks" :key="track.track_asset_id">
                <td>{{ track.name }}</td>
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
            <span>{{ mapFeatures.length }} features</span>
          </header>
          <svg class="map-canvas" viewBox="0 0 800 420" role="img" aria-label="Map features">
            <rect x="0" y="0" width="800" height="420" />
            <g v-for="(feature, index) in mapFeatures" :key="index">
              <circle
                v-if="(feature.geometry as any)?.type === 'Point'"
                :cx="(((feature.geometry as any).coordinates[0] - 43.8) / 0.5) * 800"
                :cy="420 - ((((feature.geometry as any).coordinates[1] - 39.8) / 0.5) * 420)"
                r="6"
              />
              <polyline
                v-else-if="(feature.geometry as any)?.type === 'LineString'"
                :points="(feature.geometry as any).coordinates.map((coord: number[]) => `${((coord[0] - 43.8) / 0.5) * 800},${420 - (((coord[1] - 39.8) / 0.5) * 420)}`).join(' ')"
              />
            </g>
          </svg>
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
