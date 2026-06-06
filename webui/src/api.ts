export type StorageConfig = {
  name: string;
  kind: string;
  root: string;
  mode: string;
};

export type PluginManifest = {
  id: string;
  name: string;
  version: string;
  description: string;
  depends_on?: string[];
  runtime: string;
  status: string;
};

export type ExplorerRow = {
  asset_id: string;
  name: string;
  media_kind: string;
  storage_url: string;
  relative_path: string;
  size_bytes: number;
  mtime: string;
  hash_status: string;
  sha512_hex?: string;
};

export type Job = {
  id: string;
  kind: string;
  status: string;
  progress_current: number;
  progress_total?: number;
  counters: Record<string, number>;
  error?: string;
  logs?: Array<{ level: string; message: string; created_at: string }>;
};

export type Stats = {
  assets: number;
  locations: number;
  photos: number;
  videos: number;
  tracks: number;
  unhashed: number;
  hashed: number;
  total_bytes: number;
};

export type BackendStatus = {
  store_backend: string;
  plugins: number;
  capabilities: Array<{ name: string; available: boolean; installed: boolean }>;
  stats: Stats;
  preview_cache: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error ?? `${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

export const api = {
  health: () => request<{ status: string }>("/api/v1/health"),
  status: () => request<BackendStatus>("/api/v1/backend/status"),
  storages: () => request<StorageConfig[]>("/api/v1/storages"),
  plugins: () => request<PluginManifest[]>("/api/v1/plugins"),
  jobs: () => request<Job[]>("/api/v1/jobs"),
  explorer: () => request<ExplorerRow[]>("/api/v1/explorer"),
  stats: () => request<Stats>("/api/v1/stats"),
  startDiscovery: () => request<Job>("/api/v1/discovery/start", { method: "POST" }),
  startHash: () => request<Job>("/api/v1/hash/start", { method: "POST" })
};
