#!/bin/sh
set -eu

die() {
	printf '%s\n' "workspace development setup: $*" >&2
	exit 1
}

[ "$#" -eq 0 ] || die "usage: scripts/setup-dev.sh"

for tool in chmod dirname ln mkdir mktemp mv readlink rm sed uname; do
	command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done
command -v go >/dev/null 2>&1 || die "Go 1.24 or newer is required"

if ! os_name=$(uname -s 2>/dev/null); then
	die "could not determine the operating system; run this from Linux, macOS, or WSL"
fi
case "$os_name" in
	Linux|linux) expected_go_os=linux ;;
	Darwin|darwin) expected_go_os=darwin ;;
	MSYS_NT*|MINGW*|CYGWIN*|Windows_NT*)
		die "native Windows is unsupported; run the development setup in Windows Terminal's Linux/WSL environment"
		;;
	*)
		die "unsupported operating system '$os_name'; supported hosts are Linux/WSL and macOS"
		;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P) || die "could not locate the scripts directory"
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd -P) || die "could not locate the repository root"
[ -f "$repo_root/go.mod" ] || die "go.mod is missing; '$repo_root' is not the repository root"
[ -d "$repo_root/cmd/workspace" ] || die "cmd/workspace is missing from '$repo_root'"

if ! go_version=$(CDPATH= cd -- "$repo_root" && go version 2>&1); then
	die "could not execute Go: $go_version"
fi
version_parts=$(printf '%s\n' "$go_version" | sed -n 's/.*go\([0-9][0-9]*\)\.\([0-9][0-9]*\).*/\1 \2/p')
[ -n "$version_parts" ] || die "could not determine Go version from '$go_version'; Go 1.24 or newer is required"
go_major=${version_parts%% *}
go_minor=${version_parts#* }
if [ "$go_major" -lt 1 ] || { [ "$go_major" -eq 1 ] && [ "$go_minor" -lt 24 ]; }; then
	die "Go 1.24 or newer is required; found $go_version"
fi

if ! go_os=$(CDPATH= cd -- "$repo_root" && go env GOOS 2>/dev/null); then
	die "could not determine Go's target OS; unset a broken GOOS override and retry"
fi
[ "$go_os" = "$expected_go_os" ] || die "Go is configured for GOOS=$go_os on a $os_name host; unset GOOS or set it to $expected_go_os"

if [ "${WORKSPACE_DEV_BUILD_DIR+x}" = x ]; then
	[ -n "$WORKSPACE_DEV_BUILD_DIR" ] || die "WORKSPACE_DEV_BUILD_DIR must not be empty"
	build_dir_raw=$WORKSPACE_DEV_BUILD_DIR
else
	build_dir_raw=$repo_root/bin
fi

if [ "${WORKSPACE_DEV_INSTALL_DIR+x}" = x ]; then
	[ -n "$WORKSPACE_DEV_INSTALL_DIR" ] || die "WORKSPACE_DEV_INSTALL_DIR must not be empty"
	install_dir_raw=$WORKSPACE_DEV_INSTALL_DIR
else
	home=${HOME:-}
	[ -n "$home" ] || die "HOME is not set; set WORKSPACE_DEV_INSTALL_DIR explicitly"
	install_dir_raw=$home/.local/bin
fi

resolve_dir() {
	raw=$1
	case "$raw" in
		/*) candidate=$raw ;;
		*) candidate="$repo_root/$raw" ;;
	esac
	mkdir -p "$candidate" || die "cannot create directory '$candidate'"
	[ -d "$candidate" ] || die "path '$candidate' is not a directory"
	CDPATH= cd -- "$candidate" && pwd -P
}

build_dir=$(resolve_dir "$build_dir_raw") || die "could not resolve build directory"
install_dir=$(resolve_dir "$install_dir_raw") || die "could not resolve install directory"
artifact_path=$build_dir/workspace
install_path=$install_dir/workspace

if [ -e "$install_path" ] || [ -L "$install_path" ]; then
	if [ ! -L "$install_path" ]; then
		die "refusing to replace existing file '$install_path'; choose another WORKSPACE_DEV_INSTALL_DIR"
	fi
	if ! existing_target=$(readlink "$install_path"); then
		die "could not inspect existing symlink '$install_path'; it was left unchanged"
	fi
	[ "$existing_target" = "$artifact_path" ] || die "refusing to replace unrelated symlink '$install_path' -> '$existing_target'; choose another WORKSPACE_DEV_INSTALL_DIR"
fi

temp_binary=$(mktemp "$build_dir/.workspace.dev.XXXXXX") || die "cannot create a temporary build beside '$artifact_path'"
cleanup() {
	if [ -n "${temp_binary:-}" ]; then
		rm -f "$temp_binary"
	fi
}
trap cleanup EXIT HUP INT TERM

if ! (CDPATH= cd -- "$repo_root" && CGO_ENABLED=0 go build -trimpath -o "$temp_binary" ./cmd/workspace); then
	die "development build failed; existing artifact '$artifact_path' was left unchanged"
fi
[ -f "$temp_binary" ] || die "Go did not produce a development binary; existing artifact '$artifact_path' was left unchanged"
if ! chmod 0755 "$temp_binary"; then
	die "could not make temporary development binary executable; existing artifact '$artifact_path' was left unchanged"
fi
if ! mv -f "$temp_binary" "$artifact_path"; then
	die "could not atomically replace '$artifact_path'; the previous artifact was left unchanged"
fi
temp_binary=

if [ ! -e "$install_path" ] && [ ! -L "$install_path" ]; then
	ln -s "$artifact_path" "$install_path" || die "could not create development command '$install_path'"
fi

printf 'Built development binary: %s\n' "$artifact_path"
printf 'Installed development command: %s -> %s\n' "$install_path" "$artifact_path"
case ":${PATH:-}:" in
	*:"$install_dir":*) ;;
	*) printf 'PATH hint: export PATH="%s:$PATH"\n' "$install_dir" ;;
esac
