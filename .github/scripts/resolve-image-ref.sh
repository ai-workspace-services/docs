#!/usr/bin/env bash
set -euo pipefail

tags_raw="${TAGS_RAW:-}"
image_ref="$(printf '%s\n' "${tags_raw}" | tr ',' '\n' | sed '/^$/d' | grep -E ':sha-[0-9a-f]{40}$|:[0-9a-f]{40}$' | head -n 1)"

if [[ -z "${image_ref}" ]]; then
  echo "failed to resolve image ref from metadata tags" >&2
  exit 1
fi

image_no_digest="${image_ref%@*}"
image_tag="${image_no_digest##*:}"

{
  echo "image_ref=${image_ref}"
  echo "image_tag=${image_tag}"
} >> "$GITHUB_OUTPUT"
