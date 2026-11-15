package parity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRoot is the committed fixture tree: one directory per provider, each of which is that
// provider's source root.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "testdata", "fixtures"))
	if err != nil {
		t.Fatalf("resolving the fixture root: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture root %s: %v", root, err)
	}
	return root
}

func recordPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "oracle", name+".json")
}

// TestDumpMatchesTheRecordedUpstreamAgreement is the contract's comparative evidence, offline: the
// canonical dump this tree produces must equal the dump a pristine checkout at the pinned revision
// produced for the same fixture bytes. The committed files were recorded by
//
//	python3 scripts/upstream-compare.py --record
//
// which runs this very program in both trees and refuses to record unless the two agree byte for
// byte, so this test re-checks that agreement on every run of the ordinary test suite.
func TestDumpMatchesTheRecordedUpstreamAgreement(t *testing.T) {
	for _, testCase := range Cases {
		t.Run(testCase.Provider, func(t *testing.T) {
			recorded, err := os.ReadFile(recordPath(t, testCase.Provider))
			if err != nil {
				t.Fatalf("reading the recorded dump: %v", err)
			}
			dump, err := Run(context.Background(), Options{
				Fixtures: fixtureRoot(t),
				WorkDir:  t.TempDir(),
				Provider: testCase.Provider,
				Fixture:  testCase.Fixture,
			})
			if err != nil {
				t.Fatalf("running the oracle: %v", err)
			}
			encoded, err := Marshal(dump)
			if err != nil {
				t.Fatalf("marshalling the dump: %v", err)
			}
			if string(encoded) != string(recorded) {
				t.Errorf("the dump for %s differs from the recorded upstream dump; the trees disagree "+
					"or the record is stale (re-run scripts/upstream-compare.py --record with the "+
					"pinned checkout)\n--- recorded: %d bytes\n--- now:      %d bytes",
					testCase.Provider, len(recorded), len(encoded))
				reportFirstDifference(t, string(recorded), string(encoded))
			}
			if dump.Tool != ToolID {
				t.Errorf("dump tool id = %q, want %q", dump.Tool, ToolID)
			}
		})
	}
}

// TestDumpNamesTheFixtureBytesItRead pins that the dump is tied to the fixture content, so a fixture
// edit cannot leave a record that silently describes different bytes.
func TestDumpNamesTheFixtureBytesItRead(t *testing.T) {
	root := fixtureRoot(t)
	testCase := Cases[0]
	dump, err := Run(context.Background(), Options{
		Fixtures: root,
		WorkDir:  t.TempDir(),
		Provider: testCase.Provider,
		Fixture:  testCase.Fixture,
	})
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}
	fixture := filepath.Join(root, "claude", filepath.FromSlash(dump.Fixture.RelativeToRoot))
	content, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading the fixture the dump names: %v", err)
	}
	if got := digest(content); got != dump.Fixture.SHA256 {
		t.Errorf("fixture sha256 = %s, dump says %s", got, dump.Fixture.SHA256)
	}
	if dump.Fixture.Bytes != len(content) {
		t.Errorf("fixture size = %d, dump says %d", len(content), dump.Fixture.Bytes)
	}
	if dump.Fixture.RelativeToRoot == "" || filepath.IsAbs(dump.Fixture.RelativeToRoot) {
		t.Errorf("the dump names an absolute fixture path: %q", dump.Fixture.RelativeToRoot)
	}
}

// TestDumpCarriesNoHostSpecificValues pins the exclusions the dump claims: it must not leak this
// checkout's location, the user's home directory, or the temporary work directory, or two trees could
// never agree on it.
func TestDumpCarriesNoHostSpecificValues(t *testing.T) {
	workDir := t.TempDir()
	dump, err := Run(context.Background(), Options{
		Fixtures: fixtureRoot(t),
		WorkDir:  workDir,
		Provider: Cases[1].Provider,
		Fixture:  Cases[1].Fixture,
	})
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}
	encoded, err := Marshal(dump)
	if err != nil {
		t.Fatalf("marshalling the dump: %v", err)
	}
	needles := []string{workDir, fixtureRoot(t)}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		needles = append(needles, home)
	}
	if repo, err := filepath.Abs(filepath.Join("..", "..", "..")); err == nil {
		needles = append(needles, repo)
	}
	for _, needle := range needles {
		if strings.Contains(string(encoded), needle) {
			t.Errorf("the dump contains a host-specific path (%q), which two checkouts can never share", needle)
		}
	}
	if len(dump.ExcludedFields) == 0 {
		t.Error("the dump does not name the fields it excludes")
	}
}

// reportFirstDifference prints the first differing line, which is what a reader needs to tell a
// stale record from a behavioural difference.
func reportFirstDifference(t *testing.T, recorded, current string) {
	t.Helper()
	recordedLines := strings.Split(recorded, "\n")
	currentLines := strings.Split(current, "\n")
	for index := 0; index < len(recordedLines) && index < len(currentLines); index++ {
		if recordedLines[index] != currentLines[index] {
			t.Logf("first difference at line %d:\n  recorded: %s\n  now:      %s",
				index+1, recordedLines[index], currentLines[index])
			return
		}
	}
	t.Logf("the files share a prefix; recorded %d line(s), now %d line(s)",
		len(recordedLines), len(currentLines))
}
