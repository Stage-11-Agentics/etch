package enable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func desiredBlock() string {
	return excludeBegin + "\n" + strings.Join(excludeBody, "\n") + "\n" + excludeEnd + "\n"
}

func TestReplaceBlockAppendsToEmpty(t *testing.T) {
	out, changed := replaceBlock(nil, desiredBlock())
	if !changed {
		t.Fatal("expected change on empty content")
	}
	if string(out) != desiredBlock() {
		t.Errorf("got %q", out)
	}
}

func TestReplaceBlockAppendsAfterForeignContent(t *testing.T) {
	foreign := "*.log\nbuild/\n"
	out, changed := replaceBlock([]byte(foreign), desiredBlock())
	if !changed {
		t.Fatal("expected change")
	}
	if string(out) != foreign+desiredBlock() {
		t.Errorf("foreign content not preserved byte-for-byte: %q", out)
	}
}

func TestReplaceBlockAddsNewlineBeforeAppending(t *testing.T) {
	foreign := "*.log" // no trailing newline
	out, _ := replaceBlock([]byte(foreign), desiredBlock())
	if string(out) != foreign+"\n"+desiredBlock() {
		t.Errorf("got %q", out)
	}
}

func TestReplaceBlockNoChangeWhenCurrent(t *testing.T) {
	content := "*.log\n" + desiredBlock() + "post/\n"
	out, changed := replaceBlock([]byte(content), desiredBlock())
	if changed {
		t.Errorf("expected no change, got %q", out)
	}
}

func TestReplaceBlockReplacesStaleBlockInPlace(t *testing.T) {
	stale := excludeBegin + "\nold-entry\n" + excludeEnd + "\n"
	content := "before/\n" + stale + "after/\n"
	out, changed := replaceBlock([]byte(content), desiredBlock())
	if !changed {
		t.Fatal("expected change")
	}
	want := "before/\n" + desiredBlock() + "after/\n"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestParseConfigKey(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantVal   string
		wantFound bool
		wantClean bool
	}{
		{"disable output", "[core]\n\tbare = false\n[etch]\n\tenabled = false\n", "false", true, true},
		{"enable output", "[etch]\n\tenabled = true\n", "true", true, true},
		{"absent key", "[core]\n\tbare = false\n", "", false, true},
		{"absent section value elsewhere", "[other]\n\tenabled = false\n", "", false, true},
		{"case-insensitive", "[ETCH]\n\tEnabled = FALSE\n", "FALSE", true, true},
		{"last assignment wins", "[etch]\n\tenabled = false\n\tenabled = true\n", "true", true, true},
		{"trailing comment", "[etch]\n\tenabled = false # off for now\n", "false", true, true},
		{"bare key means true", "[etch]\n\tenabled\n", "true", true, true},
		{"quoted value", "[etch]\n\tenabled = \"false\"\n", "false", true, true},
		{"etch subsection does not match", "[etch \"sub\"]\n\tenabled = false\n", "", false, true},
		{"include directive forces fallback", "[include]\n\tpath = other\n[etch]\n\tenabled = false\n", "", false, false},
		{"includeIf forces fallback", "[includeIf \"gitdir:/x\"]\n\tpath = other\n", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTemp(t, tc.content)
			val, found, clean := parseConfigKey(path)
			if val != tc.wantVal || found != tc.wantFound || clean != tc.wantClean {
				t.Errorf("parseConfigKey = (%q, %v, %v), want (%q, %v, %v)",
					val, found, clean, tc.wantVal, tc.wantFound, tc.wantClean)
			}
		})
	}
}

func TestParseConfigKeyMissingFile(t *testing.T) {
	val, found, clean := parseConfigKey("/nonexistent/config")
	if val != "" || found || !clean {
		t.Errorf("missing file should be (\"\", false, true), got (%q, %v, %v)", val, found, clean)
	}
}

func TestGitConfigBool(t *testing.T) {
	for _, v := range []string{"false", "FALSE", "no", "off", "0"} {
		if gitConfigBool(v) {
			t.Errorf("gitConfigBool(%q) = true, want false", v)
		}
	}
	for _, v := range []string{"true", "yes", "on", "1", "garbage", ""} {
		if !gitConfigBool(v) {
			t.Errorf("gitConfigBool(%q) = false, want true (capture is the safe default)", v)
		}
	}
}

func TestReplaceBlockBeginWithoutEndAppendsClean(t *testing.T) {
	content := "x/\n" + excludeBegin + "\norphaned\n"
	out, changed := replaceBlock([]byte(content), desiredBlock())
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.HasSuffix(string(out), desiredBlock()) {
		t.Errorf("expected clean block appended, got %q", out)
	}
	if !strings.HasPrefix(string(out), content) {
		t.Errorf("existing content not preserved: %q", out)
	}
}
