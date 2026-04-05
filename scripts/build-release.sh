#!/bin/sh

set -eu

version="${1:-}"
if [ -z "$version" ]; then
	echo "usage: scripts/build-release.sh <version>" >&2
	exit 1
fi

repo_root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

dist_dir="$repo_root/dist"
app_name="chatbubbles"
cli_name="chatbubbles-cli"

rm -rf "$dist_dir"
mkdir -p "$dist_dir"

ldflags="-X github.com/kacy/chatbubbles/internal/buildinfo.Version=$version"

for arch in amd64 arm64; do
	stage_dir="$dist_dir/${app_name}_${version}_darwin_${arch}"
	mkdir -p "$stage_dir"

	CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -ldflags "$ldflags" -o "$stage_dir/$app_name" ./cmd/chatbubbles
	CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -o "$stage_dir/$cli_name" ./cmd/chatbubbles-cli
	cp "$repo_root/README.md" "$stage_dir/README.md"

	(
		cd "$dist_dir"
		tar -czf "$(basename "$stage_dir").tar.gz" "$(basename "$stage_dir")"
		rm -rf "$(basename "$stage_dir")"
	)
done

(
	cd "$dist_dir"
	shasum -a 256 ./*.tar.gz > checksums.txt
)
