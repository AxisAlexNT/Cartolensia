# Encryption And At-Rest Privacy Hardening Plan

## Summary

Cartolensia stores highly sensitive derived data: PostgreSQL metadata, OCR text,
transcripts, captions, thumbnails/previews, embeddings, faces, places, job logs,
exports, and local AI outputs. Anyone with raw filesystem access to the
Cartolensia data directory can learn private information even without original
media access.

Use a layered default that protects sensitive Cartolensia-local data without
requiring users to reinstall the operating system:

- Recommended default: a dedicated encrypted Cartolensia data vault mounted at
  `/var/lib/cartolensia`, backed by LUKS2 plus ext4.
- Use TPM auto-unlock when available, with a recovery passphrase.
- Use `fscrypt` per-directory encryption as the portable fallback when LUKS is
  not practical.
- Avoid FUSE encryption for PostgreSQL by default.
- Keep Cartolensia-side encryption focused on encrypted backups/exports, secret
  handling, diagnostics, and encryption-readiness checks.
- Do not field-encrypt all searchable metadata by default because that would
  break search, OCR, captions, places, transcripts, embeddings, and local query
  planning.

This protects DB metadata, OCR, transcripts, captions, thumbnails/previews,
map/cache data, exports, embeddings, AI outputs, and local config. It does not
protect originals stored on a separate NAS unless that NAS also encrypts its own
disks.

## Options, Pros, Cons

| Option | Pros | Cons | Recommendation |
|---|---|---|---|
| Full OS LUKS | Strong, standard, protects swap/logs/everything | Requires reinstall or invasive migration; boot unlock complexity | Document as best practice, not required |
| Dedicated LUKS2 Cartolensia vault | Strong, standard, low overhead, works with PostgreSQL, isolates corruption to normal FS/DB files | Needs one-time setup and unlock policy | Default production recommendation |
| LUKS2 loopback image | No repartition required; easy backup/move | Image file can fragment; size planning required | Good fallback when no spare partition |
| TPM auto-unlock plus recovery key | No password every boot; non-pro friendly | Protects powered-off theft, not root compromise while running | Default if TPM available |
| Passphrase-only unlock | Simple and secure | Needs password after reboot | Fallback when TPM unavailable |
| fscrypt directory encryption | Low overhead, per-directory, no block device | Requires ext4/f2fs support; key/session management; less universal in containers | Secondary fallback |
| gocryptfs/CryFS | Portable per-file encryption; localized corruption | FUSE overhead and DB durability concerns | Use only for exports/backups, not PostgreSQL |
| PostgreSQL pgcrypto field encryption | Protects selected DB fields from raw DB reads | Breaks indexing/search unless duplicate indexes leak data; complex key rotation | Only for secrets/tokens, not media metadata |
| Encrypted backups only | Easy and essential | Does not protect live DB/cache | Required, but insufficient alone |

## Implementation Plan

### 1. Data Classification And Path Policy

Add a Sensitive Data Map in docs and diagnostics:

- Sensitive:
  - PostgreSQL data directory;
  - `/var/lib/cartolensia/cache`;
  - previews/thumbnails;
  - OCR, captions, transcripts, places, faces, embeddings, AI tags;
  - exports/backups;
  - admin password files, API tokens, Samba credentials;
  - local LLM/chat/knowledge graph data.
- Less sensitive but still private by association:
  - model/component caches;
  - logs;
  - job histories.
- Originals:
  - outside Cartolensia's encryption boundary when mounted from NAS/Samba;
  - must remain strict read-only.

### 2. Recommended Production Vault

Create scripts and docs for:

- `scripts/security/create-cartolensia-vault.sh`
  - creates a LUKS2 encrypted block device or loopback image;
  - formats ext4;
  - mounts at `/var/lib/cartolensia`;
  - sets ownership to `cartolensia`;
  - refuses to operate on originals/Samba paths.
