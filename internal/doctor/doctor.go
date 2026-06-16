// Package doctor implements `entire-agent-etch doctor` — the one-command
// answer to "is Etch actually working in this repo?" (ETCH-46, the
// rollout's standing risk: capture silently breaks — binary moved, hooks
// dropped, recovery stuck). Read-only by construction: doctor never writes.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/config"
	"github.com/Stage-11-Agentics/etch/internal/enable"
	"github.com/Stage-11-Agentics/etch/internal/install"
	"github.com/Stage-11-Agentics/etch/internal/recovery"
	"github.com/Stage-11-Agentics/etch/internal/version"
)

// Check statuses. fail makes doctor exit non-zero; warn and info never do.
const (
	statusOK   = "ok"
	statusInfo = "info"
	statusWarn = "warn"
	statusFail = "fail"
)

type check struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type report struct {
	Repo     string           `json:"repo"`
	Healthy  bool             `json:"healthy"`
	Warnings bool             `json:"warnings"`
	Checks   map[string]check `json:"checks"`
}

// checkOrder fixes the human-output row order (the JSON map is unordered).
var checkOrder = []string{
	"binary", "currency", "enablement", "hooks", "refspec",
	"sessions", "wip-buffers", "stamps", "propagation", "dedupe",
}

const defaultWarnAgeDays = 14

// defaultBinaryStaleDays is the age past which the installed binary is flagged
// as possibly stale. Deliberately tighter than the session warn-age: a build
// that's a week behind has likely missed capture/ingestion changes (the c11
// pilot ran a days-old build that silently dropped ~188 Codex sessions).
const defaultBinaryStaleDays = 7

// Run implements the `doctor` subcommand.
func Run(args []string) error {
	jsonOut := false
	warnAge := defaultWarnAgeDays
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--warn-age":
			if i+1 >= len(args) {
				return fmt.Errorf("--warn-age requires a value (days)")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return fmt.Errorf("--warn-age: invalid day count %q", args[i+1])
			}
			warnAge = n
			i++
		default:
			return fmt.Errorf("unknown flag %q (doctor accepts --json and --warn-age <days>)", args[i])
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, common, ok := enable.RepoDirs(cwd)
	if !ok {
		return fmt.Errorf("not inside a git repository (cwd=%s)", cwd)
	}
	stateRoot := filepath.Dir(common)

	r := report{Repo: root, Checks: map[string]check{}}
	keyState := enable.KeyState(cwd)

	r.Checks["binary"] = checkBinary()
	r.Checks["currency"] = checkCurrency(time.Now(), defaultBinaryStaleDays)
	r.Checks["enablement"] = checkEnablement(keyState)
	r.Checks["hooks"] = checkHooks(root, keyState)
	r.Checks["refspec"] = checkRefspec(cwd)
	r.Checks["sessions"] = checkSessions(cwd, warnAge)
	r.Checks["wip-buffers"] = checkWips(stateRoot)
	stamps, dedupe := checkWorktrees(cwd, keyState)
	r.Checks["stamps"] = stamps
	r.Checks["dedupe"] = dedupe
	r.Checks["propagation"] = checkPropagation(cwd, keyState)

	failed := 0
	for _, c := range r.Checks {
		switch c.Status {
		case statusFail:
			failed++
		case statusWarn:
			r.Warnings = true
		}
	}
	r.Healthy = failed == 0

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			return err
		}
	} else {
		printHuman(r)
	}

	if failed > 0 {
		return fmt.Errorf("doctor: %d check(s) failed", failed)
	}
	return nil
}

func printHuman(r report) {
	glyph := map[string]string{statusOK: "✓", statusInfo: "•", statusWarn: "!", statusFail: "✗"}
	fmt.Printf("etch doctor — %s\n", r.Repo)
	for _, name := range checkOrder {
		c, ok := r.Checks[name]
		if !ok {
			continue
		}
		fmt.Printf("  %s %-12s %s\n", glyph[c.Status], name, c.Detail)
	}
	switch {
	case !r.Healthy:
		fmt.Println("FAIL — capture is broken here; fix the ✗ lines")
	case r.Warnings:
		fmt.Println("warnings — capture is working but check the ! lines")
	default:
		fmt.Println("ok — capture healthy")
	}
}

