package commands

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const (
	etchPushRefspec  = "refs/etch/sessions/*:refs/etch/sessions/*"
	etchFetchRefspec = "+refs/etch/sessions/*:refs/etch/sessions/*"
	// legacyFetchRefspec is the pre-ETCH-38 form (no leading '+'); rerunning
	// setup-refspec upgrades it in place.
	legacyFetchRefspec = "refs/etch/sessions/*:refs/etch/sessions/*"
	// headPushRefspec recreates default-ish push behavior (push.default=current)
	// alongside the etch refspec, because any remote.<name>.push entry replaces
	// git's implicit default push and would otherwise hijack a bare `git push`
	// into pushing only etch refs (ETCH-16).
	headPushRefspec = "HEAD"
)

func RunSetupRefspec(args []string) error {
	requested, err := parseRemoteFlag(args)
	if err != nil {
		return err
	}

	remote, err := selectRemote(requested)
	if err != nil {
		return err
	}

	if err := configureFetch(remote); err != nil {
		return err
	}
	headManaged, hasForeign, err := configurePush(remote)
	if err != nil {
		return err
	}

	fmt.Printf("etch refspec configured for push and fetch on remote %q\n", remote)
	if headManaged {
		fmt.Println("note: a bare 'git push' now pushes the current branch (push.default=current semantics) plus etch session refs; branches without an upstream are created on the remote")
	}
	if hasForeign {
		fmt.Printf("note: remote %q already has push refspecs configured; they remain authoritative — only the etch refspec was added\n", remote)
	}
	fmt.Printf("run 'git fetch %s' to pull existing session refs\n", remote)
	return nil
}

// parseRemoteFlag extracts --remote <name> / --remote=<name> from args.
// Returns "" when no flag is given. Any other argument is an error.
func parseRemoteFlag(args []string) (string, error) {
	requested := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--remote":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--remote requires a value")
			}
			i++
			requested = args[i]
		case strings.HasPrefix(arg, "--remote="):
			requested = strings.TrimPrefix(arg, "--remote=")
			if requested == "" {
				return "", fmt.Errorf("--remote requires a value")
			}
		default:
			return "", fmt.Errorf("unknown argument %q (usage: setup-refspec [--remote <name>])", arg)
		}
	}
	return requested, nil
}

// selectRemote picks the remote to configure. A remote is usable only if it
// has a URL — a config-only remote with refspec entries but no URL (the
// phantom-origin case, ETCH-18) is reported, never configured.
func selectRemote(requested string) (string, error) {
	usable, phantoms, err := listRemotes()
	if err != nil {
		return "", err
	}

	if requested != "" {
		for _, r := range usable {
			if r == requested {
				return requested, nil
			}
		}
		for _, r := range phantoms {
			if r == requested {
				return "", fmt.Errorf("remote %q has no URL; set one (git remote set-url %s <url>) then re-run", requested, requested)
			}
		}
		return "", fmt.Errorf("remote %q not found; add it (git remote add %s <url>) then re-run", requested, requested)
	}

	for _, r := range usable {
		if r == "origin" {
			return "origin", nil
		}
	}

	switch len(usable) {
	case 0:
		for _, r := range phantoms {
			if r == "origin" {
				return "", fmt.Errorf("remote \"origin\" exists but has no URL; set one (git remote set-url origin <url>) then re-run")
			}
		}
		return "", fmt.Errorf("no usable git remote found; add one (git remote add origin <url>) then re-run")
	case 1:
		fmt.Printf("no \"origin\" remote; configuring remote %q\n", usable[0])
		return usable[0], nil
	default:
		return "", fmt.Errorf("multiple remotes found (%s) and none is \"origin\"; run with --remote <name> for each remote you want to sync etch refs with", strings.Join(usable, ", "))
	}
}

// listRemotes returns remotes with a URL (usable) and without one (phantoms).
func listRemotes() (usable, phantoms []string, err error) {
	out, err := gitOutput("remote")
	if err != nil {
		return nil, nil, fmt.Errorf("git remote: %w", err)
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		url, _ := gitOutput("config", "--get", "remote."+name+".url")
		if strings.TrimSpace(url) == "" {
			phantoms = append(phantoms, name)
		} else {
			usable = append(usable, name)
		}
	}
	// `git remote` does not list URL-less remotes that exist only as config
	// sections; sweep config for those (the ETCH-18 phantom repro).
	cfg, _ := gitOutput("config", "--get-regexp", `^remote\..*\.(fetch|push)$`)
	for _, line := range strings.Split(cfg, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		parts := strings.Split(fields[0], ".")
		if len(parts) < 3 {
			continue
		}
		name := strings.Join(parts[1:len(parts)-1], ".")
		if name == "" || contains(usable, name) || contains(phantoms, name) {
			continue
		}
		phantoms = append(phantoms, name)
	}
	sort.Strings(usable)
	sort.Strings(phantoms)
	return usable, phantoms, nil
}

// configureFetch ensures the '+'-prefixed etch fetch refspec is present,
// upgrading the legacy no-'+' entry in place if found (ETCH-38).
func configureFetch(remote string) error {
	key := "remote." + remote + ".fetch"
	values := configValues(key)

	if contains(values, legacyFetchRefspec) {
		// Exact-match unset of the legacy value; regex-escape '*' and '+'.
		pattern := "^" + regexEscape(legacyFetchRefspec) + "$"
		if err := gitRun("config", "--unset-all", key, pattern); err != nil {
			return fmt.Errorf("removing legacy fetch refspec: %w", err)
		}
	}
	if !contains(configValues(key), etchFetchRefspec) {
		if err := gitRun("config", "--add", key, etchFetchRefspec); err != nil {
			return fmt.Errorf("adding fetch refspec: %w", err)
		}
	}
	return nil
}

// configurePush ensures the etch push refspec is present. When no foreign
// (user-configured) push refspecs exist, the push list is etch-managed: HEAD
// is also ensured so a bare `git push` keeps pushing the current branch
// (ETCH-16) — this heals legacy configs left by older binaries that wrote only
// the etch refspec. Foreign refspecs block HEAD injection entirely; only the
// etch refspec is added alongside them.
func configurePush(remote string) (headManaged, hasForeign bool, err error) {
	key := "remote." + remote + ".push"
	before := configValues(key)

	for _, v := range before {
		if v != etchPushRefspec && v != headPushRefspec {
			hasForeign = true
		}
	}

	if !contains(before, etchPushRefspec) {
		if err := gitRun("config", "--add", key, etchPushRefspec); err != nil {
			return false, false, fmt.Errorf("adding push refspec: %w", err)
		}
	}
	if !hasForeign && !contains(before, headPushRefspec) {
		if err := gitRun("config", "--add", key, headPushRefspec); err != nil {
			return false, false, fmt.Errorf("adding HEAD push refspec: %w", err)
		}
	}
	return !hasForeign, hasForeign, nil
}

func configValues(key string) []string {
	out, _ := gitOutput("config", "--get-all", key)
	var values []string
	for _, line := range strings.Split(out, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func regexEscape(s string) string {
	r := strings.NewReplacer(`*`, `\*`, `+`, `\+`, `.`, `\.`)
	return r.Replace(s)
}
