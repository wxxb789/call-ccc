package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.kenn.io/agentsview/internal/ccc/opencodefixture"
)

// openCodeSourceFixtureID is the committed OpenCode fixture's own session id (fixturegen's
// openCodeSession): the source direction's reference in these cases.
const openCodeSourceFixtureID = "ses_0008opencodefixture00000001"

// openCodeSourceFixturePath is the committed fixture document.
func openCodeSourceFixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(fixtureRoot(t), "opencode", openCodeSourceFixtureID+".json")
}

// openCodeSourceStore materialises the committed fixture document into one throw-away OpenCode store and
// answers the directory that holds it: the directory a caller names with OPENCODE_DIR, and the one whose
// `opencode.db` the inherited provider's discovery reads.
func openCodeSourceStore(t *testing.T) string {
	t.Helper()
	document, err := os.ReadFile(openCodeSourceFixturePath(t))
	if err != nil {
		t.Fatalf("reading the OpenCode fixture: %v", err)
	}
	root := filepath.Join(t.TempDir(), "opencode")
	if _, id, err := opencodefixture.Store(root, document); err != nil {
		t.Fatalf("building the OpenCode store: %v", err)
	} else if id != openCodeSourceFixtureID {
		t.Fatalf("the store holds %s, want %s", id, openCodeSourceFixtureID)
	}
	return root
}

// TestContinueFromOpenCodeWritesTheEstablishedReport pins the goal's interface clause for this source:
// an OpenCode session is named the way every other source is (`opencode:<session id>`, resolved through
// the provider's configured root), and the run answers with the same report shape and the same native
// resume identity -- no new flag, no new catalog, no second command.
func TestContinueFromOpenCodeWritesTheEstablishedReport(t *testing.T) {
	isolatedEnvironment(t)
	store := openCodeSourceStore(t)
	t.Setenv("OPENCODE_DIR", store)
	targetHome := t.TempDir()

	code, stdout, stderr := runCLI(t, "continue", "opencode:"+openCodeSourceFixtureID, "--to", "codex",
		"--target-home", targetHome, "--target-binary", missingTargetBinary, "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr: %s", code, stderr)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("parsing the report: %v\n%s", err, stdout)
	}
	// The report's own shape is the established one: the same top-level members every source's
	// conversion writes, so a consumer that reads another source's report reads this one. `warnings` is
	// optional by the report's own contract (it is absent when a run has none), and it is the only
	// member these eight may be joined by.
	wantKeys := []string{
		"conversion", "installation", "ref", "source", "target", "target_session", "upstream_revision",
		"verification",
	}
	for _, key := range wantKeys {
		if _, ok := document[key]; !ok {
			t.Errorf("the report holds no %s member: %v", key, slices.Sorted(maps.Keys(document)))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(document)) {
		if !slices.Contains(wantKeys, key) && key != "warnings" {
			t.Errorf("the report holds an unexpected %s member: %v", key, slices.Sorted(maps.Keys(document)))
		}
	}
	ref, _ := document["ref"].(map[string]any)
	if ref["Provider"] != "opencode" || ref["ID"] != openCodeSourceFixtureID {
		t.Errorf("ref = %v", ref)
	}
	if document["target"] != "codex" {
		t.Errorf("target = %v", document["target"])
	}
	source, _ := document["source"].(map[string]any)
	identity, _ := source["identity"].(map[string]any)
	if identity["provider"] != "opencode" || identity["agent"] != "OpenCode" {
		t.Errorf("source identity = %v", identity)
	}
	wantPath := filepath.Join(store, "opencode.db") + "#" + openCodeSourceFixtureID
	if identity["source_path"] != wantPath {
		t.Errorf("source path = %v, want the store's own virtual path %q", identity["source_path"], wantPath)
	}
	conversion, _ := document["conversion"].(map[string]any)
	if conversion["state"] != "converted" || conversion["format"] != "codex-rollout-jsonl" {
		t.Errorf("conversion = %v", conversion)
	}
	carried, _ := conversion["carried"].(map[string]any)
	if carried["tool_calls"] != float64(1) || carried["tool_results"] != float64(1) {
		t.Errorf("carried = %v, want the fixture's call and terminal result", carried)
	}
	installation, _ := document["installation"].(map[string]any)
	if installation["state"] != "installed" {
		t.Errorf("installation = %v", installation)
	}
	target, _ := document["target_session"].(map[string]any)
	sessionID, _ := target["session_id"].(string)
	if sessionID == "" || sessionID == openCodeSourceFixtureID {
		t.Errorf("target session = %v, want a fresh identity", target)
	}
	resume, _ := target["resume_command"].(string)
	if !strings.Contains(resume, "CODEX_HOME="+targetHome) || !strings.Contains(resume, sessionID) {
		t.Errorf("resume command = %q, want the target home %q and the session id", resume, targetHome)
	}
	verification, _ := document["verification"].(map[string]any)
	structure, _ := verification["structure"].(map[string]any)
	if structure["state"] != "verified" {
		t.Errorf("structure = %v, want the installed session read back as written", structure)
	}

	// The human block carries the same facts for a reader: which source was read (the store's virtual
	// path), that the boundary was not taken for a container member, and what the target resumes.
	humanHome := t.TempDir()
	code, human, stderr := runCLI(t, "continue", "opencode:"+openCodeSourceFixtureID, "--to", "claude",
		"--target-home", humanHome, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d for the human block, want 0; stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"opencode:" + openCodeSourceFixtureID + " → claude",
		"source:       " + wantPath,
		"conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)",
		"boundary:     not taken (the source read resolved member " + openCodeSourceFixtureID,
		"converted:    claude-code-transcript-jsonl",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("the human block does not hold %q:\n%s", want, human)
		}
	}
}

