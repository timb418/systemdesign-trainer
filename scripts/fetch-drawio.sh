#!/usr/bin/env bash
set -euo pipefail
# Apache-2.0 static editor from https://github.com/jgraph/drawio
VERSION="${DRAWIO_VERSION:-v26.2.15}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/third_party/drawio"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo "Downloading draw.io $VERSION…"
curl -fsSL "https://github.com/jgraph/drawio/archive/refs/tags/${VERSION}.tar.gz" | tar -xz -C "$TMP"
SRC="$(echo "$TMP"/drawio-*/src/main/webapp)"
if [[ ! -d "$SRC" ]]; then
  echo "не найден src/main/webapp в архиве" >&2
  exit 1
fi
rm -rf "$DEST"
mkdir -p "$(dirname "$DEST")"
cp -a "$SRC" "$DEST"
echo "Готово: $DEST (откройте приложение — доска с /drawio/)"
