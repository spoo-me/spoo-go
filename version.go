package spoo

import (
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
)

// modulePath is the SDK's own module path, looked up in the consuming
// binary's build info to discover the release version at runtime.
const modulePath = "github.com/spoo-me/spoo-go"

// Version overrides the version reported in the default X-Spoo-Client
// tag. It is normally left at its zero state ("dev"): a Go library has
// no build step, so instead of hand-bumping a constant on every tag,
// the SDK resolves its release version at runtime from the consuming
// binary's build info (see resolveVersion). Set Version to any other
// value to force what the default tag reports.
var Version = "dev"

var versionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,16}$`)

// buildInfoVersion caches the one-time build info scan. The module
// version cannot change during a process lifetime, so the scan runs
// once and the result is reused for every client.
var buildInfoVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return versionFromBuildInfo(info)
})

// versionFromBuildInfo scans a binary's module dependency list for the
// SDK and returns its bare semver ("0.5.3"), or "" when the SDK is not
// present as a dependency (the SDK's own tests, vendored builds
// stripped of module info).
func versionFromBuildInfo(info *debug.BuildInfo) string {
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		v := dep.Version
		if dep.Replace != nil {
			v = dep.Replace.Version
		}
		return normalizeModuleVersion(v)
	}
	return ""
}

// normalizeModuleVersion converts a Go module version ("v0.5.3") to
// the bare form the X-Spoo-Client tag carries ("0.5.3", matching what
// sdk-ts sends). Placeholder versions from local replacements report
// as absent.
func normalizeModuleVersion(v string) string {
	if v == "" || v == "(devel)" {
		return ""
	}
	return strings.TrimPrefix(v, "v")
}

// resolveVersion picks the version for the default client tag: an
// explicit Version override wins, then the release version recorded in
// the consuming binary's build info, then "dev".
func resolveVersion() string {
	if Version != "dev" {
		return Version
	}
	if v := buildInfoVersion(); v != "" {
		return v
	}
	return "dev"
}

// defaultClientTag identifies the SDK (and its version, when
// well-formed) to the backend so API traffic can be attributed per
// client. Apps building on the SDK set their own tag with
// option.WithClientTag.
func defaultClientTag() string {
	if v := resolveVersion(); versionRe.MatchString(v) {
		return "sdk-go/" + v
	}
	return "sdk-go"
}
