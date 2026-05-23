// Package version holds the build-time version string.
// It is set via -ldflags during the container build:
//
//	go build -ldflags="-X 'github.com/dave/choresy/internal/version.Version=1.2.3'"
package version

// Version is the application version. Defaults to "dev" when not injected at build time.
var Version = "dev"
