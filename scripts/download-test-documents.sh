#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
destination="$project_root/testdata/documents/downloaded"
mkdir -p "$destination"

download() {
  local name="$1"
  local url="$2"
  if [[ -s "$destination/$name" ]]; then
    printf 'exists  %s\n' "$name"
  else
    printf 'fetch   %s\n' "$name"
    curl --fail --location --retry 2 --output "$destination/$name" "$url"
  fi
  if [[ "$(head -c 4 "$destination/$name")" != "%PDF" ]]; then
    printf 'downloaded file is not a PDF: %s\n' "$name" >&2
    rm -f "$destination/$name"
    exit 1
  fi
}

download "form-w9.pdf" "https://www.irs.gov/pub/irs-pdf/fw9.pdf"
download "form-i9.pdf" "https://www.uscis.gov/sites/default/files/document/forms/i-9.pdf"
download "closing-disclosure.pdf" "https://files.consumerfinance.gov/f/201403_cfpb_closing-disclosure_cover-H25E.pdf"
download "cms-1500-sample.pdf" "https://www.cms.gov/sites/default/files/repo-new/18/2013_PQRS_sampleCMS1500claim_12-19-2012.pdf"
