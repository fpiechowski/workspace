package release

import (
	"fmt"
	"strings"
)

// Repository is the single source of truth for the public distribution
// identity. Keep release URLs derived from it so the updater and installer
// cannot silently drift to different projects.
const Repository = "fpiechowski/workspace"

const (
	GitHubAPIBaseURL = "https://api.github.com"
	GitHubReleaseURL = "https://github.com/" + Repository + "/releases"
)

type Target struct {
	GOOS   string
	GOARCH string
	Name   string
}

var supportedTargets = []Target{
	{GOOS: "linux", GOARCH: "amd64", Name: "linux_amd64"},
	{GOOS: "linux", GOARCH: "arm64", Name: "linux_arm64"},
	{GOOS: "darwin", GOARCH: "amd64", Name: "darwin_amd64"},
	{GOOS: "darwin", GOARCH: "arm64", Name: "darwin_arm64"},
}

func SupportedTargets() []Target {
	return append([]Target(nil), supportedTargets...)
}

func TargetFor(goos, goarch string) (Target, bool) {
	for _, target := range supportedTargets {
		if target.GOOS == goos && target.GOARCH == goarch {
			return target, true
		}
	}
	return Target{}, false
}

func SupportedTargetNames() string {
	names := make([]string, 0, len(supportedTargets))
	for _, target := range supportedTargets {
		names = append(names, target.GOOS+"/"+target.GOARCH)
	}
	return strings.Join(names, ", ")
}

func ArchiveName(version string, target Target) string {
	return fmt.Sprintf("workspace_%s_%s.tar.gz", strings.TrimPrefix(version, "v"), target.Name)
}
