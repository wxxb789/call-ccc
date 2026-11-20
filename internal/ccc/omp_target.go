package ccc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Oh My Pi (OMP) is a Pi-family harness: its sessions are the same v3 tree of `id`/`parentId` entries
// the Pi writer already writes, and the inherited AgentsView reader parses both through one carried
// parser. What this file adds is the part of the target that is **OMP's own**, each line of it measured
// against the installed harness (18.18.7) and recorded in docs/evidence/0006-omp-continuation.md:
//
//   - the file opens with a fixed-width title slot line the harness rewrites in place when it titles a
//     session (measured: every session OMP itself writes starts with one, exactly 256 bytes including
//     its line feed, and a file without one still loads -- so the line is the format's own shape rather
//     than a requirement, and this route writes it empty because a title is the harness's own to make);
//   - every assistant record has to state a non-empty `model` and `provider` or the next turn fails
//     while the harness builds its request (measured: dropping either member from one record of an
//     otherwise loadable session answers `undefined is not an object (evaluating 'e.length')`);
//   - the session's own model is restored from a `model_change` record, and one the target home cannot
//     resolve in its place is refused (`SessionModelRestoreError`); the record must stand on the walk
//     the harness's loader makes from the file's leaf, or it is not read at all (measured: a binding
//     entry nothing chains to is ignored and the session falls back to the pairs its messages carry);
//   - the project directory a session is filed under is **not** Pi's spelling: OMP spells a working
//     directory under the temporary directory or under the home directory in a shortened form of its
//     own, and moves a session it finds under the Pi spelling into that directory itself.
//
// Everything else -- the entry ids, the tree relations the source recorded, the dialogue projection,
// the tool pairing, the compaction record's own semantics and every loss category -- is the family's
// shared work (pi_target.go, route_shared.go), which is why this route is a shape and a spelling rather
// than a second writer.

// piFamilyOMP is the family name OMP's own sentences in the report use, and the value the shared
// writer's family-aware wording switches on.
const piFamilyOMP = "omp"

// OMPFormatVersion is the OMP version this writer's format was verified against, and the version the
// report states when the installed binary could not be probed:
//
//	omp --version   →  omp/18.18.7
const OMPFormatVersion = "18.18.7"

// OMPTargetFormat names the on-disk format an OMP target session is written in: the session JSONL OMP
// keeps under `<agent dir>/sessions/<project spelling>/<timestamp>_<session id>.jsonl`, which is what
// its own loader resumes and what the inherited AgentsView OMP reader parses.
const OMPTargetFormat = "omp-session-jsonl"

// OMP session entry kinds this route writes that Pi's writer has no record for: the rewritable title
// slot and the session's own model binding. Both are OMP's own names, read from the harness's records.
const (
	piEntryTitle       = "title"
	piEntryModelChange = "model_change"
)

// ompTitleSlotBytes is the width of the title slot line, line feed included. Measured: the harness
// serialises the slot with `SESSION_TITLE_SLOT_BYTES = 256` and every one of the 300 sampled first lines
// of a real OMP home is exactly 256 bytes long, which is what lets the harness rewrite a title in place
// without moving the rest of the file.
const ompTitleSlotBytes = 256

