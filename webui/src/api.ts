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

export type ExplorerFolder = {
  name: string;
  path: string;
  file_count: number;
  total_bytes: number;
  latest_mtime: string;
};

export type ExplorerFile = ExplorerRow;

export type ExplorerView = {
  current_path: string;
  parent_path?: string;
  folders: ExplorerFolder[];
  files: ExplorerFile[];
  file_count: number;
  folder_count: number;
  total_bytes: number;
  offset: number;
  limit: number;
};

export type Asset = {
  id: string;
  media_kind: string;
  display_name: string;
  first_seen_at: string;
  updated_at: string;
  locations: Array<{
    id: string;
    asset_id: string;
    storage_name: string;
    storage_url: string;
    relative_path: string;
    file_name: string;
    extension: string;
    mime: string;
    media_kind: string;
    size_bytes: number;
    mtime: string;
    hash_status: string;
    sha512_hex?: string;
    content_id?: string;
    last_seen_at: string;
  }>;
};

export type PreviewInfo = {
  status: string;
  url?: string;
  cache_key?: string;
  message?: string;
};

export type Principal = {
  id: string;
  name: string;
  email?: string;
  role: string;
};

export type AuthMe = {
  principal: Principal;
  auth_mode: string;
};

export type LoginResult = {
  principal: Principal;
  session?: { id: string; expires_at: string; principal: Principal };
};

export type APIToken = {
  id: string;
  user_id: string;
  name: string;
  scopes: string[];
  expires_at?: string;
  created_at: string;
  last_used_at?: string;
  revoked_at?: string;
};

export type AssetDetail = {
  asset: Asset;
  locations: Asset["locations"];
  original_url?: string;
  preview_url?: string;
  preview: PreviewInfo;
  content: Record<string, unknown>;
  timestamps: Record<string, string>;
  metadata: Record<string, unknown>;
};

export type TrackSummary = {
  track_asset_id: string;
  name: string;
  point_count: number;
  start_time?: string;
  end_time?: string;
  min_lat?: number;
  min_lon?: number;
  max_lat?: number;
  max_lon?: number;
};

export type TrackDetail = {
  summary: TrackSummary;
  points: Array<{
    id?: number;
    track_asset_id?: string;
    recorded_at: string;
    lat: number;
    lon: number;
    elevation_m?: number;
    speed_mps?: number;
    source: string;
  }>;
};

export type TrackCandidate = {
  track: TrackSummary;
  overlap_start?: string;
  overlap_end?: string;
  confidence: number;
};

export type Job = {
  id: string;
  kind: string;
  status: string;
  progress_current: number;
  progress_total?: number;
  counters: Record<string, number>;
  attempts: number;
  max_attempts: number;
  error?: string;
  worker_id?: string;
  lease_expires_at?: string;
  cancel_requested_at?: string;
  next_run_at?: string;
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
  auth_mode: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
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
  me: () => request<AuthMe>("/api/v1/auth/me"),
  login: (email: string, password: string) =>
    request<LoginResult>("/api/v1/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => request<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  tokens: () => request<APIToken[]>("/api/v1/auth/tokens"),
  createToken: (name: string, scopes: string[]) =>
    request<{ token: APIToken; secret: string }>("/api/v1/auth/tokens", {
      method: "POST",
      body: JSON.stringify({ name, scopes })
    }),
  status: () => request<BackendStatus>("/api/v1/backend/status"),
  storages: () => request<StorageConfig[]>("/api/v1/storages"),
  plugins: () => request<PluginManifest[]>("/api/v1/plugins"),
  jobs: () => request<Job[]>("/api/v1/jobs"),
  explorer: () => request<ExplorerRow[]>("/api/v1/explorer"),
  explorerFolders: (path = "") =>
    request<ExplorerView>(`/api/v1/explorer?view=folders&path=${encodeURIComponent(path)}&sort=name`),
  asset: (id: string) => request<AssetDetail>(`/api/v1/assets/${encodeURIComponent(id)}`),
  tracks: () => request<TrackSummary[]>("/api/v1/tracks"),
  track: (id: string) => request<TrackDetail>(`/api/v1/tracks/${encodeURIComponent(id)}`),
  syncCandidates: (assetId: string) =>
    request<TrackCandidate[]>(`/api/v1/sync/candidates?asset_id=${encodeURIComponent(assetId)}`),
  map: (bbox = "43.8,39.8,44.3,40.3", zoom = 10) =>
    request<Record<string, unknown>>(`/api/v1/map?bbox=${encodeURIComponent(bbox)}&zoom=${zoom}`),
  stats: () => request<Stats>("/api/v1/stats"),
  startDiscovery: () => request<Job>("/api/v1/discovery/start", { method: "POST" }),
  startHash: () => request<Job>("/api/v1/hash/start", { method: "POST" }),
  cancelJob: (id: string) => request<Job>(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, { method: "POST" })
};
