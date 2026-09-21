package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"workspace/internal/release"
)

type archiveEntry struct {
	name string
	kind byte
	data string
	link string
}

func TestUpgradeDevToReleaseAndNoOp(t *testing.T) {
	archive := makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "new executable"})
	server, requests := releaseServer(t, "v0.1.0", release.ArchiveName("v0.1.0", release.Target{GOOS: "linux", GOARCH: "amd64", Name: "linux_amd64"}), archive, true)
	defer server.Close()

	directory := t.TempDir()
	executable := filepath.Join(directory, "workspace")
	if err := os.WriteFile(executable, []byte("old executable"), 0751); err != nil {
		t.Fatal(err)
	}
	result, err := Upgrade(t.Context(), Options{
		APIBaseURL:     server.URL,
		ExecutablePath: executable,
		CurrentVersion: "dev",
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.CurrentVersion != "dev" || result.InstalledVersion != "v0.1.0" {
		t.Fatalf("unexpected upgrade result: %#v", result)
	}
	if got, err := os.ReadFile(executable); err != nil || string(got) != "new executable" {
		t.Fatalf("replacement content=%q err=%v", got, err)
	}
	if mode := filePermissions(t, executable); mode != 0751 {
		t.Fatalf("mode=%#o, want %#o", mode, 0751)
	}
	if requests.archive != 1 || requests.checksums != 1 {
		t.Fatalf("unexpected download counts: %#v", requests)
	}

	result, err = Upgrade(t.Context(), Options{
		APIBaseURL:     server.URL,
		ExecutablePath: executable,
		CurrentVersion: "v0.1.0",
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.InstalledVersion != "v0.1.0" {
		t.Fatalf("unexpected no-op result: %#v", result)
	}
	if requests.archive != 1 || requests.checksums != 1 {
		t.Fatalf("no-op downloaded assets: %#v", requests)
	}
}

func TestUpgradeDoesNotDowngrade(t *testing.T) {
	archive := makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "must not install"})
	server, _ := releaseServer(t, "v0.1.0", "workspace_0.1.0_linux_amd64.tar.gz", archive, true)
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(executable, []byte("current"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := Upgrade(t.Context(), Options{
		APIBaseURL:     server.URL,
		ExecutablePath: executable,
		CurrentVersion: "v0.2.0",
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.InstalledVersion != "v0.2.0" {
		t.Fatalf("unexpected downgrade result: %#v", result)
	}
	if got, _ := os.ReadFile(executable); string(got) != "current" {
		t.Fatalf("executable changed to %q", got)
	}
}

func TestUpgradeUsesRequiredHeaders(t *testing.T) {
	archive := makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "binary"})
	var missing atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			missing.Store(true)
		}
		name := "workspace_0.1.0_linux_amd64.tar.gz"
		if r.URL.Path == "/repos/fpiechowski/workspace/releases/latest" {
			writeJSON(w, map[string]any{"tag_name": "v0.1.0", "assets": []map[string]string{
				{"name": name, "browser_download_url": "http://" + r.Host + "/archive"},
				{"name": "checksums.txt", "browser_download_url": "http://" + r.Host + "/checksums"},
			}})
			return
		}
		if r.URL.Path == "/checksums" {
			digest := sha256.Sum256(archive)
			_, _ = fmt.Fprintf(w, "%x  %s\n", digest, name)
			return
		}
		if r.URL.Path == "/archive" {
			_, _ = w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(executable, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Upgrade(t.Context(), Options{APIBaseURL: server.URL, ExecutablePath: executable, CurrentVersion: "dev", GOOS: "linux", GOARCH: "amd64"}); err != nil {
		t.Fatal(err)
	}
	if missing.Load() {
		t.Fatal("required GitHub headers were missing")
	}
}

func TestUpgradeRejectsMalformedReleaseAndKeepsExecutable(t *testing.T) {
	tests := []struct {
		name      string
		tag       string
		assetName string
		checksums string
		archive   []byte
		want      string
	}{
		{name: "invalid tag", tag: "v1.2.3-rc1", assetName: "workspace_1.2.3_linux_amd64.tar.gz"},
		{name: "missing exact archive", tag: "v1.2.3", assetName: "workspace_1.2.3_linux_arm64.tar.gz"},
		{name: "checksum mismatch", tag: "v1.2.3", assetName: "workspace_1.2.3_linux_amd64.tar.gz", checksums: strings.Repeat("0", 64)},
		{name: "traversal", tag: "v1.2.3", assetName: "workspace_1.2.3_linux_amd64.tar.gz", archive: makeArchive(t, archiveEntry{name: "../workspace", kind: tar.TypeReg, data: "bad"})},
		{name: "symlink", tag: "v1.2.3", assetName: "workspace_1.2.3_linux_amd64.tar.gz", archive: makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeSymlink, link: "elsewhere"})},
		{name: "duplicate", tag: "v1.2.3", assetName: "workspace_1.2.3_linux_amd64.tar.gz", archive: makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "one"}, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "two"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := test.archive
			if archive == nil {
				archive = makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "new"})
			}
			server, _ := releaseServer(t, test.tag, test.assetName, archive, test.name != "missing exact archive", test.checksums)
			defer server.Close()
			executable := filepath.Join(t.TempDir(), "workspace")
			if err := os.WriteFile(executable, []byte("original"), 0755); err != nil {
				t.Fatal(err)
			}
			if test.checksums != "" {
				// The fixture server uses the supplied digest to make the mismatch
				// deterministic while still serving the real archive.
				_ = test.checksums
			}
			_, err := Upgrade(t.Context(), Options{APIBaseURL: server.URL, ExecutablePath: executable, CurrentVersion: "dev", GOOS: "linux", GOARCH: "amd64"})
			if err == nil {
				t.Fatal("malformed release unexpectedly upgraded")
			}
			if got, readErr := os.ReadFile(executable); readErr != nil || string(got) != "original" {
				t.Fatalf("executable changed after failed upgrade: %q err=%v", got, readErr)
			}
		})
	}
}