// OMPTargetOptions configures one conversion to OMP. SessionID and CLIVersion are normally left empty:
// the identity must be fresh and the version is probed or pinned by the caller. Provider and Model are
// the *target home's* own answers, asked of the harness (the route's Facts step) -- never the source's.
type OMPTargetOptions struct {
	// SessionID overrides the fresh identity; a test uses it to pin the bytes. An empty value asks for
	// one, which is what a conversion needs.
	SessionID string
	// CLIVersion is the target version the document is written as, when the caller probed one.
	CLIVersion string
	// Now pins the instant the header, the title slot and the file name are derived from when the
	// document recorded no message time at all.
	Now time.Time
	// MaxToolNames bounds how many tool names the report's vocabulary category spells out.
	MaxToolNames int
	// Provider is the provider label the target home's own configuration resolves for this session, or
	// empty when the harness answered none. It is written on every assistant record (OMP requires one)
	// and into the session's model binding, and it is never the source's: the inherited read answers no
	// endpoint attribution for a message.
	Provider string
	// Model is the model id the target home's own configuration resolves for this session, or empty when
	// the harness answered none. It is written into the session's model binding, which is what a session
	// is restored from, and on any assistant record whose own source message recorded no model, because
	// the format requires a non-empty model there.
	Model string
	// ProviderDetail says what the harness answered about its own configured model, or why it was not
	// asked (a dry run) or answered nothing. It is carried into the report's target_model_binding
	// caveat, so the record's presence or absence is never left to be inferred from a count.
	ProviderDetail string
}

// piModelChange is the payload of the session-level model binding this route writes, in OMP's own field
// names: the model the harness resolved for itself, as one `<provider>/<id>` string. `Fallback` is
// written as a boolean because the harness's own records carry it; it is false here, which is what the
// harness's answer means -- the model it named is the one its configuration selects for a new session,
// not a fallback chosen because nothing was configured.
type piModelChange struct {
	Model    string
	Fallback bool
}

// MarshalJSON writes the binding in the harness's own field order: the model, then whether it was a
// fallback.
func (m piModelChange) MarshalJSON() ([]byte, error) {
	return orderedJSON{write: func(object *jsonObject) error {
		if err := object.field("model", m.Model); err != nil {
			return err
		}
		return object.field("resolvedModelIsFallback", m.Fallback)
	}}.MarshalJSON()
}

// piTitleSlotLine is the file's first line: OMP's rewritable title slot, holding an empty title and
// padded so the whole line -- line feed included -- is exactly ompTitleSlotBytes long, which is the
// shape and the width the harness's own writer produces and the one its in-place rewrite assumes.
//
// The title is empty because a session's title is the harness's own generation: OMP starts every
// session it writes with this exact record (an empty `title` and the rest of the width in `pad`) and
// fills it later, so a conversion writes the state the harness itself starts from rather than a title
// this build would have to invent. The record is the one line of the file that is not an entry of the
// conversation's tree: it carries no `id` and no `parentId`.
func piTitleSlotLine(timestamp string) []byte {
	withoutPad := fmt.Sprintf(`{"type":%q,"v":1,"title":"","updatedAt":%q,"pad":""}`, piEntryTitle, timestamp)
	// The line feed is part of the width the harness reserves, so the padding is what is left of the
	// slot after the record and that one byte. A timestamp of the family's own shape leaves room for a
	// long title; the clamp keeps a caller's exotic spelling from producing a line the harness could not
	// rewrite, and the test that pins the width is what holds the real shape to it.
	padding := ompTitleSlotBytes - 1 - len(withoutPad)
	if padding < 0 {
		padding = 0
	}
	line := fmt.Sprintf(`{"type":%q,"v":1,"title":"","updatedAt":%q,"pad":%q}`,
		piEntryTitle, timestamp, strings.Repeat(" ", padding))
	return []byte(line + "\n")
}

// ConvertToOMPSession turns a Conversation into an OMP session: a fresh identity, one entry per piece of
// evidence the family can express, OMP's own title slot and model binding around them, and the report of
// what it cannot carry. It writes no file; installation is the caller's step (InstallOMPSession).
//
// The source session's recorded working directory is required, for the family's reason: the session is
// filed under the project directory OMP's own spelling derives from it, so a session installed without
// one could never be found while a report called it installed. A document that records none is refused
// here (ErrNoTargetCwd), before anything is written.
func ConvertToOMPSession(conv Conversation, opts OMPTargetOptions) (PiSession, error) {
	if conv.selectionReduced() {
		// A document a selection produced is refused before anything else is read of it: this writer does
		// not consume the seam, and re-walking the window would re-derive the cross-record facts the seam
		// exists to carry (goal 0009's D6). The pair gate keeps the CLI from reaching this at all; an
		// embedding caller is told the same sentence.
		return PiSession{}, selectionUnsupportedWriterRefusal(TargetOMP)
	}
	return convertToPiFamilySession(conv, PiTargetOptions{
		SessionID:    opts.SessionID,
		CLIVersion:   opts.CLIVersion,
		Now:          opts.Now,
		MaxToolNames: opts.MaxToolNames,
	}, ompFamilyConversion(opts))
}

