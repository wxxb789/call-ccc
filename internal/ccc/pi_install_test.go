package ccc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallPiSessionIsExclusiveAndPrivate pins the two rules an installation has to hold: nothing at
// the target name is ever replaced, and the file it publishes is its owner's alone.
func TestInstallPiSessionIsExclusiveAndPrivate(t *testing.T) {
	home := t.TempDir()
	session := PiSession{
		SessionID: "11111111-1111-4111-8111-111111111111",
		FileName:  "2026-01-02T00-00-00-000Z_11111111-1111-4111-8111-111111111111.jsonl",
		Directory: "--home-user-project",
		Data:      []byte("{\"type\":\"session\",\"version\":3,\"id\":\"11111111-1111-4111-8111-111111111111\"}\n"),
	}
	path, mechanism, err := InstallPiSession(session, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	want := filepath.Join(home, piSessionsSubdir, "--home-user-project", session.FileName)
	if path != want {
		t.Errorf("installed at %q, want %q", path, want)
	}
	if mechanism != "hard-link" {
		t.Errorf("mechanism = %q, want the exclusive hard link", mechanism)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the installed session is not there: %v", err)
	}
	if runtime.GOOS == "windows" {
		if piSessionFileMode.Perm() != 0o600 {
			t.Errorf("the installer asks for mode %v, want 0600 (Windows reports no POSIX mode bits, "+
				"so the request is the half of the rule that is checkable here)", piSessionFileMode)
		}
	} else if perm := info.Mode().Perm(); perm != piSessionFileMode.Perm() || perm&0o077 != 0 {
		t.Errorf("the installed session's permissions are %v, want %o", perm, piSessionFileMode.Perm())
	}

	// A second installation at the same name is refused, and the file's bytes are the first installation's.
	if _, _, err := InstallPiSession(session, home); err == nil {
		t.Fatal("a taken name was installed over")
	} else if !strings.Contains(err.Error(), ErrTargetExists.Error()) {
		t.Errorf("error = %v, want it to report the taken name", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(session.Data) {
		t.Errorf("the installed bytes changed: %q (%v)", data, err)
	}
	// No temporary file is left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the target directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the target directory holds %d entries, want the one session: %v", len(entries), entries)
	}
}

// TestInstallPiSessionRefusesAFileSystemWithoutHardLinks pins the refusal: this installer's only
// mechanism that takes a name without replacing what is there is a hard link, so a filesystem without
// them must be answered with an error rather than published by a rename.
func TestInstallPiSessionRefusesAFileSystemWithoutHardLinks(t *testing.T) {
	original := linkSession
	linkSession = func(string, string) error { return os.ErrInvalid }
	defer func() { linkSession = original }()

	session := PiSession{
		SessionID: "22222222-2222-4222-8222-222222222222",
		FileName:  "2026-01-02T00-00-00-000Z_22222222-2222-4222-8222-222222222222.jsonl",
		Directory: "--home-user-project",
		Data:      []byte("{}\n"),
	}
	home := t.TempDir()
	if _, _, err := InstallPiSession(session, home); err == nil {
		t.Fatal("the installation was accepted on a filesystem without hard links")
	} else if !strings.Contains(err.Error(), ErrNoExclusiveInstall.Error()) {
		t.Errorf("error = %v, want it to name the missing exclusive mechanism", err)
	}
	if _, err := os.Stat(filepath.Join(home, piSessionsSubdir, "--home-user-project", session.FileName)); !os.IsNotExist(err) {
		t.Errorf("a refused installation published something: %v", err)
	}
}
