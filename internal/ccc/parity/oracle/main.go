// Command oracle writes the canonical parse → archive → read dump for one fixture.
//
// The same source file is used in two places: this tree, and a pristine checkout of the pinned
// AgentsView revision. scripts/upstream-compare.py runs both and diffs the output, which is the
// contract's comparative evidence that the inherited source side behaves identically. It is also
// how the committed dump under internal/ccc/parity/testdata/oracle was recorded, so the offline test
// compares this tree against bytes a pristine upstream produced.
//
//	go run ./internal/ccc/parity/oracle -fixtures DIR -work-dir DIR -provider claude
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.kenn.io/agentsview/internal/ccc/parity"
)

func main() {
	fixtures := flag.String("fixtures", "", "directory holding one subdirectory per provider")
	workDir := flag.String("work-dir", "", "isolated directory the archive is written into")
	provider := flag.String("provider", "", "fixture to compare")
	fixture := flag.String("fixture", "", "the session file to read, relative to the provider's root")
	out := flag.String("out", "-", "file to write the dump to, or - for stdout")
	flag.Parse()

	if *fixtures == "" || *provider == "" {
		fmt.Fprintln(os.Stderr, "oracle: -fixtures and -provider are required")
		os.Exit(2)
	}
	dir := *workDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "ccc-parity-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "oracle:", err)
			os.Exit(1)
		}
		defer os.RemoveAll(dir)
	}
	dump, err := parity.Run(context.Background(), parity.Options{
		Fixtures: *fixtures,
		WorkDir:  dir,
		Provider: *provider,
		Fixture:  *fixture,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(1)
	}
	encoded, err := parity.Marshal(dump)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(1)
	}
	if *out == "-" {
		os.Stdout.Write(encoded)
		return
	}
	if err := writeFile(*out, encoded); err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(1)
	}
}

// writeFile writes the dump where the caller asked, creating the parent directory: the comparison
// harness writes each side's dump into a scratch tree, not into the repository.
func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