// ompFamilyConversion is OMP's own half of a family conversion: the pin, the spelling its installer
// files under, and the shape of the records OMP's format carries around the conversation.
func ompFamilyConversion(opts OMPTargetOptions) piFamilyConversion {
	provider := strings.TrimSpace(opts.Provider)
	model := strings.TrimSpace(opts.Model)
	binding := ""
	if provider != "" && model != "" {
		binding = provider + "/" + model
	}
	return piFamilyConversion{
		PinVersion: OMPFormatVersion,
		ProjectDir: OMPProjectDir,
		Shape: piFamilyShape{
			Name:                   piFamilyOMP,
			titleSlot:              true,
			assistantProvider:      provider,
			assistantModelFallback: model,
			sessionBinding:         binding,
			sessionBindingDetail:   ompBindingDetail(provider, model, opts.ProviderDetail),
		},
	}
}

// ompBindingDetail words what the report says about the session's own model record: which model the
// harness answered for itself and what that record is for, or -- when nothing was written -- why not.
// The sentence is built here because this is where the answer and its absence are both in hand, and the
// report must state the difference rather than leave a caller to infer it from a missing record.
func ompBindingDetail(provider, model, detail string) string {
	if provider != "" && model != "" {
		stated := fmt.Sprintf("the session records the model the target home's own configuration "+
			"resolves for a new session of it, `%s/%s`, because that is what the harness restores a "+
			"session's model from: a session that records none is restored from the pairs its own "+
			"records carry -- the source's, which this home need not define -- and is refused there "+
			"(`SessionModelRestoreError`, measured on 18.18.7). The binding is the target's answer, "+
			"never the source's", provider, model)
		if strings.TrimSpace(detail) != "" {
			stated += "; the harness answered: " + strings.TrimSpace(detail)
		}
		return stated
	}
	stated := "no session-level model record was written: the harness was not asked for its own " +
		"configured model, or answered none"
	if strings.TrimSpace(detail) != "" {
		stated += " (" + strings.TrimSpace(detail) + ")"
	}
	return stated + ". Without that record the harness restores this session's model from the pairs " +
		"its own records carry -- the source's model, which the target home need not define -- and " +
		"refuses the session on a home that does not know it (`SessionModelRestoreError`, measured on " +
		"18.18.7); a home with no model configured at all cannot start the harness, so no installed " +
		"session can be opened there whatever it records"
}

// OMPProjectDir spells a working directory the way OMP's own session discovery does
// (`getDefaultSessionDirPath`, whose name for a working directory the shipped 18.18.7 computes as
// follows):
//
//   - a working directory inside the OS temporary directory is spelled `-tmp` + its path relative to
//     that directory, with every separator and drive colon replaced by `-` (`/tmp/x/y` → `-tmp-x-y`);
//     this branch wins when both apply;
//   - otherwise a working directory inside the home directory is spelled `-` + its home-relative path
//     (`/home/user/repos/call-ccc` → `-repos-call-ccc` when the home is `/home/user`);
//   - otherwise the whole absolute path is used, with its leading separator removed and every
//     separator or colon replaced by `-`, wrapped in `--` on both sides -- which is Pi's spelling, and
//     the only case where the two harnesses agree.
//
// Measured against the installed harness: a session written for `/home/azureuser/repos/call-ccc` was
// filed under `-repos-call-ccc`, one for `/tmp/omp-probe/work` under `-tmp-omp-probe-work`, and one for
// a path outside both under the `--…--` spelling. A session filed under the *Pi* spelling is moved into
// this directory by the harness itself, so the installer writes this spelling and nothing else.
func OMPProjectDir(cwd string) string {
	home, _ := os.UserHomeDir()
	return ompProjectDirFor(cwd, home, os.TempDir())
}