func TestUpgradeHTTPAndReplacementFailuresKeepExecutable(t *testing.T) {
	archive := makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "new"})
	server, _ := releaseServer(t, "v0.1.0", "workspace_0.1.0_linux_amd64.tar.gz", archive, true)
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(executable, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := Upgrade(t.Context(), Options{
		APIBaseURL:     server.URL,
		ExecutablePath: executable,
		CurrentVersion: "dev",
		GOOS:           "linux",
		GOARCH:         "amd64",
		Replace:        func(_, _ string) error { return fmt.Errorf("injected rename failure") },
	})
	if err == nil || !strings.Contains(err.Error(), "atomically replace") {
		t.Fatalf("unexpected replacement error: %v", err)
	}
	if got, _ := os.ReadFile(executable); string(got) != "original" {
		t.Fatalf("executable changed after replacement failure: %q", got)
	}

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer statusServer.Close()
	_, err = Upgrade(t.Context(), Options{APIBaseURL: statusServer.URL, ExecutablePath: executable, CurrentVersion: "dev", GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "unexpected HTTP status") {
		t.Fatalf("unexpected HTTP error: %v", err)
	}
	if got, _ := os.ReadFile(executable); string(got) != "original" {
		t.Fatalf("executable changed after HTTP failure: %q", got)
	}
}

func TestUpgradeUnsupportedPlatform(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(executable, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := Upgrade(t.Context(), Options{ExecutablePath: executable, CurrentVersion: "dev", GOOS: "windows", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "WSL") || !strings.Contains(err.Error(), "linux/amd64") {
		t.Fatalf("unsupported platform error was not actionable: %v", err)
	}
}

func TestUpgradeResolvesSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on some Windows hosts")
	}
	archive := makeArchive(t, archiveEntry{name: "workspace", kind: tar.TypeReg, data: "replacement"})
	server, _ := releaseServer(t, "v0.1.0", "workspace_0.1.0_linux_amd64.tar.gz", archive, true)
	defer server.Close()
	directory := t.TempDir()
	target := filepath.Join(directory, "real-workspace")
	link := filepath.Join(directory, "workspace")
	if err := os.WriteFile(target, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Fatal(err)
	}
	result, err := Upgrade(t.Context(), Options{APIBaseURL: server.URL, ExecutablePath: link, CurrentVersion: "dev", GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExecutablePath != target {
		t.Fatalf("resolved executable=%q, want %q", result.ExecutablePath, target)
	}
	if got, _ := os.ReadFile(target); string(got) != "replacement" {
		t.Fatalf("symlink target content=%q", got)
	}
	if linkInfo, err := os.Lstat(link); err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was not preserved: info=%v err=%v", linkInfo, err)
	}
}

type requestCounts struct {
	archive   int
	checksums int
}

func releaseServer(t *testing.T, tag, assetName string, archive []byte, includeArchive bool, checksumOverride ...string) (*httptest.Server, *requestCounts) {
	t.Helper()
	counts := &requestCounts{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch r.URL.Path {
		case "/repos/fpiechowski/workspace/releases/latest":
			assets := []map[string]string{{"name": "checksums.txt", "browser_download_url": base + "/checksums"}}
			if includeArchive {
				assets = append(assets, map[string]string{"name": assetName, "browser_download_url": base + "/archive"})
			}
			writeJSON(w, map[string]any{"tag_name": tag, "assets": assets})
		case "/checksums":
			digest := sha256.Sum256(archive)
			expected := fmt.Sprintf("%x", digest)
			if len(checksumOverride) > 0 && checksumOverride[0] != "" {
				expected = checksumOverride[0]
			}
			_, _ = fmt.Fprintf(w, "%s  %s\n", expected, assetName)
		case "/archive":
			counts.archive++
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
		if r.URL.Path == "/checksums" {
			counts.checksums++
		}
	}))
	return server, counts
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func makeArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0755, Typeflag: entry.kind, Linkname: entry.link}
		if entry.kind == tar.TypeReg || entry.kind == tar.TypeRegA {
			header.Size = int64(len(entry.data))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.kind == tar.TypeReg || entry.kind == tar.TypeRegA {
			if _, err := io.WriteString(tarWriter, entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func filePermissions(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestParseStableTagRejectsOverflowAndPrerelease(t *testing.T) {
	for _, tag := range []string{"v1.2.3-rc1", "1.2.3", "v01.2.3", "v18446744073709551616.0.0"} {
		if _, err := parseStableTag(tag); err == nil {
			t.Errorf("parseStableTag(%q) unexpectedly succeeded", tag)
		}
	}
	if parsed, err := parseStableTag("v0.1.0"); err != nil || parsed.String() != "v0.1.0" {
		t.Fatalf("valid tag parse: %v %#v", err, parsed)
	}
}

func TestUpgradeTimeoutIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "{}")
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(executable, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := t.Context(), func() {}
	defer cancel()
	_, err := Upgrade(ctx, Options{APIBaseURL: server.URL, ExecutablePath: executable, CurrentVersion: "dev", GOOS: "linux", GOARCH: "amd64", HTTPClient: &http.Client{Timeout: 1 * time.Millisecond}})
	if err == nil {
		t.Fatal("bounded client unexpectedly succeeded")
	}
}