// TestInspectOpenCodeSourceNamesTheStoreAndTheConversation pins the inspection surface for this source:
// the identity the inherited provider resolves, the store and member it read, the conversation shape, and
// the tool vocabulary the source recorded -- the facts an agent chooses a session to continue by.
func TestInspectOpenCodeSourceNamesTheStoreAndTheConversation(t *testing.T) {
	isolatedEnvironment(t)
	store := openCodeSourceStore(t)
	t.Setenv("OPENCODE_DIR", store)

	code, stdout, stderr := runCLI(t, "inspect", "opencode:"+openCodeSourceFixtureID, "--json")
	if code != 0 {
		t.Fatalf("exit %d, want 0; stderr: %s", code, stderr)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("parsing the inspection: %v\n%s", err, stdout)
	}
	if document["agent"] != "OpenCode" {
		t.Errorf("agent = %v", document["agent"])
	}
	source, _ := document["source"].(map[string]any)
	identity, _ := source["identity"].(map[string]any)
	if identity["session_id"] != "opencode:"+openCodeSourceFixtureID {
		t.Errorf("session id = %v", identity["session_id"])
	}
	counts, _ := source["counts"].(map[string]any)
	if counts["messages"] != float64(5) || counts["tool_calls"] != float64(1) ||
		counts["tool_results"] != float64(2) {
		t.Errorf("counts = %v, want the store's own records as the reader answers them", counts)
	}
	tools, _ := source["tool_names"].([]any)
	if len(tools) != 1 || tools[0] != "read" {
		t.Errorf("tool names = %v", tools)
	}

	code, human, stderr := runCLI(t, "inspect", "opencode:"+openCodeSourceFixtureID)
	if code != 0 {
		t.Fatalf("exit %d for the human block, want 0; stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"agent:        OpenCode",
		"source:       " + filepath.Join(store, "opencode.db") + "#" + openCodeSourceFixtureID,
		"conversation: 5 message(s), 2 user, 3 assistant, 1 tool call(s)",
		"tools:        read",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("the inspection does not hold %q:\n%s", want, human)
		}
	}
}

// TestContinueFromAnUnknownOpenCodeSessionFailsWithoutWriting pins the failure half of the interface:
// a reference the provider cannot resolve is the same not-found failure every other source answers, it
// names the reference, and nothing is written into a target home for it.
func TestContinueFromAnUnknownOpenCodeSessionFailsWithoutWriting(t *testing.T) {
	isolatedEnvironment(t)
	store := openCodeSourceStore(t)
	t.Setenv("OPENCODE_DIR", store)
	targetHome := filepath.Join(t.TempDir(), "codex home")

	const unknown = "ses_unknownopencodefixture00001"
	code, stdout, stderr := runCLI(t, "continue", "opencode:"+unknown, "--to", "codex",
		"--target-home", targetHome, "--json")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout: %s", code, stdout)
	}
	if !strings.Contains(stderr, "session not found: opencode:"+unknown) {
		t.Errorf("stderr = %q, want the reference the provider could not resolve", stderr)
	}
	if _, err := os.Stat(targetHome); !os.IsNotExist(err) {
		t.Errorf("the refused run created the target home %s (%v)", targetHome, err)
	}
}
