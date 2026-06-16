// Package version carries the binary's build identity. Version is the static,
// human-facing Stage11 form; Commit and BuildDate are stamped at build time via
// -ldflags (see the Makefile). When those are unstamped — a `go install`,
// `go run`, `go test`, or a hand `go build` — BuildInfo falls back to the VCS
// metadata the Go toolchain embeds, so the binary can still report where it
// came from. This is what lets `doctor` tell a current binary from a stale one
// (the c11 load-pilot ran a days-old build silently because the binary carried
// no build identity at all).
package version

import (
	"runtime/debug"
	"strings"
)

// Version is the canonical human-facing Stage11 version (X.XX.XXX form). It is
// intentionally static — it does not move per build, which is exactly why a
// version string alone cannot distinguish a fresh binary from a stale one.
const Version = "0.01.001"

// Commit and BuildDate are injected at build time via:
//
//	-ldflags "-X .../internal/version.Commit=<short-sha>[-dirty] -X .../internal/version.BuildDate=<rfc3339>"
//
// They are empty in unstamped builds; BuildInfo fills the gap from the VCS
// stamp. A "-dirty" suffix on Commit marks a build off an uncommitted tree.
var (
	Commit    = ""
	BuildDate = ""
)

// Build is the resolved build identity of this binary.
type Build struct {
	Version   string // static human-facing version
	Commit    string // short revision, "-dirty" suffix stripped into Dirty
	BuildDate string // RFC3339 UTC build/commit time, or "" if unknown
	Dirty     bool   // built from a modified (uncommitted) tree
	Known     bool   // a real commit AND date were resolved from some source
}

// BuildInfo resolves this binary's identity: ldflags-injected values win, with
// the Go VCS stamp as fallback for anything left empty.
func BuildInfo() Build {
	return resolve(Commit, BuildDate, vcsSettings())
}

// vcsSettings returns the embedded build settings, or nil when unavailable.
func vcsSettings() []debug.BuildSetting {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Settings
	}
	return nil
}

// resolve is the pure core of BuildInfo, split out so the ldflags-win and
// VCS-fallback paths are unit-testable without a real build.
func resolve(commit, buildDate string, settings []debug.BuildSetting) Build {
	b := Build{Version: Version, Commit: commit, BuildDate: buildDate}

	// A "-dirty" suffix is the Makefile's clean-tree signal; lift it into Dirty.
	if strings.HasSuffix(b.Commit, "-dirty") {
		b.Commit = strings.TrimSuffix(b.Commit, "-dirty")
		b.Dirty = true
	}

	// Fill any gap from the VCS stamp the toolchain embeds.
	if b.Commit == "" || b.BuildDate == "" {
		var rev, vtime string
		var modified bool
		for _, s := range settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.time":
				vtime = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if b.Commit == "" && rev != "" {
			b.Commit = shortRev(rev)
			b.Dirty = b.Dirty || modified
		}
		if b.BuildDate == "" && vtime != "" {
			b.BuildDate = vtime
		}
	}

	b.Known = b.Commit != "" && b.BuildDate != ""
	return b
}

// shortRev truncates a full git revision to the conventional 7-char short form.
func shortRev(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
