package ccc

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.kenn.io/agentsview/internal/parser"
)

// ompFixtureTargetOptions is the answer the route's facts step gives on a home that resolves a model: the
// pair the harness itself named for a new session of it.
func ompFixtureTargetOptions() OMPTargetOptions {
	return OMPTargetOptions{
		Provider:       "home-proxy",
		Model:          "home-model",
		ProviderDetail: "the harness resolved `home-proxy/home-model` for a new session of it",
	}
}

// ompTitleSlotWidth is the width of the file's first line, line feed included, measured against the
// harness's own serialiser (`SESSION_TITLE_SLOT_BYTES = 256`) and against 300 first lines of a real OMP
// home, every one of them exactly this long.
const ompTitleSlotWidth = 256

// TestOMPSessionRecordShape pins the entries, their order and their fields, including the records OMP's
// own format carries that Pi's does not:
//
//   - the file opens with a fixed-width title slot line holding an empty title (OMP's own initial state,
//     which the harness rewrites in place when it titles a session);
//   - the session's own model binding is an entry of the file's tree, and the source's first record hangs
//     off it: measured, a binding entry nothing chains to is not read at all, and a session that records
//     no model is restored from the pairs its messages carry -- refused by a home that does not know the
//     source's model;
//   - every assistant record states a non-empty `model` and `provider`: measured, dropping either from
//     one record of an otherwise loadable session makes the next turn fail while the harness builds its
//     request.
func TestOMPSessionRecordShape(t *testing.T) {
	session, err := ConvertToOMPSession(piFixtureConversation(), ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	data := session.Data
	firstLine := string(data)
	if index := strings.IndexByte(firstLine, '\n'); index >= 0 {
		firstLine = firstLine[:index+1]
	}
	if len(firstLine) != ompTitleSlotWidth {
		t.Fatalf("the title slot line is %d bytes, want the harness's own %d", len(firstLine), ompTitleSlotWidth)
	}
	var slot struct {
		Type      string `json:"type"`
		Version   int    `json:"v"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updatedAt"`
		Pad       string `json:"pad"`
	}
	if err := json.Unmarshal([]byte(firstLine), &slot); err != nil {
		t.Fatalf("the title slot is not one JSON object: %v", err)
	}
	if slot.Type != piEntryTitle || slot.Version != 1 {
		t.Errorf("slot = %+v, want the harness's own title record (type %q, v 1)", slot, piEntryTitle)
	}
	if slot.Title != "" {
		t.Errorf("title = %q, want it empty: a session's title is the harness's own to generate", slot.Title)
	}
	if slot.UpdatedAt == "" || strings.TrimSpace(slot.Pad) != "" || slot.Pad == "" {
		t.Errorf("slot = %+v, want an instant and nothing but padding in pad", slot)
	}

	raw := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	lines := decodePiSession(t, data)
	if lines[0].Type != piEntryTitle {
		t.Fatalf("the first line is a %s, not the title slot", lines[0].Type)
	}
	var header struct {
		Version int    `json:"version"`
		ID      string `json:"id"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(raw[1]), &header); err != nil {
		t.Fatalf("decoding the header: %v", err)
	}
	if header.Version != PiSessionVersion || header.ID != session.SessionID {
		t.Errorf("header = %+v, want version %d and the document's own id", header, PiSessionVersion)
	}
	if header.Cwd != "/work/project" {
		t.Errorf("header cwd = %q, want the source's recorded working directory", header.Cwd)
	}

	// The binding, and the walk it has to stand on. The decoded lines include the file's header, which is
	// not an entry of the tree: the title slot is line 1, the header line 2, and the binding the first
	// entry after them.
	if lines[1].Type != piEntrySession {
		t.Fatalf("line 2 is a %s, want the session header", lines[1].Type)
	}
	if lines[2].Type != piEntryModelChange {
		t.Fatalf("the first entry is a %s, want the session's model binding", lines[2].Type)
	}
	var binding struct {
		Model    string `json:"model"`
		Fallback bool   `json:"resolvedModelIsFallback"`
	}
	if err := json.Unmarshal([]byte(raw[2]), &binding); err != nil {
		t.Fatalf("decoding the model binding: %v", err)
	}
	if binding.Model != "home-proxy/home-model" {
		t.Errorf("binding = %q, want the harness's own `<provider>/<id>` answer", binding.Model)
	}
	if binding.Fallback {
		t.Error("the binding claims a fallback model: the harness answered this one for itself")
	}
	if lines[2].ParentID != nil {
		t.Errorf("the binding's parentId = %v, want null: it is where this file's walk starts", *lines[2].ParentID)
	}
	if lines[3].Type != piEntryMessage || lines[3].ParentID == nil || *lines[3].ParentID != lines[2].ID {
		t.Fatalf("the first record's entry is not chained to the binding (%+v): measured, a binding nothing "+
			"chains to is not restored, so the session is opened without a resolvable model", lines[3])
	}

	// Every assistant record's pair, and the model each one states.
	assistants := 0
	for index, line := range lines {
		if line.Type != piEntryMessage {
			continue
		}
		var message struct {
			Role     string          `json:"role"`
			Model    string          `json:"model"`
			Provider string          `json:"provider"`
			Content  json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(line.Message, &message); err != nil {
			t.Fatalf("line %d's message is not one object: %v", index, err)
		}
		if message.Role != piRoleAssistant {
			continue
		}
		assistants++
		if strings.TrimSpace(message.Model) == "" {
			t.Errorf("line %d's assistant record states no model: measured, the harness fails to build the "+
				"next request when one assistant record has none", index)
		}
		if strings.TrimSpace(message.Provider) == "" {
			t.Errorf("line %d's assistant record states no provider: measured, the harness fails to build "+
				"the next request when one assistant record has none (an empty provider is not a value it "+
				"accepts)", index)
		}
		if message.Model != "claude-sonnet-4-5" {
			t.Errorf("line %d's model = %q, want the model the source's own message recorded",
				index, message.Model)
		}
		if message.Provider != "home-proxy" {
			t.Errorf("line %d's provider = %q, want the target home's own endpoint, which is the only "+
				"answer the read can support (it answers no endpoint for a message)", index, message.Provider)
		}
	}
	if assistants == 0 {
		t.Fatal("the converted session holds no assistant record")
	}
}

// TestOMPSessionStatesTheHomesModelOnARecordTheSourceLeftModelLess pins the OMP-only half of the required
// per-record pair: the harness's request builder reads a non-empty `model` off every assistant record, and
// the inherited read answers no model for a message that recorded none -- so the record carries the
// harness's own configured model id beside the target home's provider, and the caveat's count states both
// what travelled and what the target answered, so the id cannot be read as evidence of who wrote the words.
func TestOMPSessionStatesTheHomesModelOnARecordTheSourceLeftModelLess(t *testing.T) {
	conversation := piFixtureConversation()
	// The first assistant message's own model goes missing: the read answers none for it, and the OMP
	// writer is the family's one format that still has to state something.
	conversation.Messages[1].Model = ""
	session, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	fallbacks, travelled := 0, 0
	for index, entry := range ompEntries(t, session.Data) {
		if entry.Type != piEntryMessage || entry.Role != piRoleAssistant {
			continue
		}
		if entry.Provider != "home-proxy" {
			t.Errorf("line %d's assistant record states provider %q, want the target home's own configured "+
				"endpoint", index, entry.Provider)
		}
		switch entry.Model {
		case "home-model":
			fallbacks++
		case "claude-sonnet-4-5":
			travelled++
		case "":
			t.Errorf("line %d's assistant record states no model: measured, the harness fails to build the "+
				"next request when one assistant record has none", index)
		default:
			t.Errorf("line %d's assistant record states model %q, want the model the source recorded or, "+
				"for the record it recorded none for, the harness's own configured id", index, entry.Model)
		}
	}
	if fallbacks != 1 || travelled != 1 {
		t.Errorf("assistant models = %d answered by the target home, %d carried from the source; want one "+
			"of each (the fixture's first assistant message records no model, its second records one)",
			fallbacks, travelled)
	}
	for _, loss := range session.Losses {
		if loss.Category != "message_model_attribution" {
			continue
		}
		// The count is the records whose model travelled plus the records the target home answered for:
		// dropping the fallback would leave the record model-less (caught above) and this count one short.
		if loss.Count != 2 {
			t.Errorf("message_model_attribution count = %d, want 2: the one source model that travelled and "+
				"the one the target home answered with", loss.Count)
		}
		return
	}
	t.Error("the report carries no message_model_attribution entry")
}

// TestOMPSessionExplainsTheModelsItSuppliedForEveryRecord pins the caveat when *no* source model travelled:
// every assistant record then carries the harness's own configured id, the count is exactly those records,
// and the detail is the OMP explanation saying those ids are the target home's answer -- the explanation
// whose absence would leave the file's non-empty models unexplained (the sibling case keeps one source
// model, which is why it does not see this branch).
func TestOMPSessionExplainsTheModelsItSuppliedForEveryRecord(t *testing.T) {
	conversation := piFixtureConversation()
	for index := range conversation.Messages {
		conversation.Messages[index].Model = ""
	}
	session, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	fallbacks, travelled := 0, 0
	for _, entry := range ompEntries(t, session.Data) {
		if entry.Type != piEntryMessage || entry.Role != piRoleAssistant {
			continue
		}
		switch entry.Model {
		case "home-model":
			fallbacks++
		case "":
			t.Errorf("an assistant record states no model: measured, the harness fails to build the next " +
				"request when one assistant record has none")
		default:
			travelled++
			t.Errorf("an assistant record states %q, want the target home's own configured id: the source "+
				"recorded no model at all", entry.Model)
		}
	}
	if fallbacks != 2 {
		t.Fatalf("assistant models = %d answered by the target home and %d carried from the source; want "+
			"the fixture's two assistant messages all answered by the target home", fallbacks, travelled)
	}
	for _, loss := range session.Losses {
		if loss.Category != "message_model_attribution" {
			continue
		}
		if loss.Count != fallbacks {
			t.Errorf("message_model_attribution count = %d, want %d: every record the target home supplied "+
				"a model for is counted", loss.Count, fallbacks)
		}
		for _, want := range []string{"request builder", "configured model id"} {
			if !strings.Contains(loss.Detail, want) {
				t.Errorf("message_model_attribution detail = %q, want it to explain the substituted ids "+
					"(%q)", loss.Detail, want)
			}
		}
		if strings.Contains(loss.Detail, "the source recorded no model on any message") {
			t.Errorf("message_model_attribution detail = %q, want the substitution explained rather than "+
				"only the missing source model", loss.Detail)
		}
		return
	}
	t.Error("the report carries no message_model_attribution entry")
}

// TestOMPSessionWithoutTheHomesAnswerRecordsNoBinding pins the dry-run shape and the report that goes with
// it: the harness is not started, so no session model binding is written and no endpoint label is stated
// on the assistant records -- and both facts are named in the report rather than left to be inferred from
// a missing record. The report's own sentence has to say what the missing binding costs, because a session
// without one is restored from the pairs its messages carry.
func TestOMPSessionWithoutTheHomesAnswerRecordsNoBinding(t *testing.T) {
	session, err := ConvertToOMPSession(piFixtureConversation(), OMPTargetOptions{
		ProviderDetail: "a dry run does not start the target harness",
	})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(session.Data), piEntryModelChange) {
		t.Error("a conversion that asked no harness wrote a session model binding")
	}
	for _, loss := range session.Losses {
		switch loss.Category {
		case "target_model_binding":
			if loss.Count != 0 {
				t.Errorf("target_model_binding count = %d, want 0: no binding was written", loss.Count)
			}
			for _, want := range []string{"a dry run does not start the target harness", "SessionModelRestoreError", "model"} {
				if !strings.Contains(loss.Detail, want) {
					t.Errorf("target_model_binding detail = %q, want it to name %q", loss.Detail, want)
				}
			}
		case "message_model_attribution":
			if !strings.Contains(loss.Detail, "request builder") {
				t.Errorf("message_model_attribution detail = %q, want the measured reason the record states "+
					"an endpoint at all", loss.Detail)
			}
		}
	}

	// A harness that answered a provider but no model leaves the same hole, and the binding is written
	// only when both halves are answered: a `<provider>/<model>` string with one half missing names
	// nothing the harness could restore.
	partial, err := ConvertToOMPSession(piFixtureConversation(), OMPTargetOptions{Provider: "home-proxy"})
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	if strings.Contains(string(partial.Data), piEntryModelChange) {
		t.Error("a conversion with a provider and no model wrote a session model binding")
	}
}

// TestOMPSessionRecordsTheHomesBindingWhenBothHalvesAreAnswered pins the record a converted session is
// opened through, and the report entry that names it: the harness's own answer, never the source's.
func TestOMPSessionRecordsTheHomesBindingWhenBothHalvesAreAnswered(t *testing.T) {
	session, err := ConvertToOMPSession(piFixtureConversation(), ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	var binding string
	for _, line := range strings.Split(strings.TrimSpace(string(session.Data)), "\n") {
		var entry struct {
			Type  string `json:"type"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type == piEntryModelChange {
			binding = entry.Model
		}
	}
	if binding != "home-proxy/home-model" {
		t.Errorf("binding = %q, want the harness's own answer", binding)
	}
	for _, loss := range session.Losses {
		if loss.Category != "target_model_binding" {
			continue
		}
		if loss.Count != 1 {
			t.Errorf("target_model_binding count = %d, want the one record written", loss.Count)
		}
		for _, want := range []string{"home-proxy/home-model", "restores a session's model", "never the source's"} {
			if !strings.Contains(loss.Detail, want) {
				t.Errorf("detail = %q, want it to state %q", loss.Detail, want)
			}
		}
		return
	}
	t.Error("the report carries no target_model_binding entry")
}

// TestOMPSessionReportsTheOMPOnlyLossCategories pins the family difference in the report: OMP's two own
// categories are listed on every one of its conversions with their counts, and Pi's list does not grow
// them.
func TestOMPSessionReportsTheOMPOnlyLossCategories(t *testing.T) {
	ompSession, err := ConvertToOMPSession(piFixtureConversation(), ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	seen := map[string]Loss{}
	for _, loss := range ompSession.Losses {
		seen[loss.Category] = loss
	}
	for _, category := range []string{"target_model_binding", "title_slot_written"} {
		if _, ok := seen[category]; !ok {
			t.Errorf("the OMP report carries no %s entry", category)
		}
	}
	if seen["title_slot_written"].Count != 1 {
		t.Errorf("title_slot_written count = %d, want the one line written",
			seen["title_slot_written"].Count)
	}
	if !strings.Contains(seen["title_slot_written"].Detail, "rewrites in place") {
		t.Errorf("title_slot_written detail = %q, want it to say what the harness does with the line",
			seen["title_slot_written"].Detail)
	}

	piSession, err := ConvertToPiSession(piFixtureConversation(), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting to Pi: %v", err)
	}
	for _, loss := range piSession.Losses {
		if loss.Category == "title_slot_written" || loss.Category == "target_model_binding" {
			t.Errorf("the Pi report carries the OMP-only category %s (count %d)", loss.Category, loss.Count)
		}
	}
}

// TestOMPSessionOmitsAPiOnlyRecord pins the other direction of the family difference: the Pi writer's
// records that OMP's own writer does not produce (there is none it must not) and, more usefully, that the
// OMP file's *first* line is the title slot rather than the header -- the shape a Pi file has.
func TestOMPSessionOmitsAPiOnlyRecord(t *testing.T) {
	session, err := ConvertToOMPSession(piFixtureConversation(), ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	lines := strings.SplitN(string(session.Data), "\n", 2)
	var first struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("the first line is not JSON: %v", err)
	}
	if first.Type != piEntryTitle {
		t.Errorf("the first line is a %s, want the title slot: Pi's file starts with the header instead, and "+
			"this is the one shape the two formats do not share", first.Type)
	}

	piSession, err := ConvertToPiSession(piFixtureConversation(), PiTargetOptions{})
	if err != nil {
		t.Fatalf("converting to Pi: %v", err)
	}
	if strings.Contains(string(piSession.Data), `"type":"title"`) {
		t.Error("the Pi file carries an OMP title slot")
	}
}

// TestOMPSessionRefusesASourceWithoutAWorkingDirectory pins the shared family rule for this route: a
// session is filed under the project directory OMP's own spelling derives from the working directory, so
// a document that records none is refused before anything is written.
func TestOMPSessionRefusesASourceWithoutAWorkingDirectory(t *testing.T) {
	conversation := piFixtureConversation()
	conversation.Source.Cwd = ""
	if _, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions()); err != ErrNoTargetCwd {
		t.Fatalf("error = %v, want ErrNoTargetCwd", err)
	}
}

// TestOMPProjectDirMatchesOMPsOwnSpelling pins the three branches of the directory OMP files a session
// under, against the installed harness's own rule (`getDefaultSessionDirPath`): a working directory inside
// the OS temporary directory spells `-tmp` + its relative path (and that branch wins when both apply), one
// inside the home directory spells `-` + its home-relative path, and anything else uses the absolute
// `--…--` spelling -- which is the only case where Pi's rule and OMP's agree.
//
// The base directories and every case's working directory are the host's own resolved spellings
// (`filepath.Abs`): the rule resolves the working directory before comparing it, and on a platform whose
// rooted paths carry a volume (Windows) a base left as `/tmp` has no relative spelling to the resolved
// path at all -- the temporary and home branches would be unreachable there and every row would pin the
// absolute branch. The base directories are still passed in rather than read from this host, because the
// rule has three branches and a test that could only reach one of them on the machine it runs on would
// pin one third of it.
func TestOMPProjectDirMatchesOMPsOwnSpelling(t *testing.T) {
	home := absHostPath(t, "/home/example")
	tmp := absHostPath(t, "/tmp")
	// The absolute branch spells the whole resolved path, so on a platform with a volume its name carries
	// that too (a Windows drive): its expectation is built from the host-shaped path with this test's own
	// spelling of the rule, and the drive-colon half -- which no POSIX path exercises -- is pinned
	// literally below.
	outside := absHostPath(t, "/srv/work/project")
	runsOn := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	outsideWant := "--" + runsOn.Replace(strings.TrimLeft(outside, `/\`)) + "--"
	for _, testCase := range []struct {
		name string
		cwd  string
		want string
	}{
		{"a directory under the temporary directory", "/tmp/work/project", "-tmp-work-project"},
		{"the temporary directory itself", "/tmp", "-tmp"},
		{"a directory under the home directory", "/home/example/repos/call-ccc", "-repos-call-ccc"},
		{"the home directory itself", "/home/example", "-"},
		{"a directory under both, where the temporary one wins",
			"/tmp/home-example/work", "-tmp-home-example-work"},
		{"a directory outside both", outside, outsideWant},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ompProjectDirFor(testCase.cwd, home, tmp); got != testCase.want {
				t.Errorf("ompProjectDirFor(%q) = %q, want %q", testCase.cwd, got, testCase.want)
			}
		})
	}
	// The drive colon's own row: the absolute branch replaces it like a separator, and a POSIX path never
	// carries one for the table above to exercise.
	if got := ompSeparatorSpelling(`C:\work\api`); got != "C--work-api" {
		t.Errorf(`ompSeparatorSpelling(C:\work\api) = %q, want "C--work-api": every separator and the drive `+
			"colon is spelled `-`", got)
	}

	// The rule is not Pi's, and the difference is what this route must not lose: for a working directory
	// under the home directory or the temporary directory the two harnesses spell it differently, and a
	// session filed under Pi's spelling is moved by OMP itself.
	if got := ompProjectDirFor("/home/example/repos/call-ccc", home, tmp); got == PiProjectDir("/home/example/repos/call-ccc") {
		t.Errorf("the OMP spelling equals Pi's (%q): measured, OMP computes its own shortened form for a "+
			"working directory under the home or temporary directory", got)
	}
	if got := ompProjectDirFor(outside, home, tmp); got != PiProjectDir(outside) {
		t.Errorf("the outside-both spelling = %q, want Pi's own %q: that is the one branch the harnesses "+
			"share", got, PiProjectDir(outside))
	}
}

// absHostPath resolves one spelled path the way the rule under test resolves a working directory, so a
// case's base directories and its expected names agree about the host's own spelling of an absolute path.
func absHostPath(t *testing.T, path string) string {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolving %q: %v", path, err)
	}
	return absolute
}

// TestOMPTitleSlotLineIsAFixedWidthLine pins the line's width on its own, because it is the property the
// harness's in-place rewrite depends on: OMP rewrites the slot by replacing the file's first 256 bytes and
// keeping the rest, so a line of another width would move (or cut) every entry after it.
func TestOMPTitleSlotLineIsAFixedWidthLine(t *testing.T) {
	for _, stamp := range []string{
		"2026-05-04T09:15:00.000Z",
		"2026-05-04T09:15:00.000+08:00",
		"2026-05-04T09:15:00Z",
	} {
		line := piTitleSlotLine(stamp)
		if len(line) != ompTitleSlotWidth {
			t.Errorf("piTitleSlotLine(%q) is %d bytes, want %d (line feed included)",
				stamp, len(line), ompTitleSlotWidth)
		}
		if !strings.HasSuffix(string(line), "\n") {
			t.Errorf("piTitleSlotLine(%q) does not end in a line feed", stamp)
		}
		var decoded map[string]any
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("piTitleSlotLine(%q) is not JSON: %v", stamp, err)
		}
		if decoded["type"] != piEntryTitle || decoded["title"] != "" {
			t.Errorf("piTitleSlotLine(%q) = %v, want an empty title record", stamp, decoded)
		}
	}
}

// ompEntry is one decoded line of an OMP session, with the members these cases ask about.
type ompEntry struct {
	Type     string
	ID       string
	ParentID *string
	Role     string
	Text     string
	Model    string
	Provider string
}

// ompEntries decodes a written OMP session's lines into the members the writer's cases assert on.
func ompEntries(t *testing.T, data []byte) []ompEntry {
	t.Helper()
	var entries []ompEntry
	for index, line := range decodePiSession(t, data) {
		entry := ompEntry{Type: line.Type, ID: line.ID, ParentID: line.ParentID}
		if len(line.Message) > 0 && string(line.Message) != "null" {
			var message struct {
				Role     string `json:"role"`
				Model    string `json:"model"`
				Provider string `json:"provider"`
				Content  any    `json:"content"`
			}
			if err := json.Unmarshal(line.Message, &message); err != nil {
				t.Fatalf("line %d's message is not one object: %v", index, err)
			}
			entry.Role, entry.Model, entry.Provider = message.Role, message.Model, message.Provider
			switch content := message.Content.(type) {
			case string:
				entry.Text = content
			case []any:
				for _, block := range content {
					if text, ok := block.(map[string]any)["text"].(string); ok {
						entry.Text += text
					}
				}
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// ompWalkToRoot walks a file's own tree from the entry written last back through `parentId` -- the walk the
// harness's loader makes when it builds a session's context -- and answers the entries it reached, by id.
func ompWalkToRoot(t *testing.T, entries []ompEntry) map[string]bool {
	t.Helper()
	byID := map[string]ompEntry{}
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	reached := map[string]bool{}
	current := entries[len(entries)-1]
	for {
		reached[current.ID] = true
		if current.ParentID == nil {
			return reached
		}
		next, ok := byID[*current.ParentID]
		if !ok {
			t.Fatalf("entry %q names a parent %q the file does not hold", current.ID, *current.ParentID)
		}
		current = next
	}
}

// TestOMPSessionChainsTheSourcesOwnRelationsOntoTheBinding pins the tree the writer produces: the source's
// own relations are reproduced (an entry hangs off the entry of the record the source named, so a branch
// the conversation left stays off the walk), the source's first record hangs off the session's model
// binding rather than standing parentless (measured: a binding nothing chains to is not restored), and the
// walk the harness's loader makes from the file's last entry reaches both.
func TestOMPSessionChainsTheSourcesOwnRelationsOntoTheBinding(t *testing.T) {
	conversation := Conversation{
		Source: SourceIdentity{Provider: "claude", SessionID: "source-session", Cwd: "/work/project"},
		Messages: []Message{
			{Ordinal: 0, Role: string(parser.RoleUser), Dialogue: "First.", SourceUUID: "a",
				Timestamp: time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)},
			{Ordinal: 1, Role: string(parser.RoleAssistant), Dialogue: "Second.", SourceUUID: "b",
				SourceParentUUID: "a", Model: "claude-sonnet-4-5",
				Timestamp: time.Date(2026, 5, 5, 10, 0, 1, 0, time.UTC)},
			// The source's last record, hanging off the same prompt as the second one: the abandoned
			// branch's own turn, and the file's last entry.
			{Ordinal: 2, Role: string(parser.RoleAssistant), Dialogue: "An abandoned branch.", SourceUUID: "c",
				SourceParentUUID: "a", Model: "claude-sonnet-4-5",
				Timestamp: time.Date(2026, 5, 5, 10, 0, 2, 0, time.UTC)},
		},
		Fidelity: Fidelity{ReadFrom: "source"},
	}
	session, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions())
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	entries := ompEntries(t, session.Data)

	bindingID := ""
	entryOfText := map[string]ompEntry{}
	for _, entry := range entries {
		if entry.Type == piEntryModelChange {
			bindingID = entry.ID
		}
		if entry.Type == piEntryMessage {
			entryOfText[entry.Text] = entry
		}
	}
	if bindingID == "" {
		t.Fatal("the file carries no session model binding")
	}
	if len(entryOfText) != 3 {
		t.Fatalf("the file holds %d message(s), want the three the source recorded", len(entryOfText))
	}

	first := entryOfText["First."]
	if first.ParentID == nil || *first.ParentID != bindingID {
		t.Errorf("the source's first record hangs off %v, want the binding %q: measured, a binding the "+
			"walk does not reach is not restored", first.ParentID, bindingID)
	}
	second := entryOfText["Second."]
	if second.ParentID == nil || *second.ParentID != first.ID {
		t.Errorf("the record the source placed under %q hangs off %v, want that record's own entry %q",
			"a", second.ParentID, first.ID)
	}
	branch := entryOfText["An abandoned branch."]
	if branch.ParentID == nil || *branch.ParentID != first.ID {
		t.Errorf("the branch's own turn hangs off %v, want the entry of the record it named %q",
			branch.ParentID, first.ID)
	}
	if branch.ID != entries[len(entries)-1].ID {
		t.Errorf("the file's last entry is %q, want the source's last record's own entry %q",
			entries[len(entries)-1].ID, branch.ID)
	}

	reached := ompWalkToRoot(t, entries)
	for _, want := range []ompEntry{first, branch} {
		if !reached[want.ID] {
			t.Errorf("the walk from the file's last entry does not reach %q", want.Text)
		}
	}
	if !reached[bindingID] {
		t.Error("the walk from the file's last entry does not reach the session's model binding")
	}
	if reached[second.ID] {
		t.Errorf("the walk from the file's last entry reaches %q, which the source placed off this line",
			second.Text)
	}
}
