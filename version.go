package spoo

import "regexp"

// Version is the SDK release, updated on each tag.
var Version = "dev"

var versionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,16}$`)

// defaultClientTag identifies the SDK (and its version, when
// well-formed) to the backend so API traffic can be attributed per
// client. Apps building on the SDK set their own tag with
// [WithClientTag].
func defaultClientTag() string {
	if versionRe.MatchString(Version) {
		return "sdk-go/" + Version
	}
	return "sdk-go"
}
