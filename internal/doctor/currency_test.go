package doctor

// White-box unit tests for the binary-currency decision. currencyFromBuild is
// pure — it takes the resolved build, a clock, and a threshold — so staleness
// is exercised deterministically without building binaries of varying ages.

import (
	"strings"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/version"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCurrencyFreshBuildOK(t *testing.T) {
	b := version.Build{Version: "0.01.001", Commit: "abc1234", BuildDate: "2026-06-14T00:00:00Z", Known: true}
	c := currencyFromBuild(b, at("2026-06-16T00:00:00Z"), 7)
	if c.Status != statusOK {
		t.Errorf("2-day-old build with 7d threshold: status %q (%s), want ok", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "abc1234") || !strings.Contains(c.Detail, "v0.01.001") {
		t.Errorf("detail should surface version + commit, got %q", c.Detail)
	}
	if !strings.Contains(c.Detail, "built") {
		t.Errorf("detail should surface build age, got %q", c.Detail)
	}
}

func TestCurrencyStaleBuildWarns(t *testing.T) {
	b := version.Build{Version: "0.01.001", Commit: "abc1234", BuildDate: "2026-05-01T00:00:00Z", Known: true}
	c := currencyFromBuild(b, at("2026-06-16T00:00:00Z"), 7)
	if c.Status != statusWarn {
		t.Errorf("46-day-old build with 7d threshold: status %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "stale") {
		t.Errorf("stale warning should say so, got %q", c.Detail)
	}
}

func TestCurrencyDirtyWarns(t *testing.T) {
	// Dirty warns even when freshly built.
	b := version.Build{Version: "0.01.001", Commit: "abc1234", BuildDate: "2026-06-16T00:00:00Z", Dirty: true, Known: true}
	c := currencyFromBuild(b, at("2026-06-16T01:00:00Z"), 7)
	if c.Status != statusWarn {
		t.Errorf("dirty build: status %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "dirty") {
		t.Errorf("dirty warning should say so, got %q", c.Detail)
	}
}

func TestCurrencyUnknownWarns(t *testing.T) {
	b := version.Build{Version: "0.01.001", Known: false}
	c := currencyFromBuild(b, at("2026-06-16T00:00:00Z"), 7)
	if c.Status != statusWarn {
		t.Errorf("unknown identity: status %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "unknown") {
		t.Errorf("unknown-identity warning should say so, got %q", c.Detail)
	}
}

// Currency never fails — capture can still work off an old binary; the point is
// a visible warning, not a broken-pipeline exit code.
func TestCurrencyNeverFails(t *testing.T) {
	cases := []version.Build{
		{Version: "0.01.001", Known: false},
		{Version: "0.01.001", Commit: "abc1234", BuildDate: "2000-01-01T00:00:00Z", Known: true},
		{Version: "0.01.001", Commit: "abc1234", BuildDate: "2026-06-16T00:00:00Z", Dirty: true, Known: true},
	}
	for _, b := range cases {
		if c := currencyFromBuild(b, at("2026-06-16T00:00:00Z"), 7); c.Status == statusFail {
			t.Errorf("currency must never fail; build %+v -> %q", b, c.Status)
		}
	}
}
