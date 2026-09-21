// Package upgrade implements release discovery, verification and atomic
// replacement for the standalone workspace executable. It deliberately does
// not import the workspace domain or resolve a project directory.
package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"workspace/internal/buildinfo"
	"workspace/internal/release"
)

const (
	defaultHTTPTimeout       = 30 * time.Second
	defaultMaxAPIBytes       = 1 << 20
	defaultMaxArchiveBytes   = 128 << 20
	defaultMaxExecutableSize = 64 << 20
)

var (
	upgradeMu   sync.Mutex
	stableTagRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

type Options struct {
	HTTPClient        *http.Client
	APIBaseURL        string
	ExecutablePath    string
	CurrentVersion    string
	GOOS              string
	GOARCH            string
	MaxAPIBytes       int64
	MaxArchiveBytes   int64
	MaxExecutableSize int64

	// Replace is a test seam for simulating atomic replacement failures. A
	// production caller should leave it nil so os.Rename is used.
	Replace func(source, destination string) error
}

type Result struct {
	CurrentVersion   string `json:"current_version" yaml:"current_version"`
	InstalledVersion string `json:"installed_version" yaml:"installed_version"`
	Updated          bool   `json:"updated" yaml:"updated"`
	ExecutablePath   string `json:"executable_path" yaml:"executable_path"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type stableVersion struct {
	Major uint64
	Minor uint64
	Patch uint64
}

func (v stableVersion) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v stableVersion) Compare(other stableVersion) int {
	for _, pair := range [][2]uint64{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func Upgrade(ctx context.Context, opts Options) (Result, error) {
	current := opts.CurrentVersion
	if current == "" {
		current = buildinfo.Version
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	target, ok := release.TargetFor(goos, goarch)
	if !ok {
		return Result{}, unsupportedPlatformError(goos, goarch)
	}

	executable, err := resolveExecutablePath(opts.ExecutablePath)
	if err != nil {
		return Result{}, err
	}

	releaseInfo, err := fetchLatest(ctx, opts)
	if err != nil {
		return Result{}, err
	}
	latest, err := parseStableTag(releaseInfo.TagName)
	if err != nil {
		return Result{}, fmt.Errorf("latest GitHub release has invalid tag %q: %w", releaseInfo.TagName, err)
	}

	result := Result{CurrentVersion: current, InstalledVersion: latest.String(), ExecutablePath: executable}
	if currentVersion, ok := parseCurrentVersion(current); ok && currentVersion.Compare(latest) >= 0 {
		result.InstalledVersion = currentVersion.String()
		return result, nil
	} else if !ok && !isDevelopmentVersion(current) {
		return Result{}, fmt.Errorf("current workspace version %q is not a stable vMAJOR.MINOR.PATCH version", current)
	}

	// The process-wide mutex avoids duplicate work in one process. The lock
	// file extends the same guarantee to two workspace processes sharing an
	// installation directory. Both are acquired only after the no-op decision,
	// so a read-only installation can still report updated: false successfully.
	upgradeMu.Lock()
	defer upgradeMu.Unlock()
	unlock, err := lockDestination(filepath.Dir(executable))
	if err != nil {
		return Result{}, err
	}
	defer unlock()

	archiveName := release.ArchiveName(latest.String(), target)
	archiveAsset, err := exactAsset(releaseInfo.Assets, archiveName)
	if err != nil {
		return Result{}, err
	}
	checksumAsset, err := exactAsset(releaseInfo.Assets, "checksums.txt")
	if err != nil {
		return Result{}, err
	}

	checksumBody, err := downloadBytes(ctx, opts, checksumAsset.BrowserDownloadURL, "checksums.txt", maxOrDefault(opts.MaxAPIBytes, defaultMaxAPIBytes))
	if err != nil {
		return Result{}, err
	}
	checksums, err := parseChecksums(checksumBody)
	if err != nil {
		return Result{}, fmt.Errorf("invalid checksums.txt: %w", err)
	}
	expected, ok := checksums[archiveName]
	if !ok {
		return Result{}, fmt.Errorf("checksums.txt has no exact entry for %s", archiveName)
	}

	archivePath, err := downloadFile(ctx, opts, archiveAsset.BrowserDownloadURL, "release archive", maxOrDefault(opts.MaxArchiveBytes, defaultMaxArchiveBytes))
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(archivePath)

	actual, err := sha256File(archivePath)
	if err != nil {
		return Result{}, fmt.Errorf("hash release archive: %w", err)
	}
	if !strings.EqualFold(expected, actual) {
		return Result{}, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", archiveName, expected, actual)
	}

	executableBytes, err := extractExecutable(archivePath, maxOrDefault(opts.MaxExecutableSize, defaultMaxExecutableSize))
	if err != nil {
		return Result{}, err
	}
	if err := replaceExecutable(filepath.Dir(executable), executable, executableBytes, fileMode(executable), opts.Replace); err != nil {
		return Result{}, err
	}
	result.Updated = true
	return result, nil
}

func maxOrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func unsupportedPlatformError(goos, goarch string) error {
	message := fmt.Sprintf("runtime target %s/%s is not supported; supported release targets are %s", goos, goarch, release.SupportedTargetNames())
	if goos == "windows" {
		message += "; native Windows runtime binaries are not published, run the Linux build inside WSL"
	}
	return fmt.Errorf("unsupported runtime target: %s", message)
}

func resolveExecutablePath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve running workspace executable: %w", err)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve workspace executable symlink %q: %w", abs, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve workspace executable target: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat running workspace executable %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("running workspace executable %q is not a regular file", resolved)
	}
	return resolved, nil
}

func lockDestination(directory string) (func(), error) {
	path := filepath.Join(directory, ".workspace-upgrade.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("another workspace upgrade is already in progress in %s", directory)
		}
		return nil, fmt.Errorf("lock workspace executable directory %q: %w", directory, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close workspace upgrade lock: %w", err)
	}
	return func() { _ = os.Remove(path) }, nil
}

func fetchLatest(ctx context.Context, opts Options) (githubRelease, error) {
	base := strings.TrimRight(opts.APIBaseURL, "/")
	if base == "" {
		base = release.GitHubAPIBaseURL
	}
	url := base + "/repos/" + release.Repository + "/releases/latest"
	body, err := requestBytes(ctx, opts, url, "GitHub latest release", "application/vnd.github+json", maxOrDefault(opts.MaxAPIBytes, defaultMaxAPIBytes))
	if err != nil {
		return githubRelease{}, err
	}
	var info githubRelease
	if err := json.Unmarshal(body, &info); err != nil {
		return githubRelease{}, fmt.Errorf("decode GitHub latest release: %w", err)
	}
	if strings.TrimSpace(info.TagName) == "" {
		return githubRelease{}, errors.New("GitHub latest release did not include tag_name")
	}
	if len(info.Assets) == 0 {
		return githubRelease{}, fmt.Errorf("GitHub latest release %q has no downloadable assets", info.TagName)
	}
	return info, nil
}

func requestBytes(ctx context.Context, opts Options, url, label, accept string, limit int64) ([]byte, error) {
	client := httpClient(opts.HTTPClient)
	requestCtx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", label, err)
	}
	req.Header.Set("User-Agent", "workspace-upgrader/"+buildinfo.Version)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download %s: unexpected HTTP status %s", label, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds the %d-byte safety limit", label, limit)
	}
	return data, nil
}

func downloadBytes(ctx context.Context, opts Options, url, label string, limit int64) ([]byte, error) {
	return requestBytes(ctx, opts, url, label, "application/octet-stream", limit)
}

func downloadFile(ctx context.Context, opts Options, url, label string, limit int64) (string, error) {
	client := httpClient(opts.HTTPClient)
	requestCtx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create %s request: %w", label, err)
	}
	req.Header.Set("User-Agent", "workspace-upgrader/"+buildinfo.Version)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download %s: unexpected HTTP status %s", label, resp.Status)
	}
	file, err := os.CreateTemp("", "workspace-upgrade-archive-*")
	if err != nil {
		return "", fmt.Errorf("create temporary %s: %w", label, err)
	}
	path := file.Name()
	remove := func() { _ = os.Remove(path) }
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, limit+1))
	if copyErr != nil {
		_ = file.Close()
		remove()
		return "", fmt.Errorf("read %s: %w", label, copyErr)
	}
	if written > limit {
		_ = file.Close()
		remove()
		return "", fmt.Errorf("%s exceeds the %d-byte safety limit", label, limit)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		remove()
		return "", fmt.Errorf("sync temporary %s: %w", label, err)
	}
	if err := file.Close(); err != nil {
		remove()
		return "", fmt.Errorf("close temporary %s: %w", label, err)
	}
	return path, nil
}

func httpClient(input *http.Client) *http.Client {
	if input == nil {
		return &http.Client{Timeout: defaultHTTPTimeout}
	}
	client := *input
	if client.Timeout <= 0 || client.Timeout > defaultHTTPTimeout {
		client.Timeout = defaultHTTPTimeout
	}
	return &client
}

func parseStableTag(tag string) (stableVersion, error) {
	matches := stableTagRE.FindStringSubmatch(tag)
	if len(matches) != 4 {
		return stableVersion{}, fmt.Errorf("expected vMAJOR.MINOR.PATCH")
	}
	values := make([]uint64, 3)
	for i := range values {
		value, err := strconv.ParseUint(matches[i+1], 10, 64)
		if err != nil {
			return stableVersion{}, fmt.Errorf("component %q is not a valid integer: %w", matches[i+1], err)
		}
		values[i] = value
	}
	return stableVersion{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func parseCurrentVersion(version string) (stableVersion, bool) {
	parsed, err := parseStableTag(version)
	return parsed, err == nil
}

func isDevelopmentVersion(version string) bool {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "", "dev", "unknown":
		return true
	default:
		return false
	}
}

func exactAsset(assets []githubAsset, name string) (githubAsset, error) {
	var match githubAsset
	count := 0
	for _, asset := range assets {
		if asset.Name == name {
			count++
			match = asset
		}
	}
	if count == 0 {
		return githubAsset{}, fmt.Errorf("latest release is missing exact asset %q", name)
	}
	if count != 1 {
		return githubAsset{}, fmt.Errorf("latest release contains %d assets named %q; refusing ambiguous download", count, name)
	}
	if strings.TrimSpace(match.BrowserDownloadURL) == "" {
		return githubAsset{}, fmt.Errorf("latest release asset %q has no download URL", name)
	}
	return match, nil
}

func parseChecksums(body []byte) (map[string]string, error) {
	checksums := make(map[string]string)
	for lineNumber, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("line %d is not a SHA-256 filename entry", lineNumber+1)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("line %d has an invalid SHA-256 digest", lineNumber+1)
		}
		prefix := fields[0]
		remainder := strings.TrimSpace(line[len(prefix):])
		filename := strings.TrimSpace(remainder)
		if filename == "" {
			return nil, fmt.Errorf("line %d has an empty filename", lineNumber+1)
		}
		if _, exists := checksums[filename]; exists {
			return nil, fmt.Errorf("line %d duplicates filename %q", lineNumber+1, filename)
		}
		checksums[filename] = strings.ToLower(prefix)
	}
	if len(checksums) == 0 {
		return nil, errors.New("manifest is empty")
	}
	return checksums, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extractExecutable(path string, maxSize int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open downloaded release archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open release archive gzip stream: %w", err)
	}
	gzipReader.Multistream(false)
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var executable []byte
	entries := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		entries++
		if header.Name != "workspace" {
			return nil, fmt.Errorf("release archive contains unexpected path %q; expected only workspace", header.Name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("release archive workspace entry is not a regular file (type %d)", header.Typeflag)
		}
		if entries > 1 {
			return nil, errors.New("release archive contains duplicate workspace executable entries")
		}
		if header.Size <= 0 || header.Size > maxSize {
			return nil, fmt.Errorf("release executable size %d is outside the allowed range", header.Size)
		}
		limited := io.LimitReader(tarReader, maxSize+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return nil, fmt.Errorf("read release executable: %w", err)
		}
		if int64(len(data)) != header.Size {
			return nil, fmt.Errorf("release executable entry size mismatch: header=%d data=%d", header.Size, len(data))
		}
		executable = data
	}
	if entries != 1 || len(executable) == 0 {
		return nil, errors.New("release archive must contain exactly one non-empty workspace executable")
	}
	return executable, nil
}

func fileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() == 0 {
		return 0755
	}
	return info.Mode().Perm()
}

func replaceExecutable(directory, destination string, executable []byte, mode os.FileMode, replace func(string, string) error) error {
	stage, err := os.CreateTemp(directory, ".workspace-upgrade-*")
	if err != nil {
		return fmt.Errorf("stage upgraded workspace executable: %w", err)
	}
	stagePath := stage.Name()
	defer os.Remove(stagePath)
	if err := stage.Chmod(mode); err != nil {
		_ = stage.Close()
		return fmt.Errorf("preserve workspace executable permissions: %w", err)
	}
	if _, err := stage.Write(executable); err != nil {
		_ = stage.Close()
		return fmt.Errorf("write staged workspace executable: %w", err)
	}
	if err := stage.Sync(); err != nil {
		_ = stage.Close()
		return fmt.Errorf("sync staged workspace executable: %w", err)
	}
	if err := stage.Close(); err != nil {
		return fmt.Errorf("close staged workspace executable: %w", err)
	}
	if replace == nil {
		replace = os.Rename
	}
	if err := replace(stagePath, destination); err != nil {
		return fmt.Errorf("atomically replace workspace executable: %w", err)
	}
	// Directory fsync is best effort because some supported filesystems do not
	// expose it. The executable itself was synced before the atomic rename.
	if directoryFile, err := os.Open(directory); err == nil {
		_ = directoryFile.Sync()
		_ = directoryFile.Close()
	}
	return nil
}
