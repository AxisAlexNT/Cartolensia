#!/usr/bin/env bash
set -euo pipefail

export CARTOLENSIA_SYNTHETIC_ROOT="${CARTOLENSIA_SYNTHETIC_ROOT:-testdata/synthetic_media}"
export CARTOLENSIA_SYNTHETIC_FOLDERS="${CARTOLENSIA_SYNTHETIC_FOLDERS:-20}"
export CARTOLENSIA_SYNTHETIC_FILES_PER_FOLDER="${CARTOLENSIA_SYNTHETIC_FILES_PER_FOLDER:-50}"

start="$(date +%s)"
bash scripts/generate-synthetic-fixture.sh
end="$(date +%s)"
echo "generation_seconds=$((end - start))"
echo "synthetic_root=$CARTOLENSIA_SYNTHETIC_ROOT"
echo "Configure a temporary storage root to this path for a bounded discovery performance run."