// checkBinary: is entire-agent-etch resolvable on PATH, and is it the same
// build as the one running? FAIL when not on PATH — installed hooks
// dispatch by name, so capture is dead without it. Compares commit, not just
// the static version: the version string never moves, so a PATH binary built
// from a different commit looks identical on version alone (exactly the gap
// that hid the stale c11 pilot build).
func checkBinary() check {
	onPath, err := exec.LookPath("entire-agent-etch")
	if err != nil {
		return check{statusFail, "entire-agent-etch is not on PATH — installed hooks cannot dispatch"}
	}
	b := version.BuildInfo()
	selfDesc := buildDesc(b.Version, b.Commit)
	self, err := os.Executable()
	if err != nil {
		return check{statusOK, fmt.Sprintf("on PATH at %s (%s)", onPath, selfDesc)}
	}
	resolvedPath, _ := filepath.EvalSymlinks(onPath)
	resolvedSelf, _ := filepath.EvalSymlinks(self)
	if resolvedPath == resolvedSelf {
		return check{statusOK, fmt.Sprintf("on PATH at %s (%s, this build)", onPath, selfDesc)}
	}
	// Different file: ask the PATH binary for its identity.
	pathVersion, pathCommit := identOf(onPath)
	switch {
	case pathVersion == b.Version && pathCommit == b.Commit:
		return check{statusOK, fmt.Sprintf("on PATH at %s (%s; invoked from %s, same build)", onPath, selfDesc, self)}
	case pathVersion == b.Version:
		// Same version, different commit — the static version masks a real
		// difference. Whichever the hooks call (the PATH build) is what matters.
		return check{statusWarn, fmt.Sprintf("PATH binary %s is %s but this invocation is %s — same version, different build; hooks use the PATH build", onPath, buildDesc(pathVersion, pathCommit), selfDesc)}
	default:
		return check{statusWarn, fmt.Sprintf("PATH resolves to %s (%s) but this invocation is %s — hooks will use the PATH build", onPath, buildDesc(pathVersion, pathCommit), selfDesc)}
	}
}

// buildDesc renders a binary's identity as "v<version> <commit>", omitting the
// commit when unknown.
func buildDesc(ver, commit string) string {
	if commit == "" {
		return "v" + ver
	}
	return "v" + ver + " " + commit
}

// identOf execs a binary's `info` and extracts its version and commit.
func identOf(bin string) (version, commit string) {
	out, err := exec.Command(bin, "info").Output() //nolint:gosec // PATH-resolved etch binary
	if err != nil {
		return "unknown", ""
	}
	var info struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if json.Unmarshal(out, &info) != nil || info.Version == "" {
		return "unknown", ""
	}
	return info.Version, info.Commit
}

// checkCurrency reports this binary's build identity and warns when it looks
// stale — the routine, visible signal that "the installed binary may be old."
func checkCurrency(now time.Time, warnAgeDays int) check {
	return currencyFromBuild(version.BuildInfo(), now, warnAgeDays)
}

