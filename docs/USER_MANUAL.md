# User Manual

## Core Navigation

- Explorer: browse folders and assets.
- Discovery: run bounded indexing and metadata extraction.
- Map: inspect geotagged assets and tracks.
- GPS/KML Tracks: browse parsed tracks and track media links.
- Video Track Player: synchronize a video with GPS tracks.
- Base AI / OCR / Captions / Faces / Safety Review: inspect metadata outputs and run manual jobs.
- Components: install or import optional tools and model caches.

## Search

Search is PostgreSQL/local and supports mixed queries.

Examples:

- `ext:mp4`
- `filename:PXL_20260512*`
- `ocr:station`
- `transcript:station`
- `caption:train`
- `place:Yerevan`
- `kind:video mp4`

Search results show match reasons. Multi-term queries are bounded and paginated.

## Asset Detail

Asset detail combines:

- file metadata;
- geotags and reverse-geocoded places;
- OCR blocks and full OCR text;
- captions;
- transcripts;
- audio features;
- faces;
- safety/private state;
- related assets and timeline context.

Middle-click or modifier-click on asset links opens a new tab and keeps the current overlay or page intact.

## Video Track Player

The player can use:

- filename-derived timestamp candidates;
- EXIF or ffprobe timestamps;
- manual start/end/offset settings.

It auto-suggests overlapping tracks when enabled in settings.

## Offline Operations

If a feature is missing, go to Settings -> Components and provide the missing tool or model archive. Cartolensia should not assume Internet access.
