package ccc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRunCodexTurnResolvesARelativeExecutableBeforeChangingDirectory pins the resume turn's one
// directory-sensitive path. A relative Cmd.Path is evaluated by the operating system relative to the
// directory the child runs in, and this turn runs in the source session's working directory -- so the
// caller's own spelling ("./tools/codex", the form the probe found and every earlier verification stage
// used) would be looked for under that directory instead: a different executable, or none.
//
// The stub is this test binary under a relative spelling, run with a filter that matches no test: it
// exits 0 immediately, which is what separates "the operating system launched the harness" from "it
// could not be found".
func TestRunCodexTurnResolvesARelativeExecutableBeforeChangingDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolving the test binary: %v", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatalf("resolving the test binary absolutely: %v", err)
	}
	// The relative spelling is relative to the process's own directory, which is where a caller who
	// typed "./tools/codex" stood.
	t.Chdir(filepath.Dir(executable))
	relative := "." + string(filepath.Separator) + filepath.Base(executable)

	_, diagnostics, err := runCodexTurn(context.Background(), relative, CodexResumeOptions{
		WorkingDir: t.TempDir(),
		Home:       t.TempDir(),
	}, 2*time.Minute, []string{"-test.run=^$"})
	if err != nil {
		t.Fatalf("the turn did not run the harness the caller's relative path names (%v); diagnostics: %q",
			err, summariseText(diagnostics, 200))
	}
}

// TestBoundedBufferKeepsTheFirstBytes pins what the app server's diagnostic buffer is for: the first
// bytes written are kept, everything past the bound is dropped, and a write is still answered with the
// length it was given -- which is the io.Writer contract os/exec's copier relies on, since a short
// answer there is reported as a copy error instead of as the diagnostics themselves.
func TestBoundedBufferKeepsTheFirstBytes(t *testing.T) {
	buffer := newBoundedBuffer(4)
	if written, err := buffer.Write([]byte("abcd")); written != 4 || err != nil {
		t.Fatalf("Write = %d, %v; want 4, <nil>", written, err)
	}
	if written, err := buffer.Write([]byte("efgh")); written != 4 || err != nil {
		t.Fatalf("a write past the bound answered %d, %v; want 4, <nil>", written, err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Errorf("snapshot = %q, want the first 4 bytes", got)
	}
	cut := newBoundedBuffer(4)
	if written, err := cut.Write([]byte("abcdef")); written != 6 || err != nil {
		t.Fatalf("a write crossing the bound answered %d, %v; want 6, <nil>", written, err)
	}
	if got := cut.String(); got != "abcd" {
		t.Errorf("a write crossing the bound left %q, want the first 4 bytes", got)
	}
}

// TestBoundedBufferIsSafeForConcurrentWritesAndSnapshots pins the synchronization the failure path
// depends on.
//
// os/exec copies the app server's stderr into this buffer on a goroutine of its own, and a verification
// that fails reads the excerpt (stderrExcerpt) while that copy is still running: the process is only
// stopped by the deferred Close, after the failure detail has been built. Unsynchronized, that snapshot
// is a data race on the buffer's own state, and the diagnostics a caller reads come from a torn read
// rather than from a snapshot.
//
// The case is written to be run under -race (`go test -race ./internal/ccc/`), which is where that race
// is diagnosed; without the detector it holds the invariant a reader asserts -- every snapshot is
// bounded by the buffer's limit and holds only bytes some writer wrote -- and a corrupted read would
// fail here (a panic, or a snapshot outside those bounds) rather than pass silently.
func TestBoundedBufferIsSafeForConcurrentWritesAndSnapshots(t *testing.T) {
	const (
		bound    = 64
		writers  = 4
		rounds   = 500
		reads    = 200
		alphabet = "abcd"
	)
	buffer := newBoundedBuffer(bound)
	var group sync.WaitGroup
	for index := range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			chunk := []byte{alphabet[index]}
			for range rounds {
				buffer.Write(chunk)
			}
		}()
	}
	// The snapshots are taken while the writers run: this is exactly the read a failure report makes
	// before the app server has been stopped.
	for range reads {
		snapshot := buffer.String()
		if len(snapshot) > bound {
			t.Fatalf("a snapshot of %d byte(s) exceeds the bound of %d", len(snapshot), bound)
		}
		if strings.Trim(snapshot, alphabet) != "" {
			t.Fatalf("a snapshot holds a byte no writer wrote: %q", snapshot)
		}
	}
	group.Wait()
	if snapshot := buffer.String(); len(snapshot) != bound {
		t.Errorf("after %d write(s) the buffer holds %d byte(s), want the bound (%d)",
			writers*rounds, len(snapshot), bound)
	}
}
