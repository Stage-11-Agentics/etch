package refs_test

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Stage-11-Agentics/etch/internal/refs"
	"github.com/Stage-11-Agentics/etch/internal/testutil"
)

var (
	sampleSessionJSON = []byte(`{"schema_version":"etch.session.v1","session_id":"01JWB8K3XQPNR7TV0ZYM4GD2AH","status":"complete"}`)
	sampleTraceJSON   = []byte(`{"version":"1.0","traces":[{"agent_id":"claude-code","session_id":"01JWB8K3XQPNR7TV0ZYM4GD2AH"}]}`)
	sampleMeta        = refs.RefMeta{
		Runtime:      "claude-code",
		Model:        "claude-opus-4-7",
		Status:       "complete",
		Branch:       "feat/login-button",
		CommitCount:  2,
		DurationSecs: 913,
		EndTime:      time.Unix(1748271442, 0),
	}
	sampleSessionID = "01JWB8K3XQPNR7TV0ZYM4GD2AH"
)

func gitShow(t *testing.T, repo, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", ref, err)
	}
	return string(out)
}

func gitCatFile(t *testing.T, repo, flag, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "cat-file", flag, ref)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git cat-file %s %s: %v", flag, ref, err)
	}
	return string(out)
}

func TestWriteSessionRef_Basic(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	refName := "refs/etch/sessions/" + sampleSessionID

	err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err != nil {
		t.Fatalf("WriteSessionRef: %v", err)
	}

	// Ref must exist
	cmd := exec.Command("git", "rev-parse", refName)
	cmd.Dir = repo
	commitSHA, err := cmd.Output()
	if err != nil {
		t.Fatalf("ref %s does not exist: %v", refName, err)
	}
	if strings.TrimSpace(string(commitSHA)) == "" {
		t.Fatal("commit SHA is empty")
	}

	// Commit must have no parent (orphan)
	raw := gitCatFile(t, repo, "-p", refName)
	if strings.Contains(raw, "parent ") {
		t.Error("commit has a parent — should be an orphan")
	}

	// Tree must have exactly two entries: agent-trace.json and session.json
	treeLines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "100644 blob") {
			treeLines = append(treeLines, line)
		}
	}
	treeSHA := ""
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "tree ") {
			treeSHA = strings.TrimPrefix(line, "tree ")
			break
		}
	}
	if treeSHA == "" {
		t.Fatal("no tree SHA found in commit")
	}
	treeRaw := gitCatFile(t, repo, "-p", treeSHA)
	entries := strings.Split(strings.TrimSpace(treeRaw), "\n")
	if len(entries) != 2 {
		t.Fatalf("expected 2 tree entries, got %d: %v", len(entries), entries)
	}
	hasSession := false
	hasTrace := false
	for _, e := range entries {
		if strings.HasSuffix(e, "\tsession.json") {
			hasSession = true
		}
		if strings.HasSuffix(e, "\tagent-trace.json") {
			hasTrace = true
		}
	}
	if !hasSession {
		t.Error("tree missing session.json")
	}
	if !hasTrace {
		t.Error("tree missing agent-trace.json")
	}
}

func TestWriteSessionRef_BlobContent(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	refName := "refs/etch/sessions/" + sampleSessionID

	err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err != nil {
		t.Fatalf("WriteSessionRef: %v", err)
	}

	got := gitShow(t, repo, refName+":session.json")
	if strings.TrimSpace(got) != strings.TrimSpace(string(sampleSessionJSON)) {
		t.Errorf("session.json content mismatch:\ngot:  %s\nwant: %s", got, sampleSessionJSON)
	}

	got = gitShow(t, repo, refName+":agent-trace.json")
	if strings.TrimSpace(got) != strings.TrimSpace(string(sampleTraceJSON)) {
		t.Errorf("agent-trace.json content mismatch:\ngot:  %s\nwant: %s", got, sampleTraceJSON)
	}
}

