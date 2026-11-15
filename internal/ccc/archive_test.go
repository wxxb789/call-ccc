package ccc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/parser"
)

// codexStaleCallID is the shell call the stale-archive case's Codex rollout records, and the identity
// its output event carries.
const codexStaleCallID = "call_01Stale00000000000000000001"

// TestIngestThenArchiveRead covers the archive half of the contract: a real source file is parsed and
// written into a SQLite archive by the inherited sync engine, read back through the inherited read
// path, and the original source is byte-identical afterwards. Every path it touches is inside the
// test's own temporary directory.
func TestIngestThenArchiveRead(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	codexFile := fixturePath(t, filepath.Join("codex", "2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl"))
	before := hashTree(t, fixtureRoot(t))

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile, codexFile); err != nil {
		t.Fatalf("ingesting the fixtures: %v", err)
	}

	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	for _, ref := range []Ref{claudeFixtureRef, codexFixtureRef} {
		reader := NewSourceReader(cfg)
		fromSource, err := reader.Read(ctx, ref)
		if err != nil {
			t.Fatalf("%s: reading the source: %v", ref.ID, err)
		}
		status, found, err := archive.Status(ctx, ref)
		if err != nil {
			t.Fatalf("%s: archive status: %v", ref.ID, err)
		}
		if !found {
			t.Fatalf("%s: the ingested session is not in the archive", ref.ID)
		}
		fromArchive, err := archive.Read(ctx, ref)
		if err != nil {
			t.Fatalf("%s: reading the archive: %v", ref.ID, err)
		}
		archiveCounts := Summarize(fromArchive.Messages)
		if status.MessageCount != archiveCounts.Messages {
			t.Errorf("%s: status reports %d message(s), the read returned %d",
				ref.ID, status.MessageCount, archiveCounts.Messages)
		}
		if fromArchive.Fidelity.ReadFrom != "archive" || fromArchive.Fidelity.ContentShown != "tool-payloads" {
			t.Errorf("%s: archive read fidelity = %+v", ref.ID, fromArchive.Fidelity)
		}
		if fromArchive.Fidelity.MalformedLines != fromSource.Fidelity.MalformedLines {
			t.Errorf("%s: malformed-line count differs (source %d, archive %d)",
				ref.ID, fromSource.Fidelity.MalformedLines, fromArchive.Fidelity.MalformedLines)
		}
		same, folded, warnings := compareMessages(fromSource.Messages, fromArchive.Messages)
		if !same {
			t.Errorf("%s: archive and source messages disagree: %v", ref.ID, warnings)
		}
		if status.SourcePath == "" || status.Project == "" {
			t.Errorf("%s: archive status lost source identity: %+v", ref.ID, status)
		}
		if ref == claudeFixtureRef {
			// The Claude fixture's tool result arrives as its own message in the parse result and is
			// stored on the call; the archive must still carry its content.
			if folded != 1 {
				t.Errorf("claude: folded tool-result rows = %d, want 1", folded)
			}
			// The fixture's call is a Read, and the inherited default configuration blocks result
			// content for the Read and Glob categories: the archive keeps the length (and a digest it
			// does not expose) but not the body. That is upstream's own default, reproduced here.
			foldedResults := fromArchive.Messages[2].ToolResults
			if len(foldedResults) != 1 {
				t.Fatalf("claude: folded tool results = %+v, want one", foldedResults)
			}
			if foldedResults[0].ContentSize != 37 || foldedResults[0].ContentRaw != "" {
				t.Errorf("claude: blocked-category result should keep its length and drop its body: %+v",
					foldedResults[0])
			}
		}
		if ref == codexFixtureRef {
			if folded != 0 {
				t.Errorf("codex: unexpected folded rows: %d", folded)
			}
			// A shell call is not a blocked category, so its result body survives the round trip.
			found := ""
			for _, message := range fromArchive.Messages {
				for _, result := range message.ToolResults {
					if strings.Contains(result.ContentRaw, "maxBackoffSeconds") {
						found = result.ContentRaw
					}
				}
			}
			if found == "" {
				t.Errorf("codex: the archive dropped the shell result body")
			}
		}
		sourceCounts, archiveCounts := Summarize(fromSource.Messages), Summarize(fromArchive.Messages)
		if sourceCounts.ToolCalls != archiveCounts.ToolCalls ||
			sourceCounts.CompactBoundaries != archiveCounts.CompactBoundaries ||
			sourceCounts.SystemRecords != archiveCounts.SystemRecords {
			t.Errorf("%s: tool/compaction counts differ (source %+v, archive %+v)",
				ref.ID, sourceCounts, archiveCounts)
		}
		t.Logf("%s: stored id %q, %d message(s), %d folded row(s), content shown %q",
			ref.ID, status.SessionID, status.MessageCount, folded, fromArchive.Fidelity.ContentShown)
	}

	after := hashTree(t, fixtureRoot(t))
	if len(before) != len(after) {
		t.Fatalf("the fixture tree changed size: %d -> %d file(s)", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("fixture %s changed while being read", path)
		}
	}
}

// TestIngestorNeedsAnExplicitAbsolutePath pins the first half of the non-destructive rule: a write
// path is always named by the caller, never defaulted.
func TestIngestorNeedsAnExplicitAbsolutePath(t *testing.T) {
	cfg := fixtureConfig(t)
	if _, err := NewIngestor(cfg, "relative/sessions.db"); err == nil {
		t.Fatal("NewIngestor accepted a relative path")
	}
	if _, err := NewIngestor(cfg, ""); err == nil {
		t.Fatal("NewIngestor accepted an empty path")
	}
	if _, err := NewIngestor(cfg, cfg.DBPath); err != nil {
		t.Fatalf("NewIngestor rejected an explicit isolated archive path: %v", err)
	}
}