// currencyFromBuild is the pure decision: given a resolved build, the current
// time, and a staleness threshold, classify the binary. Never fails (capture
// can still work off an old build); it only ever informs or warns.
func currencyFromBuild(b version.Build, now time.Time, warnAgeDays int) check {
	id := buildDesc(b.Version, b.Commit)
	if b.Dirty {
		id += " (dirty)"
	}

	if !b.Known {
		return check{statusWarn, id + " — build identity unknown (no commit/date stamped); cannot tell if this binary is current. Rebuild & reinstall: make install PREFIX=$HOME/.local"}
	}

	bt, err := time.Parse(time.RFC3339, b.BuildDate)
	if err != nil {
		return check{statusWarn, id + " — unparseable build date " + strconv.Quote(b.BuildDate)}
	}
	age := now.Sub(bt)
	detail := fmt.Sprintf("%s, built %s ago (%s)", id, humanDuration(age), bt.UTC().Format("2006-01-02"))

	if b.Dirty {
		return check{statusWarn, detail + " — built from a dirty tree; its identity is not a clean commit, so currency can't be trusted"}
	}
	if age > time.Duration(warnAgeDays)*24*time.Hour {
		return check{statusWarn, detail + fmt.Sprintf(" — older than %dd; the installed binary may be stale (rebuild & reinstall: make install PREFIX=$HOME/.local)", warnAgeDays)}
	}
	return check{statusOK, detail}
}

func checkEnablement(keyState string) check {
	switch keyState {
	case "true":
		return check{statusOK, "operator mode (etch.enabled=true; all worktrees, all branches)"}
	case "false":
		return check{statusInfo, "explicitly disabled (etch.enabled=false) — every dispatch path fast-exits"}
	default:
		return check{statusOK, "no etch.enabled key — team-mode default (committed hooks capture, stamps would too)"}
	}
}

// checkHooks: per-event etch coverage in committed settings.json and the
// local stamp, at the current worktree root. Missing/partial coverage is
// the ticket's hard failure — unless capture is explicitly disabled.
func checkHooks(root, keyState string) check {
	committed, err := install.EtchEntries(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		return check{statusFail, fmt.Sprintf("cannot parse .claude/settings.json: %v", err)}
	}
	stamped, err := install.EtchEntries(enable.LocalSettingsPath(root))
	if err != nil {
		return check{statusFail, fmt.Sprintf("cannot parse .claude/settings.local.json: %v", err)}
	}

	events := install.EventNames()
	covered := 0
	for _, ev := range events {
		if len(committed[ev]) > 0 || len(stamped[ev]) > 0 {
			covered++
		}
	}
	detail := fmt.Sprintf("committed %d/%d, stamp %d/%d", countCovered(committed, events), len(events), countCovered(stamped, events), len(events))

	if covered == len(events) {
		return check{statusOK, detail}
	}
	if keyState == "false" {
		return check{statusInfo, detail + " — capture is explicitly disabled, so coverage is moot"}
	}
	if covered == 0 {
		return check{statusFail, detail + " — no etch hooks in this worktree; sessions here are invisible (run install-hooks or etch enable)"}
	}
	return check{statusFail, fmt.Sprintf("partial coverage (%d/%d events; %s) — re-run install-hooks or etch enable", covered, len(events), detail)}
}

func countCovered(entries map[string][]string, events []string) int {
	n := 0
	for _, ev := range events {
		if len(entries[ev]) > 0 {
			n++
		}
	}
	return n
}

// checkRefspec reports the facts per remote. No refspec is a valid posture
// (public repos capture local-only), so it is info, never a warning.
func checkRefspec(cwd string) check {
	remotes := gitLines(cwd, "remote")
	if len(remotes) == 0 {
		return check{statusInfo, "no remotes — local-only capture"}
	}
	var with []string
	for _, remote := range remotes {
		entries := append(
			gitLines(cwd, "config", "--get-all", "remote."+remote+".fetch"),
			gitLines(cwd, "config", "--get-all", "remote."+remote+".push")...)
		for _, e := range entries {
			if strings.Contains(e, "refs/etch/sessions") {
				with = append(with, remote)
				break
			}
		}
	}
	if len(with) == 0 {
		return check{statusInfo, "no etch refspec on any remote — local-only capture (correct posture for public repos)"}
	}
	return check{statusOK, "etch refspec configured on: " + strings.Join(with, ", ")}
}