func TestWriteSessionRef_AuthorCommitter(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err != nil {
		t.Fatalf("WriteSessionRef: %v", err)
	}

	raw := gitCatFile(t, repo, "-p", "refs/etch/sessions/"+sampleSessionID)
	if !strings.Contains(raw, "author etch <etch@localhost>") {
		t.Error("author is not etch <etch@localhost>")
	}
	if !strings.Contains(raw, "committer etch <etch@localhost>") {
		t.Error("committer is not etch <etch@localhost>")
	}
}

func TestWriteSessionRef_Timestamp(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err != nil {
		t.Fatalf("WriteSessionRef: %v", err)
	}

	raw := gitCatFile(t, repo, "-p", "refs/etch/sessions/"+sampleSessionID)
	expectedTS := fmt.Sprintf("%d", sampleMeta.EndTime.Unix())
	if !strings.Contains(raw, "author etch <etch@localhost> "+expectedTS) {
		t.Errorf("author timestamp mismatch, expected %s in:\n%s", expectedTS, raw)
	}
	if !strings.Contains(raw, "committer etch <etch@localhost> "+expectedTS) {
		t.Errorf("committer timestamp mismatch, expected %s in:\n%s", expectedTS, raw)
	}
}

func TestWriteSessionRef_CommitMessage(t *testing.T) {
	repo := testutil.NewTestRepo(t)

	err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err != nil {
		t.Fatalf("WriteSessionRef: %v", err)
	}

	cmd := exec.Command("git", "log", "--format=%B", "-1", "refs/etch/sessions/"+sampleSessionID)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	msg := strings.TrimSpace(string(out))

	expected := fmt.Sprintf("etch session %s\nagent: claude-code / claude-opus-4-7\nstatus: complete\nbranch: feat/login-button\ncommits: 2\nduration: 913s",
		sampleSessionID)

	if msg != expected {
		t.Errorf("commit message mismatch:\ngot:\n%s\n\nwant:\n%s", msg, expected)
	}
}

func TestWriteSessionRef_Concurrent(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	const n = 20

	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sid := fmt.Sprintf("01JWB8K3XQPNR7TV0ZYM4G%05d", idx)
			meta := sampleMeta
			meta.Branch = fmt.Sprintf("feat/branch-%d", idx)
			sessionJSON := []byte(fmt.Sprintf(`{"schema_version":"etch.session.v1","session_id":"%s","status":"complete"}`, sid))
			traceJSON := []byte(fmt.Sprintf(`{"version":"1.0","traces":[{"agent_id":"claude-code","session_id":"%s"}]}`, sid))
			errs[idx] = refs.WriteSessionRef(repo, sid, sessionJSON, traceJSON, meta)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("session %d failed: %v", i, err)
		}
	}

	// Verify all refs exist
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname)", "refs/etch/sessions/")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("for-each-ref: %v", err)
	}
	refLines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(refLines) != n {
		t.Errorf("expected %d refs, got %d", n, len(refLines))
	}
}

// resolveRef returns the SHA a ref points at.
func resolveRef(t *testing.T, repo, refName string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", refName)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", refName, err)
	}
	return strings.TrimSpace(string(out))
}

// TestWriteSessionRef_CreateOnly (ETCH-40 finding 3): an existing complete
// record is never overwritten — not even by another complete record.
func TestWriteSessionRef_CreateOnly(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	refName := "refs/etch/sessions/" + sampleSessionID

	if err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta); err != nil {
		t.Fatalf("first write: %v", err)
	}
	firstSHA := resolveRef(t, repo, refName)

	otherJSON := []byte(`{"schema_version":"etch.session.v1","session_id":"` + sampleSessionID + `","status":"complete","note":"imposter"}`)
	err := refs.WriteSessionRef(repo, sampleSessionID, otherJSON, sampleTraceJSON, sampleMeta)
	if !errors.Is(err, refs.ErrRefExists) {
		t.Fatalf("second write: want ErrRefExists, got %v", err)
	}

	if got := resolveRef(t, repo, refName); got != firstSHA {
		t.Errorf("ref moved despite ErrRefExists: %s -> %s", firstSHA, got)
	}
	if content := gitShow(t, repo, refName+":session.json"); strings.Contains(content, "imposter") {
		t.Error("the second record's content replaced the first")
	}
}