// TestIngestorRefusesAHeldWriteLock pins the ownership rule: while another writer holds the data
// directory's write-owner lock, call/ccc refuses to write there and leaves nothing behind.
func TestIngestorRefusesAHeldWriteLock(t *testing.T) {
	cfg := fixtureConfig(t)
	dir := t.TempDir()
	holder := flock.New(filepath.Join(dir, writeOwnerLockFile))
	if held, err := holder.TryLock(); err != nil || !held {
		t.Fatalf("taking the write-owner lock for the test: held=%v err=%v", held, err)
	}
	defer holder.Unlock()

	ingestor, err := NewIngestor(cfg, filepath.Join(dir, "sessions.db"))
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	err = ingestor.IngestPaths(context.Background(), fixturePath(t,
		filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")))
	if err == nil {
		t.Fatal("IngestPaths wrote while another writer held the write-owner lock")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sessions.db")); !os.IsNotExist(statErr) {
		t.Errorf("the refused ingest created an archive anyway (stat err = %v)", statErr)
	}
}

// TestIngestorReusesAStaleWriteLock pins upstream's recovery rule: the lock file outlives the writer
// that owned it (flock releases on process exit), so an unheld file must not block a write.
func TestIngestorReusesAStaleWriteLock(t *testing.T) {
	cfg := fixtureConfig(t)
	dir := t.TempDir()
	stale := flock.New(filepath.Join(dir, writeOwnerLockFile))
	if held, err := stale.TryLock(); err != nil || !held {
		t.Fatalf("preparing the stale lock file: held=%v err=%v", held, err)
	}
	if err := stale.Unlock(); err != nil {
		t.Fatalf("releasing the stale lock: %v", err)
	}
	ingestor, err := NewIngestor(cfg, filepath.Join(dir, "sessions.db"))
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(context.Background(), fixturePath(t,
		filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))); err != nil {
		t.Fatalf("ingesting past an unheld lock file: %v", err)
	}
}

// TestArchiveMissingIsReported pins that a missing archive is a reported state, not a failure.
func TestArchiveMissingIsReported(t *testing.T) {
	cfg := fixtureConfig(t)
	if _, err := OpenArchive(context.Background(), filepath.Join(cfg.DataDir, "absent.db")); err == nil {
		t.Fatal("OpenArchive accepted a missing archive")
	}
	empty := filepath.Join(cfg.DataDir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("writing the empty file: %v", err)
	}
	if _, err := OpenArchive(context.Background(), empty); err == nil {
		t.Fatal("OpenArchive accepted an empty archive")
	}
}

// TestArchiveReadDoesNotChangeTheArchive pins the read path's non-destructive claim: reading an
// archive leaves the database file byte-identical and creates no file beyond SQLite's own WAL
// sidecars (which exist because the writer left the database in WAL mode).
func TestArchiveReadDoesNotChangeTheArchive(t *testing.T) {
	cfg := fixtureConfig(t)
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, fixturePath(t,
		filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	dir := filepath.Dir(archivePath)
	before := hashTree(t, dir)["sessions.db"]

	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	if _, _, err := archive.Status(ctx, claudeFixtureRef); err != nil {
		t.Fatalf("status: %v", err)
	}
	if _, err := archive.Read(ctx, claudeFixtureRef); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if after := hashTree(t, dir)["sessions.db"]; after != before {
		t.Errorf("reading the archive changed sessions.db (%s -> %s)", before, after)
	}
	allowed := map[string]bool{
		"sessions.db": true, "sessions.db-wal": true, "sessions.db-shm": true, writeOwnerLockFile: true,
	}
	for path := range hashTree(t, dir) {
		if !allowed[path] {
			t.Errorf("reading the archive created %s", path)
		}
	}
}

// TestResolveRejectsAnUnknownProviderAndEmptyID pins the two identifier failures that must not be
// answered with a guess.
func TestResolveRejectsAnUnknownProviderAndEmptyID(t *testing.T) {
	reader := NewSourceReader(fixtureConfig(t))
	ctx := context.Background()
	if _, _, err := reader.Resolve(ctx, Ref{Provider: "no-such-agent", ID: "x"}); err == nil {
		t.Error("Resolve accepted an unknown provider")
	}
	if _, _, err := reader.Resolve(ctx, Ref{Provider: "claude", ID: "  "}); err == nil {
		t.Error("Resolve accepted an empty session id")
	}
	if _, err := reader.Read(ctx, Ref{Provider: "claude", ID: "00000000-0000-4000-8000-000000000000"}); err == nil {
		t.Error("Read answered a session the fixtures do not contain")
	}
}

// TestPathReferenceResolvesTheSameSession pins that a path reference and an id reference reach the
// same session through the provider's own lookup.
func TestPathReferenceResolvesTheSameSession(t *testing.T) {
	cfg := fixtureConfig(t)
	reader := NewSourceReader(cfg)
	path := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	byPath, err := reader.Read(context.Background(), Ref{Provider: "claude", ID: path})
	if err != nil {
		t.Fatalf("reading by path: %v", err)
	}
	if byPath.Source.Provider != string(parser.AgentClaude) {
		t.Errorf("path read reports provider %q", byPath.Source.Provider)
	}
	if byPath.Source.SourcePath != path {
		t.Errorf("path read reports source %q, want %q", byPath.Source.SourcePath, path)
	}
}

// TestHostQualifiedReferenceResolvesThatHostsSession pins the identity rule for a session synced from
// another machine.
//
// The archive stores one as `<host>~<the id it had there>` and the host is part of its identity
// (parser.StripHostPrefix). A reference that names a host used to have the host stripped before the
// lookup, so `codex:host-a~codex:<native id>` resolved `codex:<native id>` -- this machine's session
// with the same native id -- whenever that row existed: an inspection of a remote session reported a
// different conversation's identity, counts and messages, and exited successfully. Here the archive
// holds a local row, two `host-a` rows and one `host-b` row of the same native id, seeded through the
// inherited database API (which is how a remote sync writes them), and every way that lookup can answer
// is pinned: the stored spelling, the host-qualified spelling without an agent prefix, a host this
// archive has nothing from, and two rows of the requested host.
func TestHostQualifiedReferenceResolvesThatHostsSession(t *testing.T) {
	const native = "0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812"
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	seedCodex := func(id, machine, content string) {
		t.Helper()
		dbtest.SeedSessionWithMessages(t, database, id, "example-api",
			[]db.Message{dbtest.UserMsg(id, 0, content)},
			func(session *db.Session) { session.Agent = "codex"; session.Machine = machine })
	}
	localID := "codex:" + native
	remoteID := "host-a~codex:" + native
	bareHostB := "host-b~" + native
	seedCodex(localID, "local", "the session recorded on this machine")
	seedCodex(remoteID, "host-a", "the session recorded on host-a")
	seedCodex("host-a~codex-fork:"+native, "host-a", "the fork recorded on host-a")
	seedCodex("host-b~codex:"+native, "host-b", "the session recorded on host-b")
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	ctx := context.Background()
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	// The reference form the CLI takes: `<provider>:<host>~<native id>`, resolved exactly, so the row
	// it names is the answer and neither the local row nor host-a's fork is.
	qualified := Ref{Provider: "codex", ID: remoteID}
	status, found, err := archive.Status(ctx, qualified)
	if err != nil {
		t.Fatalf("status of %s: %v", qualified.ID, err)
	}
	if !found {
		t.Fatalf("the archive does not resolve %s", qualified.ID)
	}
	if status.SessionID != remoteID {
		t.Errorf("%s resolved %q, want %q", qualified.ID, status.SessionID, remoteID)
	}
	conversation, err := archive.Read(ctx, qualified)
	if err != nil {
		t.Fatalf("reading %s: %v", qualified.ID, err)
	}
	if len(conversation.Messages) != 1 ||
		!strings.Contains(conversation.Messages[0].Content, "host-a") {
		t.Errorf("%s read the wrong conversation: %+v", qualified.ID, conversation.Messages)
	}

	// A host-qualified native id that is not stored in that spelling still names that host's session
	// when that host has exactly one: the answer is narrowed to the requested host, so the local row --
	// which shares the native id -- is not a candidate.
	if status, found, err := archive.Status(ctx, Ref{Provider: "codex", ID: bareHostB}); err != nil ||
		!found || status.SessionID != "host-b~codex:"+native {
		t.Errorf("%s: found=%v err=%v status=%+v, want host-b's row", bareHostB, found, err, status)
	}

	// Two rows of the requested host are the candidates the reference could mean: reported, not picked.
	_, _, err = archive.Status(ctx, Ref{Provider: "codex", ID: "host-a~" + native})
	var ambiguous *AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("two host-a rows resolved without a report: err=%v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("ambiguous candidates = %v, want both host-a rows", ambiguous.Candidates)
	}

	// A host this archive has nothing from is not found: neither another host's row nor the local row
	// of that native id is offered in its place.
	for _, id := range []string{"host-c~codex:" + native, "host-a~codex:" + native + "x"} {
		ref := Ref{Provider: "codex", ID: id}
		if status, found, err := archive.Status(ctx, ref); err != nil || found {
			t.Errorf("%s resolved %+v (found=%v err=%v), want not found", id, status, found, err)
		}
	}
}

// TestHostQualifiedReferenceIsReportedWithoutASource pins what an inspection does with a reference to a
// session recorded on another machine: the archive answers it, and the source side says why it cannot
// rather than reporting a local session of the same native id.
func TestHostQualifiedReferenceIsReportedWithoutASource(t *testing.T) {
	cfg := fixtureConfig(t)
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	remoteID := "host-a~codex:" + codexFixtureRef.ID
	dbtest.SeedSessionWithMessages(t, database, remoteID, "example-api",
		[]db.Message{dbtest.UserMsg(remoteID, 0, "the session recorded on host-a")},
		func(session *db.Session) { session.Agent = "codex"; session.Machine = "host-a" })
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	// The fixtures hold a Codex session under the same native id, so a reader that ignored the host
	// would report *that* conversation from the source side.
	reader := NewSourceReader(cfg)
	ctx := context.Background()
	ref := Ref{Provider: "codex", ID: remoteID}
	inspection, err := Inspect(ctx, InspectOptions{
		Ref: ref, Config: cfg, Reader: reader, ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting %s: %v", ref.ID, err)
	}
	if inspection.Source != nil {
		t.Errorf("a host-qualified reference read a local source: %+v", inspection.Source.Identity)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != remoteID {
		t.Errorf("the inspection reports %q, want %q", inspection.Archive.Status.SessionID, remoteID)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "another machine") {
		t.Errorf("the inspection does not say why the source is unreadable: %v", inspection.Warnings)
	}
}

// TestPathReferenceFindsTheArchivedSession pins the reference form the README documents
// (`ccc inspect codex:/path/to/<id>.jsonl`) against the archive: a path reference must resolve
// through the file the session was read from, because the stored id is `<agent>:<native id>` and a
// path never is one. Before this held, an inspection by path said the session was not in the archive
// yet while an inspection by id found it.
func TestPathReferenceFindsTheArchivedSession(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	byPath := Ref{Provider: "claude", ID: claudeFile}
	status, found, err := archive.Status(ctx, byPath)
	if err != nil {
		t.Fatalf("status by path: %v", err)
	}
	if !found {
		t.Fatalf("the archive does not resolve the session by its source path %s", claudeFile)
	}
	fromPath, err := archive.Read(ctx, byPath)
	if err != nil {
		t.Fatalf("reading by path: %v", err)
	}
	fromID, err := archive.Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading by id: %v", err)
	}
	if Summarize(fromPath.Messages) != Summarize(fromID.Messages) {
		t.Errorf("the path reference read a different session: %+v vs %+v",
			Summarize(fromPath.Messages), Summarize(fromID.Messages))
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         byPath,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting by path: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("inspecting by path did not find the archived session: warnings %v", inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != status.SessionID {
		t.Errorf("the inspection reports session %q, the status %q",
			inspection.Archive.Status.SessionID, status.SessionID)
	}
}

// TestRelativePathReferenceFindsTheArchivedSession pins the archive half of resolving a relative
// filesystem reference.
//
// The archive stores the absolute path `sessions.file_path` was written with, so a relative spelling
// cannot equal it and the lookup used to miss the session the source read had just found. The roots a
// relative path is relative to come from the configuration, which is why an archive opened without one
// cannot answer this question at all -- and says so by not finding it rather than by guessing a
// directory.
func TestRelativePathReferenceFindsTheArchivedSession(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	relative := Ref{Provider: "claude", ID: filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")}

	archive, err := openArchive(ctx, archivePath, cfg.AgentDirs)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	status, found, err := archive.Status(ctx, relative)
	if err != nil {
		t.Fatalf("status of %s: %v", relative.ID, err)
	}
	if !found {
		t.Fatalf("the archive does not resolve the relative path %s", relative.ID)
	}

	// The inspection a caller takes (the CLI's own path) resolves the relative reference on both sides.
	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         relative,
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting %s: %v", relative.ID, err)
	}
	if inspection.Source == nil {
		t.Fatalf("the inspection read no source for %s: %v", relative.ID, inspection.Warnings)
	}
	if inspection.Archive == nil {
		t.Fatalf("the inspection found no archived session for %s: %v", relative.ID, inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != status.SessionID {
		t.Errorf("the inspection reports %q, the direct lookup %q",
			inspection.Archive.Status.SessionID, status.SessionID)
	}

	// An archive opened without a configuration has no root to resolve the reference against, and does
	// not answer with a session it cannot be sure the reference names.
	plain, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive without a configuration: %v", err)
	}
	defer plain.Close()
	if status, found, err := plain.Status(ctx, relative); err != nil || found {
		t.Errorf("an archive with no roots resolved %s: found=%v err=%v status=%+v",
			relative.ID, found, err, status)
	}
}

// TestArchiveReferenceDoesNotCrossProviders pins that the inherited raw-suffix lookup is narrowed to
// the provider a reference names. That lookup answers "which stored ids end in this native id" and has
// no agent predicate, so without the narrowing `codex:<a claude session's id>` resolves the Claude row:
// the inspection reports another agent's conversation under the requested label and the CLI exits
// successfully although no Codex session exists anywhere.
func TestArchiveReferenceDoesNotCrossProviders(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	// The archive holds the Claude session and nothing else, and its stored id is `claude:<native id>`:
	// the same native id asked for as a Codex session is not this archive's Codex session.
	crossed := Ref{Provider: "codex", ID: claudeFixtureRef.ID}
	if status, found, err := archive.Status(ctx, crossed); err != nil || found {
		t.Errorf("a codex reference resolved the claude row: found=%v err=%v status=%+v",
			found, err, status)
	}
	if _, err := archive.Read(ctx, crossed); err == nil {
		t.Error("reading a codex reference returned the claude row")
	} else if !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a codex reference failed with %v, want a not-found answer", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         crossed,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if inspection.Archive != nil {
		t.Errorf("the inspection reported the claude row for a codex reference: %+v", inspection.Archive)
	}
	if !strings.Contains(strings.Join(inspection.Warnings, "\n"),
		"no readable source session for codex:"+claudeFixtureRef.ID) {
		t.Errorf("the inspection does not say the codex source was unreadable: %v", inspection.Warnings)
	}
}

// TestDeletedSourcePathFallsBackToTheArchive pins what an inspection does when the source file an
// archive was written from is gone: the archived session is reported, and the unreadable source is a
// warning rather than the loss of the whole inspection.
//
// The provider's changed-path classifier answers for an absent path -- that is what keeps a watcher
// from dropping a deletion -- so a deleted source used to be reported as a *source* the reference
// names, and the read then failed while fingerprinting. The archive, which holds the session, was
// never consulted: the error escaped `Inspect` before the archive was opened. A classification with no
// file behind it is not a local source, so the reference resolves to nothing readable and the archive
// answers.
func TestDeletedSourcePathFallsBackToTheArchive(t *testing.T) {
	// A throw-away provider root: deleting the transcript must not touch the committed fixture.
	root := t.TempDir()
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	relative := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	source := filepath.Join(root, "claude", relative)
	data, err := os.ReadFile(fixturePath(t, filepath.Join("claude", relative)))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatalf("writing the transcript copy: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, source); err != nil {
		t.Fatalf("ingesting the transcript: %v", err)
	}

	// The session is archived; now its source file is gone, which is the state an archive exists for.
	if err := os.Remove(source); err != nil {
		t.Fatalf("removing the source transcript: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         Ref{Provider: "claude", ID: source},
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting a deleted source path: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("the inspection reported a source for a deleted path: %+v", inspection.Source.Identity)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	// Claude's stored ids carry no agent prefix, so the row the archive answers with is the fixture's
	// own native id -- the same spelling the recorded oracle dump shows for this fixture.
	if want := claudeFixtureRef.ID; inspection.Archive.Status.SessionID != want {
		t.Errorf("the inspection reports %q, want %q", inspection.Archive.Status.SessionID, want)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "no readable source session for claude:"+source) {
		t.Errorf("the inspection does not say the source was unreadable: %v", inspection.Warnings)
	}
}

// TestArchiveDoesNotAnswerACrossProviderHostQualifiedReference pins the provider half of the
// host-qualified rule: the exact spelling a reference names is answered only by a row the requested
// provider recorded.
//
// A remote row's id is `<host>~<the id it had there>` and Claude's stored ids carry no agent prefix,
// so `codex:host-a~<a claude native id>` *is* the archive's spelling of a Claude row. An exact
// `GetSession` answer returned without asking whose conversation it is made an inspection report the
// Claude session under the Codex label and exit successfully, although no Codex session exists there.
func TestArchiveDoesNotAnswerACrossProviderHostQualifiedReference(t *testing.T) {
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	remoteID := "host-a~" + claudeFixtureRef.ID
	dbtest.SeedSessionWithMessages(t, database, remoteID, "example-api",
		[]db.Message{dbtest.UserMsg(remoteID, 0, "the claude session recorded on host-a")},
		func(session *db.Session) { session.Agent = "claude"; session.Machine = "host-a" })
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	ctx := context.Background()
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	crossed := Ref{Provider: "codex", ID: remoteID}
	if status, found, err := archive.Status(ctx, crossed); err != nil || found {
		t.Errorf("a codex reference resolved the claude row: found=%v err=%v status=%+v",
			found, err, status)
	}
	if _, err := archive.Read(ctx, crossed); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a codex reference answered %v, want a not-found answer", err)
	}

	// The row is answered for the provider that recorded it, so the rule is the provider and not the
	// host: a reader that refused host-qualified references wholesale would pass the checks above.
	status, found, err := archive.Status(ctx, Ref{Provider: "claude", ID: remoteID})
	if err != nil || !found {
		t.Fatalf("claude:%s answered found=%v err=%v, want the stored row", remoteID, found, err)
	}
	if status.SessionID != remoteID || status.Agent != "claude" {
		t.Errorf("claude:%s resolved %+v, want the claude row it names", remoteID, status)
	}
}

// TestArchiveFindsAHostQualifiedRowBeyondTheFirstCandidates pins that a filtered lookup does not
// establish absence from a candidate list it truncated before filtering.
//
// The inherited suffix lookup answers "up to limit" rows, most recently active first, and this
// package filters that answer by provider and host. With more rows ending in the native id than the
// first limit, a row of the requested host that is older than all of them used to be filtered out of a
// truncated answer: `codex:host-a~<native id>` misses the exact lookup (the stored spelling carries the
// agent prefix) and then received only the newer hosts' rows, so the session it names was reported as
// absent although it is in the archive. The lookup now grows its answer until the inherited query
// reports it complete.
func TestArchiveFindsAHostQualifiedRowBeyondTheFirstCandidates(t *testing.T) {
	const native = "3f7a0c14-2b6d-4e59-8c1a-5d2f9e0b7a41"
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	seed := func(id, host, started, content string) {
		t.Helper()
		dbtest.SeedSessionWithMessages(t, database, id, "example-api",
			[]db.Message{dbtest.UserMsg(id, 0, content)},
			func(session *db.Session) {
				session.Agent = "codex"
				session.Machine = host
				session.StartedAt = &started
			})
	}
	// Six copies of the same native id from other hosts, every one more recently active than the row
	// the reference names: the first page the inherited lookup answers is entirely theirs.
	for index := range 6 {
		host := fmt.Sprintf("host-z%d", index)
		seed(host+"~codex:"+native, host, fmt.Sprintf("2026-10-%02dT10:00:00Z", index+8),
			"a copy recorded on "+host)
	}
	requestedID := "host-a~codex:" + native
	seed(requestedID, "host-a", "2026-10-01T10:00:00Z", "the session the reference names")
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	ctx := context.Background()
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	qualified := Ref{Provider: "codex", ID: "host-a~" + native}
	status, found, err := archive.Status(ctx, qualified)
	if err != nil {
		t.Fatalf("status of %s: %v", qualified.ID, err)
	}
	if !found {
		t.Fatalf("%s was reported absent although the archive holds %s", qualified.ID, requestedID)
	}
	if status.SessionID != requestedID {
		t.Errorf("%s resolved %q, want %q", qualified.ID, status.SessionID, requestedID)
	}
}

// TestArchiveDisclosesACandidateListItCannotComplete pins the other end of that lookup: when the
// archive holds more rows ending in the requested native id than it reads, neither "not found" nor one
// row is an answer the lookup can support, and it says so instead of asserting one.
func TestArchiveDisclosesACandidateListItCannotComplete(t *testing.T) {
	const native = "b41d9e37-0f5c-4a2b-8d63-1e7c9f0a2b58"
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	for index := range rawSuffixCandidateCap + 1 {
		id := fmt.Sprintf("flood-%d~codex:%s", index, native)
		dbtest.SeedSession(t, database, id, "example-api", func(session *db.Session) {
			session.Agent = "codex"
			session.Machine = fmt.Sprintf("flood-%d", index)
		})
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	ctx := context.Background()
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	_, found, err := archive.Status(ctx, Ref{Provider: "codex", ID: "host-a~" + native})
	if found || err == nil {
		t.Fatalf("a candidate list the lookup cannot complete answered found=%v err=%v", found, err)
	}
	if !errors.Is(err, ErrTruncatedCandidates) {
		t.Errorf("the refusal is %v, want %v", err, ErrTruncatedCandidates)
	}
}

// TestInspectReportsAnArchiveOnlySession pins the rule the plan states for a session that exists only
// in an archive (an inspection of a provider root that no longer holds the source): the archived
// session is reported, and the unreadable source is a warning rather than a silent absence.
func TestInspectReportsAnArchiveOnlySession(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}

	// The provider root this inspection sees is empty: the source is unreadable here.
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {t.TempDir()}}
	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("the inspection read a source session from an empty root: %+v", inspection.Source)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: warnings %v", inspection.Warnings)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "no readable source session for claude:"+claudeFixtureRef.ID) {
		t.Errorf("the inspection does not say the source was unreadable: %v", inspection.Warnings)
	}
}

// TestInspectDoesNotCallAStaleArchiveFolded pins the comparison's folded-row rule: a tool-result-only
// source row is counted as folded only when the archive actually holds that result.
//
// The archive here was written from the same session before its tool result arrived, so it keeps the
// call and no outcome, and every message it holds also appears in the source read. The source's extra
// row is a tool-result-only message -- the shape the comparison counts as folded -- so the row used to
// be counted as stored inside its call: the two reads were reported as agreeing, and the inspection
// said the result "is stored in the archive folded into their tool call" although the archive holds no
// result event at all. The archived pairing decides now: the archived rows are searched for the result
// the source row carries (its tool-use id and content size) before it is counted, and a result the
// archive does not hold is a difference, named.
func TestInspectDoesNotCallAStaleArchiveFolded(t *testing.T) {
	cfgStale, staleFile := claudeTranscript(t, false)
	cfgFull, _ := claudeTranscript(t, true)

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfgStale, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, staleFile); err != nil {
		t.Fatalf("ingesting the session without its result: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Config:      cfgFull,
		Reader:      NewSourceReader(cfgFull),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the grown session against the stale archive: %v", err)
	}
	if inspection.Source == nil {
		t.Fatalf("the grown source was not read: %v", inspection.Warnings)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	archived := inspection.Archive
	if archived.SameMessagesAsSource == nil || *archived.SameMessagesAsSource {
		t.Errorf("a stale archive was reported as holding every source message: same=%v warnings=%v",
			archived.SameMessagesAsSource != nil && *archived.SameMessagesAsSource, archived.Warnings)
	}
	if archived.FoldedToolResults != 0 {
		t.Errorf("the stale archive was credited with %d folded tool result(s)",
			archived.FoldedToolResults)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if strings.Contains(warnings, "folded into their tool call") {
		t.Errorf("the inspection claims a result the archive does not hold is stored in it: %v",
			inspection.Warnings)
	}
	if !strings.Contains(warnings, "the archive does not hold") {
		t.Errorf("the difference the comparison found is not named: %v", inspection.Warnings)
	}
}

// TestCompareMessagesNamesEveryDifference pins the comparison's verdicts directly, on the message
// lists rather than through a parsed session: a row only one read has is a difference, a row both have
// that disagrees about role or content is a difference, and a tool-result-only source row is counted as
// folded only when the archive holds that result somewhere -- as a result row of its own or as the
// event the archive folded onto the call.
//
// The comparison is what the `agreement` line and every warning an inspection prints about an archive
// come from, so each of those answers is pinned here rather than only through a fixture whose shape
// happens to exercise one of them.
func TestCompareMessagesNamesEveryDifference(t *testing.T) {
	const toolUseID = "toolu_01Compared000000000000001"
	base := []Message{
		{Ordinal: 0, Role: "user", Content: "summarize the retry policy"},
		{Ordinal: 1, Role: "assistant", Content: "I will read it first.",
			ToolCalls: []ToolCall{{ToolUseID: toolUseID, Name: "Read"}}},
	}
	resultOnly := Message{Ordinal: 2, Role: "user",
		ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result", ContentSize: 37}}}

	t.Run("a message only the archive has", func(t *testing.T) {
		archived := append(append([]Message{}, base...),
			Message{Ordinal: 2, Role: "assistant", Content: "a row the source lost"})
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want a difference and nothing folded", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "archive message #2 is not in the source read") {
			t.Errorf("warnings = %v, want the archive-only row named", warnings)
		}
	})

	t.Run("a message both reads have but disagree about", func(t *testing.T) {
		archived := append([]Message{}, base...)
		archived[0].Content = "summarize the retry policy, briefly"
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want a difference and nothing folded", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "archive message #0 differs from the source") {
			t.Errorf("warnings = %v, want the disagreeing row named", warnings)
		}
	})

	t.Run("a text message only the source has", func(t *testing.T) {
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "assistant", Content: "later"})
		same, folded, warnings := compareMessages(source, base)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want a difference and nothing folded", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "source message #2 is not in the archive") {
			t.Errorf("warnings = %v, want the source-only row named", warnings)
		}
	})

	t.Run("a result row the archive folded onto its call", func(t *testing.T) {
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentSize: 37}}
		source := append(append([]Message{}, base...), resultOnly)
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 1 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the row counted as folded and no warning",
				same, folded, warnings)
		}
	})

	t.Run("a result row the archive never received", func(t *testing.T) {
		source := append(append([]Message{}, base...), resultOnly)
		same, folded, warnings := compareMessages(source, base)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the missing result to be a difference", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the unheld result named by its tool-use id", warnings)
		}
	})

	t.Run("an attached result the archive never received", func(t *testing.T) {
		// The Codex shape: the outcome is an event on the call's own row, so both reads have the same
		// ordinals, roles and content, and the missing outcome is the only difference there is.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, base)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the missing outcome to be a difference", same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], "archive message #1 does not hold the tool result") ||
			!strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the row's missing outcome named", warnings)
		}
	})

	t.Run("an attached result the archive holds on the same row", func(t *testing.T) {
		// The archive stores the same outcome as a result row of its own on that row: the key -- the
		// call and the content length -- is what has to match, not the label the provider gave it.
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentSize: 3}}
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the held result accepted",
				same, folded, warnings)
		}
	})

	t.Run("an attached result both reads show and disagree about", func(t *testing.T) {
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "31\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the differing body to be a difference", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "different body for tool result") ||
			!strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the differing body named", warnings)
		}
	})

	t.Run("a body only the archive shows is not a difference", func(t *testing.T) {
		// A writer that kept the length and no text (a blocked category, a narrowed archive content
		// policy) leaves the archive's body empty; the key still matches, and a comparison that read
		// the empty body as "differs" would report a difference the archive's own read cannot support.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want no difference", same, folded, warnings)
		}
	})

	t.Run("two archived bodies under one key, one of them the source's", func(t *testing.T) {
		// One call can have more than one archived body under one key (the call's own content and the
		// event it was restored from, or an event delivered twice). A difference is reported only when
		// none of them is the source's body, so a stale duplicate cannot turn agreement into a mismatch.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "30\n", ContentSize: 3},
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "31\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the matching body accepted",
				same, folded, warnings)
		}
	})

	t.Run("an attached result the archive stores in the decoded spelling", func(t *testing.T) {
		// The source's content field is the provider's raw JSON (`"30\n"`, quotes included); the archive
		// stores the text that JSON decodes to. The two are the same body, and a comparison that
		// demanded the raw bytes would report every result as differing.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: `"30\n"`, ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the decoded body accepted",
				same, folded, warnings)
		}
	})

	t.Run("a summarized call body on a row both reads have is not compared", func(t *testing.T) {
		// The archive can store a *summary* on the call -- several agents' outputs merged, or a
		// narrowed policy's text -- that the source row's own body was never equal to. Only the
		// message's own result rows are in scope for a row both reads have; a comparison that read the
		// summary would report a difference the archive's own read cannot support.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "shell",
			ResultContent: "agent-1:\n30\n\nagent-2:\n40\n", ResultContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the call's summary out of scope",
				same, folded, warnings)
		}
	})

	t.Run("a folded result whose body the two reads disagree about", func(t *testing.T) {
		// Claude's shape: the source's result row is folded into the call, so nothing shares its
		// ordinal. Same call, same content length, different outcome -- the key alone cannot tell them
		// apart, and the archive holds the body on the call row.
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result",
				ContentRaw: `"PASS"`, ContentSize: 4}}})
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Bash",
			ResultContent: "FAIL", ResultContentSize: 4}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 1 {
			t.Errorf("same=%v folded=%d, want the row counted as folded and its body compared",
				same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "different body") ||
			!strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the differing body named", warnings)
		}
	})

	t.Run("a folded result whose archive body is an event on the call", func(t *testing.T) {
		// The archive read materializes a call's result events as the message's own result rows, so a
		// folded result's body can be found there too -- the comparison asks for the body wherever the
		// archive keeps it, not only on the call row.
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result",
				ContentRaw: `"PASS"`, ContentSize: 4}}})
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "FAIL", ContentSize: 4}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 1 || len(warnings) != 1 {
			t.Errorf("same=%v folded=%d warnings=%v, want the event's body compared",
				same, folded, warnings)
		}
	})

	t.Run("a folded result whose body the archive narrowed away", func(t *testing.T) {
		// The blocked-category shape: the key is held and the body is not, so there is nothing to
		// compare and the row stays agreement -- an inspection must not report a difference its own
		// archive read cannot support.
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result",
				ContentRaw: `"PASS"`, ContentSize: 4}}})
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read",
			ResultContentSize: 4}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 1 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the narrowed body accepted",
				same, folded, warnings)
		}
	})
}