// checkSessions: age of the newest captured session — the silent-breakage
// signal. Zero sessions is a fact, not an error.
func checkSessions(cwd string, warnAgeDays int) check {
	refs := gitLines(cwd, "for-each-ref", "--format=%(creatordate:unix)", "refs/etch/sessions/")
	if len(refs) == 0 {
		return check{statusInfo, "no sessions captured yet"}
	}
	newest := int64(0)
	for _, line := range refs {
		if ts, err := strconv.ParseInt(line, 10, 64); err == nil && ts > newest {
			newest = ts
		}
	}
	age := time.Since(time.Unix(newest, 0))
	detail := fmt.Sprintf("newest %s ago (%d total)", humanDuration(age), len(refs))
	if age > time.Duration(warnAgeDays)*24*time.Hour {
		return check{statusWarn, detail + fmt.Sprintf(" — older than %dd; capture may have silently stopped", warnAgeDays)}
	}
	return check{statusOK, detail}
}

// checkWips: orphaned .wip buffers older than the recovery timeout mean
// recovery isn't firing (it runs on the next session_start). Buffers whose
// recorded agent is verifiably alive are long-running live sessions —
// recovery correctly refuses to touch those, so they never warn.
func checkWips(stateRoot string) check {
	dir := filepath.Join(stateRoot, ".etch", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return check{statusOK, "none"}
	}
	live, orphaned := 0, 0
	var oldestOrphan time.Time
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".wip.jsonl") {
			continue
		}
		alive, known := recovery.WipAgentAlive(filepath.Join(dir, e.Name()))
		if known && alive {
			live++
			continue
		}
		orphaned++
		if fi, err := e.Info(); err == nil {
			if oldestOrphan.IsZero() || fi.ModTime().Before(oldestOrphan) {
				oldestOrphan = fi.ModTime()
			}
		}
	}
	if live+orphaned == 0 {
		return check{statusOK, "none"}
	}
	if orphaned == 0 {
		return check{statusOK, fmt.Sprintf("%d live session(s), no orphans", live)}
	}
	settings, err := config.Load(stateRoot)
	configNote := ""
	if err != nil {
		// A corrupt settings file is itself a finding — don't let it turn
		// into a "past the 0h timeout" lie.
		settings = config.Defaults()
		configNote = " (.etch/settings.json unreadable — using default timeout)"
	}
	timeout := time.Duration(settings.RecoveryTimeoutHours) * time.Hour
	age := time.Since(oldestOrphan)
	detail := fmt.Sprintf("%d live, %d orphaned (oldest orphan %s old)", live, orphaned, humanDuration(age))
	if age > timeout {
		return check{statusWarn, detail + fmt.Sprintf(" — past the %dh recovery timeout; recovery fires at the next session_start in this repo%s", settings.RecoveryTimeoutHours, configNote)}
	}
	return check{statusOK, detail + " — within the recovery timeout" + configNote}
}

