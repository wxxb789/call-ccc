package ccc

import (
	"context"
	"os"
	"path/filepath"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/parser"
)

// ompSessionsSubdir is the subdirectory of an OMP data directory that holds its sessions. It is the
// same name Pi's own tree uses, which is the one thing about the layout the two harnesses share: the
// *project directory below it* is where they differ (OMPProjectDir).
const ompSessionsSubdir = "sessions"

// ompHomeParts is the data directory OMP reads when the caller names none: `<home>/.omp/agent`. Pi's is
// `<home>/.pi/agent`, so the two never see each other's sessions.
var ompHomeParts = []string{".omp", "agent"}

// ompSessionFileMode is the mode the installer asks the OS to give a session file it writes: readable
// and writable by its owner, by nobody else. It is named for the same reason the other installers'
// modes are: the *request* is the checkable half of the rule on a host without POSIX mode bits.
const ompSessionFileMode os.FileMode = 0o600

// ResolveOMPHome answers the OMP data directory a conversion installs into: the caller's explicit
// directory, else $PI_CODING_AGENT_DIR, else ~/.omp/agent. The boolean reports whether the answer came
// from the caller, which is what decides whether a missing directory may be created.
//
// The environment variable is the *same name* Pi uses -- measured: the installed harness reads its data
// directory from `PI_CODING_AGENT_DIR` (its own help page says so, and a scratch directory set through
// it holds the sessions the harness writes). That is OMP's own answer for the harness, not an
// assumption borrowed from Pi, and it is why a conversion run with that variable set writes into the
// directory it names whichever of the two targets it was asked for.
//
// A value is read to answer two things, and only the first of them trims: trimming decides whether a
// directory was named at all, and the value is then resolved exactly as it was written, because a
// directory name may begin or end with a space.
func ResolveOMPHome(explicit string) (string, bool, error) {
	return resolveRouteHome(explicit, ompHomeEnv, ompHomeParts...)
}

// InstallOMPSession writes a converted session into an OMP data directory and returns the path it
// occupies, plus the mechanism that put it there.
//
// The path is `<home>/sessions/<OMPProjectDir(cwd)>/<file name>`: OMP's own lookup reads the sessions
// tree of the data directory it was told about, one project directory below it, exactly as Pi's does
// -- but the project directory is OMP's own spelling, and a session filed under Pi's spelling is moved
// into OMP's by the harness itself (measured), which would leave the file somewhere other than the path
// this call reports.
//
// Nothing existing is ever replaced: the bytes go to a sibling temporary file first and the final name
// is taken with a hard link (atomic and exclusive where the filesystem has them); a filesystem without
// hard links is refused (ErrNoExclusiveInstall) rather than published by a rename, which would replace
// whatever is at the destination.
func InstallOMPSession(session PiSession, home string) (string, string, error) {
	return installFile(home, ompSessionsSubdir, session.Directory, session.FileName,
		session.Data, ompSessionFileMode)
}

// VerifyOMPSessionStructure reads the installed session back through the inherited AgentsView OMP
// reader and compares it, item by item, with what the writer intended to carry. sessionsDir is the
// target's sessions directory; cfg supplies the provider configuration (its OMP roots are replaced,
// never mutated: the map is copied).
//
// The read-back is the file this call names, read through the inherited reader's own path reference, so
// the check compares the file the conversion published (or the dry run's copy of the same bytes) rather
// than resolving an id through the target's session tree.
//
// The comparison itself is the Pi route's (comparePiRoundTrip), because the two formats are one family:
// the same reader, the same entry shape, the same records. The reader keeps an OMP session's title slot
// line as the session's title and drops the model binding the way it drops Pi's own setting records, so
// neither is a difference in what the file's conversation holds.
func VerifyOMPSessionStructure(
	ctx context.Context, sessionsDir string, session PiSession, cfg config.Config,
) (StructureVerification, error) {
	return verifyStructure(ctx, sessionsDir, parser.AgentOMP, structureMethodOMP,
		structureReaderOMP, structureReaderOMP,
		filepath.Join(sessionsDir, session.Directory, session.FileName),
		os.ReadFile,
		func(readBack Conversation, installed []byte) roundTripComparison {
			return comparePiRoundTrip(session, readBack, installed)
		}, cfg)
}