// TestWriteSessionRef_UpgradeIncompleteToComplete: the one legitimate
// replacement — a truthful complete record upgrades a premature crash record.
func TestWriteSessionRef_UpgradeIncompleteToComplete(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	refName := "refs/etch/sessions/" + sampleSessionID

	crashJSON := []byte(`{"schema_version":"etch.session.v1","session_id":"` + sampleSessionID + `","status":"incomplete","exit_reason":"crash"}`)
	crashMeta := sampleMeta
	crashMeta.Status = "incomplete"
	if err := refs.WriteSessionRef(repo, sampleSessionID, crashJSON, sampleTraceJSON, crashMeta); err != nil {
		t.Fatalf("crash write: %v", err)
	}

	if err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta); err != nil {
		t.Fatalf("upgrade write: want success, got %v", err)
	}

	content := gitShow(t, repo, refName+":session.json")
	if !strings.Contains(content, `"status":"complete"`) {
		t.Errorf("ref must hold the complete record after upgrade:\n%s", content)
	}
}

// TestWriteSessionRef_NeverDowngrade: an incomplete/crash record must never
// replace a committed complete record (the finding-3 corruption scenario).
func TestWriteSessionRef_NeverDowngrade(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	refName := "refs/etch/sessions/" + sampleSessionID

	if err := refs.WriteSessionRef(repo, sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta); err != nil {
		t.Fatalf("complete write: %v", err)
	}
	firstSHA := resolveRef(t, repo, refName)

	crashJSON := []byte(`{"schema_version":"etch.session.v1","session_id":"` + sampleSessionID + `","status":"incomplete","exit_reason":"crash"}`)
	crashMeta := sampleMeta
	crashMeta.Status = "incomplete"
	err := refs.WriteSessionRef(repo, sampleSessionID, crashJSON, sampleTraceJSON, crashMeta)
	if !errors.Is(err, refs.ErrRefExists) {
		t.Fatalf("downgrade: want ErrRefExists, got %v", err)
	}

	if got := resolveRef(t, repo, refName); got != firstSHA {
		t.Errorf("complete record was downgraded: %s -> %s", firstSHA, got)
	}
	content := gitShow(t, repo, refName+":session.json")
	if !strings.Contains(content, `"status":"complete"`) {
		t.Errorf("ref must still hold the complete record:\n%s", content)
	}
}

// TestWriteSessionRef_ConcurrentSameULID: many writers race one ULID — the
// create-only guard must leave exactly one surviving record, and every loser
// must see ErrRefExists (not a silent overwrite, not a spurious failure).
func TestWriteSessionRef_ConcurrentSameULID(t *testing.T) {
	repo := testutil.NewTestRepo(t)
	const n = 10

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sessionJSON := []byte(fmt.Sprintf(`{"schema_version":"etch.session.v1","session_id":"%s","status":"complete","writer":%d}`, sampleSessionID, idx))
			errs[idx] = refs.WriteSessionRef(repo, sampleSessionID, sessionJSON, sampleTraceJSON, sampleMeta)
		}(i)
	}
	wg.Wait()

	winners := 0
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, refs.ErrRefExists):
			// expected loser
		default:
			t.Errorf("writer %d: unexpected error: %v", i, err)
		}
	}
	if winners != 1 {
		t.Errorf("expected exactly 1 winner, got %d", winners)
	}
}

func TestWriteSessionRef_InvalidRepo(t *testing.T) {
	err := refs.WriteSessionRef("/nonexistent/path", sampleSessionID, sampleSessionJSON, sampleTraceJSON, sampleMeta)
	if err == nil {
		t.Error("expected error for non-existent repo path")
	}
}
