create index if not exists idx_asset_locations_relative_path_pattern
    on asset_locations(relative_path text_pattern_ops);

create index if not exists idx_asset_locations_storage_relative_path_pattern
    on asset_locations(storage_id, relative_path text_pattern_ops);

create index if not exists idx_asset_locations_folder_sort_name
    on asset_locations(storage_id, lower(file_name), file_name, id);

create index if not exists idx_asset_locations_folder_sort_mtime
    on asset_locations(storage_id, mtime desc, lower(file_name), id);

create index if not exists idx_asset_locations_folder_sort_size
    on asset_locations(storage_id, size_bytes desc, lower(file_name), id);
