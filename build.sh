#!/bin/sh
# Builds the frontend, embeds it into internal/webui/dist, then builds the
# single julius binary. See README.md "Run the production build (single binary)".
set -eu

cd "$(dirname "$0")"

(cd frontend && npm install && npm run build)

rm -rf internal/webui/dist
mkdir -p internal/webui/dist
cp -r frontend/dist/* internal/webui/dist/

go build -tags embedded_frontend -o julius ./cmd/julius

echo "build ok: ./julius"
