// Package buildinfo provides build metadata for the Governator CLI.
package buildinfo

// Build metadata variables set via linker flags during build.
var (
	// Version is the semantic version string (e.g., "1.2.3").
	Version = "dev"
	// Commit is the git commit SHA (e.g., "8d3f2a1").
	Commit = "unknown"
	// BuiltAt is the build timestamp in RFC3339 format (e.g., "2025-02-14T09:30:00Z").
	BuiltAt = "unknown"
)

// String returns the formatted version information as expected by the CLI contract.
// Format: "version=<semver> commit=<git-sha> built_at=<rfc3339>"
func String() string {
	return "version=" + Version + " commit=" + Commit + " built_at=" + BuiltAt
}
