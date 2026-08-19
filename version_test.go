package spoo

import (
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "module absent",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "github.com/other/mod", Version: "v1.0.0"},
			}},
			want: "",
		},
		{
			name: "no deps",
			info: &debug.BuildInfo{},
			want: "",
		},
		{
			name: "module present",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: "github.com/other/mod", Version: "v1.0.0"},
				{Path: modulePath, Version: "v0.5.3"},
			}},
			want: "0.5.3",
		},
		{
			name: "replaced module uses replacement version",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: modulePath, Version: "v0.5.3", Replace: &debug.Module{
					Path: "github.com/fork/spoo-go", Version: "v0.5.4",
				}},
			}},
			want: "0.5.4",
		},
		{
			name: "local replacement reports absent",
			info: &debug.BuildInfo{Deps: []*debug.Module{
				{Path: modulePath, Version: "v0.5.3", Replace: &debug.Module{
					Path: "../spoo-go", Version: "(devel)",
				}},
			}},
			want: "",
		},
	}
	for _, tt := range tests {
		if got := versionFromBuildInfo(tt.info); got != tt.want {
			t.Errorf("%s: versionFromBuildInfo() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNormalizeModuleVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v0.5.3":  "0.5.3",
		"0.5.3":   "0.5.3",
		"(devel)": "",
		"":        "",
	} {
		if got := normalizeModuleVersion(in); got != want {
			t.Errorf("normalizeModuleVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolveVersionFallsBackToDev pins the in-repo behavior: the test
// binary's build info has no spoo-go dependency entry, so resolution
// falls through to "dev". The real-consumption path (a binary that
// depends on the SDK reporting the tagged semver) is exercised by
// downstream consumers such as spoo-cli.
func TestResolveVersionFallsBackToDev(t *testing.T) {
	if v := resolveVersion(); v != "dev" {
		t.Fatalf("resolveVersion() = %q, want dev", v)
	}
}

func TestResolveVersionHonorsOverride(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()
	Version = "9.9.9"
	if v := resolveVersion(); v != "9.9.9" {
		t.Fatalf("resolveVersion() with override = %q, want 9.9.9", v)
	}
}
