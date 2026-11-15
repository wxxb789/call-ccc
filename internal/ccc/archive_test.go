package ccc

import (
	"context"
	"errors"
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