// checkWorktrees covers stamp coverage and dedupe sanity in one worktree
// sweep. Stamp coverage is judged against the key state; dedupe sanity
// applies to whatever stamps exist regardless of mode.
func checkWorktrees(cwd, keyState string) (stamps, dedupe check) {
	worktrees, err := enable.ListWorktrees(cwd)
	if err != nil {
		c := check{statusWarn, fmt.Sprintf("could not list worktrees: %v", err)}
		return c, c
	}

	total, stampedCount, anyStampCount := 0, 0, 0
	var gaps, unguarded []string
	stampsExist := false
	for _, wt := range worktrees {
		if _, err := os.Stat(wt); err != nil {
			continue // pruned/missing — not this check's business
		}
		total++
		entries, err := install.EtchEntries(enable.LocalSettingsPath(wt))
		if err != nil {
			gaps = append(gaps, filepath.Base(wt)+" (unparseable)")
			continue
		}
		covered := countCovered(entries, install.EventNames())
		if covered > 0 {
			stampsExist = true
			anyStampCount++
		}
		if covered == len(install.EventNames()) {
			stampedCount++
		} else {
			gaps = append(gaps, filepath.Base(wt))
		}
		for _, cmds := range entries {
			for _, cmd := range cmds {
				if !strings.Contains(cmd, enable.DedupeGuardMarker) {
					unguarded = append(unguarded, filepath.Base(wt))
				}
			}
		}
	}

	switch {
	case keyState == "true" && len(gaps) == 0:
		stamps = check{statusOK, fmt.Sprintf("%d/%d worktree(s) stamped", stampedCount, total)}
	case keyState == "true":
		stamps = check{statusWarn, fmt.Sprintf("%d/%d worktree(s) stamped; missing: %s — re-run etch enable", stampedCount, total, strings.Join(dedupeStrings(gaps), ", "))}
	case stampsExist:
		stamps = check{statusWarn, fmt.Sprintf("stamps present in %d worktree(s) but etch.enabled is %s — hand-stamps or leftover state; run etch enable to formalize (or disable to clean up)", anyStampCount, keyStateWord(keyState))}
	default:
		stamps = check{statusInfo, "no operator-mode stamps (team mode)"}
	}

	switch {
	case !stampsExist:
		dedupe = check{statusInfo, "no stamps to check"}
	case len(unguarded) == 0:
		dedupe = check{statusOK, "all stamps carry the committed-entries-win guard"}
	default:
		dedupe = check{statusWarn, "stamp(s) missing the settings.json grep guard in: " + strings.Join(dedupeStrings(unguarded), ", ") + " — double capture risk on branches with committed hooks"}
	}

	// Grep-guard false positive: settings.json mentions the binary without
	// carrying full etch hook coverage — every stamp would yield to nothing.
	if stampsExist {
		root, _, _ := enable.RepoDirs(cwd)
		settingsPath := filepath.Join(root, ".claude", "settings.json")
		if data, err := os.ReadFile(settingsPath); err == nil && strings.Contains(string(data), "entire-agent-etch") { //nolint:gosec // repo-root derived path
			committed, err := install.EtchEntries(settingsPath)
			if err == nil && countCovered(committed, install.EventNames()) < len(install.EventNames()) {
				dedupe = check{statusWarn, "settings.json mentions entire-agent-etch without full etch hook coverage — stamps yield to it (grep guard false positive) and capture silently stops"}
			}
		}
	}
	return stamps, dedupe
}

func keyStateWord(keyState string) string {
	if keyState == "" {
		return "not set"
	}
	return keyState
}

// checkPropagation: is the post-checkout self-propagation in place, and
// can it actually fire for new worktrees? A RELATIVE core.hooksPath is
// resolved per-worktree, so the shared hook never runs at worktree add —
// the documented husky-style gap.
func checkPropagation(cwd, keyState string) check {
	if keyState != "true" {
		return check{statusInfo, "n/a (operator mode off)"}
	}
	hooksDir, err := enable.EffectiveHooksDir(cwd)
	if err != nil {
		return check{statusWarn, fmt.Sprintf("could not resolve hooks dir: %v", err)}
	}
	hooksPath := strings.Join(gitLines(cwd, "config", "--get", "core.hooksPath"), "")
	if !enable.HasPostCheckoutBlock(hooksDir) {
		return check{statusWarn, fmt.Sprintf("post-checkout block missing from %s — new worktrees will not auto-stamp (re-run etch enable; non-shell hooks need stamp-worktree called manually)", hooksDir)}
	}
	if hooksPath != "" && !filepath.IsAbs(hooksPath) {
		return check{statusWarn, fmt.Sprintf("core.hooksPath is RELATIVE (%q) — resolved per-worktree, so new worktrees will not auto-stamp unless they contain %s themselves", hooksPath, hooksPath)}
	}
	return check{statusOK, "post-checkout block installed (" + hooksDir + ")"}
}

// --- small helpers ----------------------------------------------------------

func gitLines(dir string, args ...string) []string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "moments"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