// ompProjectDirFor is OMPProjectDir with the two directories whose answers the spelling depends on
// passed in, so the three branches can be exercised on one host (a test cannot make this machine's home
// or temporary directory be somewhere else, and a rule pinned only on the host it runs on is the shape
// docs/conventions.md forbids).
func ompProjectDirFor(cwd, home, tmp string) string {
	resolved := cwd
	if absolute, err := filepath.Abs(cwd); err == nil {
		resolved = absolute
	}
	if relative, ok := ompWithin(tmp, resolved); ok {
		return ompJoinSpelling("-tmp", relative)
	}
	if relative, ok := ompWithin(home, resolved); ok {
		return ompJoinSpelling("-", relative)
	}
	trimmed := strings.TrimLeft(resolved, `/\`)
	return "--" + ompSeparatorSpelling(trimmed) + "--"
}

// ompWithin answers a path's spelling relative to one base directory and whether the path is inside it
// at all, which is the test the harness's own spelling makes: an empty relative path (the path *is* the
// base), and a relative path that neither starts with `..` nor is absolute, are inside; anything else is
// not. A base and a path on different volumes (Windows) have no relative spelling at all and are the
// same answer: not inside.
func ompWithin(base, path string) (string, bool) {
	if strings.TrimSpace(base) == "" {
		return "", false
	}
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return "", false
	}
	if relative == "" || relative == "." {
		return "", true
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	if filepath.IsAbs(relative) {
		return "", false
	}
	return relative, true
}

// ompJoinSpelling joins a spelling's own prefix to a relative path the way the harness does: the
// relative path spelled with `-`, and the prefix either already ending in one or given one before it.
func ompJoinSpelling(prefix, relative string) string {
	spelled := ompSeparatorSpelling(relative)
	if spelled == "" {
		return prefix
	}
	if strings.HasSuffix(prefix, "-") {
		return prefix + spelled
	}
	return prefix + "-" + spelled
}

// ompSeparatorSpelling replaces every separator and drive colon with `-`, which is what each branch of
// the harness's spelling does before it joins the pieces.
func ompSeparatorSpelling(path string) string {
	return strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(path)
}

// ompFormatLimits is what this pair of formats cannot express at all, as opposed to what one run
// happened to leave out: a limit carries no count, because it holds for every conversion. The family's
// shared limits apply with OMP's own harness named in their sentences, and OMP's own records add two --
// the title slot and the model binding its loader restores a session from. Pi's `model_provider_binding`
// limit is deliberately not among them: it says a session binds no provider, which an OMP session does.
func ompFormatLimits() []FormatLimit {
	return append(piFormatLimits(piFamilyShape{Name: piFamilyOMP}),
		FormatLimit{
			Category: "title_slot",
			Detail: "the target's session file opens with a fixed-width title slot the harness rewrites " +
				"in place when it titles a session. A conversion writes it with an empty title (the " +
				"harness's own initial state); a title is the harness's own to generate, and the source's " +
				"own title -- where the read answers one at all -- has no field in the family's formats " +
				"that a converted session could carry it in",
		},
		FormatLimit{
			Category: "session_model_binding",
			Detail: "an OMP session records the model it runs under, and its loader restores that model " +
				"when it opens the session: a record whose model the target home cannot resolve is " +
				"refused (`SessionModelRestoreError`). A converted session therefore records the " +
				"*target home's* own configured model, asked of the harness -- never the source's, which " +
				"this home need not define -- and a home whose configuration changes so that this model " +
				"disappears refuses the session exactly as it would refuse one of its own",
		},
	)
}
