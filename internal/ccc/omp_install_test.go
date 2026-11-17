package ccc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestInstallOMPSessionFilesUnderOMPsOwnSpelling pins the OMP-specific installation rule on a real host:
// the session is filed under the project directory *OMP's* spelling computes -- which for a working
// directory under this machine's temporary directory is the shortened `-tmp…` form -- and not under Pi's
// `--…--` spelling. Measured: a session filed under Pi's spelling is moved into OMP's directory by the
// harness itself, so the conversion's own report would name a path the file no longer occupies.
func TestInstallOMPSessionFilesUnderOMPsOwnSpelling(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	session := PiSession{
		SessionID: "11111111-1111-4111-8111-111111111111",
		FileName:  "2026-01-02T00-00-00-000Z_11111111-1111-4111-8111-111111111111.jsonl",
		Directory: OMPProjectDir(cwd),
		Cwd:       cwd,
		Data:      []byte("{\"type\":\"session\",\"version\":3,\"id\":\"11111111-1111-4111-8111-111111111111\"}\n"),
	}
	path, mechanism, err := InstallOMPSession(session, home)
	if err != nil {
		t.Fatalf("installing: %v", err)
	}
	want := filepath.Join(home, ompSessionsSubdir, session.Directory, session.FileName)
	if path != want {
		t.Errorf("installed at %q, want %q", path, want)
	}
	if mechanism != "hard-link" {
		t.Errorf("mechanism = %q, want the exclusive hard link", mechanism)
	}
	if got := filepath.Base(filepath.Dir(path)); got != ompProjectDirFor(cwd, mustHomeDir(t), os.TempDir()) {
		t.Errorf("the session is filed under %q, want OMP's own spelling of the working directory", got)
	}
	// The whole point of the rule: on this host the two harnesses spell a temporary working directory
	// differently, so this conversion cannot be the Pi route's directory with another name.
	piPath := filepath.Join(home, piSessionsSubdir, PiProjectDir(cwd), session.FileName)
	if filepath.Dir(piPath) == filepath.Dir(path) {
		t.Errorf("OMP's project directory and Pi's are the same here (%q), and this host is one where the "+
			"installed harness's own rule gives two different names for a directory under the temporary "+
			"tree", filepath.Dir(path))
	}
	if _, err := os.Stat(piPath); !os.IsNotExist(err) {
		t.Errorf("the installer also filed the session under Pi's spelling (%s)", piPath)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the installed session is not there: %v", err)
	}
	if runtime.GOOS == "windows" {
		if ompSessionFileMode.Perm() != 0o600 {
			t.Errorf("the installer asks for mode %v, want 0600 (Windows reports no POSIX mode bits, "+
				"so the request is the half of the rule that is checkable here)", ompSessionFileMode)
		}
	} else if perm := info.Mode().Perm(); perm != ompSessionFileMode.Perm() || perm&0o077 != 0 {
		t.Errorf("the installed session's permissions are %v, want %o", perm, ompSessionFileMode.Perm())
	}

	// A second installation at the same name is refused, and the file's bytes are the first one's.
	if _, _, err := InstallOMPSession(session, home); err == nil {
		t.Fatal("a taken name was installed over")
	} else if !strings.Contains(err.Error(), ErrTargetExists.Error()) {
		t.Errorf("error = %v, want it to report the taken name", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(session.Data) {
		t.Errorf("the installed bytes changed: %q (%v)", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("reading the target directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the target directory holds %d entries, want the one session: %v", len(entries), entries)
	}
}

// mustHomeDir answers the home directory the spelling depends on, failing the case if the host cannot name
// one: the rule's home branch is exercised by the table-driven case in omp_target_test.go, and here the
// host's own answer is what the installed directory is compared against.
func mustHomeDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("this host names no home directory: %v", err)
	}
	return home
}

// TestInstallOMPSessionRefusesAFileSystemWithoutHardLinks pins the refusal: this installer's only
// mechanism that takes a name without replacing what is there is a hard link, so a filesystem without
// them must be answered with an error rather than published by a rename.
func TestInstallOMPSessionRefusesAFileSystemWithoutHardLinks(t *testing.T) {
	original := linkSession
	linkSession = func(string, string) error { return os.ErrInvalid }
	defer func() { linkSession = original }()

	session := PiSession{
		SessionID: "22222222-2222-4222-8222-222222222222",
		FileName:  "2026-01-02T00-00-00-000Z_22222222-2222-4222-8222-222222222222.jsonl",
		Directory: "-tmp-project",
		Data:      []byte("{}\n"),
	}
	home := t.TempDir()
	if _, _, err := InstallOMPSession(session, home); err == nil {
		t.Fatal("the installation was accepted on a filesystem without hard links")
	} else if !strings.Contains(err.Error(), ErrNoExclusiveInstall.Error()) {
		t.Errorf("error = %v, want it to name the missing exclusive mechanism", err)
	}
	if _, err := os.Stat(filepath.Join(home, ompSessionsSubdir, "-tmp-project", session.FileName)); !os.IsNotExist(err) {
		t.Errorf("a refused installation published something: %v", err)
	}
}

// TestResolveOMPHome pins where a conversion installs: the caller's own directory, else the variable the
// harness itself reads ($PI_CODING_AGENT_DIR — the same name Pi reads, which is OMP's own answer and not
// an assumption borrowed from Pi), else its default under the user's home. Only a directory the caller
// named may be created.
func TestResolveOMPHome(t *testing.T) {
	t.Setenv(ompHomeEnv, "")
	home := t.TempDir()
	defaultHome := filepath.Join(home, ".omp", "agent")

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(ompHomeEnv, "")

	explicit, fromCaller, err := ResolveOMPHome(filepath.Join(home, "explicit"))
	if err != nil || explicit != filepath.Join(home, "explicit") || !fromCaller {
		t.Errorf("explicit = (%q, %v, %v), want the caller's own directory", explicit, fromCaller, err)
	}
	t.Setenv(ompHomeEnv, filepath.Join(home, "from-env"))
	fromEnv, fromCaller, err := ResolveOMPHome("")
	if err != nil || fromEnv != filepath.Join(home, "from-env") || fromCaller {
		t.Errorf("from env = (%q, %v, %v), want the variable's directory and not the caller's",
			fromEnv, fromCaller, err)
	}
	t.Setenv(ompHomeEnv, "")
	byDefault, fromCaller, err := ResolveOMPHome("")
	if err != nil || fromCaller {
		t.Errorf("default = (%q, %v, %v), want a directory this run did not create", byDefault, fromCaller, err)
	}
	if !strings.HasSuffix(byDefault, filepath.Join(".omp", "agent")) {
		t.Errorf("default = %q, want it under the user's home as .omp/agent", byDefault)
	}
	if byDefault == defaultHome && err != nil {
		t.Errorf("default home %q should not be an error", defaultHome)
	}
}

// TestVerifyOMPSessionStructureReadsTheInstalledSessionBack pins the structural half for this route: the
// file the conversion published is read through the inherited AgentsView OMP reader, its own items are
// compared with what the writer placed in it, and the verification names that reader rather than Pi's --
// the two formats are one family, but the report has to say which reader answered.
func TestVerifyOMPSessionStructureReadsTheInstalledSessionBack(t *testing.T) {
	for _, fixture := range []struct {
		name string
		ref  Ref
		want ReadBackCounts
	}{
		{"the ordinary session", ompFixtureRef,
			ReadBackCounts{Messages: 5, UserMessages: 2, AssistantMessages: 3, ToolCalls: 1, ToolResults: 1}},
		{"the compacted session with its own customs", ompHistoryFixtureRef,
			ReadBackCounts{Messages: 6, UserMessages: 2, AssistantMessages: 4, CompactionBoundaries: 1}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			conversation := readFixtureConversation(t, fixture.ref)
			session, err := ConvertToOMPSession(conversation, ompFixtureTargetOptions())
			if err != nil {
				t.Fatalf("converting: %v", err)
			}
			home := t.TempDir()
			path, _, err := InstallOMPSession(session, home)
			if err != nil {
				t.Fatalf("installing: %v", err)
			}
			verification, err := VerifyOMPSessionStructure(context.Background(),
				filepath.Join(home, ompSessionsSubdir), session, fixtureConfig(t))
			if err != nil {
				t.Fatalf("verifying: %v", err)
			}
			if verification.State != VerificationStateVerified {
				t.Fatalf("structure = %+v, want the installed session read back as written", verification)
			}
			if !strings.Contains(verification.Method, "OMP") {
				t.Errorf("method = %q, want the OMP reader named", verification.Method)
			}
			if verification.TargetPath != path {
				t.Errorf("target path = %q, want the file the conversion published (%q)",
					verification.TargetPath, path)
			}
			want := fixture.want
			want.SessionID = "omp:" + session.SessionID
			if verification.ReadBack != want {
				t.Errorf("read_back = %+v, want %+v", verification.ReadBack, want)
			}
		})
	}
}
