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
  taken_at?: string;
  metadata?: Record<string, unknown>;
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
  cache_path?: string;
  message?: string;
};

export type Principal = {
  id: string;
  name: string;
  email?: string;
  role: string;
  auth_method?: string;
  scopes?: string[];
};

export type AuthMe = {
  principal: Principal;
  auth: AuthStatus;
};

export type AuthStatus = {
  mode: string;
  warning?: string;
  session_cookie_name?: string;
  session_cookie_secure?: boolean;
  csrf_required?: boolean;
  csrf_header?: string;
  oidc_enabled?: boolean;
  oauth_enabled?: boolean;
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
  distance_m?: number;
  duration_seconds?: number;
  elevation_min_m?: number;
  elevation_max_m?: number;
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

export type Album = {
  id: string;
  parent_id?: string;
  slug: string;
  title: string;
  description: string;
  sort_order: number;
  item_count: number;
  created_at: string;
  updated_at: string;
};

export type AlbumItemPage = {
  items: Array<{ album_id: string; asset: Asset; note: string; sort_order: number; added_at: string }>;
  page: { limit: number; offset: number; total: number };
};

export type PreviewCacheStats = {
  entries: number;
  ready: number;
  failed: number;
  bytes: number;
  oldest_unix?: number;
};

export type PreviewCacheEntry = {
  id: string;
  asset_id: string;
  content_id?: string;
  variant: string;
  width: number;
  height: number;
  format: string;
  cache_path: string;
  status: string;
  size_bytes: number;
  created_at: string;
  last_accessed_at?: string;
  error?: string;
};

export type ScanRun = {
  id: string;
  job_id?: string;
  storage_name: string;
  mode: string;
  prefixes: string[];
  max_files: number;
  max_bytes: number;
  hash_requested: boolean;
  metadata_requested: boolean;
  previews_requested: boolean;
  mark_missing: boolean;
  dry_run: boolean;
  report: Record<string, unknown>;
  created_at: string;
  finished_at?: string;
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
  logs?: Array<{ id?: number; level: string; message: string; created_at: string }>;
};

export type JobStats = {
  queued: number;
  running: number;
  cancel_requested: number;
  failed: number;
  succeeded: number;
  cancelled: number;
  active_worker_ids: string[];
  last_errors: Array<Record<string, unknown>>;
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
  auth: AuthStatus;
  tools?: Record<string, unknown>;
};

export type TranscodingCapabilities = {
  ffmpeg: { available: boolean; path?: string; version?: string };
  ffprobe: { available: boolean; path?: string; version?: string };
  encoders: Array<{ name: string; description?: string; codec_family?: string; hardware?: string }>;
  hardware: Record<string, boolean>;
  safety: string;
};

let csrfHeader = "X-CSRF-Token";
let csrfToken = "";

async function refreshCSRF(): Promise<void> {
  const response = await fetch("/api/v1/auth/csrf", { credentials: "same-origin" });
  if (!response.ok) return;
  const body = (await response.json()) as { required?: boolean; header?: string; token?: string };
  if (body.required && body.header && body.token) {
    csrfHeader = body.header;
    csrfToken = body.token;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = init?.method?.toUpperCase() ?? "GET";
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (init?.headers) Object.assign(headers, init.headers as Record<string, string>);
  if (!["GET", "HEAD", "OPTIONS"].includes(method) && path !== "/api/v1/auth/login" && path !== "/api/v1/auth/csrf") {
    await refreshCSRF().catch(() => undefined);
    if (csrfToken) headers[csrfHeader] = csrfToken;
  }
  const response = await fetch(path, {
    credentials: "same-origin",
    ...init,
    headers
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
  csrf: refreshCSRF,
  login: (email: string, password: string) =>
    request<LoginResult>("/api/v1/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => request<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  changePassword: (oldPassword: string, newPassword: string) =>
    request<{ status: string }>("/api/v1/auth/password", {
      method: "POST",
      body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
    }),
  tokens: () => request<APIToken[]>("/api/v1/auth/tokens"),
  createToken: (name: string, scopes: string[]) =>
    request<{ token: APIToken; secret: string }>("/api/v1/auth/tokens", {
      method: "POST",
      body: JSON.stringify({ name, scopes })
    }),
  status: () => request<BackendStatus>("/api/v1/backend/status"),
  storages: () => request<StorageConfig[]>("/api/v1/storages"),
  plugins: () => request<PluginManifest[]>("/api/v1/plugins"),
  plugin: (id: string) => request<PluginManifest>(`/api/v1/plugins/${encodeURIComponent(id)}`),
  pluginHealth: (id: string) => request<Record<string, unknown>>(`/api/v1/plugins/${encodeURIComponent(id)}/health`),
  jobs: (query = "") => request<Job[]>(`/api/v1/jobs${query ? `?${query}` : ""}`),
  jobStats: () => request<JobStats>("/api/v1/jobs/stats"),
  job: (id: string) => request<Job>(`/api/v1/jobs/${encodeURIComponent(id)}`),
  jobLogs: (id: string) => request<{ logs: Job["logs"]; next_after_id: number }>(`/api/v1/jobs/${encodeURIComponent(id)}/logs`),
  explorer: () => request<ExplorerRow[]>("/api/v1/explorer"),
  explorerFolders: (path = "") =>
    request<ExplorerView>(`/api/v1/explorer?view=folders&path=${encodeURIComponent(path)}&sort=name`),
  asset: (id: string) => request<AssetDetail>(`/api/v1/assets/${encodeURIComponent(id)}`),
  albums: () => request<Album[]>("/api/v1/albums?tree=true"),
  createAlbum: (title: string, description = "", parentId = "") =>
    request<Album>("/api/v1/albums", {
      method: "POST",
      body: JSON.stringify({ title, description, parent_id: parentId })
    }),
  albumItems: (albumId: string) => request<AlbumItemPage>(`/api/v1/albums/${encodeURIComponent(albumId)}/items`),
  addAlbumItems: (albumId: string, assetIds: string[]) =>
    request<AlbumItemPage>(`/api/v1/albums/${encodeURIComponent(albumId)}/items`, {
      method: "POST",
      body: JSON.stringify({ asset_ids: assetIds })
    }),
  removeAlbumItem: (albumId: string, assetId: string) =>
    request<{ status: string }>(`/api/v1/albums/${encodeURIComponent(albumId)}/items/${encodeURIComponent(assetId)}`, {
      method: "DELETE"
    }),
  tracks: () => request<TrackSummary[]>("/api/v1/tracks"),
  gpsTracks: () => request<TrackSummary[]>("/api/v1/gps/tracks"),
  gpsTrack: (id: string) => request<TrackDetail>(`/api/v1/gps/tracks/${encodeURIComponent(id)}`),
  gpsTrackPoints: (id: string, maxPoints = 500) =>
    request<TrackDetail["points"]>(`/api/v1/gps/tracks/${encodeURIComponent(id)}/points?simplify=true&max_points=${maxPoints}`),
  gpsTrackAssets: (id: string, offsetSeconds = 0) =>
    request<{ assets: Asset[]; page: { limit: number; offset: number; total: number } }>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/assets?offset_seconds=${offsetSeconds}&include_geotagged=true&include_ungeotagged=true`
    ),
  snapTrackMedia: (id: string, offsetSeconds = 0) =>
    request<Job>(`/api/v1/gps/tracks/${encodeURIComponent(id)}/snap-media`, {
      method: "POST",
      body: JSON.stringify({ offset_seconds: offsetSeconds })
    }),
  track: (id: string) => request<TrackDetail>(`/api/v1/tracks/${encodeURIComponent(id)}`),
  syncCandidates: (assetId: string) =>
    request<TrackCandidate[]>(`/api/v1/sync/candidates?asset_id=${encodeURIComponent(assetId)}`),
  map: (params: Record<string, string | number | boolean> = {}) => {
    const query = new URLSearchParams({
      bbox: "43.8,39.8,44.3,40.3",
      zoom: "10",
      cluster: "true"
    });
    Object.entries(params).forEach(([key, value]) => query.set(key, String(value)));
    return request<Record<string, unknown>>(`/api/v1/map?${query.toString()}`);
  },
  mapStatus: () => request<Record<string, unknown>>("/api/v1/map/status"),
  stats: () => request<Stats>("/api/v1/stats"),
  startDiscovery: () => request<Job>("/api/v1/discovery/start", { method: "POST" }),
  startHash: () => request<Job>("/api/v1/hash/start", { method: "POST" }),
  startMetadata: () =>
    request<Job>("/api/v1/metadata/enrich/start", {
      method: "POST",
      body: JSON.stringify({ include_video: true, include_images: true, include_tracks: true, only_missing: false })
    }),
  startPreviews: () =>
    request<Job>("/api/v1/previews/start", { method: "POST", body: JSON.stringify({ only_missing: true }) }),
  previewStatus: () => request<{ cache_dir: string; stats: PreviewCacheStats }>("/api/v1/previews/status"),
  previewCache: () => request<PreviewCacheEntry[]>("/api/v1/previews/cache?limit=50"),
  previewCleanup: (dryRun = true, maxBytes = 0) =>
    request<Record<string, unknown>>("/api/v1/previews/cleanup", {
      method: "POST",
      body: JSON.stringify({ dry_run: dryRun, max_bytes: maxBytes })
    }),
  dryRunDiscovery: (payload: {
    storage: string;
    prefixes: string[];
    max_files?: number;
    max_bytes?: number;
    include_extensions?: string[];
  }) => request<{ job: Job; scan_run: ScanRun }>("/api/v1/discovery/dry-run", { method: "POST", body: JSON.stringify(payload) }),
  dryRunReport: (jobId: string) => request<ScanRun>(`/api/v1/discovery/dry-run/${encodeURIComponent(jobId)}/report`),
  cancelJob: (id: string) => request<Job>(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, { method: "POST" }),
  retryJob: (id: string, force = false) =>
    request<Job>(`/api/v1/jobs/${encodeURIComponent(id)}/retry`, { method: "POST", body: JSON.stringify({ force }) }),
  transcodingCapabilities: () => request<TranscodingCapabilities>("/api/v1/transcoding/capabilities"),
  transcodingStatus: () => request<Record<string, unknown>>("/api/v1/transcoding/status"),
  aiStatus: () => request<Record<string, unknown>>("/api/v1/ai/status"),
  vectorStatus: () => request<Record<string, unknown>>("/api/v1/vector/status")
};
