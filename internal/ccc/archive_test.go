package ccc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

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
		if fromArchive.Fidelity.ReadFrom != "archive" || fromArchive.Fidelity.Projection != "full" {
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
		t.Logf("%s: stored id %q, %d message(s), %d folded row(s), projection %q",
			ref.ID, status.SessionID, status.MessageCount, folded, status.Projection)
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