// claudeBashTranscript writes one small Claude session -- a user turn and a `Bash` call whose result the
// transcript records verbatim -- under a throw-away provider root, and returns that root's configuration
// with the file's path. Bash is not one of the inherited default configuration's blocked result
// categories, so an archive of the file keeps the body: two same-length outputs are what a case needs to
// hold an archive's recorded outcome against a later transcript of the same session.
func claudeBashTranscript(t *testing.T, output string) (config.Config, string) {
	t.Helper()
	const toolUseID = "toolu_01Folded0000000000000001"
	root := t.TempDir()
	relative := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	path := filepath.Join(root, "claude", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	lines := []string{
		`{"cwd":"/home/user/code/example-api","message":{"content":"Check the retry policy."},` +
			`"sessionId":"` + claudeFixtureRef.ID + `","timestamp":"2026-05-04T09:15:00.000Z",` +
			`"type":"user","version":"1.0.30"}`,
		`{"message":{"content":[{"id":"` + toolUseID + `","input":{"command":"make check"},` +
			`"name":"Bash","type":"tool_use"}],"model":"claude-sonnet-4-5"},` +
			`"timestamp":"2026-05-04T09:15:04.000Z","type":"assistant","version":"1.0.30"}`,
		`{"message":{"content":[{"content":` + jsonString(t, output) + `,"tool_use_id":"` +
			toolUseID + `","type":"tool_result"}]},"timestamp":"2026-05-04T09:15:06.000Z","type":"user"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	return cfg, path
}

// TestInspectReportsADifferentFoldedResultBody pins the comparison end to end for a folded tool result:
// the source row that carried the outcome has no ordinal in the archive, because the writer stored the
// result on the call, and the key it is held under -- the call and the content length -- cannot tell two
// same-length outcomes apart. An archive recording PASS for a session whose transcript now says FAIL was
// reported as holding every source message. The comparison reads the body wherever the archive keeps it
// and decodes the source's raw content field the way the writer did.
//
// The second half is the control: the same archive read against a transcript that recorded the same
// output must still agree, so the new answer cannot pass by reporting every folded result as differing.
func TestInspectReportsADifferentFoldedResultBody(t *testing.T) {
	ctx := context.Background()
	archiveCfg, archiveFile := claudeBashTranscript(t, "PASS\n")
	changedCfg, changedFile := claudeBashTranscript(t, "FAIL\n")
	if archiveFile == changedFile {
		t.Fatal("the two transcripts are the same file")
	}
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(archiveCfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, archiveFile); err != nil {
		t.Fatalf("ingesting the transcript: %v", err)
	}

	for name, testCase := range map[string]struct {
		cfg  config.Config
		same bool
	}{
		"unchanged transcript": {cfg: archiveCfg, same: true},
		"changed outcome":      {cfg: changedCfg, same: false},
	} {
		t.Run(name, func(t *testing.T) {
			inspection, err := Inspect(ctx, InspectOptions{
				Ref:         claudeFixtureRef,
				Config:      testCase.cfg,
				Reader:      NewSourceReader(testCase.cfg),
				ArchivePath: archivePath,
			})
			if err != nil {
				t.Fatalf("inspecting: %v", err)
			}
			if inspection.Source == nil {
				t.Fatalf("the source was not read: %v", inspection.Warnings)
			}
			if inspection.Archive == nil {
				t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
			}
			archived := inspection.Archive
			if archived.SameMessagesAsSource == nil {
				t.Fatal("the comparison did not run")
			}
			if *archived.SameMessagesAsSource != testCase.same {
				t.Errorf("same_messages_as_source = %v, want %v (warnings: %v)",
					*archived.SameMessagesAsSource, testCase.same, archived.Warnings)
			}
			if archived.FoldedToolResults != 1 {
				t.Errorf("folded tool results = %d, want 1", archived.FoldedToolResults)
			}
			warnings := strings.Join(inspection.Warnings, "\n")
			if testCase.same {
				if strings.Contains(warnings, "different body") {
					t.Errorf("the unchanged outcome was reported as a different body: %v",
						inspection.Warnings)
				}
				return
			}
			if !strings.Contains(warnings, "different body") ||
				!strings.Contains(warnings, "toolu_01Folded") {
				t.Errorf("the differing outcome is not named: %v", inspection.Warnings)
			}
		})
	}
}

// TestInspectExposesAProviderFailureAndFallsBackToTheArchive pins the inspection's end of the reader's
// classification: when the selected session's provider fails operationally -- the inherited Cursor
// shape, an outcome carrying a SourceError and no result -- the inspection reports the provider's own
// diagnostics *and* still reports the archived session, instead of reducing the failure to "no readable
// source session", which says nothing an operator can act on and hides a retryable failure.
func TestInspectExposesAProviderFailureAndFallsBackToTheArchive(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}

	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
	provider.source = parser.SourceRef{
		Provider:       parser.AgentClaude,
		DisplayPath:    claudeFile,
		FingerprintKey: claudeFile,
	}
	provider.outcome = parser.ParseOutcome{
		ResultSetComplete: true,
		SourceErrors: []parser.SourceError{{
			SourceKey:   "cursor-state-store",
			DisplayPath: "/home/user/.cursor/state.db",
			Err:         errors.New("the store is locked by another reader"),
			Retryable:   true,
		}},
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref: claudeFixtureRef, Config: cfg, Reader: reader, ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("the failed source read was reported as a source: %+v", inspection.Source)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "the store is locked by another reader") {
		t.Errorf("the provider's diagnostics are not exposed: %v", inspection.Warnings)
	}
	if strings.Contains(warnings, "no readable source session") {
		t.Errorf("a provider failure is reported as absence: %v", inspection.Warnings)
	}
}

// TestInspectDoesNotCallAStaleArchiveCompleteAfterACallOutcomeArrived pins the comparison's other
// shape: Codex records a call's output as an event on the call's own row, so an archive written before
// the output arrived has the same ordinals, roles and content as the later source read -- the *only*
// difference is an outcome attached to a row both reads have. The comparison used to look at role and
// content alone, so it reported that archive as holding the whole conversation and said nothing about
// the missing outcome. Now the results a source row records are asked of the archived row too.
//
// The second half is the control: an archive of the same session *with* the output must still be
// reported as agreeing, so the new answer cannot pass by reporting every attached result as missing.
func TestInspectDoesNotCallAStaleArchiveCompleteAfterACallOutcomeArrived(t *testing.T) {
	ctx := context.Background()
	cfgStale, staleFile := codexTranscript(t, false)
	cfgFull, fullFile := codexTranscript(t, true)

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfgStale, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, staleFile); err != nil {
		t.Fatalf("ingesting the rollout without its call output: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         codexFixtureRef,
		Config:      cfgFull,
		Reader:      NewSourceReader(cfgFull),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the grown session against the stale archive: %v", err)
	}
	if inspection.Source == nil {
		t.Fatalf("the grown source was not read: %v", inspection.Warnings)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	archived := inspection.Archive
	if archived.SameMessagesAsSource == nil || *archived.SameMessagesAsSource {
		t.Errorf("a stale archive was reported as holding every source message: same=%v warnings=%v",
			archived.SameMessagesAsSource != nil && *archived.SameMessagesAsSource, archived.Warnings)
	}
	if archived.FoldedToolResults != 0 {
		t.Errorf("the stale archive was credited with %d folded tool result(s)",
			archived.FoldedToolResults)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "does not hold the tool result") {
		t.Errorf("the missing call output is not named: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, codexStaleCallID) {
		t.Errorf("the missing outcome is not named by its tool-use id: %v", inspection.Warnings)
	}

	// The control: the same session ingested *with* its output reads as agreement, so the rule above
	// cannot be satisfied by calling every attached result missing.
	fullArchivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	fullIngestor, err := NewIngestor(cfgFull, fullArchivePath)
	if err != nil {
		t.Fatalf("building the ingestor for the full session: %v", err)
	}
	if err := fullIngestor.IngestPaths(ctx, fullFile); err != nil {
		t.Fatalf("ingesting the rollout with its call output: %v", err)
	}
	fullInspection, err := Inspect(ctx, InspectOptions{
		Ref:         codexFixtureRef,
		Config:      cfgFull,
		Reader:      NewSourceReader(cfgFull),
		ArchivePath: fullArchivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the session against its own archive: %v", err)
	}
	if fullInspection.Archive == nil {
		t.Fatalf("the current archive was not reported: %v", fullInspection.Warnings)
	}
	current := fullInspection.Archive
	if current.SameMessagesAsSource == nil || !*current.SameMessagesAsSource {
		t.Errorf("an archive holding the call output was reported as disagreeing: warnings=%v",
			current.Warnings)
	}
	if current.FoldedToolResults != 0 {
		t.Errorf("the current archive was credited with %d folded tool result(s)",
			current.FoldedToolResults)
	}
}

// codexTranscript writes one small Codex rollout -- a user turn, a shell call, and with includeOutput
// the output event the call produced -- under a throw-away provider root, and returns that root's
// configuration with the file's path. The two shapes are the same session at two moments, which is what
// a test needs to hold a source read against an archive written before the call's outcome arrived.
func codexTranscript(t *testing.T, includeOutput bool) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	relative := filepath.Join("2026", "05", "04",
		"rollout-2026-05-04T09-15-00-"+codexFixtureRef.ID+".jsonl")
	path := filepath.Join(root, "codex", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	lines := []string{
		`{"payload":{"cwd":"/home/user/code/example-api","id":"` + codexFixtureRef.ID +
			`","originator":"codex_cli_rs"},"timestamp":"2026-05-04T09:15:00.000Z",` +
			`"type":"session_meta"}`,
		`{"payload":{"content":[{"text":"Where is the retry cap configured?","type":"input_text"}],` +
			`"role":"user"},"timestamp":"2026-05-04T09:15:02.000Z","type":"response_item"}`,
		`{"payload":{"arguments":{"command":"grep -rn maxBackoffSeconds internal/retry"},` +
			`"call_id":"` + codexStaleCallID + `","name":"shell","type":"function_call"},` +
			`"timestamp":"2026-05-04T09:15:05.000Z","type":"response_item"}`,
	}
	if includeOutput {
		lines = append(lines, `{"payload":{"call_id":"`+codexStaleCallID+
			`","output":"internal/retry/config.go:41: maxBackoffSeconds = 30",`+
			`"type":"function_call_output"},"timestamp":"2026-05-04T09:15:06.000Z",`+
			`"type":"response_item"}`)
	}
	lines = append(lines, `{"payload":{"content":[{"text":"It is set in internal/retry/config.go.",`+
		`"type":"output_text"}],"role":"assistant"},"timestamp":"2026-05-04T09:15:07.000Z",`+
		`"type":"response_item"}`)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the rollout: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentCodex: {filepath.Join(root, "codex")}}
	return cfg, path
}

// claudeTranscript writes one small Claude session -- a user turn and a `Read` tool call, and with
// includeResult the result the call produced -- under a throw-away provider root, and returns that
// root's configuration with the file's path. The two shapes are the same session at two moments, which
// is what a test needs to hold a source read against an archive written before its latest row arrived.
func claudeTranscript(t *testing.T, includeResult bool) (config.Config, string) {
	t.Helper()
	const toolUseID = "toolu_01Stale0000000000000001"
	root := t.TempDir()
	relative := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	path := filepath.Join(root, "claude", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	lines := []string{
		`{"cwd":"/home/user/code/example-api","message":{"content":"Summarize the retry policy."},` +
			`"sessionId":"` + claudeFixtureRef.ID + `","timestamp":"2026-05-04T09:15:00.000Z",` +
			`"type":"user","version":"1.0.30"}`,
		`{"message":{"content":[{"id":"` + toolUseID + `","input":{"file_path":` +
			`"/home/user/code/example-api/internal/retry/policy.go"},"name":"Read","type":"tool_use"}],` +
			`"model":"claude-sonnet-4-5"},"timestamp":"2026-05-04T09:15:04.000Z",` +
			`"type":"assistant","version":"1.0.30"}`,
	}
	if includeResult {
		lines = append(lines, `{"message":{"content":[{"content":"package retry\n","tool_use_id":"`+
			toolUseID+`","type":"tool_result"}]},"timestamp":"2026-05-04T09:15:06.000Z","type":"user"}`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	return cfg, path
}

// TestArchiveReadNamesWhatItShows pins that an archive read reports what its rows show. The storage
// policy belongs to the writer's handle and a read-only handle cannot see it, so the read names the
// payloads and text it got back; a read that named a policy would report omissions that never
// happened (a session that recorded no tool payload is not a narrowed archive). The Claude fixture
// carries a tool call, so the three policies leave the three distinguishable shapes.
func TestArchiveReadNamesWhatItShows(t *testing.T) {
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	cases := []struct {
		policy   config.ArchiveContent
		shown    string
		warning  string
		warnsNil bool
	}{
		{policy: config.ArchiveContentFull, shown: "tool-payloads", warnsNil: true},
		{policy: config.ArchiveContentTranscripts, shown: "message-text",
			warning: "shows no tool inputs or results"},
		{policy: config.ArchiveContentUsage, shown: "none",
			warning: "shows no message text and no tool inputs or results"},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.policy), func(t *testing.T) {
			cfg := fixtureConfig(t)
			cfg.ArchiveContent = testCase.policy
			archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
			ingestor, err := NewIngestor(cfg, archivePath)
			if err != nil {
				t.Fatalf("building the ingestor: %v", err)
			}
			ctx := context.Background()
			if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
				t.Fatalf("ingesting the fixture under %s: %v", testCase.policy, err)
			}
			archive, err := OpenArchive(ctx, archivePath)
			if err != nil {
				t.Fatalf("opening the archive: %v", err)
			}
			defer archive.Close()
			fromArchive, err := archive.Read(ctx, claudeFixtureRef)
			if err != nil {
				t.Fatalf("reading the archive: %v", err)
			}
			if fromArchive.Fidelity.ContentShown != testCase.shown {
				t.Errorf("the archive written under %s reads back as content shown %q, want %q",
					testCase.policy, fromArchive.Fidelity.ContentShown, testCase.shown)
			}

			inspection, err := Inspect(ctx, InspectOptions{
				Ref:         claudeFixtureRef,
				Reader:      NewSourceReader(cfg),
				ArchivePath: archivePath,
			})
			if err != nil {
				t.Fatalf("inspecting: %v", err)
			}
			if inspection.Archive == nil {
				t.Fatalf("the archived session was not reported: warnings %v", inspection.Warnings)
			}
			if inspection.Archive.ContentShown != testCase.shown {
				t.Errorf("the inspection reports content shown %q, want %q",
					inspection.Archive.ContentShown, testCase.shown)
			}
			warnings := strings.Join(inspection.Warnings, "\n")
			if testCase.warnsNil {
				if strings.Contains(warnings, "shows no message text") ||
					strings.Contains(warnings, "shows no tool inputs") {
					t.Errorf("a read carrying tool payloads was reported as narrowed: %v", inspection.Warnings)
				}
				return
			}
			if !strings.Contains(warnings, testCase.warning) {
				t.Errorf("the %s read was not reported: %v", testCase.policy, inspection.Warnings)
			}
			if strings.Contains(warnings, "projection") {
				t.Errorf("the warning names a storage policy the read cannot see: %v", inspection.Warnings)
			}
		})
	}
}

// TestArchiveReadDoesNotCallAPayloadFreeReadNarrowed pins the other half: a session that recorded no
// tool payload at all, ingested under the full policy, reads back with no tool payloads -- and that is
// a fact about the rows, not evidence that the writer dropped anything. The read must say so; the
// label this replaced ("transcripts") claimed the archive stored a projection that never narrowed
// anything, and a warning built on it reported an omission that never happened.
func TestArchiveReadDoesNotCallAPayloadFreeReadNarrowed(t *testing.T) {
	source, cfg := toolFreeFixture(t)
	cfg.ArchiveContent = config.ArchiveContentFull
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, source); err != nil {
		t.Fatalf("ingesting the tool-free session: %v", err)
	}
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	fromArchive, err := archive.Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	if fromArchive.Fidelity.ContentShown != "message-text" {
		t.Errorf("a tool-free session under the full policy reads as %q, want %q",
			fromArchive.Fidelity.ContentShown, "message-text")
	}
	if status, found, err := archive.Status(ctx, claudeFixtureRef); err != nil || !found {
		t.Fatalf("the tool-free session is not in the archive: found=%v err=%v", found, err)
	} else if status.MessageCount != len(fromArchive.Messages) {
		t.Errorf("the session row records %d message(s), the read returned %d",
			status.MessageCount, len(fromArchive.Messages))
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: warnings %v", inspection.Warnings)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "the archive read shows no tool inputs or results") {
		t.Errorf("the read does not say what it showed: %v", inspection.Warnings)
	}
	if strings.Contains(warnings, "projection") {
		t.Errorf("the warning claims a storage policy the read cannot see: %v", inspection.Warnings)
	}
}

// toolFreeFixture writes the Claude fixture without its tool records under a throw-away provider root
// and returns that root's configuration: a session that recorded two text turns and one compaction
// boundary, and no tool call at all. It is derived from the committed fixture bytes rather than
// committed itself, so the two shapes cannot drift apart, and every path it writes is temporary.
func toolFreeFixture(t *testing.T) (string, config.Config) {
	t.Helper()
	fixture := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	var kept []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, `"tool_use"`) || strings.Contains(line, `"tool_result"`) {
			continue
		}
		kept = append(kept, line)
	}
	root := t.TempDir()
	source := filepath.Join(root, "claude", "-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("preparing the tool-free provider root: %v", err)
	}
	if err := os.WriteFile(source, []byte(strings.Join(kept, "\n")), 0o600); err != nil {
		t.Fatalf("writing the tool-free session: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	return source, cfg
}
