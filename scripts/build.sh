#!/bin/sh
# Puts the lazyherd binary at bin/lazyherd for `herdr plugin install`: the
# release named by the manifest's version, or a Go build of this checkout
# when that release does not exist yet (or cannot be downloaded).
set -eu
cd "$(dirname "$0")/.."
mkdir -p bin

version=$(sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml)
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
esac

download() {
  url="https://github.com/chriopter/lazyherd/releases/download/v$version/lazyherd_${version}_${os}_${arch}.tar.gz"
  echo "lazyherd: downloading $url"
  curl -fsSL -o bin/release.tar.gz "$url" || { rm -f bin/release.tar.gz; return 1; }
  tar -xzf bin/release.tar.gz -C bin lazyherd
  rm -f bin/release.tar.gz
}

build() {
  command -v go >/dev/null || return 1
  echo "lazyherd: building from source"
  go build -ldflags "-s -w -X main.version=$version" -o bin/lazyherd .
}

download || build || {
  echo "lazyherd: no release $version for $os/$arch and no Go toolchain to build one" >&2
  exit 1
}
chmod +x bin/lazyherd
bin/lazyherd -v
