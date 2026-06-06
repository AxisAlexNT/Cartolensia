<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  api,
  type AssetDetail,
  type BackendStatus,
  type ExplorerRow,
  type ExplorerView,
  type Job,
  type PluginManifest,
  type Stats,
  type StorageConfig
} from "./api";

const nav = [
  "Explorer",
  "Discovery",
  "Storages",
  "Plugins",
  "Stats",
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
const storages = ref<StorageConfig[]>([]);
const plugins = ref<PluginManifest[]>([]);
const stats = ref<Stats | null>(null);
const backend = ref<BackendStatus | null>(null);

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
    const [explorerRows, jobRows, storageRows, pluginRows, statData, backendStatus] = await Promise.all([
      api.explorer(),
      api.jobs(),
      api.storages(),
      api.plugins(),
      api.stats(),
      api.status()
    ]);
    rows.value = explorerRows;
    explorer.value = await api.explorerFolders(explorerPath.value);
    jobs.value = jobRows;
    storages.value = storageRows;
    plugins.value = pluginRows;
    stats.value = statData;
    backend.value = backendStatus;
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
}

async function startDiscovery() {
  await api.startDiscovery();
  await refresh();
}

async function startHash() {
  await api.startHash();
  await refresh();
}

async function cancelJob(id: string) {
  await api.cancelJob(id);
  await refresh();
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
        <span>{{ backend?.store_backend ?? "starting" }} store</span>
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
            </div>
          </header>
          <div class="job-list">
            <article v-for="job in jobs" :key="job.id" class="job">
              <div class="job-row">
                <strong>{{ job.kind }}</strong>
                <button v-if="canCancel(job)" type="button" @click="cancelJob(job.id)">Cancel</button>
              </div>
              <span>{{ job.status }} · attempt {{ job.attempts ?? 0 }} / {{ job.max_attempts ?? 0 }}</span>
              <span>{{ job.progress_current }} / {{ job.progress_total ?? "?" }}</span>
              <small>{{ job.logs?.at(-1)?.message ?? job.error }}</small>
            </article>
          </div>
        </section>

        <section v-else-if="active === 'Asset Detail'" class="panel">
          <header class="panel-head">
            <h2>{{ assetDetail?.asset.display_name ?? "Asset" }}</h2>
            <button type="button" @click="setActive('Explorer')">Back</button>
          </header>
          <div v-if="assetDetail" class="detail-grid">
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
              <span>{{ plugin.id }} · {{ plugin.status }}</span>
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

        <section v-else class="panel">
          <header class="panel-head"><h2>{{ active }}</h2></header>
          <p>{{ activePlugin?.description ?? "Plugin surface reserved for the next MVP phase." }}</p>
          <p class="muted">Backend manifest is loaded; feature execution is intentionally stubbed.</p>
        </section>
      </section>
    </div>
  </main>
</template>
