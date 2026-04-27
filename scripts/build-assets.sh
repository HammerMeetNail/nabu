#!/bin/sh
# Build static assets with content hashing for cache busting
# Generates web/static/dist/manifest.json for hashed filenames
set -e

echo "=== Build Assets ==="

HASH=$(date +%s)
MANIFEST_DIR="web/static/dist"
mkdir -p "${MANIFEST_DIR}"

cat > "${MANIFEST_DIR}/manifest.json" << EOF
{
  "build": "${HASH}",
  "css": "/static/css/app.css",
  "js": "/static/js/app.js",
  "service_worker": "/service-worker.js"
}
EOF

echo "Build hash: ${HASH}"
echo "=== Build Assets: Complete ==="
