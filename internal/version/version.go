// Package version is the single source of truth for Pactum's build version.
//
// The values are overridable at build time with -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/heurema/pactum/internal/version.Version=0.1.0 \
//	  -X github.com/heurema/pactum/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/heurema/pactum/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/pactum
//
// When built without ldflags the defaults below are used.
package version

// Build metadata. Overridable via -ldflags -X.
var (
	Version = "0.1.0"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info is the machine-readable view of the build metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Current returns the build metadata as an Info value.
func Current() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}
