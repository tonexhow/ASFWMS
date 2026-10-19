#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="$ROOT/dist/asfwms"
mkdir -p "$DEST/database" "$DEST/logs" "$DEST/systems/windows" "$DEST/systems/macos" "$DEST/systems/linux"
cp -R "$ROOT/client" "$DEST/"
cp "$ROOT/database/schema.sql" "$DEST/database/schema.sql"
if [[ -f "$ROOT/database/database.db" ]]; then cp "$ROOT/database/database.db" "$DEST/database/database.db"; fi
cp "$ROOT/.env.example" "$DEST/.env.example"
if [[ -f "$ROOT/.env" ]]; then cp "$ROOT/.env" "$DEST/.env"; else cp "$ROOT/.env.example" "$DEST/.env"; fi
cp "$ROOT/README.md" "$DEST/README.md"
(
 cd "$ROOT/server"
 go test ./...
 go vet ./...
 for target in windows/amd64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  os="${target%/*}"; arch="${target#*/}"
  folder="$os"; [[ "$os" == darwin ]] && folder=macos
  if [[ "$os" == windows ]]; then
   env GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w -H=windowsgui' -o "$DEST/systems/windows/asfwms_start.exe" .
   env GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w -H=windowsgui' -o "$DEST/systems/windows/asfwms_stop.exe" ./cmd/stop
  else
   env GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$DEST/systems/$folder/asfwms_start_$arch" .
   env GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$DEST/systems/$folder/asfwms_stop_$arch" ./cmd/stop
   chmod +x "$DEST/systems/$folder/asfwms_start_$arch" "$DEST/systems/$folder/asfwms_stop_$arch"
  fi
 done
)
echo "Release ready: $DEST. Contains private local SQLite and .env; do not publish dist."
