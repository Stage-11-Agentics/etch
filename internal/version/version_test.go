package version

import (
	"runtime/debug"
	"testing"
)

// ldflags-injected values must win over the VCS stamp, and a "-dirty" suffix
// must be lifted out of the commit into the Dirty flag.
func TestResolveLdflagsWins(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "ffffffffffffffff"},
		{Key: "vcs.time", Value: "2020-01-01T00:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}
	b := resolve("abc1234", "2026-06-10T12:00:00Z", vcs)

	if b.Commit != "abc1234" {
		t.Errorf("commit = %q, want ldflags value abc1234", b.Commit)
	}
	if b.BuildDate != "2026-06-10T12:00:00Z" {
		t.Errorf("build date = %q, want ldflags value", b.BuildDate)
	}
	if b.Dirty {
		t.Error("clean ldflags commit must not be marked dirty")
	}
	if !b.Known {
		t.Error("a build with commit + date must be Known")
	}
	if b.Version != Version {
		t.Errorf("version = %q, want %q", b.Version, Version)
	}
}

func TestResolveDirtySuffix(t *testing.T) {
	b := resolve("abc1234-dirty", "2026-06-10T12:00:00Z", nil)
	if b.Commit != "abc1234" {
		t.Errorf("commit = %q, want -dirty stripped to abc1234", b.Commit)
	}
	if !b.Dirty {
		t.Error("a -dirty suffix must set Dirty")
	}
}

// With no ldflags values, resolve falls back to the embedded VCS stamp,
// shortening the revision and honoring vcs.modified.
func TestResolveVCSFallback(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
		{Key: "vcs.time", Value: "2026-06-12T08:30:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}
	b := resolve("", "", vcs)

	if b.Commit != "0123456" {
		t.Errorf("commit = %q, want short VCS revision 0123456", b.Commit)
	}
	if b.BuildDate != "2026-06-12T08:30:00Z" {
		t.Errorf("build date = %q, want VCS time", b.BuildDate)
	}
	if !b.Dirty {
		t.Error("vcs.modified=true must set Dirty in the fallback path")
	}
	if !b.Known {
		t.Error("VCS-resolved commit + date must be Known")
	}
}

// No ldflags and no VCS stamp: identity is unknown.
func TestResolveUnknown(t *testing.T) {
	b := resolve("", "", nil)
	if b.Known {
		t.Error("with neither ldflags nor VCS data, Known must be false")
	}
	if b.Commit != "" || b.BuildDate != "" {
		t.Errorf("expected empty identity, got commit=%q date=%q", b.Commit, b.BuildDate)
	}
}

// A partial stamp (commit but no date) is not enough to call the build Known.
func TestResolvePartialIsUnknown(t *testing.T) {
	b := resolve("abc1234", "", nil)
	if b.Known {
		t.Error("commit without a build date must not be Known")
	}
}
