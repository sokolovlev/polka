// Package buildinfo holds the version and commit baked into the binary at
// build time (spec 9.1), served on /version and as the build_info metric.
//
// Set them with -ldflags:
//
//	-X github.com/sokolovlev/polka/libs/go/buildinfo.Version=1.2.3
//	-X github.com/sokolovlev/polka/libs/go/buildinfo.Commit=<sha>
package buildinfo

import "runtime"

// Overridden by -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
)

// Info describes a running binary.
type Info struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
}

// Get returns the build info for the named service.
func Get(service string) Info {
	return Info{
		Service:   service,
		Version:   Version,
		Commit:    Commit,
		GoVersion: runtime.Version(),
	}
}
