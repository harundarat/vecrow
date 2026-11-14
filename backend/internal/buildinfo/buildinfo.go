// Package buildinfo exposes version metadata injected at build time with
// -ldflags "-X github.com/harundarat/vecrow/backend/internal/buildinfo.Version=..."
package buildinfo

var (
	// Version is the release version or git describe output.
	Version = "dev"
	// Commit is the short git commit hash.
	Commit = "none"
)
