#!/bin/sh
set -eu

die() {
	printf '%s\n' "workspace install: $*" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
command -v awk >/dev/null 2>&1 || die "awk is required"
command -v mktemp >/dev/null 2>&1 || die "mktemp is required"

if command -v sha256sum >/dev/null 2>&1; then
	checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	checksum_tool=shasum
else
	die "sha256sum or shasum -a 256 is required"
fi

raw_os=${WORKSPACE_INSTALL_OS:-$(uname -s)}
raw_arch=${WORKSPACE_INSTALL_ARCH:-$(uname -m)}
case "$raw_os" in
	Linux|linux) target_os=linux ;;
	Darwin|darwin) target_os=darwin ;;
	*) die "unsupported operating system '$raw_os'; supported targets are Linux and macOS (Windows users must run the Linux build in WSL)" ;;
esac
case "$raw_arch" in
	x86_64|amd64) target_arch=amd64 ;;
	aarch64|arm64) target_arch=arm64 ;;
	*) die "unsupported architecture '$raw_arch'; supported architectures are amd64 and arm64" ;;
esac

home=${HOME:-}
[ -n "$home" ] || die "HOME is not set and WORKSPACE_INSTALL_DIR was not supplied"
install_dir=${WORKSPACE_INSTALL_DIR:-"$home/.local/bin"}
[ -n "$install_dir" ] || die "WORKSPACE_INSTALL_DIR must not be empty"
mkdir -p "$install_dir" || die "cannot create install directory '$install_dir'"
[ -d "$install_dir" ] || die "install path '$install_dir' is not a directory"

base=${WORKSPACE_RELEASE_BASE_URL:-https://github.com/fpiechowski/workspace/releases/latest/download}
base=${base%/}
tmp_root=${TMPDIR:-/tmp}
tmp_dir=$(mktemp -d "$tmp_root/workspace-install.XXXXXX") || die "cannot create a private temporary directory"
cleanup() {
	rm -rf "$tmp_dir"
	if [ -n "${staged:-}" ]; then
		rm -f "$staged"
	fi
}
trap cleanup EXIT HUP INT TERM

target="${target_os}_${target_arch}"
curl -fsSL --max-time 60 "$base/checksums.txt" -o "$tmp_dir/checksums.txt" || die "could not download checksums.txt from the latest GitHub Release"

archive_info=$(awk -v target="$target" '
length($1) == 64 && $1 !~ /[^0-9A-Fa-f]/ && NF == 2 {
    pattern = "^workspace_[0-9]+\\.[0-9]+\\.[0-9]+_" target "\\.tar\\.gz$"
    if ($2 ~ pattern) {
        print $1 "\t" $2
        count++
    }
}
END {
    if (count != 1) exit 1
}
' "$tmp_dir/checksums.txt") || die "checksums.txt does not contain exactly one stable archive for $target"

expected_hash=$(printf '%s\n' "$archive_info" | awk -F '\t' '{print $1}')
archive_name=$(printf '%s\n' "$archive_info" | awk -F '\t' '{print $2}')
version=$(printf '%s\n' "$archive_name" | sed -n "s/^workspace_\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)_${target}\.tar\.gz$/\1/p")
[ -n "$version" ] || die "checksums.txt selected an invalid archive name '$archive_name'"

archive_path="$tmp_dir/$archive_name"
curl -fsSL --max-time 120 "$base/$archive_name" -o "$archive_path" || die "could not download $archive_name from the latest GitHub Release"
if [ "$checksum_tool" = sha256sum ]; then
	actual_hash=$(sha256sum "$archive_path" | awk '{print $1}')
else
	actual_hash=$(shasum -a 256 "$archive_path" | awk '{print $1}')
fi
actual_hash=$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')
expected_hash=$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')
[ "$actual_hash" = "$expected_hash" ] || die "SHA-256 checksum verification failed for $archive_name"

entries=$(tar -tzf "$archive_path") || die "release archive '$archive_name' is malformed"
[ "$entries" = "workspace" ] || die "release archive '$archive_name' must contain only a root workspace executable"
mkdir "$tmp_dir/extract"
tar -xzf "$archive_path" -C "$tmp_dir/extract" || die "could not extract $archive_name"
[ -f "$tmp_dir/extract/workspace" ] || die "release archive did not contain a workspace executable"
[ ! -L "$tmp_dir/extract/workspace" ] || die "release archive executable must not be a symlink"
chmod 0755 "$tmp_dir/extract/workspace" || die "could not set executable permissions"

staged=$(mktemp "$install_dir/.workspace.new.XXXXXX") || die "cannot create a staging file in '$install_dir'"
cp "$tmp_dir/extract/workspace" "$staged" || die "could not stage workspace in '$install_dir'"
chmod 0755 "$staged" || die "could not set installed executable permissions"
mv -f "$staged" "$install_dir/workspace" || die "could not atomically install workspace in '$install_dir'"

printf 'Installed workspace v%s to %s/workspace\n' "$version" "$install_dir"
case ":${PATH:-}:" in
	*:"$install_dir":*) ;;
	*) printf 'PATH hint: export PATH="%s:$PATH"\n' "$install_dir" ;;
esac