- `scripts/security/enroll-vault-tpm.sh`
  - uses systemd/LUKS2 TPM enrollment when available;
  - always creates or keeps a recovery passphrase.
- `scripts/security/check-vault.sh`
  - verifies `/var/lib/cartolensia` is encrypted;
  - verifies PostgreSQL/cache/export/model/component dirs are inside the vault;
  - verifies `/originals` remains read-only and outside the vault.

Default layout:

```text
/var/lib/cartolensia/
  postgres/
  cache/
  exports/
  models/
  components/
  ai-venv/
  logs/
  secrets/
```

### 3. Cartolensia Readiness UI

Add Settings -> Security / Encryption:

- Data vault encrypted: yes/no/unknown.
- PostgreSQL data inside encrypted vault: yes/no.
- Cache/previews inside encrypted vault: yes/no.
- Exports encrypted by default: yes/no.
- Originals read-only: yes/no.
- Secrets file permissions: ok/warning.
- Swap encryption detected: yes/no/unknown.
- Backups encryption recipient configured: yes/no.

Never show secret contents.

### 4. Encrypted Backup And Export Policy

Add first-class encrypted backup/export support:

- Essential export remains DB/config/metadata only, no originals.
- Add optional encryption:
  - preferred: `age`;
  - fallback: `gpg`;
  - last resort: `7z` AES-256 with passphrase.

Add config:

```yaml
security:
  backup_encryption:
    enabled: true
    provider: age
    recipient_file: /var/lib/cartolensia/secrets/backup-age-recipient.txt
```

Backups should fail closed if encryption is enabled but the recipient is
missing.

### 5. App-Side Secret Encryption

Keep searchable metadata plaintext inside the encrypted vault, but harden true
secrets:

- Hash passwords with a modern password hash.
- Store API token hashes, not token plaintext.
- Redact DB URLs, Samba credentials, token material, and password paths in
  diagnostics.
- Optionally encrypt stored external provider API keys with a local key file
  under `/var/lib/cartolensia/secrets`.

Do not encrypt OCR/captions/transcripts row-by-row in v1, because search and
knowledge graph features depend on indexed text.

### 6. Operational Defaults

Production templates should default to:

- `/var/lib/cartolensia` as encrypted-vault target.
- `persistent_previews: false` unless the user opts in.
- encrypted backups.
- secrets under `/var/lib/cartolensia/secrets`.
- self-signed TLS certificate reused if present and not expired.
- no writes to originals.
- no missing-file marking on unavailable storages.

## Test Plan

Add tests/scripts for:

- vault scripts refuse `/originals`, Samba mounts, and `/mnt/Models/rclone`;
- encrypted-vault detection handles LUKS, fscrypt, and unknown/plain filesystems;
- production config warns when DB/cache/export paths are outside encrypted vault;
- essential export can be encrypted and restored;
- backup encryption fails if enabled but recipient/passphrase is missing;
- diagnostics redact secrets;
- unavailable originals do not trigger deletion or missing-file marking;
- PostgreSQL runs on the encrypted vault path;
- cache cleanup only deletes inside configured Cartolensia cache roots.

Manual production validation:

- reboot with TPM auto-unlock enabled;
- reboot with recovery passphrase fallback;
- confirm app starts after unlock;
- confirm locked vault prevents PostgreSQL/cache access;
- confirm unlocking restores service without reindexing;
- confirm backup archive cannot be read without key/passphrase.

## Assumptions And Defaults

- Default recommendation is LUKS2 encrypted Cartolensia data vault, not full OS
  reinstall.
- TPM auto-unlock is preferred for non-pro users, with a recovery passphrase
  stored offline.
- If TPM is unavailable, use passphrase unlock at boot, not repeated app
  prompts.
- Do not use FUSE encryption for PostgreSQL by default.
- Do not field-encrypt all metadata because Cartolensia's core value depends on
  searchable local metadata.
- Originals on NAS/Samba require separate NAS-side encryption; Cartolensia
  protects only its local derived metadata/cache/export environment.

