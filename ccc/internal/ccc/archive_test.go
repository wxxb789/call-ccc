package ccc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/testjsonl"
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

// TestPathRecordIDsSelectsAndOrdersTheRowsOfAPath pins the selection and the order a filesystem
// reference resolves through, on the rows the archive answers with rather than on a query plan.
//
// The four rules are checked in one place because they compose the answer: a row of another path is not
// a candidate, a user-trashed row is not an answer while a source-missing (reconciled) row is, and the
// ids come back sorted although the records do not -- the inherited record query orders nothing, and a
// candidate list that moved with the plan would change what an ambiguous reference reports.
func TestPathRecordIDsSelectsAndOrdersTheRowsOfAPath(t *testing.T) {
	records := []db.SessionPathRecord{
		{ID: "second-conversation", FilePath: "/sessions/a.jsonl", MessageCount: 3},
		{ID: "trashed-conversation", FilePath: "/sessions/a.jsonl", Trashed: true},
		{ID: "another-path", FilePath: "/sessions/b.jsonl"},
		{ID: "first-conversation", FilePath: "/sessions/a.jsonl"},
		{ID: "reconciled-conversation", FilePath: "/sessions/a.jsonl", SourceMissing: true},
	}
	want := []string{"first-conversation", "reconciled-conversation", "second-conversation"}
	if got := pathRecordIDs(records, []string{"/sessions/a.jsonl"}); !slices.Equal(got, want) {
		t.Errorf("pathRecordIDs = %v, want %v", got, want)
	}
	// Every spelling answers, and the spellings of one reference are one candidate list, not one per
	// spelling: the answer stays sorted across the two paths.
	want = []string{"another-path", "first-conversation", "reconciled-conversation", "second-conversation"}
	if got := pathRecordIDs(records, []string{"/sessions/a.jsonl", "/sessions/b.jsonl"}); !slices.Equal(got, want) {
		t.Errorf("pathRecordIDs = %v, want %v", got, want)
	}
	// A spelling no row records answers nothing rather than the rows of another path.
	if got := pathRecordIDs(records, []string{"/sessions/c.jsonl"}); len(got) != 0 {
		t.Errorf("pathRecordIDs = %v, want no candidate", got)
	}
}

