#!/usr/bin/env bash
set -euo pipefail
toolchain_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$toolchain_root"
toolchain_version="$(awk '$1 == "go" {print $2}' go.mod)"
for toolchain_file in Containerfile ops/recovery/Containerfile; do
    toolchain_image="$(awk '$1 == "FROM" && $2 ~ /library\/golang:/ {print $2}' "$toolchain_file")"
    [[ "$toolchain_image" == "docker.io/library/golang:${toolchain_version}-alpine" ]] || {
        echo "$toolchain_file and go.mod use different release compilers" >&2; exit 1;
    }
done
[[ "$(go env GOVERSION)" == "go${toolchain_version}" ]] || {
    echo "Use GOTOOLCHAIN=go${toolchain_version} for release validation" >&2; exit 1;
}
echo "Release compiler alignment verified: go${toolchain_version}"
