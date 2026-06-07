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

export type FaceDetection = {
  id: string;
  asset_id: string;
  plugin_id?: string;
  x: number;
  y: number;
  width: number;
  height: number;
  confidence?: number;
  cluster_id?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
};

export type FaceCluster = {
  id: string;
  label: string;
  representative_face_id?: string;
  face_count: number;
  asset_count: number;
  ignored_count: number;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type GeoAlignMarker = {
  asset_id: string;
  name: string;
  media_kind: string;
  thumbnail_url?: string;
  original_lat?: number;
  original_lon?: number;
  manual_lat?: number;
  manual_lon?: number;
  staged_lat: number;
  staged_lon: number;
  status: string;
  track_candidates: Array<Record<string, unknown>>;
  modified: boolean;
  metadata?: Record<string, unknown>;
};

export type GeoAlignSession = {
  id: string;
  asset_ids: string[];
  track_ids: string[];
  markers: GeoAlignMarker[];
  bbox: { min_lon: number; min_lat: number; max_lon: number; max_lat: number };
  read_only: boolean;
  created_at: string;
  updated_at: string;
};

export type VideoTrackPlayerSession = {
  id: string;
  video_asset_id: string;
  track_ids: string[];
  timestamp_mode: string;
  offset_seconds: number;
  warnings?: string[];
  created_at: string;
  metadata?: Record<string, unknown>;
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
  ai_tags?: Record<string, unknown>[];
  ai_predictions?: Record<string, unknown>[];
  face_detections?: Record<string, unknown>[];
  embeddings?: Record<string, unknown>[];
  places?: AssetPlaceRecord[];
  ocr_blocks?: OCRBlock[];
};

export type AssetPlaceRecord = {
  coordinate_source: string;
  geo_source?: string;
  lat: number;
  lon: number;
  place_name: string;
  display_name: string;
  provider: string;
  source: string;
  match: string;
  bbox: { min_lon: number; min_lat: number; max_lon: number; max_lat: number };
  metadata?: Record<string, unknown>;
};

export type OCRBlock = {
  id: string;
  asset_id: string;
  text: string;
  language?: string;
  engine?: string;
  confidence?: number;
  x: number;
  y: number;
  width: number;
  height: number;
  model_name?: string;
  created_at: string;
  metadata?: Record<string, unknown>;
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
  source_format?: string;
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

export type TrackPointInfo = {
  track: TrackSummary;
  clicked: { lat: number; lon: number };
  nearest: { lat: number; lon: number };
  nearest_point?: TrackDetail["points"][number];
  nearest_segment_index?: number;
  distance_m: number;
  relative_time_seconds?: number;
  timestamp?: string;
  speed_mps?: number;
  elevation_m?: number;
  has_timestamps: boolean;
  source_format?: string;
  track_distance_m?: number;
  track_duration_seconds?: number;
};

export type TrackProfile = {
  track: TrackSummary;
  metric: "altitude" | "speed" | string;
  unit: string;
  has_values: boolean;
  has_timestamps: boolean;
  min?: number;
  max?: number;
  series: Array<{
    index: number;
    distance_m: number;
    relative_seconds?: number;
    timestamp?: string;
    value?: number;
    lat: number;
    lon: number;
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
  duplicate_groups: number;
  duplicate_locations: number;
  total_bytes: number;
};

export type DuplicateGroup = {
  content_id?: string;
  sha512_hex: string;
  size_bytes: number;
  asset_count: number;
  total_bytes: number;
  assets: Array<{
    asset_id: string;
    display_name: string;
    media_kind: string;
    storage_name: string;
    relative_path: string;
    storage_url: string;
    mtime: string;
  }>;
};

export type DuplicatePage = {
  groups: DuplicateGroup[];
  page: { limit: number; offset: number; total: number };
};

export type MonthBucket = {
  month: string;
  count: number;
  photos: number;
  videos: number;
  tracks: number;
  total_bytes: number;
  first_at?: string;
  last_at?: string;
};

export type BackendStatus = {
  store_backend: string;
  plugins: number;
  capabilities: Array<{ name: string; available: boolean; installed: boolean }>;
  stats: Stats;
  preview_cache: string;
  auth_mode: string;
  auth: AuthStatus;
  http?: {
    addr: string;
    tls_enabled: boolean;
    tls_cert_configured: boolean;
    tls_auto_self_signed: boolean;
  };
  tools?: Record<string, unknown>;
};

export type TranscodingCapabilities = {
  ffmpeg: { available: boolean; path?: string; version?: string };
  ffprobe: { available: boolean; path?: string; version?: string };
  encoders: Array<{ name: string; description?: string; codec_family?: string; hardware?: string }>;
  hardware: Record<string, boolean>;
  safety: string;
};

export type StreamOption = {
  id: string;
  label: string;
  available: boolean;
  url?: string;
  profile?: string;
  session_endpoint?: string;
  description?: string;
  disabled_reason?: string;
  built_in?: boolean;
  hardware?: string;
  codec?: string;
  mode?: string;
  parameter_value?: string;
};

export type StreamOptions = {
  asset_id: string;
  media_kind: string;
  direct_url: string;
  range: boolean;
  storage?: string;
  storage_mode?: string;
  options: StreamOption[];
};

export type IndexingScope = {
  storage: string;
  prefixes: string[];
  assets: number;
  photos: number;
  videos: number;
  tracks: number;
  hashed: number;
  unhashed: number;
  geotagged: number;
  preview_ready: number;
  total_bytes: number;
  track_like_files: number;
};

export type IndexingStatus = {
  scope: IndexingScope;
  latest_jobs: Record<string, Job>;
};

export type IndexingStartResult = {
  pipeline_id: string;
  scope: IndexingScope;
  queued_jobs: Job[];
  options: Record<string, boolean>;
  note?: string;
};

export type TranscodeSession = {
  id: string;
  asset_id: string;
  profile: string;
  hardware?: string;
  encoder?: string;
  container?: string;
  playlist_url: string;
  status: string;
  created_at: string;
  error?: string;
  stderr_tail?: string;
  command?: string[];
  segment_count?: number;
  output_bytes?: number;
};

export type TranscodingPreset = {
  id: string;
  name: string;
  built_in: boolean;
  available: boolean;
  disabled_reason?: string;
  hardware: string;
  codec: string;
  ffmpeg_encoder: string;
  mode: string;
  parameter_value: string;
  container: string;
  extra_args?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type SearchResult = {
  asset: Asset;
  matched: string[];
  explanation: string;
};

export type SearchPlace = {
  query: string;
  name: string;
  display_name: string;
  provider: string;
  source: string;
  lat: number;
  lon: number;
  bbox: { min_lon: number; min_lat: number; max_lon: number; max_lat: number };
  matched_assets: number;
};

export type PlaceCacheEntry = {
  id?: string;
  name: string;
  normalized_name?: string;
  aliases?: string[];
  provider?: string;
  display_name?: string;
  country?: string;
  region?: string;
  city?: string;
  road?: string;
  lat: number;
  lon: number;
  bbox: { min_lon: number; min_lat: number; max_lon: number; max_lat: number };
  source?: string;
  metadata?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
  last_used_at?: string;
};

export type PlacesResponse = {
  places: PlaceCacheEntry[];
  mode: string;
  online_enabled: boolean;
  note?: string;
};

export type SearchResponse = {
  query: string;
  tokens: string[];
  backend?: string;
  backend_mode?: string;
  results: SearchResult[];
  tracks?: Array<{ track: TrackSummary; matched: string[]; explanation: string }>;
  places?: SearchPlace[];
  warnings: string[];
  page: { limit: number; offset: number; total: number };
};

export type SearchPlacesResponse = {
  backend: string;
  mode: string;
  online_enabled: boolean;
  provider: string;
  places: SearchPlace[];
  note?: string;
};

export type TileSource = {
  id: string;
  name: string;
  enabled: boolean;
  template: string;
  attribution: string;
  policy: string;
  cache_dir?: string;
};

export type SettingsPayload = {
  tabs: Array<{ id: string; label: string; runtime: boolean }>;
  runtime_settings: Record<string, unknown>;
  pending_settings?: Record<string, unknown>;
  restart_required: Record<string, unknown>;
  yaml_bound_fields: string[];
  effective: Record<string, unknown>;
};

export type DBExport = {
  id: string;
  path: string;
  size_bytes: number;
  download_url: string;
  created_at: string;
};

export type FileBrowseRoot = {
  id: string;
  label: string;
  path: string;
  kind: string;
  read_only: boolean;
  warning?: string;
};

export type FileBrowseEntry = {
  name: string;
  path: string;
  kind: "file" | "folder" | string;
  size_bytes?: number;
  modified_at?: string;
  selectable: boolean;
  readable: boolean;
};

export type FileBrowseResponse = {
  roots: Record<string, FileBrowseRoot>;
  root?: FileBrowseRoot;
  current_path?: string;
  absolute?: string;
  parent?: string;
  entries: FileBrowseEntry[];
  warnings?: string[];
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

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeExplorer(value: ExplorerView | null | undefined): ExplorerView {
  return {
    current_path: value?.current_path ?? "",
    parent_path: value?.parent_path,
    folders: asArray(value?.folders),
    files: asArray(value?.files),
    file_count: value?.file_count ?? 0,
    folder_count: value?.folder_count ?? 0,
    total_bytes: value?.total_bytes ?? 0,
    offset: value?.offset ?? 0,
    limit: value?.limit ?? 0
  };
}

function normalizeAsset(asset: Asset): Asset {
  return {
    ...asset,
    metadata: asset.metadata ?? {},
    locations: asArray(asset.locations)
  };
}

function normalizeAlbumItems(value: AlbumItemPage | null | undefined): AlbumItemPage {
  return {
    items: asArray(value?.items).map((item) => ({ ...item, asset: normalizeAsset(item.asset) })),
    page: value?.page ?? { limit: 0, offset: 0, total: 0 }
  };
}

function normalizeFeatureCollection(value: Record<string, unknown> | null | undefined): Record<string, unknown> {
  return {
    type: "FeatureCollection",
    clustering: value?.clustering ?? "none",
    zoom: value?.zoom ?? 10,
    ...value,
    features: asArray(value?.features as Array<Record<string, unknown>> | null | undefined)
  };
}

function normalizeDuplicatePage(value: DuplicatePage | null | undefined): DuplicatePage {
  return {
    groups: asArray(value?.groups).map((group) => ({ ...group, assets: asArray(group.assets) })),
    page: value?.page ?? { limit: 0, offset: 0, total: 0 }
  };
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
  storages: async () => asArray(await request<StorageConfig[] | null>("/api/v1/storages")),
  createStorage: (storage: StorageConfig, validateOnly = false) =>
    request<Record<string, unknown>>("/api/v1/storages", {
      method: "POST",
      body: JSON.stringify({ ...storage, validate_only: validateOnly })
    }),
  updateStorage: (name: string, storage: StorageConfig, validateOnly = false) =>
    request<Record<string, unknown>>(`/api/v1/storages/${encodeURIComponent(name)}`, {
      method: "PATCH",
      body: JSON.stringify({ ...storage, validate_only: validateOnly })
    }),
  validateStorage: (name: string) =>
    request<Record<string, unknown>>(`/api/v1/storages/${encodeURIComponent(name)}/validate`),
  plugins: async () => asArray(await request<PluginManifest[] | null>("/api/v1/plugins")),
  plugin: (id: string) => request<PluginManifest>(`/api/v1/plugins/${encodeURIComponent(id)}`),
  pluginHealth: (id: string) => request<Record<string, unknown>>(`/api/v1/plugins/${encodeURIComponent(id)}/health`),
  jobs: async (query = "") => asArray(await request<Job[] | null>(`/api/v1/jobs${query ? `?${query}` : ""}`)),
  jobStats: () => request<JobStats>("/api/v1/jobs/stats"),
  job: (id: string) => request<Job>(`/api/v1/jobs/${encodeURIComponent(id)}`),
  jobLogs: (id: string) => request<{ logs: Job["logs"]; next_after_id: number }>(`/api/v1/jobs/${encodeURIComponent(id)}/logs`),
  explorer: async (query = "") => asArray(await request<ExplorerRow[] | null>(`/api/v1/explorer${query ? `?${query}` : ""}`)),
  explorerFolders: (path = "", query = "") => {
    const params = new URLSearchParams(query);
    params.set("view", "folders");
    params.set("path", path);
    if (!params.has("sort")) params.set("sort", "name");
    return request<ExplorerView | null>(`/api/v1/explorer?${params.toString()}`).then(normalizeExplorer);
  },
  asset: (id: string) =>
    request<AssetDetail>(`/api/v1/assets/${encodeURIComponent(id)}`).then((detail) => ({
      ...detail,
      asset: normalizeAsset(detail.asset),
      locations: asArray(detail.locations),
      preview: detail.preview ?? { status: "not_implemented" },
      content: detail.content ?? {},
      timestamps: detail.timestamps ?? {},
      metadata: detail.metadata ?? {}
    })),
  duplicates: (limit = 50, offset = 0) =>
    request<DuplicatePage | null>(`/api/v1/duplicates?limit=${limit}&offset=${offset}`).then(normalizeDuplicatePage),
  assetMonths: async (query = "") =>
    asArray(await request<MonthBucket[] | null>(`/api/v1/assets/months${query ? `?${query}` : ""}`)),
  albums: async () => asArray(await request<Album[] | null>("/api/v1/albums?tree=true")),
  createAlbum: (title: string, description = "", parentId = "") =>
    request<Album>("/api/v1/albums", {
      method: "POST",
      body: JSON.stringify({ title, description, parent_id: parentId })
    }),
  albumItems: (albumId: string) =>
    request<AlbumItemPage | null>(`/api/v1/albums/${encodeURIComponent(albumId)}/items`).then(normalizeAlbumItems),
  addAlbumItems: (albumId: string, assetIds: string[]) =>
    request<AlbumItemPage | null>(`/api/v1/albums/${encodeURIComponent(albumId)}/items`, {
      method: "POST",
      body: JSON.stringify({ asset_ids: assetIds })
    }).then(normalizeAlbumItems),
  removeAlbumItem: (albumId: string, assetId: string) =>
    request<{ status: string }>(`/api/v1/albums/${encodeURIComponent(albumId)}/items/${encodeURIComponent(assetId)}`, {
      method: "DELETE"
    }),
  tracks: async () => asArray(await request<TrackSummary[] | null>("/api/v1/tracks")),
  gpsTracks: async () => asArray(await request<TrackSummary[] | null>("/api/v1/gps/tracks")),
  gpsTrack: (id: string) =>
    request<TrackDetail>(`/api/v1/gps/tracks/${encodeURIComponent(id)}`).then((detail) => ({
      ...detail,
      points: asArray(detail.points)
    })),
  gpsTrackPoints: (id: string, maxPoints = 500) =>
    request<TrackDetail["points"] | null>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/points?simplify=true&max_points=${maxPoints}`
    ).then(asArray),
  gpsTrackPointInfo: (id: string, lat: number, lon: number) =>
    request<TrackPointInfo>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/point-info?lat=${encodeURIComponent(String(lat))}&lon=${encodeURIComponent(String(lon))}`
    ),
  gpsTrackProfile: (id: string, metric: "altitude" | "speed", maxPoints = 1000) =>
    request<TrackProfile>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/profile?metric=${encodeURIComponent(metric)}&simplify=true&max_points=${maxPoints}`
    ).then((profile) => ({ ...profile, series: asArray(profile.series) })),
  gpsTrackAssets: (id: string, offsetSeconds = 0, mediaKind = "photo,video") =>
    request<{ assets: Asset[]; page: { limit: number; offset: number; total: number }; reason?: string }>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/assets?offset_seconds=${offsetSeconds}&include_geotagged=true&include_ungeotagged=true&exclude_track_assets=true&media_kind=${encodeURIComponent(mediaKind)}`
    ).then((page) => ({ ...page, assets: asArray(page.assets).map(normalizeAsset) })),
  gpsTrackNearbyAssets: (id: string, distanceM = 100) =>
    request<{ assets: Array<{ asset: Asset; distance_m: number; nearest_lat: number; nearest_lon: number }>; page: { limit: number; offset: number; total: number } }>(
      `/api/v1/gps/tracks/${encodeURIComponent(id)}/nearby-assets?distance_m=${encodeURIComponent(String(distanceM))}`
    ).then((page) => ({
      ...page,
      assets: asArray(page.assets).map((item) => ({ ...item, asset: normalizeAsset(item.asset) }))
    })),
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
      zoom: "10",
      cluster: "true"
    });
    Object.entries(params).forEach(([key, value]) => query.set(key, String(value)));
    return request<Record<string, unknown> | null>(`/api/v1/map?${query.toString()}`).then(normalizeFeatureCollection);
  },
  mapStatus: () => request<Record<string, unknown>>("/api/v1/map/status"),
  mapTracks: (params: Record<string, string | number | boolean> = {}) => {
    const query = new URLSearchParams({ zoom: "10" });
    Object.entries(params).forEach(([key, value]) => query.set(key, String(value)));
    return request<Record<string, unknown> | null>(`/api/v1/map/tracks?${query.toString()}`).then(normalizeFeatureCollection);
  },
  tileSources: async () => asArray(await request<TileSource[] | null>("/api/v1/map/tile-sources")),
  stats: () => request<Stats>("/api/v1/stats"),
  startDiscovery: (payload: {
    storage: string;
    prefixes: string[];
    max_files: number;
    max_bytes: number;
    mark_missing?: boolean;
    hash?: boolean;
    metadata?: boolean;
    previews?: boolean;
  }) => request<Job>("/api/v1/discovery/start", { method: "POST", body: JSON.stringify(payload) }),
  startIndexing: (payload: {
    storage: string;
    prefixes: string[];
    max_files: number;
    max_bytes: number;
    include_extensions?: string[];
    exclude_patterns?: string[];
    index_files?: boolean;
    hash?: boolean;
    metadata?: boolean;
    previews?: boolean;
    parse_tracks?: boolean;
    geotag_exif?: boolean;
    snap_to_tracks?: boolean;
    refresh_map?: boolean;
  }) => request<IndexingStartResult>("/api/v1/indexing/start", { method: "POST", body: JSON.stringify(payload) }),
  indexingLatest: (storage: string, prefixes: string[]) => {
    const query = new URLSearchParams();
    if (storage) query.set("storage", storage);
    for (const prefix of prefixes) query.append("prefixes", prefix);
    return request<IndexingStatus>(`/api/v1/indexing/latest?${query.toString()}`);
  },
  cancelIndexing: (pipelineId: string) =>
    request<Record<string, unknown>>(`/api/v1/indexing/${encodeURIComponent(pipelineId)}/cancel`, { method: "POST" }),
  startHash: (payload: {
    scope?: string;
    asset_id?: string;
    asset_ids?: string[];
    storage?: string;
    prefix?: string;
    prefixes?: string[];
    album_id?: string;
    max_files?: number;
  } = {}) => request<Job>("/api/v1/hash/start", { method: "POST", body: JSON.stringify(payload) }),
  startMetadata: (maxFiles = 50) =>
    request<Job>("/api/v1/metadata/enrich/start", {
      method: "POST",
      body: JSON.stringify({
        include_video: true,
        include_images: true,
        include_tracks: true,
        only_missing: false,
        max_files: maxFiles
      })
    }),
  startMetadataScoped: (payload: {
    storage?: string;
    prefixes?: string[];
    asset_ids?: string[];
    max_files?: number;
    only_missing?: boolean;
  }) =>
    request<Job>("/api/v1/metadata/enrich/start", {
      method: "POST",
      body: JSON.stringify({
        include_video: true,
        include_images: true,
        include_tracks: true,
        only_missing: payload.only_missing ?? false,
        ...payload
      })
    }),
  startPreviews: (maxFiles = 50) =>
    request<Job>("/api/v1/previews/start", {
      method: "POST",
      body: JSON.stringify({ only_missing: true, media_kind: "photo", max_files: maxFiles })
    }),
  startPreviewsScoped: (payload: {
    storage?: string;
    prefixes?: string[];
    asset_ids?: string[];
    max_files?: number;
    only_missing?: boolean;
  }) =>
    request<Job>("/api/v1/previews/start", {
      method: "POST",
      body: JSON.stringify({ only_missing: payload.only_missing ?? true, media_kind: "photo", ...payload })
    }),
  previewStatus: () => request<{ cache_dir: string; stats: PreviewCacheStats }>("/api/v1/previews/status"),
  previewCache: async () => asArray(await request<PreviewCacheEntry[] | null>("/api/v1/previews/cache?limit=50")),
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
  transcodingCapabilities: () =>
    request<TranscodingCapabilities>("/api/v1/transcoding/capabilities").then((caps) => ({
      ...caps,
      encoders: asArray(caps.encoders),
      hardware: caps.hardware ?? {}
    })),
  streamOptions: (assetId: string) => request<StreamOptions>(`/api/v1/media/${encodeURIComponent(assetId)}/stream-options`),
  trackPreview: (assetId: string, maxPoints = 1200) =>
    request<Record<string, unknown>>(`/api/v1/media/${encodeURIComponent(assetId)}/track-preview?max_points=${maxPoints}`),
  transcodingPresets: async () => asArray(await request<TranscodingPreset[] | null>("/api/v1/transcoding/presets")),
  saveTranscodingPreset: (preset: Partial<TranscodingPreset>) =>
    request<TranscodingPreset>("/api/v1/transcoding/presets", { method: "POST", body: JSON.stringify(preset) }),
  validateTranscodingPreset: (preset: Partial<TranscodingPreset>, assetId?: string, dryRun = false) =>
    request<Record<string, unknown>>("/api/v1/transcoding/presets/validate", {
      method: "POST",
      body: JSON.stringify({ preset, asset_id: assetId, dry_run: dryRun, duration_seconds: 2 })
    }),
  testTranscodingHardware: (preset: Partial<TranscodingPreset>, assetId: string) =>
    request<Record<string, unknown>>("/api/v1/transcoding/hardware-test", {
      method: "POST",
      body: JSON.stringify({ preset, asset_id: assetId, dry_run: true, duration_seconds: 2 })
    }),
  deleteTranscodingPreset: (id: string) =>
    request<{ status: string }>(`/api/v1/transcoding/presets/${encodeURIComponent(id)}`, { method: "DELETE" }),
  startTranscodeSession: (assetId: string, profile: string, preset?: Partial<TranscodingPreset>) =>
    request<TranscodeSession>(`/api/v1/media/${encodeURIComponent(assetId)}/transcode-session`, {
      method: "POST",
      body: JSON.stringify({ profile, preset })
    }),
  transcodeSessionStatus: (sessionId: string) =>
    request<TranscodeSession>(`/api/v1/media/transcode-sessions/${encodeURIComponent(sessionId)}/status`),
  stopTranscodeSession: (sessionId: string) =>
    request<{ status: string }>(`/api/v1/media/transcode-sessions/${encodeURIComponent(sessionId)}`, { method: "DELETE" }),
  transcodingStatus: () => request<Record<string, unknown>>("/api/v1/transcoding/status"),
  transcodingMetricsStatus: () => request<Record<string, unknown>>("/api/v1/transcoding/metrics/status"),
  aiStatus: () => request<Record<string, unknown>>("/api/v1/ai/status"),
  aiWorkers: () => request<Record<string, unknown>>("/api/v1/ai/workers"),
  aiJob: (kind: "classify" | "faces" | "describe" | "safety" | "embed" | "ocr", payload: Record<string, unknown>) =>
    request<Record<string, unknown>>(`/api/v1/ai/jobs/${kind}`, { method: "POST", body: JSON.stringify(payload) }),
  deleteOCRBlock: (assetId: string, blockId: string) =>
    request<{ status: string }>(`/api/v1/assets/${encodeURIComponent(assetId)}/ocr/${encodeURIComponent(blockId)}`, {
      method: "DELETE"
    }),
  aiSummary: () => request<Record<string, unknown>>("/api/v1/ai/summary"),
  aiTags: () => request<Record<string, unknown>>("/api/v1/ai/tags"),
  aiPredictions: (limit = 100) => request<Record<string, unknown>>(`/api/v1/ai/predictions?limit=${limit}`),
  aiFaces: (limit = 100) => request<Record<string, unknown>>(`/api/v1/ai/faces?limit=${limit}`),
  aiSafety: () => request<Record<string, unknown>>("/api/v1/ai/safety"),
  faceClusters: () => request<{ clusters: FaceCluster[]; total: number; provisional_note?: string }>("/api/v1/faces/clusters"),
  faceClusterAssets: (clusterId: string) =>
    request<{ cluster_id: string; faces: FaceDetection[]; assets: Asset[]; total: number }>(
      `/api/v1/faces/clusters/${encodeURIComponent(clusterId)}/assets`
    ).then((response) => ({
      ...response,
      faces: asArray(response.faces),
      assets: asArray(response.assets).map(normalizeAsset)
    })),
  updateFaceCluster: (clusterId: string, payload: { label?: string; metadata?: Record<string, unknown> }) =>
    request<FaceCluster>(`/api/v1/faces/clusters/${encodeURIComponent(clusterId)}`, {
      method: "PATCH",
      body: JSON.stringify(payload)
    }),
  createFaceDetection: (payload: {
    asset_id: string;
    x: number;
    y: number;
    width: number;
    height: number;
    confidence?: number;
    label?: string;
  }) =>
    request<FaceDetection>("/api/v1/faces/detections", {
      method: "POST",
      body: JSON.stringify(payload)
    }),
  ignoreFaceDetection: (detectionId: string) =>
    request<FaceDetection>(`/api/v1/faces/detections/${encodeURIComponent(detectionId)}/ignore`, { method: "POST" }),
  deleteFaceDetection: (detectionId: string) =>
    request<FaceDetection>(`/api/v1/faces/detections/${encodeURIComponent(detectionId)}`, { method: "DELETE" }),
  createGeoAlignSession: (payload: { asset_ids?: string[]; track_ids?: string[]; limit?: number }) =>
    request<GeoAlignSession>("/api/v1/geo-align/session", { method: "POST", body: JSON.stringify(payload) }),
  getGeoAlignSession: (id: string) => request<GeoAlignSession>(`/api/v1/geo-align/sessions/${encodeURIComponent(id)}`),
  moveGeoAlignMarker: (sessionId: string, assetId: string, lat: number, lon: number) =>
    request<GeoAlignMarker>(
      `/api/v1/geo-align/sessions/${encodeURIComponent(sessionId)}/marker/${encodeURIComponent(assetId)}`,
      { method: "PATCH", body: JSON.stringify({ lat, lon }) }
    ),
  resetGeoAlignMarker: (sessionId: string, assetId: string) =>
    request<GeoAlignMarker>(
      `/api/v1/geo-align/sessions/${encodeURIComponent(sessionId)}/marker/${encodeURIComponent(assetId)}`,
      { method: "PATCH", body: JSON.stringify({ reset: true }) }
    ),
  resetGeoAlignSession: (id: string) =>
    request<GeoAlignSession>(`/api/v1/geo-align/sessions/${encodeURIComponent(id)}/reset`, { method: "POST" }),
  applyGeoAlignSession: (id: string) =>
    request<Record<string, unknown>>(`/api/v1/geo-align/sessions/${encodeURIComponent(id)}/apply`, { method: "POST" }),
  createVideoTrackPlayerSession: (payload: {
    video_asset_id: string;
    track_ids: string[];
    timestamp_mode?: string;
    offset_seconds?: number;
  }) => request<VideoTrackPlayerSession>("/api/v1/video-track-player/session", { method: "POST", body: JSON.stringify(payload) }),
  videoTrackPlayerPosition: (sessionId: string, timeMS: number) =>
    request<Record<string, unknown>>(
      `/api/v1/video-track-player/sessions/${encodeURIComponent(sessionId)}/position?time_ms=${encodeURIComponent(String(Math.round(timeMS)))}`
    ),
  vectorStatus: () => request<Record<string, unknown>>("/api/v1/vector/status"),
  vectorSearch: (q: string, limit = 20) =>
    request<Record<string, unknown>>(
      `/api/v1/search/vector?q=${encodeURIComponent(q)}&limit=${encodeURIComponent(String(limit))}`
    ),
  search: (q: string, limit = 100, offset = 0) =>
    request<SearchResponse>(
      `/api/v1/search?q=${encodeURIComponent(q)}&limit=${encodeURIComponent(String(limit))}&offset=${encodeURIComponent(String(offset))}`
    ).then((response) => ({
      ...response,
      results: asArray(response.results).map((item) => ({ ...item, asset: normalizeAsset(item.asset) })),
      warnings: asArray(response.warnings),
      tokens: asArray(response.tokens),
      page: response.page ?? { limit, offset, total: 0 }
    })),
  searchPlaces: () => request<SearchPlacesResponse>("/api/v1/search/places"),
  places: (q = "") =>
    request<PlacesResponse>(`/api/v1/places${q.trim() ? `?q=${encodeURIComponent(q.trim())}` : ""}`),
  createPlace: (place: PlaceCacheEntry) =>
    request<PlaceCacheEntry>("/api/v1/places", {
      method: "POST",
      body: JSON.stringify(place)
    }),
  updatePlace: (id: string, place: Partial<PlaceCacheEntry>) =>
    request<PlaceCacheEntry>(`/api/v1/places/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(place)
    }),
  deletePlace: (id: string) =>
    request<Record<string, unknown>>(`/api/v1/places/${encodeURIComponent(id)}`, {
      method: "DELETE"
    }),
  settings: () => request<SettingsPayload>("/api/v1/settings"),
  pendingSettings: () => request<Record<string, unknown>>("/api/v1/settings/pending"),
  patchPendingSettings: (settings: Record<string, unknown>) =>
    request<Record<string, unknown>>("/api/v1/settings/pending", {
      method: "PATCH",
      body: JSON.stringify(settings)
    }),
  clearPendingSettings: () => request<Record<string, unknown>>("/api/v1/settings/pending", { method: "DELETE" }),
  pluginSettings: (pluginId: string) =>
    request<Record<string, unknown>>(`/api/v1/plugins/${encodeURIComponent(pluginId)}/settings`),
  patchPluginSettings: (pluginId: string, settings: Record<string, unknown>) =>
    request<Record<string, unknown>>(`/api/v1/plugins/${encodeURIComponent(pluginId)}/settings`, {
      method: "PATCH",
      body: JSON.stringify(settings)
    }),
  patchRuntimeSettings: (settings: Record<string, unknown>) =>
    request<Record<string, unknown>>("/api/v1/settings/runtime", {
      method: "PATCH",
      body: JSON.stringify(settings)
    }),
  dbExport: () => request<DBExport>("/api/v1/admin/db/export", { method: "POST" }),
  dbExports: async () => asArray(await request<DBExport[] | null>("/api/v1/admin/db/exports")),
  dbImportPlan: (path: string) =>
    request<Record<string, unknown>>("/api/v1/admin/db/import-plan", {
      method: "POST",
      body: JSON.stringify({ path, confirmation_phrase: "PLAN ONLY" })
    }),
  browseFiles: (root = "", path = "", kind = "folder") => {
    const query = new URLSearchParams();
    if (root) query.set("root", root);
    if (path) query.set("path", path);
    if (kind) query.set("kind", kind);
    return request<FileBrowseResponse>(`/api/v1/files/browse?${query.toString()}`).then((response) => ({
      ...response,
      roots: response.roots ?? {},
      entries: asArray(response.entries),
      warnings: asArray(response.warnings)
    }));
  }
};