// TestDeletedSourcePathFallsBackToTheArchive pins what an inspection does when the source file an
// archive was written from is gone: the archived session is reported, and the unreadable source is a
// warning rather than the loss of the whole inspection -- before and after AgentsView reconciles the
// disappearance.
//
// The provider's changed-path classifier answers for an absent path -- that is what keeps a watcher
// from dropping a deletion -- so a deleted source used to be reported as a *source* the reference
// names, and the read then failed while fingerprinting. The archive, which holds the session, was
// never consulted: the error escaped `Inspect` before the archive was opened. A classification with no
// file behind it is not a local source, so the reference resolves to nothing readable and the archive
// answers.
//
// The second half is the state a watcher pass leaves behind: the next changed-path pass over the
// vanished path marks the stored row source-missing (`db.MarkSessionSourceMissing`) and keeps the
// conversation. The path-keyed lookup the archive used answered only a source's *current* members --
// it requires `source_missing_at IS NULL` -- so the reconciled row became invisible to the reference
// that names its path while the same row still answered a reference that names its id: an inspection
// reported "no session found" for a conversation the archive holds. The row's own path is what makes it
// this reference's session, whether or not the file it was read from is still there.
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

	byPath := Ref{Provider: "claude", ID: source}
	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         byPath,
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

	// Now the disappearance is reconciled, which is what a watcher pass does with the vanished path.
	reconcileVanishedSource(t, ingestor, source)
	requireSourceMissing(t, archivePath, claudeFixtureRef.ID)

	inspection, err = Inspect(ctx, InspectOptions{
		Ref:         byPath,
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting a reconciled deleted source path: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("the reconciled row was not reported: %v", inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != claudeFixtureRef.ID {
		t.Errorf("the reconciled path resolved %q, want %q",
			inspection.Archive.Status.SessionID, claudeFixtureRef.ID)
	}
}

// TestUnreadableSourceFallsBackToTheArchive pins the sibling of the deletion rule above: a source the
// reader *resolved* and then could not read -- a fingerprint that failed at the I/O level -- is a
// warning on the way to the archive, not the end of the inspection. The reader answers such a failure as
// a provider that could not read the source (ErrProviderFailed), the classification this inspection
// already reports as a warning; before that it escaped as a bare error and the archived session the
// reference names was never reported at all.
func TestUnreadableSourceFallsBackToTheArchive(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	sourcePath := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	cfg = archiveFixtureCopy(t, cfg, sourcePath,
		filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl"))

	reader, provider := readerOverFixedProvider(t, cfg, parser.AgentClaude)
	provider.source = parser.SourceRef{
		Provider:       parser.AgentClaude,
		Key:            sourcePath,
		DisplayPath:    sourcePath,
		FingerprintKey: sourcePath,
	}
	provider.fingerprintErr = errors.New("input/output error")

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Config:      cfg,
		Reader:      reader,
		ArchivePath: cfg.DBPath,
	})
	if err != nil {
		t.Fatalf("inspecting a source that could not be read: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("the inspection reported a source it could not read: %+v", inspection.Source.Identity)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != claudeFixtureRef.ID {
		t.Errorf("the inspection reports %q, want %q",
			inspection.Archive.Status.SessionID, claudeFixtureRef.ID)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "input/output error") {
		t.Errorf("the inspection does not carry the source's own failure: %v", inspection.Warnings)
	}
}

// reconcileVanishedSource asks the inherited sync engine to process a source path that no longer
// exists, the way a watcher's changed-path pass does. AgentsView's answer is the archive state this
// package reads: the stored row is marked source-missing and its conversation is kept.
func reconcileVanishedSource(t *testing.T, ingestor *Ingestor, path string) {
	t.Helper()
	if err := ingestor.IngestPaths(t.Context(), path); err != nil {
		t.Fatalf("reconciling the vanished source %s: %v", path, err)
	}
}

// requireSourceMissing fails the test unless the archive's row for an id carries the reconciliation's
// source-missing mark and still records the path it was read from: the rule above is only meaningful
// in that state, and the recorded path is what a path reference resolves through.
func requireSourceMissing(t *testing.T, archivePath, id string) {
	t.Helper()
	database, err := db.Open(t.Context(), archivePath)
	if err != nil {
		t.Fatalf("opening the archive to read the row: %v", err)
	}
	defer database.Close()
	var missing, filePath string
	if err := database.Reader().QueryRowContext(t.Context(),
		"SELECT COALESCE(source_missing_at, ''), COALESCE(file_path, '') FROM sessions WHERE id = ?", id).
		Scan(&missing, &filePath); err != nil {
		t.Fatalf("reading the row's source-missing mark: %v", err)
	}
	if missing == "" {
		t.Fatalf("the reconciliation did not mark %s source-missing", id)
	}
	if filePath == "" {
		t.Fatalf("the reconciled row for %s no longer records a source path", id)
	}
}

// twoRootClaudeFixture writes the committed Claude fixture under the *second* of two configured Claude
// roots and answers the configuration, both roots, the reference's relative spelling and the file's
// path. Two roots make a relative reference resolve to more than one spelling (referencePaths), which
// is the state the cases below ask the archive about.
func twoRootClaudeFixture(t *testing.T) (cfg config.Config, rootA, rootB, relative, source string) {
	t.Helper()
	cfg = fixtureConfig(t)
	rootA = filepath.Join(t.TempDir(), "claude")
	rootB = filepath.Join(t.TempDir(), "claude")
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {rootA, rootB}}
	relative = filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	source = filepath.Join(rootB, relative)
	data, err := os.ReadFile(fixturePath(t, filepath.Join("claude", relative)))
	if err != nil {
		t.Fatalf("reading the committed fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("preparing the second provider root: %v", err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatalf("writing the transcript copy: %v", err)
	}
	return cfg, rootA, rootB, relative, source
}

// ingestFile writes one source file into a fresh archive and answers the ingestor that did it.
func ingestFile(t *testing.T, cfg config.Config, source string) (*Ingestor, string) {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(t.Context(), source); err != nil {
		t.Fatalf("ingesting the transcript: %v", err)
	}
	return ingestor, archivePath
}

// TestRelativePathReferenceAsksEveryConfiguredRoot pins that a relative filesystem reference is looked
// up under *every* root the provider is configured with, not only the first.
//
// A relative reference is a candidate path under each configured root (referencePaths), and the
// archive stores the absolute path its row was written with: a lookup that asked about one spelling
// would report a session absent merely because the file was read from another configured root.
func TestRelativePathReferenceAsksEveryConfiguredRoot(t *testing.T) {
	cfg, _, _, relative, source := twoRootClaudeFixture(t)
	_, archivePath := ingestFile(t, cfg, source)
	ctx := context.Background()
	archive, err := openArchive(ctx, archivePath, cfg.AgentDirs)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	status, found, err := archive.Status(ctx, Ref{Provider: "claude", ID: relative})
	if err != nil || !found {
		t.Fatalf("the relative reference answered found=%v err=%v, want the archived row", found, err)
	}
	if status.SessionID != claudeFixtureRef.ID {
		t.Errorf("the relative reference resolved %q, want %q", status.SessionID, claudeFixtureRef.ID)
	}
}

// TestRelativePathReferenceAsksEveryRootBeforeFallingBack pins the two tiers against a reference that
// has more than one spelling: the source's current members are looked for under *every* configured
// root, and a retained row under one of them does not turn the reference into an ambiguous one.
//
// Root A holds a retained row for the reference's relative path (its file is gone), root B holds the
// file itself. Asking only the first spelling would find no current member and fall back to the rows
// the archive still holds, reporting both conversations as candidates -- an ambiguity the source side
// does not have, because it reads only the file that is there. The retained row is not lost: it stays
// the archive's answer to its own id.
func TestRelativePathReferenceAsksEveryRootBeforeFallingBack(t *testing.T) {
	cfg, rootA, _, relative, source := twoRootClaudeFixture(t)
	_, archivePath := ingestFile(t, cfg, source)
	ctx := context.Background()

	// The first root's conversation, whose source is gone: the row records the path it was read from,
	// and the inherited reconciliation's own mark (baseline, then mark) makes it source-missing.
	stored := filepath.Join(rootA, relative)
	database, err := db.Open(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive to seed the retained row: %v", err)
	}
	path := stored
	dbtest.SeedSessionWithMessages(t, database, "conversation-of-root-a", "example-api",
		[]db.Message{dbtest.UserMsg("conversation-of-root-a", 0, "the conversation root A lost")},
		func(session *db.Session) {
			session.Agent = "claude"
			session.Machine = "local"
			session.FilePath = &path
		})
	if err := database.BaselineActiveSessionSourcePaths(ctx, "local",
		[]db.SessionSourcePath{{Agent: "claude", FilePath: stored}}); err != nil {
		t.Fatalf("baselining the retained row's source: %v", err)
	}
	changed, err := database.MarkSessionSourceMissing(ctx, "local", "claude", "conversation-of-root-a", stored)
	if err != nil || !changed {
		t.Fatalf("marking the retained row source-missing: changed=%v err=%v", changed, err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	archive, err := openArchive(ctx, archivePath, cfg.AgentDirs)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	status, found, err := archive.Status(ctx, Ref{Provider: "claude", ID: relative})
	if err != nil {
		t.Fatalf("the relative reference failed: %v", err)
	}
	if !found {
		t.Fatalf("the relative reference resolved nothing, want root B's session")
	}
	if status.SessionID != claudeFixtureRef.ID || status.Agent != "claude" {
		t.Errorf("the relative reference resolved %+v, want the file that is there", status)
	}
	if _, err := archive.Read(ctx, Ref{Provider: "claude", ID: "conversation-of-root-a"}); err != nil {
		t.Errorf("the retained row is not readable by id: %v", err)
	}
}

// TestPathReferenceDoesNotCrossProviders pins that a filesystem reference is answered only by a row the
// provider the reference names recorded.
//
// The lookup asks the archive's per-provider row listing, whose agent predicate is the whole scope: a
// path is a source identity only together with the agent that owns it (the same pair the inherited
// path-keyed lookup took), so a Claude transcript asked for as a Codex session is not this archive's
// Codex session -- the inspection would report another agent's conversation under the requested label
// and exit successfully although no Codex session exists there.
func TestPathReferenceDoesNotCrossProviders(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
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

	// The archive holds the Claude session and nothing else: the same path asked for as a Codex session
	// is not found, and the provider that recorded it still answers.
	crossed := Ref{Provider: "codex", ID: claudeFile}
	if status, found, err := archive.Status(ctx, crossed); err != nil || found {
		t.Errorf("a codex reference resolved the claude row by path: found=%v err=%v status=%+v",
			found, err, status)
	}
	if _, err := archive.Read(ctx, crossed); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a codex reference by path answered %v, want a not-found answer", err)
	}
	status, found, err := archive.Status(ctx, Ref{Provider: "claude", ID: claudeFile})
	if err != nil || !found {
		t.Fatalf("claude:%s answered found=%v err=%v, want the stored row", claudeFile, found, err)
	}
	if status.SessionID != claudeFixtureRef.ID || status.Agent != "claude" {
		t.Errorf("claude:%s resolved %+v, want the claude row it names", claudeFile, status)
	}
}

// TestPathReferenceIgnoresATrashedSession pins the deletion half of the path rule: a session the user
// moved to the trash answers no path reference.
//
// The row keeps its file path when it is trashed, so a lookup that only matched the path would report a
// conversation the user deleted as the session a reference names. A reference that names the stored id
// answers nothing either (the inherited `GetSession` requires `deleted_at IS NULL`), and the two must
// not disagree about the same session.
func TestPathReferenceIgnoresATrashedSession(t *testing.T) {
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	ctx := context.Background()
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}
	database, err := db.Open(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive to trash the row: %v", err)
	}
	if err := database.SoftDeleteSession(ctx, claudeFixtureRef.ID); err != nil {
		t.Fatalf("trashing the stored row: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the archive: %v", err)
	}

	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	byPath := Ref{Provider: "claude", ID: claudeFile}
	if status, found, err := archive.Status(ctx, byPath); err != nil || found {
		t.Errorf("a trashed row answered a path reference: found=%v err=%v status=%+v", found, err, status)
	}
	if _, err := archive.Read(ctx, byPath); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a trashed row by path answered %v, want a not-found answer", err)
	}
}

// TestPathReferencePrefersTheSourcesCurrentMembers pins the two tiers of the path rule: while the
// archive records current members for a path, those answer -- a retained (source-missing) row of the
// same path is not one of them -- and a reference that names the retained row's id still reads it.
//
// The tiers mirror the two sides of an inspection. A source that is there is read by its own current
// members, the rows a parse of that file emits, and a source that is gone has no members at all, so
// the rows the archive still holds under its path are what answers
// (`TestDeletedSourcePathFallsBackToTheArchive`). A union of the two would report conversations the
// source side does not for a file that is present -- the archive and the source would answer one
// reference with different candidate lists -- which is why the retained rows are the *fallback*, not a
// second candidate source.
func TestPathReferencePrefersTheSourcesCurrentMembers(t *testing.T) {
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	shared := filepath.Join(dir, "claude", "shared.jsonl")
	seed := func(id, content string) {
		t.Helper()
		path := shared
		dbtest.SeedSessionWithMessages(t, database, id, "example-api",
			[]db.Message{dbtest.UserMsg(id, 0, content)},
			func(session *db.Session) {
				session.Agent = "claude"
				session.FilePath = &path
			})
	}
	seed("live-conversation", "the conversation the source still holds")
	seed("reconciled-conversation", "the conversation whose source went away")

	// The row that lost its source is marked the way the inherited reconciliation marks one: the exact
	// ownership is baselined first, then the mark is applied to that machine, agent, id and path.
	ctx := context.Background()
	if err := database.BaselineActiveSessionSourcePaths(ctx, "local",
		[]db.SessionSourcePath{{Agent: "claude", FilePath: shared}}); err != nil {
		t.Fatalf("baselining the source ownership: %v", err)
	}
	changed, err := database.MarkSessionSourceMissing(ctx, "local", "claude", "reconciled-conversation", shared)
	if err != nil {
		t.Fatalf("marking the source missing: %v", err)
	}
	if !changed {
		t.Fatalf("the reconciliation mark did not apply to the seeded row")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()
	status, found, err := archive.Status(ctx, Ref{Provider: "claude", ID: shared})
	if err != nil || !found {
		t.Fatalf("the path answered found=%v err=%v, want the source's current member", found, err)
	}
	if status.SessionID != "live-conversation" {
		t.Errorf("the path resolved %q, want the current member of the source", status.SessionID)
	}

	// The retained conversation is not lost: it is still the archive's answer to its own id.
	conversation, err := archive.Read(ctx, Ref{Provider: "claude", ID: "reconciled-conversation"})
	if err != nil {
		t.Fatalf("reading the retained row by id: %v", err)
	}
	if len(conversation.Messages) != 1 ||
		!strings.Contains(conversation.Messages[0].Content, "went away") {
		t.Errorf("the retained row read the wrong conversation: %+v", conversation.Messages)
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
	const otherToolUseID = "toolu_01Compared000000000000002"
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
		// summary would report a difference the archive's own read cannot support. The call's *name* is
		// the source's own here: an archive that recorded a different call would be a different message,
		// which the call comparison reports, so this case is about the body alone.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read",
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
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read",
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

	t.Run("an attached result whose outcome only the archive records", func(t *testing.T) {
		// Claude's shape: the provider's own is_error marker becomes the result's status ("errored"),
		// the writer keeps it, and the source read of the same body records no error. Both reads record
		// a status for the row -- "" is what a read records for a result its provider did not mark as an
		// error -- so the pair is not agreement, whichever of the two recorded the failure.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the archived error to be a difference", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "records a different outcome") ||
			!strings.Contains(warnings[0], toolUseID) || !strings.Contains(warnings[0], "errored") ||
			!strings.Contains(warnings[0], "no status") {
			t.Errorf("warnings = %v, want the differing outcome named with both reads' statuses",
				warnings)
		}
	})

	t.Run("an attached result whose outcome only the source records", func(t *testing.T) {
		// The same difference the other way round: the archive was written from a transcript of this
		// session that recorded an error the transcript read now does not. A comparison that only asked
		// whether some archived status matched would agree here.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the unrecorded error to be a difference", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "records a different outcome") ||
			!strings.Contains(warnings[0], "errored") || !strings.Contains(warnings[0], "no status") {
			t.Errorf("warnings = %v, want the differing outcome named with both reads' statuses",
				warnings)
		}
	})

	t.Run("an attached result both reads record as errored", func(t *testing.T) {
		// The control: a status the two reads agree about is agreement, so the rule above cannot pass by
		// reporting every result a provider marked as failed.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the matching outcome accepted",
				same, folded, warnings)
		}
	})

	t.Run("an extra archived outcome under one key", func(t *testing.T) {
		// One key can be held by more than one archived row (an event delivered twice, a result row
		// beside the event it was restored from). Agreement is about the *set* of outcomes the two reads
		// record, so an archived row recording a failure the source does not is a difference even though
		// the source's own status appears among the archive's rows.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3},
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "31\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the extra archived outcome to be a difference",
				same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "records a different outcome") {
			t.Errorf("warnings = %v, want the extra outcome named", warnings)
		}
	})

	t.Run("a result the archive holds only on its call row has no outcome to compare", func(t *testing.T) {
		// The archive stores a result on the call and nowhere else: the call carries no status (the
		// column does not exist), so the archive's read shows no outcome for the key and the comparison
		// reports nothing -- the same allowance the body comparison makes, and what keeps a provider
		// whose results the archive kept on the call from being reported as differing everywhere.
		source := append([]Message{}, base...)
		source[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", Status: "errored",
				ContentRaw: "30\n", ContentSize: 3}}
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read",
			ResultContent: "30\n", ResultContentSize: 3}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want no outcome comparison for a call-row hold",
				same, folded, warnings)
		}
	})

	t.Run("a folded result whose outcome the two reads disagree about", func(t *testing.T) {
		// Claude's folded shape, with the body identical: the archive recorded the call as errored and
		// the transcript now records the same output as a success, so the key and the body both agree and
		// the recorded outcome is the whole difference.
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result",
				ContentRaw: `"PASS"`, ContentSize: 4}}})
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", Status: "errored",
				ContentRaw: "PASS", ContentSize: 4}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 1 {
			t.Errorf("same=%v folded=%d, want the row counted as folded and its outcome compared",
				same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], "records a different outcome for tool result") ||
			!strings.Contains(warnings[0], toolUseID) ||
			!strings.Contains(warnings[0], "of source message #2") {
			t.Errorf("warnings = %v, want the folded result's outcome named", warnings)
		}
	})

	t.Run("a tool call only the archive holds", func(t *testing.T) {
		// A call is what its results are attached to, so a message whose calls differ is a different
		// message even when its role and rendered text agree: the archive recorded a call the source
		// read never saw.
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{
			{ToolUseID: toolUseID, Name: "Read"},
			{ToolUseID: otherToolUseID, Name: "Bash"},
		}
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the archive-only call to be a difference", same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], "the archive holds a tool call the source read does not") ||
			!strings.Contains(warnings[0], otherToolUseID) {
			t.Errorf("warnings = %v, want the archive-only call named by its id", warnings)
		}
	})

	t.Run("a tool call the two reads name differently", func(t *testing.T) {
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Bash"}}
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the renamed call to be a difference", same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], `named "Bash" in the archive and "Read" in the source`) {
			t.Errorf("warnings = %v, want the name difference named with both reads' names", warnings)
		}
	})

	t.Run("a call whose retained input the two reads disagree about", func(t *testing.T) {
		// The Claude `Edit` shape: the rendered message text names only the file, so two edits of one
		// file reach the message comparison as the same text while the inputs they carry differ.
		source := append([]Message{}, base...)
		source[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Edit",
			InputJSON: `{"file_path":"/work/project/retry.go","old_string":"cap = 3","new_string":"cap = 5"}`}}
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Edit",
			InputJSON: `{"file_path":"/work/project/retry.go","old_string":"cap = 3","new_string":"cap = 9"}`}}
		same, folded, warnings := compareMessages(source, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the differing input to be a difference", same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], "carries a different input in the archive than in the source") ||
			!strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the differing input named", warnings)
		}
	})

	t.Run("a call whose input only the archive retains", func(t *testing.T) {
		// The storage projection keeps call names and blanks inputs, so an input the archive does not
		// show is not one it disagrees about: reading the empty spelling as a difference would report a
		// difference the archive's own read cannot support.
		source := append([]Message{}, base...)
		source[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read"}}
		archived := append([]Message{}, base...)
		archived[1].ToolCalls = []ToolCall{{ToolUseID: toolUseID, Name: "Read",
			InputJSON: `{"file_path":"/work/project/retry.go"}`}}
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 0 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the narrowed input accepted",
				same, folded, warnings)
		}
	})

	t.Run("a result the archive holds and the source read does not", func(t *testing.T) {
		// The reverse of the stale-archive shape: the archive recorded the call's outcome and the
		// transcript read has none. Every result comparison above is driven by the source's own rows, so
		// this difference is invisible to them.
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "function_call_output", ContentRaw: "30\n", ContentSize: 3}}
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the archive-only result to be a difference", same, folded)
		}
		if len(warnings) != 1 ||
			!strings.Contains(warnings[0], "holds the tool result(s) the source read does not") ||
			!strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the archive-only result named by its tool-use id", warnings)
		}
	})

	t.Run("a result the archive folded and the source has no row for", func(t *testing.T) {
		// Claude's folded shape the other way round: the archive kept the result its writer folded onto
		// the call, and the source read's transcript has neither the result row nor the event.
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "PASS", ContentSize: 4}}
		same, folded, warnings := compareMessages(base, archived)
		if same || folded != 0 {
			t.Errorf("same=%v folded=%d, want the folded archive result to be a difference", same, folded)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], toolUseID) {
			t.Errorf("warnings = %v, want the folded result named", warnings)
		}
	})

	t.Run("an archived result the source holds on a folded row", func(t *testing.T) {
		// The control the rule above must not break: an archive read materializes a call's result event
		// as the message's own result row, while the source read of the same session records that result
		// on the tool-result-only row the archive folded away -- the same evidence at a different
		// ordinal, which is exactly what folding means.
		archived := append([]Message{}, base...)
		archived[1].ToolResults = []ToolResult{
			{ToolUseID: toolUseID, Source: "tool_result", ContentRaw: "PASS", ContentSize: 4}}
		source := append(append([]Message{}, base...), Message{Ordinal: 2, Role: "user",
			ToolResults: []ToolResult{{ToolUseID: toolUseID, Source: "tool_result",
				ContentRaw: `"PASS"`, ContentSize: 4}}})
		same, folded, warnings := compareMessages(source, archived)
		if !same || folded != 1 || len(warnings) != 0 {
			t.Errorf("same=%v folded=%d warnings=%v, want the source's folded row to hold the result",
				same, folded, warnings)
		}
	})
}

