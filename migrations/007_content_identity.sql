alter table contents drop constraint if exists contents_sha512_key;

create unique index if not exists contents_sha512_size_bytes_idx
    on contents(sha512, size_bytes);
