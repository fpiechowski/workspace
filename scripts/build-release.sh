#!/bin/sh
set -eu

die() {
	printf '%s\n' "release build: $*" >&2
	exit 1
}

[ "$#" -eq 2 ] || die "usage: scripts/build-release.sh vMAJOR.MINOR.PATCH OUTPUT_DIR"
tag=$1
output=$2

printf '%s\n' "$tag" | awk '
function component(value) { return value == "0" || value ~ /^[1-9][0-9]*$/ }
{
    if ($0 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/) exit 1
    split(substr($0, 2), parts, ".")
    if (!component(parts[1]) || !component(parts[2]) || !component(parts[3])) exit 1
}
' || die "version must be a stable vMAJOR.MINOR.PATCH tag"

[ -f go.mod ] || die "go.mod is missing; run this script from the repository root"
[ -d cmd/workspace ] || die "cmd/workspace is missing"
command -v go >/dev/null 2>&1 || die "Go is required to build release binaries"
command -v tar >/dev/null 2>&1 || die "tar is required to package release binaries"
if command -v sha256sum >/dev/null 2>&1; then
	checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	checksum_tool=shasum
else
	die "sha256sum or shasum -a 256 is required"
fi

if [ -e "$output" ] && [ ! -d "$output" ]; then
	die "output path '$output' is not a directory"
fi
mkdir -p "$output" || die "cannot create output directory '$output'"
if find "$output" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
	die "output directory '$output' is not empty"
fi

version=${tag#v}
if commit=$(git -c safe.directory="$(pwd)" rev-parse HEAD 2>/dev/null); then
	:
else
	commit=unknown
fi
build_date=$(date -u '+%Y-%m-%dT%H:%M:%SZ') || die "could not determine build date"
ldflags="-s -w -X workspace/internal/buildinfo.Version=$tag -X workspace/internal/buildinfo.Commit=$commit -X workspace/internal/buildinfo.BuildDate=$build_date"
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/workspace-release.XXXXXX") || die "cannot create build staging directory"
cleanup() {
	rm -rf "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

# Keep the target list explicit and auditable: OS, architecture and archive
# label are mapped together in the case statement below.
for target in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
	case "$target" in
		linux_amd64) goos=linux; goarch=amd64 ;;
		linux_arm64) goos=linux; goarch=arm64 ;;
		darwin_amd64) goos=darwin; goarch=amd64 ;;
		darwin_arm64) goos=darwin; goarch=arm64 ;;
		*) die "unknown release target '$target'" ;;
	esac
	artifact="$output/workspace_${version}_${target}.tar.gz"
	[ ! -e "$artifact" ] || die "duplicate release asset '$artifact'"
	stage="$temp_dir/$target"
	mkdir -p "$stage"
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$stage/workspace" ./cmd/workspace || die "failed to build $goos/$goarch"
	[ -f "$stage/workspace" ] || die "build did not produce $stage/workspace"
	chmod 0755 "$stage/workspace"
	tar -czf "$artifact" -C "$stage" workspace || die "failed to package $target"
done

if [ "$checksum_tool" = sha256sum ]; then
	(cd "$output" && sha256sum workspace_*.tar.gz > checksums.txt) || die "could not write checksums.txt"
else
	(cd "$output" && shasum -a 256 workspace_*.tar.gz > checksums.txt) || die "could not write checksums.txt"
fi

archive_count=$(find "$output" -mindepth 1 -maxdepth 1 -type f -name 'workspace_*.tar.gz' | wc -l | tr -d '[:space:]')
[ "$archive_count" = 4 ] || die "expected four release archives, found $archive_count"
checksum_count=$(wc -l < "$output/checksums.txt" | tr -d '[:space:]')
[ "$checksum_count" = 4 ] || die "expected four checksum entries, found $checksum_count"
printf 'Built release %s in %s\n' "$tag" "$output"