// claudeTranscriptShape selects the native shapes one generated transcript carries. The user turn and
// the `Bash` call with its recorded result are always there; a case may add the shapes its rule needs.
// Bash is not one of the inherited default configuration's blocked result categories, so an archive of
// the file keeps the body: two same-length outputs are what a case needs to hold an archive's recorded
// outcome against a later transcript of the same session.
type claudeTranscriptShape struct {
	// resultOutput is the tool result's recorded content, verbatim, and resultIsError marks it the
	// provider's own failed call (`is_error`).
	resultOutput  string
	resultIsError bool
	// blankDialogue puts a whitespace-only text block on the tool-result record. A blank record without
	// tool evidence is dropped by the inherited parser's own guard, so this is the shape in which a
	// whitespace-only dialogue really arrives: beside evidence the record keeps.
	blankDialogue bool
	// compactSummary appends a compaction summary record. The parser joins that record's text blocks
	// verbatim (`extractCompactSummary`), so a summary that ends in whitespace reaches the document as
	// the provider wrote it.
	compactSummary string
}

// writeClaudeTranscript writes one small Claude session in the selected shapes under a throw-away
// provider root, and returns that root's configuration with the file's path.
func writeClaudeTranscript(t *testing.T, shape claudeTranscriptShape) (config.Config, string) {
	t.Helper()
	const toolUseID = "toolu_01Folded0000000000000001"
	root := t.TempDir()
	relative := filepath.Join("-home-user-code-example-api", claudeFixtureRef.ID+".jsonl")
	path := filepath.Join(root, "claude", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	result := `{"content":` + jsonString(t, shape.resultOutput)
	if shape.resultIsError {
		result += `,"is_error":true`
	}
	result += `,"tool_use_id":"` + toolUseID + `","type":"tool_result"}`
	if shape.blankDialogue {
		result = `{"text":"\n","type":"text"},` + result
	}
	lines := []string{
		`{"cwd":"/home/user/code/example-api","message":{"content":"Check the retry policy."},` +
			`"sessionId":"` + claudeFixtureRef.ID + `","timestamp":"2026-05-04T09:15:00.000Z",` +
			`"type":"user","version":"1.0.30"}`,
		`{"message":{"content":[{"id":"` + toolUseID + `","input":{"command":"make check"},` +
			`"name":"Bash","type":"tool_use"}],"model":"claude-sonnet-4-5"},` +
			`"timestamp":"2026-05-04T09:15:04.000Z","type":"assistant","version":"1.0.30"}`,
		`{"message":{"content":[` + result + `]},"timestamp":"2026-05-04T09:15:06.000Z","type":"user"}`,
	}
	if shape.compactSummary != "" {
		lines = append(lines, `{"isCompactSummary":true,"message":{"content":[{"text":`+
			jsonString(t, shape.compactSummary)+`,"type":"text"}]},`+
			`"timestamp":"2026-05-04T09:15:08.000Z","type":"assistant"}`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	return cfg, path
}

// claudeBashTranscript writes one small Claude session -- a user turn and a `Bash` call whose result the
// transcript records verbatim -- under a throw-away provider root, and returns that root's configuration
// with the file's path. The `is_error` flag is the provider's own marker for a failed call, which the
// parser maps to the result's `errored` status (internal/parser/content.go), so a case can hold two
// transcripts apart by that field alone.
func claudeBashTranscript(t *testing.T, output string, isError bool) (config.Config, string) {
	t.Helper()
	return writeClaudeTranscript(t, claudeTranscriptShape{resultOutput: output, resultIsError: isError})
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
	archiveCfg, archiveFile := claudeBashTranscript(t, "PASS\n", false)
	changedCfg, changedFile := claudeBashTranscript(t, "FAIL\n", false)
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

// TestInspectReportsADifferentFoldedResultOutcome pins the comparison end to end for the *outcome* a
// folded tool result records. The two transcripts differ in one field only -- the Claude tool result's
// `is_error` -- so the call, the key the result is held under and the body are identical, and the
// recorded outcome is the whole difference: the inherited parser maps `is_error` to the result's status
// (`errored`, internal/parser/content.go), ingestion keeps it on the event (internal/ingest), and an
// archive written from the errored transcript was nevertheless reported as holding every source message
// of a transcript that now records the same output as a success, because the comparison read the key and
// the body and never the status both reads carry.
//
// The first half is the control: the same archive read against the transcript it was written from must
// still agree, so the answer cannot pass by reporting every folded result as differing.
func TestInspectReportsADifferentFoldedResultOutcome(t *testing.T) {
	ctx := context.Background()
	erroredCfg, erroredFile := claudeBashTranscript(t, "PASS\n", true)
	successCfg, successFile := claudeBashTranscript(t, "PASS\n", false)
	if erroredFile == successFile {
		t.Fatal("the two transcripts are the same file")
	}
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(erroredCfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, erroredFile); err != nil {
		t.Fatalf("ingesting the transcript: %v", err)
	}

	for name, testCase := range map[string]struct {
		cfg  config.Config
		same bool
	}{
		"the transcript the archive was written from": {cfg: erroredCfg, same: true},
		"the same output recorded as a success":       {cfg: successCfg, same: false},
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
				if strings.Contains(warnings, "different outcome") {
					t.Errorf("the recorded error was reported as differing from itself: %v",
						inspection.Warnings)
				}
				return
			}
			if !strings.Contains(warnings, "records a different outcome") ||
				!strings.Contains(warnings, "errored") {
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

// TestInspectKeepsAProviderFailureWhenNoArchiveHoldsTheSession pins the other half of the provider
// failure rule: with no archive holding the session, the failure is still what an inspection reports,
// and it is not turned into "no readable source session".
//
// The CLI prints an inspection's warnings when neither report exists, and this is the state it prints
// them for: a provider that failed operationally and an archive that does not answer. A reader that
// kept the diagnostics only on the path where a fallback succeeds would leave the operator with the
// absence claim and none of the reason.
func TestInspectKeepsAProviderFailureWhenNoArchiveHoldsTheSession(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
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

	// A data directory holding no archive at all: the fallback cannot succeed here, so the warnings are
	// everything the inspection can report.
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	inspection, err := Inspect(ctx, InspectOptions{
		Ref: claudeFixtureRef, Config: cfg, Reader: reader, ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("the failed source read was reported as a source: %+v", inspection.Source)
	}
	if inspection.Archive != nil {
		t.Errorf("an archive was reported although none exists: %+v", inspection.Archive)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "the store is locked by another reader") {
		t.Errorf("the provider's diagnostics are not exposed: %v", inspection.Warnings)
	}
	if strings.Contains(warnings, "no readable source session") {
		t.Errorf("a provider failure is reported as absence: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, "no archive at ") {
		t.Errorf("the missing archive is not reported: %v", inspection.Warnings)
	}
}

// TestInspectArchiveOnlyWhenTheProviderIsDisabled pins both halves of the inherited `disabled_agents`
// contract ("Disabling Session Providers", docs/configuration.md): the setting keeps the provider out
// of local discovery, so no source session is read or reported for it, and it does not take away what
// an archive already holds, because it "applies only to local filesystem discovery" and "previously
// archived sessions from a disabled provider remain available and exportable".
//
// The defect this pins: the reader answered a disabled provider with an ordinary error, and the
// inspection returned that error before reaching the archive branch, so a session the archive held was
// reported as a failure instead of as the archived session. Two things must stay distinguishable while
// the restriction holds -- the provider is *known* (its label is reported, where an unknown provider
// would be an error) and the state is not absence (the warning carries the setting's own words, and an
// inspection must not say "no readable source session" about a session that was never looked for).
func TestInspectArchiveOnlyWhenTheProviderIsDisabled(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	claudeFile := fixturePath(t, filepath.Join("claude", "-home-user-code-example-api",
		claudeFixtureRef.ID+".jsonl"))
	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, claudeFile); err != nil {
		t.Fatalf("ingesting the fixture: %v", err)
	}

	// The provider is disabled after the archive was written: an archive built earlier is exactly the
	// state the setting's own contract describes.
	cfg.DisabledAgents = []parser.AgentType{parser.AgentClaude}
	reader := NewSourceReader(cfg)
	if _, readErr := reader.Read(ctx, claudeFixtureRef); !errors.Is(readErr, ErrSourceDisabled) {
		t.Fatalf("reading a disabled provider's session: err = %v, want %v",
			readErr, ErrSourceDisabled)
	} else if !strings.Contains(readErr.Error(), "is disabled in this configuration") {
		t.Errorf("the reader lost the setting's own words: %v", readErr)
	}
	if _, _, err := reader.Resolve(ctx, claudeFixtureRef); !errors.Is(err, ErrSourceDisabled) {
		t.Errorf("resolving a disabled provider's session: err = %v, want %v", err, ErrSourceDisabled)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref: claudeFixtureRef, Config: cfg, Reader: reader, ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting a disabled provider's session: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("a disabled provider was read as a source: %+v", inspection.Source)
	}
	if inspection.Agent != "Claude Code" {
		t.Errorf("the disabled provider is reported as unknown: agent = %q", inspection.Agent)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: warnings %v", inspection.Warnings)
	}
	if inspection.Archive.SameMessagesAsSource != nil {
		t.Errorf("the archive was compared with a source read that did not happen: %v",
			*inspection.Archive.SameMessagesAsSource)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "is disabled in this configuration") {
		t.Errorf("the disabled provider is not named: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, "an archive can still hold the session") {
		t.Errorf("the warning does not say why the archive is consulted: %v", inspection.Warnings)
	}
	if strings.Contains(warnings, "no readable source session") {
		t.Errorf("a disabled provider is reported as absence: %v", inspection.Warnings)
	}
}

// TestInspectReturnsAnUnclassifiedReadError pins the switch's last clause: a read failure none of the
// inspection's classifications covers is returned to the caller, not turned into a warning beside a
// report. An unknown provider is the shape the core's own API reaches it with -- cmd/ccc refuses that
// reference while parsing it, but the engine is a public API whose callers can pass any provider name,
// and answering one with an empty inspection would report "nothing here" about a lookup that never
// happened.
func TestInspectReturnsAnUnclassifiedReadError(t *testing.T) {
	inspection, err := Inspect(context.Background(), InspectOptions{
		Ref: Ref{Provider: "no-such-provider", ID: "not-a-session"}, Config: fixtureConfig(t),
	})
	if err == nil {
		t.Fatalf("an unknown provider was reported as an inspection: %+v", inspection)
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("the failure does not name the provider lookup: %v", err)
	}
	if inspection.Source != nil || inspection.Archive != nil || inspection.Warnings != nil {
		t.Errorf("a failed inspection carries a report: %+v", inspection)
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

// TestInspectDoesNotCallAFresherArchiveAgreement pins the comparison in the direction the stale-archive
// case cannot reach: the *archive* holds the call's outcome and the transcript read no longer does. Every
// result comparison is driven by the source's own rows -- the result-presence check asks whether the
// archive holds each source result, and the body and outcome comparisons iterate the source's results --
// so an archive holding a result the source read does not was reported as *every archived message
// matching the source read*. The reverse direction asks what each archived row holds.
//
// The second half is the control: the same archive read against the transcript it was written from still
// agrees, so the new answer cannot pass by reporting every archive-written result as extra.
func TestInspectDoesNotCallAFresherArchiveAgreement(t *testing.T) {
	ctx := context.Background()
	cfgFull, fullFile := codexTranscript(t, true)
	cfgStale, staleFile := codexTranscript(t, false)

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfgFull, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, fullFile); err != nil {
		t.Fatalf("ingesting the rollout with its call output: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         codexFixtureRef,
		Config:      cfgStale,
		Reader:      NewSourceReader(cfgStale),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the earlier transcript against the archive that has the output: %v", err)
	}
	if inspection.Source == nil || inspection.Archive == nil {
		t.Fatalf("source=%v archive=%v warnings=%v, want both reads reported",
			inspection.Source != nil, inspection.Archive != nil, inspection.Warnings)
	}
	archived := inspection.Archive
	if archived.SameMessagesAsSource == nil || *archived.SameMessagesAsSource {
		t.Errorf("an archive holding a result the source read does not was reported as agreeing: "+
			"same=%v warnings=%v",
			archived.SameMessagesAsSource != nil && *archived.SameMessagesAsSource, archived.Warnings)
	}
	if archived.FoldedToolResults != 0 {
		t.Errorf("the archive was credited with %d folded tool result(s)", archived.FoldedToolResults)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "holds the tool result(s) the source read does not") {
		t.Errorf("the archive's extra result is not named: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, codexStaleCallID) {
		t.Errorf("the archive-only result is not named by its tool-use id: %v", inspection.Warnings)
	}

	// The control: the same archive against the transcript it was written from.
	currentInspection, err := Inspect(ctx, InspectOptions{
		Ref:         codexFixtureRef,
		Config:      cfgFull,
		Reader:      NewSourceReader(cfgFull),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the session against its own archive: %v", err)
	}
	if currentInspection.Archive == nil {
		t.Fatalf("the archive was not reported: %v", currentInspection.Warnings)
	}
	current := currentInspection.Archive
	if current.SameMessagesAsSource == nil || !*current.SameMessagesAsSource {
		t.Errorf("an archive holding the source read's own results was reported as disagreeing: "+
			"warnings=%v", current.Warnings)
	}
	if staleFile == fullFile {
		t.Fatal("the two transcripts are the same file")
	}
}

// TestInspectReportsARetryableResultWithItsMessages pins that a provider's own retry state travels with
// the conversation it classified. A Codex fork whose copied prefix comes from a page whose parent head is
// unreadable parses to a result that keeps the copied parent messages *and* says it needs a retry
// (parser.DataVersionNeedsRetry with a reason naming the fork) -- the inherited
// TestCodexForkWithOnlyParentPageNeedsRetry pins the provider's half. An inspection that reported only
// the messages presented copied parent history as the session's own recorded turns, with nothing saying
// its boundary is unresolved.
//
// The second half is the control: once the parent's head is readable the same reference parses to the
// fork's own turn alone, needs no retry, and the warnings say nothing about one -- so the rule cannot
// pass by flagging every read.
func TestInspectReportsARetryableResultWithItsMessages(t *testing.T) {
	ctx := context.Background()
	cfg, _ := codexForkRollouts(t, false)
	inspection, err := Inspect(ctx, InspectOptions{
		Ref:    Ref{Provider: "codex", ID: codexForkID},
		Config: cfg,
		Reader: NewSourceReader(cfg),
	})
	if err != nil {
		t.Fatalf("inspecting the fork whose parent head is unreadable: %v", err)
	}
	if inspection.Source == nil {
		t.Fatalf("the fork was not read: %v", inspection.Warnings)
	}
	source := inspection.Source
	if !source.Fidelity.NeedsRetry {
		t.Errorf("a result the provider classified as needing a retry was reported without one: %+v",
			source.Fidelity)
	}
	if !strings.Contains(source.Fidelity.RetryReason, "forked from") {
		t.Errorf("retry reason = %q, want the provider's own explanation", source.Fidelity.RetryReason)
	}
	if source.Counts.Messages < 2 {
		t.Errorf("the copied parent history the retry explains is not reported: %d message(s)",
			source.Counts.Messages)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "needing a retry") || !strings.Contains(warnings, "forked from") {
		t.Errorf("the retry and its reason are not reported: %v", inspection.Warnings)
	}

	// The control: the same reference once the parent's head is readable.
	resolvedCfg, _ := codexForkRollouts(t, true)
	resolved, err := Inspect(ctx, InspectOptions{
		Ref:    Ref{Provider: "codex", ID: codexForkID},
		Config: resolvedCfg,
		Reader: NewSourceReader(resolvedCfg),
	})
	if err != nil {
		t.Fatalf("inspecting the fork whose parent head is readable: %v", err)
	}
	if resolved.Source == nil {
		t.Fatalf("the resolved fork was not read: %v", resolved.Warnings)
	}
	if resolved.Source.Fidelity.NeedsRetry {
		t.Errorf("a resolved fork still reports a pending retry: %+v", resolved.Source.Fidelity)
	}
	if resolved.Source.Fidelity.RetryReason != "" {
		t.Errorf("retry reason = %q, want none", resolved.Source.Fidelity.RetryReason)
	}
	if strings.Contains(strings.Join(resolved.Warnings, "\n"), "needing a retry") {
		t.Errorf("a resolved read warns about a retry: %v", resolved.Warnings)
	}
	if resolved.Source.Counts.Messages >= source.Counts.Messages {
		t.Errorf("the resolved read reports %d message(s), the unresolved one %d: the copied prefix "+
			"should be gone", resolved.Source.Counts.Messages, source.Counts.Messages)
	}
}

// codexForkID is the fork in codexForkRollouts; its parent thread is codexForkParentID.
const (
	codexForkID       = "7a1f0c34-5b2e-4d61-8f47-9c0a1e2b3d45"
	codexForkParentID = "2d9b7e10-4c65-4a83-9a12-6f5e0d3c8b47"
)

// codexForkRollouts writes the missing-parent-page shape under a throw-away Codex root: a fork rollout
// whose copied prefix names a parent whose only rollout is a history page, and -- with parentHead -- the
// parent's head rollout that resolves it. The rollouts are built by the inherited internal/testjsonl
// builders, so the bytes are the shape the pinned revision's own tests use for the same rule
// (internal/parser's TestCodexForkWithOnlyParentPageNeedsRetry).
func codexForkRollouts(t *testing.T, parentHead bool) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "codex", "2026", "09", "01")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	write := func(key string, lines ...string) {
		t.Helper()
		path := filepath.Join(dir, "rollout-2026-09-01T10-00-00-"+key+".jsonl")
		if err := os.WriteFile(path, []byte(testjsonl.JoinJSONL(lines...)), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	// The parent's page: paginated history based on the thread itself, which is only resolvable once
	// the thread's own head rollout exists.
	write(codexForkParentID+"_3f7c9d21-0a4b-4e58-9c36-1d2e4f506172",
		testjsonl.CodexSessionMetaWithFieldsJSON(codexForkParentID, "/work/project", "codex_cli_rs",
			"2026-09-01T11:00:00Z", map[string]any{
				"history_mode": "paginated",
				"history_base": map[string]any{
					"thread_id":             codexForkParentID,
					"end_ordinal_exclusive": 3,
				},
			}),
		testjsonl.CodexTurnContextWithIDJSON("gpt-5", "page-turn", "2026-09-01T10:00:00Z"),
	)
	if parentHead {
		write(codexForkParentID,
			testjsonl.CodexSessionMetaWithFieldsJSON(codexForkParentID, "/work/project", "codex_cli_rs",
				"2026-09-01T11:00:00Z", map[string]any{"history_mode": "paginated"}),
			testjsonl.CodexTurnContextWithIDJSON("gpt-5", "head-turn", "2026-09-01T10:00:00Z"),
		)
	}
	write(codexForkID,
		testjsonl.CodexForkedSessionMetaJSON(codexForkID, codexForkParentID, "/work/project", "user",
			"2026-09-01T10:00:00Z"),
		testjsonl.CodexTurnContextWithIDJSON("gpt-5", "head-turn", "2026-09-01T10:00:00Z"),
		testjsonl.CodexMsgJSON("user", "copied head question", "2026-09-01T10:00:00Z"),
		testjsonl.CodexTurnContextWithIDJSON("gpt-5", "page-turn", "2026-09-01T10:00:00Z"),
		testjsonl.CodexMsgJSON("user", "copied page question", "2026-09-01T10:00:00Z"),
		testjsonl.CodexTurnContextWithIDJSON("gpt-5", "child-turn", "2026-09-01T10:10:00Z"),
		testjsonl.CodexMsgJSON("user", "child question", "2026-09-01T10:10:00Z"),
	)
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentCodex: {filepath.Join(root, "codex")}}
	return cfg, filepath.Join(dir, "rollout-2026-09-01T10-00-00-"+codexForkID+".jsonl")
}

// TestInspectReportsADifferentToolCallInput pins the comparison end to end for the shape the message text
// cannot show: two Claude transcripts whose `Edit` calls change different values in the same file render
// to the same `[Edit: <file_path>]` text, so every message comparison agrees while the inputs the two
// reads retained differ. The inherited renderer is what makes the calls' arguments absent from the text
// (internal/parser/content.go), and both adapters carry the inputs, so the comparison is the only place
// that can tell the two reads apart.
//
// The second half is the control: the same archive against a transcript that recorded the same edit
// still agrees, so the new answer cannot pass by reporting every retained input as differing.
func TestInspectReportsADifferentToolCallInput(t *testing.T) {
	ctx := context.Background()
	firstCfg, firstFile := claudeEditTranscript(t, "cap = 5")
	secondCfg, secondFile := claudeEditTranscript(t, "cap = 9")
	if firstFile == secondFile {
		t.Fatal("the two transcripts are the same file")
	}
	// The premise: the two reads' message texts are identical, so nothing but the calls' inputs can
	// separate them.
	firstRead, err := NewSourceReader(firstCfg).Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the first transcript: %v", err)
	}
	secondRead, err := NewSourceReader(secondCfg).Read(ctx, claudeFixtureRef)
	if err != nil {
		t.Fatalf("reading the second transcript: %v", err)
	}
	if len(firstRead.Messages) != len(secondRead.Messages) {
		t.Fatalf("the two transcripts read to %d and %d message(s)",
			len(firstRead.Messages), len(secondRead.Messages))
	}
	for i := range firstRead.Messages {
		if firstRead.Messages[i].Content != secondRead.Messages[i].Content {
			t.Fatalf("message #%d differs in text (%q/%q), so this case does not exercise the input",
				i, firstRead.Messages[i].Content, secondRead.Messages[i].Content)
		}
	}
	if !strings.Contains(firstRead.Messages[1].Content, "[Edit: /work/project/retry.go]") {
		t.Fatalf("the edit's rendered text is not the file-only rendering: %q",
			firstRead.Messages[1].Content)
	}
	if inputsMatch(firstRead, secondRead) {
		t.Fatal("the two transcripts retain the same call inputs, so this case has nothing to compare")
	}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(firstCfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, firstFile); err != nil {
		t.Fatalf("ingesting the first transcript: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Config:      secondCfg,
		Reader:      NewSourceReader(secondCfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the second transcript against the archive: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archive was not reported: %v", inspection.Warnings)
	}
	archived := inspection.Archive
	if archived.SameMessagesAsSource == nil || *archived.SameMessagesAsSource {
		t.Errorf("an archive retaining a different edit was reported as agreeing: same=%v warnings=%v",
			archived.SameMessagesAsSource != nil && *archived.SameMessagesAsSource, archived.Warnings)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "carries a different input in the archive than in the source") {
		t.Errorf("the differing input is not named: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, claudeEditCallID) {
		t.Errorf("the differing input is not named by its tool-use id: %v", inspection.Warnings)
	}

	// The control: the same archive against the transcript it was written from.
	sameInspection, err := Inspect(ctx, InspectOptions{
		Ref:         claudeFixtureRef,
		Config:      firstCfg,
		Reader:      NewSourceReader(firstCfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the first transcript against its own archive: %v", err)
	}
	if sameInspection.Archive == nil {
		t.Fatalf("the archive was not reported: %v", sameInspection.Warnings)
	}
	if same := sameInspection.Archive.SameMessagesAsSource; same == nil || !*same {
		t.Errorf("an archive of the same edit was reported as disagreeing: warnings=%v",
			sameInspection.Archive.Warnings)
	}
}

// inputsMatch reports whether two reads retained the same call inputs, so a case can require that the
// transcripts it compares really differ in the evidence it means to test.
func inputsMatch(first, second Conversation) bool {
	if len(first.Messages) != len(second.Messages) {
		return false
	}
	for i := range first.Messages {
		firstCalls, secondCalls := first.Messages[i].ToolCalls, second.Messages[i].ToolCalls
		if len(firstCalls) != len(secondCalls) {
			return false
		}
		for j := range firstCalls {
			if firstCalls[j].InputJSON != secondCalls[j].InputJSON {
				return false
			}
		}
	}
	return true
}

// claudeEditCallID is the `Edit` call claudeEditTranscript records.
const claudeEditCallID = "toolu_01Edit0000000000000000001"

// claudeEditTranscript writes one small Claude session -- a user turn and an `Edit` call whose two string
// arguments are the given replacement -- under a throw-away provider root, and returns that root's
// configuration with the file's path. The rendered message text names only the edited file
// (internal/parser's Edit rendering), so two transcripts that differ in this argument alone read to the
// same messages while their calls' retained inputs differ.
func claudeEditTranscript(t *testing.T, newString string) (config.Config, string) {
	t.Helper()
	root := t.TempDir()
	relative := filepath.Join("-work-project", claudeFixtureRef.ID+".jsonl")
	path := filepath.Join(root, "claude", relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("preparing the provider root: %v", err)
	}
	input := `{"file_path":"/work/project/retry.go","old_string":"cap = 3","new_string":` +
		jsonString(t, newString) + `}`
	lines := []string{
		`{"cwd":"/work/project","message":{"content":"Pin the retry cap."},` +
			`"sessionId":"` + claudeFixtureRef.ID + `","timestamp":"2026-09-01T10:00:00.000Z",` +
			`"type":"user","version":"1.0.30"}`,
		`{"message":{"content":[{"id":"` + claudeEditCallID + `","input":` + input + `,` +
			`"name":"Edit","type":"tool_use"}],"model":"claude-sonnet-4-5"},` +
			`"timestamp":"2026-09-01T10:00:04.000Z","type":"assistant","version":"1.0.30"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentClaude: {filepath.Join(root, "claude")}}
	return cfg, path
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

// TestFreebuffSessionResolvesThroughItsCodebuffProvider pins the inherited provider-to-stored-agent
// mapping across the archive's whole resolution surface.
//
// The Codebuff provider also writes free-tier sessions, under the Freebuff label
// (parser.ProviderStoredAgents records exactly that, and internal/parser's
// TestParseCodebuffSession_FreebuffClassification pins the parser's own output), so an archive row of
// such a session carries `agent = freebuff` while its id carries the `freebuff:` prefix. Every archive
// lookup compared the row's `agent` column with the provider's own name, so a session read from its
// source by a `codebuff:<project>:<timestamp>` reference stopped being findable -- by id, by
// provider-prefixed id, by stored agent label and by path -- the moment that source was gone: an
// inspection reported no session for a conversation the archive holds. The case walks one written
// archive through every spelling with the source present, with it gone, and after the disappearance is
// reconciled (the watcher's own state, where the path lookup's retained tier answers), and it holds a
// cross-provider reference to not-found so the mapping cannot pass by widening the answers.
func TestFreebuffSessionResolvesThroughItsCodebuffProvider(t *testing.T) {
	ctx := context.Background()
	cfg := fixtureConfig(t)
	root, chatPath, project, timestamp := freebuffSession(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentCodebuff: {root}}
	nativeID := project + ":" + timestamp
	freebuffID := string(parser.AgentFreebuff) + ":" + nativeID

	// The source side reads it through the Codebuff provider, which reports the Freebuff identity for
	// it; the stored label is a reference too, because it is the identity an archive row records.
	reader := NewSourceReader(cfg)
	source, err := reader.Read(ctx, Ref{Provider: "codebuff", ID: nativeID})
	if err != nil {
		t.Fatalf("reading the Freebuff source through its Codebuff provider: %v", err)
	}
	if source.Source.SessionID != freebuffID {
		t.Fatalf("the source read reports %q, want the parser's own %q", source.Source.SessionID, freebuffID)
	}
	byLabel, err := reader.Read(ctx, Ref{Provider: string(parser.AgentFreebuff), ID: nativeID})
	if err != nil {
		t.Fatalf("reading the source under its stored agent label: %v", err)
	}
	if byLabel.Source.SessionID != freebuffID {
		t.Errorf("the by-label source read reports %q, want %q", byLabel.Source.SessionID, freebuffID)
	}
	// The label is the parser's own for this session (internal/parser's
	// TestParseCodebuffSession_FreebuffClassification pins it), and the read has to carry it: the
	// registry has no def for the label, so a read that dropped the recorded one would name no agent.
	sourceLabel := byLabel.Source.Agent
	if sourceLabel == "" {
		t.Errorf("the by-label source read reports no agent label, want the one the parser recorded")
	}

	ingestor, archivePath := ingestFile(t, cfg, chatPath)
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	// Every spelling a caller can write for this session: the provider's own name with the native id
	// (the suffix lookup), the provider's name with the stored id's own prefix, the stored agent label
	// (which is a whole provider name to a caller, the archive reports it), and the path the session was
	// read from.
	spellings := []Ref{
		{Provider: "codebuff", ID: nativeID},
		{Provider: "codebuff", ID: freebuffID},
		{Provider: string(parser.AgentFreebuff), ID: nativeID},
		{Provider: "codebuff", ID: chatPath},
	}
	assertFreebuffArchiveAnswer := func(state string) {
		t.Helper()
		for _, ref := range spellings {
			status, found, err := archive.Status(ctx, ref)
			if err != nil {
				t.Fatalf("%s: status of %s:%s: %v", state, ref.Provider, ref.ID, err)
			}
			if !found {
				t.Fatalf("%s: the archive does not resolve %s:%s", state, ref.Provider, ref.ID)
			}
			if status.SessionID != freebuffID {
				t.Errorf("%s: %s:%s resolved %q, want %q",
					state, ref.Provider, ref.ID, status.SessionID, freebuffID)
			}
			if status.Agent != string(parser.AgentFreebuff) {
				t.Errorf("%s: %s:%s reports agent %q, want the row's own label %q",
					state, ref.Provider, ref.ID, status.Agent, parser.AgentFreebuff)
			}
			if status.AgentLabel != sourceLabel {
				t.Errorf("%s: %s:%s reports agent label %q, want the label the parse recorded %q",
					state, ref.Provider, ref.ID, status.AgentLabel, sourceLabel)
			}
			conversation, err := archive.Read(ctx, ref)
			if err != nil {
				t.Fatalf("%s: reading %s:%s: %v", state, ref.Provider, ref.ID, err)
			}
			if len(conversation.Messages) != 2 {
				t.Errorf("%s: %s:%s read %d message(s), want the session's two",
					state, ref.Provider, ref.ID, len(conversation.Messages))
			}
		}
	}
	assertFreebuffArchiveAnswer("while the source is there")

	// A *relative* reference to the same transcript. The roots a relative path is relative to come from
	// the configuration, and for a stored agent label they are the roots of the provider that writes it
	// (providerRoots) -- so a relative Freebuff reference resolves the same way an absolute one does,
	// through the reader and through an archive opened with the configuration, exactly as an inspection
	// opens it.
	relative := filepath.Join(project, "chats", timestamp, "chat-messages.json")
	if _, err := reader.Read(ctx, Ref{Provider: string(parser.AgentFreebuff), ID: relative}); err != nil {
		t.Errorf("reading a relative Freebuff path through its provider: %v", err)
	}
	rooted, err := openArchive(ctx, archivePath, cfg.AgentDirs)
	if err != nil {
		t.Fatalf("opening the archive with the configured roots: %v", err)
	}
	defer rooted.Close()
	for _, provider := range []string{"codebuff", string(parser.AgentFreebuff)} {
		ref := Ref{Provider: provider, ID: relative}
		status, found, err := rooted.Status(ctx, ref)
		if err != nil {
			t.Fatalf("status of the relative %s reference: %v", provider, err)
		}
		if !found || status.SessionID != freebuffID {
			t.Errorf("the relative %s path resolved found=%v id=%q, want %q",
				provider, found, status.SessionID, freebuffID)
		}
	}

	// A provider whose rows do not carry the label is not answered by the alias: the mapping widens a
	// lookup to the labels one provider writes, not to another agent's rows.
	if status, found, err := archive.Status(ctx, Ref{Provider: "codex", ID: nativeID}); err != nil || found {
		t.Errorf("codex:%s resolved %+v (found=%v err=%v), want not found", nativeID, status, found, err)
	}

	// The source is gone: every spelling must still answer, which is the state an archive exists for and
	// the state the exact-label comparison broke.
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("removing the source: %v", err)
	}
	assertFreebuffArchiveAnswer("after the source is gone")

	// And after the disappearance is reconciled -- the state a watcher pass leaves, where a path
	// reference is answered from the archive's retained rows rather than its current members.
	reconcileVanishedSource(t, ingestor, chatPath)
	requireSourceMissing(t, archivePath, freebuffID)
	assertFreebuffArchiveAnswer("after the vanished source is reconciled")

	// An inspection of the label with the source gone reports the archived session *and* the label its
	// row recorded, because the provider name has no registry display name to answer with: the label the
	// writer stored is the agent an operator reads this inspection for.
	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         Ref{Provider: string(parser.AgentFreebuff), ID: nativeID},
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the archived Freebuff session: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived session was not reported: %v", inspection.Warnings)
	}
	if inspection.Agent == "" || inspection.Agent != inspection.Archive.Status.AgentLabel {
		t.Errorf("the inspection reports agent %q, want the archived row's label %q",
			inspection.Agent, inspection.Archive.Status.AgentLabel)
	}
}

// TestHostQualifiedFreebuffRowResolvesThroughItsCodebuffProvider pins the host-qualified half of the
// same mapping: a session synced from another machine is stored as `<host>~<the id it had there>`, and a
// Freebuff session's id there starts with its own prefix (`host-a~freebuff:<project>:<timestamp>`). A
// `codebuff:` reference must resolve that stored spelling and the host-qualified native spelling the CLI
// takes -- while the *local* Codebuff row of the same project and timestamp, which shares the native id,
// is never offered in place of the remote one.
func TestHostQualifiedFreebuffRowResolvesThroughItsCodebuffProvider(t *testing.T) {
	const native = "example-project:2026-07-15T20-01-32.065Z"
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	localID := "codebuff:" + native
	remoteID := "host-a~freebuff:" + native
	seed := func(id, machine, agent, content string) {
		t.Helper()
		dbtest.SeedSessionWithMessages(t, database, id, "example-project",
			[]db.Message{dbtest.UserMsg(id, 0, content)},
			func(session *db.Session) { session.Agent = agent; session.Machine = machine })
	}
	seed(localID, "local", string(parser.AgentCodebuff), "the Codebuff session recorded here")
	seed(remoteID, "host-a", string(parser.AgentFreebuff), "the Freebuff session recorded on host-a")
	if err := database.Close(); err != nil {
		t.Fatalf("closing the seeded archive: %v", err)
	}

	ctx := context.Background()
	archive, err := OpenArchive(ctx, archivePath)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer archive.Close()

	// The stored spelling, exactly as a remote sync wrote it, and the host-qualified native spelling a
	// caller writes without knowing the stored prefix: both name host-a's Freebuff session.
	for _, ref := range []Ref{
		{Provider: "codebuff", ID: remoteID},
		{Provider: "codebuff", ID: "host-a~" + native},
	} {
		status, found, err := archive.Status(ctx, ref)
		if err != nil {
			t.Fatalf("status of %s:%s: %v", ref.Provider, ref.ID, err)
		}
		if !found {
			t.Fatalf("the archive does not resolve %s:%s", ref.Provider, ref.ID)
		}
		if status.SessionID != remoteID {
			t.Errorf("%s:%s resolved %q, want the remote row %q", ref.Provider, ref.ID, status.SessionID, remoteID)
		}
		conversation, err := archive.Read(ctx, ref)
		if err != nil {
			t.Fatalf("reading %s:%s: %v", ref.Provider, ref.ID, err)
		}
		if len(conversation.Messages) != 1 ||
			!strings.Contains(conversation.Messages[0].Content, "host-a") {
			t.Errorf("%s:%s read the wrong conversation: %+v", ref.Provider, ref.ID, conversation.Messages)
		}
	}

	// The same native id without a host is this machine's Codebuff row -- the host narrowing is what
	// keeps the two apart in both directions.
	if status, found, err := archive.Status(ctx, Ref{Provider: "codebuff", ID: native}); err != nil ||
		!found || status.SessionID != localID {
		t.Errorf("codebuff:%s: found=%v err=%v status=%+v, want the local row %q",
			native, found, err, status, localID)
	}
	// A provider whose rows do not carry the label is not answered by the alias.
	if status, found, err := archive.Status(ctx, Ref{Provider: "codex", ID: "host-a~" + native}); err != nil || found {
		t.Errorf("codex:host-a~%s resolved %+v (found=%v err=%v), want not found", native, status, found, err)
	}
}

// TestInspectNamesTheIdentityOfAPartialContainer pins what an inspection does with a container whose
// parse could not read every member: the provider's verdict and the identity it *did* read are the
// warning, no session is claimed for the container path (the archive stores members under their virtual
// paths, not the container, so that path has no row there), and the named identity is what a follow-up
// reference reads from the archive once the source is gone. The refusal is therefore actionable rather
// than a dead end.
//
// The write path agrees with the read path here, which is worth pinning too: the inherited sync engine
// reports this source as an incomplete sync while it writes the member it read, so an ingest of it is
// not a silent success either.
func TestInspectNamesTheIdentityOfAPartialContainer(t *testing.T) {
	ctx := context.Background()
	const sessionID = "5c1d2f30-9b47-4a2c-8f0e-1d2b3c4a5e60"
	root, dbPath := traeContainer(t, traeSessionEntry(sessionID)+","+traeMalformedEntry)
	cfg := fixtureConfig(t)
	cfg.AgentDirs = map[parser.AgentType][]string{parser.AgentTrae: {root}}

	archivePath := filepath.Join(t.TempDir(), "ccc", "sessions.db")
	ingestor, err := NewIngestor(cfg, archivePath)
	if err != nil {
		t.Fatalf("building the ingestor: %v", err)
	}
	if err := ingestor.IngestPaths(ctx, dbPath); err == nil {
		t.Errorf("the inherited sync engine reported a partial container as a complete sync")
	} else if !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("ingesting the container: %v", err)
	}

	inspection, err := Inspect(ctx, InspectOptions{
		Ref:         Ref{Provider: "trae", ID: dbPath},
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the container path: %v", err)
	}
	if inspection.Source != nil {
		t.Errorf("a partial container parse was reported as a source read: %+v", inspection.Source.Identity)
	}
	if inspection.Archive != nil {
		t.Errorf("the container path was claimed as an archived session: %+v", inspection.Archive.Status)
	}
	warnings := strings.Join(inspection.Warnings, "\n")
	if !strings.Contains(warnings, "trae:"+sessionID) {
		t.Errorf("the warning does not name the identity the parse read: %v", inspection.Warnings)
	}
	if !strings.Contains(warnings, "an archive can still hold the session") {
		t.Errorf("the warning does not say why the archive is consulted: %v", inspection.Warnings)
	}

	// The identity from that warning is the explicit reference that answers: the source is gone by then,
	// and the archive still holds the member the container parse read.
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("removing the source: %v", err)
	}
	inspection, err = Inspect(ctx, InspectOptions{
		Ref:         Ref{Provider: "trae", ID: "trae:" + sessionID},
		Config:      cfg,
		Reader:      NewSourceReader(cfg),
		ArchivePath: archivePath,
	})
	if err != nil {
		t.Fatalf("inspecting the identity the refusal named: %v", err)
	}
	if inspection.Archive == nil {
		t.Fatalf("the archived member was not reported: %v", inspection.Warnings)
	}
	if inspection.Archive.Status.SessionID != "trae:"+sessionID {
		t.Errorf("the archive reports session %q, want %q", inspection.Archive.Status.SessionID, "trae:"+sessionID)
	}
}

// TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead pins the narrow half of the host-qualified
// rule: the archive's own spelling of a row is answered *directly*, without the inherited suffix lookup,
// so a reference that spells that row exactly still resolves when the candidate list for the native id
// is one the lookup cannot complete (the archive shape
// TestArchiveDisclosesACandidateListItCannotComplete seeds).
//
// Which row the exact spelling answers is decided by the provider predicate, and that predicate is the
// inherited stored-label mapping: a row the Codebuff provider recorded as Freebuff is this reference's
// session, where an exact-name comparison would fall through to the truncated candidate read and refuse
// a row the caller had spelled out in full.
func TestHostQualifiedFreebuffRowAnswersBeyondTheCandidateRead(t *testing.T) {
	const native = "example-project:2026-07-15T20-01-32.065Z"
	remoteID := "host-a~freebuff:" + native
	dir := dbtest.MkdirTempWithCleanup(t)
	archivePath := filepath.Join(dir, "sessions.db")
	database := dbtest.OpenTestDBAt(t, archivePath)
	dbtest.SeedSessionWithMessages(t, database, remoteID, "example-project",
		[]db.Message{dbtest.UserMsg(remoteID, 0, "the Freebuff session recorded on host-a")},
		func(session *db.Session) {
			session.Agent = string(parser.AgentFreebuff)
			session.Machine = "host-a"
		})
	for index := range rawSuffixCandidateCap + 1 {
		id := fmt.Sprintf("flood-%d~freebuff:%s", index, native)
		dbtest.SeedSession(t, database, id, "example-project", func(session *db.Session) {
			session.Agent = string(parser.AgentFreebuff)
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

	// The candidate read cannot be completed, which is what makes the exact spelling the only way to
	// this row.
	if status, found, err := archive.Status(ctx, Ref{Provider: "codebuff", ID: "host-a~" + native}); found ||
		!errors.Is(err, ErrTruncatedCandidates) {
		t.Fatalf("the candidate read answered found=%v err=%v status=%+v, want %v",
			found, err, status, ErrTruncatedCandidates)
	}
	status, found, err := archive.Status(ctx, Ref{Provider: "codebuff", ID: remoteID})
	if err != nil {
		t.Fatalf("status of the spelled-out Freebuff row: %v", err)
	}
	if !found || status.SessionID != remoteID {
		t.Errorf("the spelled-out row resolved found=%v id=%q, want %q", found, status.SessionID, remoteID)
	}
}
