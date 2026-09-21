package buildinfo

import "runtime"

// These variables are intentionally simple strings so release builds can set
// them with go build -ldflags -X. Source builds retain useful, explicit
// defaults instead of presenting empty metadata.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is the stable, machine-readable output of workspace version.
type Info struct {
	Version   string `json:"version" yaml:"version"`
	Commit    string `json:"commit" yaml:"commit"`
	BuildDate string `json:"build_date" yaml:"build_date"`
	GOOS      string `json:"goos" yaml:"goos"`
	GOARCH    string `json:"goarch" yaml:"goarch"`
}

func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}
