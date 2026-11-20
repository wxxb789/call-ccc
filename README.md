# call/ccc

**call-with-current-conversation-continuation**

A focused CLI for continuing coding-agent conversations across native session formats.

Today the repository carries the **actual AgentsView source tree** at a pinned revision
([`docs/upstream.md`](docs/upstream.md)) and ships three working capabilities on top of it: inspecting
an existing agent session through AgentsView's own discovery, parsers and archive read path, and
continuing a session between harnesses — a Claude Code session as a new, natively resumable **Codex**,
**Claude Code**, **Pi**, **Oh My Pi** or **OpenCode** session, a Codex session as a Claude Code one, a
**Pi** or **Oh My Pi** session as a Claude Code, Codex or Pi one, and an **OpenCode** session — a row
inside that harness's own SQLite store, read through the inherited reader — as a Codex or Claude Code
one.

```bash
go build -tags fts5 -o bin/ccc ./cmd/ccc   # CGO_ENABLED=1; -tags fts5 is what upstream ships

bin/ccc inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
bin/ccc inspect codex:/path/to/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl --json

bin/ccc continue codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 --to claude
bin/ccc continue pi:01a1c335-7098-71c2-a91b-b5295c0464f5 --to claude      # Pi → Claude Code
bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to pi    # Claude Code → Pi
bin/ccc continue pi:01a1c335-7098-71c2-a91b-b5295c0464f5 --to omp         # Pi → Oh My Pi
bin/ccc continue omp:01a1c338-9e4f-7c21-8f3d-2b6a05e1d7a4 --to claude     # Oh My Pi → Claude Code
bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to opencode # Claude Code → OpenCode
bin/ccc continue opencode:ses_5f2a1c47d9e34b6a8f0c1d2e3b4a5c6d --to codex # OpenCode → Codex
# resume the Claude session this installed, from the directory its loader discovers it in:
cd <session working directory> && CLAUDE_CONFIG_DIR=<installed home> claude --resume <new session id>
# and the Pi one, from the directory that harness resolves a session id through:
cd <session working directory> && PI_CODING_AGENT_DIR=<installed home> pi --session <new session id>
# and the Oh My Pi one, whose printed line names both its data directory and the directory holding the file:
cd <session working directory> && PI_CODING_AGENT_DIR=<installed home> \
  PI_CODING_AGENT_SESSION_DIR=<installed home>/sessions/<project dir> omp --resume <new session id>
# and the OpenCode one, whose printed line names the data root it reads and pins the store's own
# database inside it (a printed line can set a variable, never remove an inherited one):
cd <session working directory> && \
  OPENCODE_DB=<installed data root>/opencode/opencode.db XDG_DATA_HOME=<installed data root> \
  opencode run --standalone --session <new session id>
```

The harnesses whose resume line carries a working directory are named above, and each one's lookup is
scoped that way: measured on Claude Code 2.1.326, a session id answers `No conversation found with
session ID: …` from any directory whose project spelling differs; Pi resolves an id across its sessions
root but *offers to fork* a session it finds only under another project rather than resuming it in
place; and Oh My Pi resolves an id from any directory but refuses a session whose *recorded* working
directory it cannot find, and files sessions under its own spelling of it (measured: a directory under
the temporary tree becomes `-tmp…`, one under the home `-…`, and only a path outside both keeps Pi's
`--…--` form), and OpenCode resolves a session from the directory its own document records (measured on
2.2.5: `opencode session list --format json` names the session from that directory and lists nothing for
it from another). The `cd` a printed line carries is therefore about the session rather than only about the
lookup: where the harness resolves an id from the directory it is run in, the `cd` is what makes the
pasted line find this session, and for all four of them the continued conversation runs in that
directory — while the Oh My Pi line additionally names the directory that holds the file, because naming
the `sessions` root above it answers that no such session is installed.

`ccc inspect` locates a session with the provider's own reader, parses it, and reports its identity,
agent, source, conversation shape (counts, tool calls, compaction boundaries, malformed lines) and —
when a data directory holds one — what the SQLite archive holds for the same session. It never writes:
the source transcript and any archive are read only. A source is found through the provider's own
configured root — the environment variable that names it (`CLAUDE_PROJECTS_DIR`,
`CODEX_SESSIONS_DIR`, `PI_DIR`, `OPENCODE_DIR`, …), else the same key in the AgentsView configuration
(`claude_project_dirs`, …), else that provider's default directory. An **OpenCode source** is a row
inside that harness's own SQLite store (`<root>/opencode.db`) rather than a file, and it is read through
the inherited reader like every other source — this build adds no SQL reader and never writes into that
store. Two consequences the report states rather than hides: the *boundary* block is `not-taken` for a
member of a store — the container's bytes hold every member, and the harness's own writes move them with
its WAL handling — so the report answers `source_check_state: not-verified` for it and never claims the
database file was re-measured; what shows the source's own *records* unchanged is the direction's native
check, which reads them back through the harness's own reader around the run
(`scripts/opencode-native-check.py --direction opencode-to-codex`). And a source record the reader renders
rather than quotes (a persisted reasoning trace it folds into the record's text as
`[Thinking]…[/Thinking]`) travels as that rendering with the caveat counted (`unprojected_dialogue`).

`ccc continue` reads that session through the same inherited reader, writes it as a **new** session of
the requested target (a Codex rollout under the target's own `sessions/` tree with `--to codex`, a
Claude Code transcript under `<config>/projects/<project-dir>/<session-id>.jsonl` with `--to claude`,
a Pi session under `<agent dir>/sessions/--<working directory spelling>--/<timestamp>_<id>.jsonl` with
`--to pi`, an Oh My Pi session under `<agent dir>/sessions/<Oh My Pi's own spelling of the working
directory>/<timestamp>_<id>.jsonl` with `--to omp` — the same family's second format, whose spelling is
its own and whose first line is the harness's fixed-width title slot, and with `--to opencode` a session
inside OpenCode's own SQLite store (`<XDG data home>/opencode/opencode.db`), written **through that
harness's own importer** (`opencode session import … --standalone`) rather than by SQL from this build,
so that target's installation needs its binary),
installs it without replacing anything,
and reports three separate states: what was converted (with every omission or narrowed meaning named
by category), whether the session is installed, and whether **the target harness itself** loaded it
(Codex's `codex app-server` `thread/resume`, Claude Code's own loader for `--to claude`, the Pi
family's own loaders in their RPC modes for `--to pi` and `--to omp`, and OpenCode's own
`session export` — the reader the harness resumes from, asked for the installed session with an
unknown-id control beside it, never `run`, which *creates* a session for an id the store does not hold)
or
continued from it (`--verify resume --resume-prompt TEXT`). It prints the target's own session id and
the command that resumes it — the data directory it installed into (`CODEX_HOME=…`, or for the harnesses
whose resume line also changes into the session's working directory `CLAUDE_CONFIG_DIR=…` /
`PI_CODING_AGENT_DIR=…`, the OpenCode data root together with the store's own database file inside it
(`XDG_DATA_HOME=… OPENCODE_DB=…`), and for Oh My Pi the directory
holding the session's file named beside that
(`PI_CODING_AGENT_SESSION_DIR=…`):
`cd <cwd> && CLAUDE_CONFIG_DIR=<home> claude --resume <id>`,
`cd <cwd> && PI_CODING_AGENT_DIR=<home> pi --session <id>`,
`cd <cwd> && PI_CODING_AGENT_DIR=<home> PI_CODING_AGENT_SESSION_DIR=<home>/sessions/<project dir> omp --resume <id>`,
`cd <cwd> && OPENCODE_DB=<data root>/opencode/opencode.db XDG_DATA_HOME=<data root> opencode run --standalone --session <id>`) plus the harness and the session id,
so the pasted command opens the home holding the session rather than the caller's default one — even
where the caller's own environment already names another store, because the printed line names the
variables a shell can set (a printed line cannot remove an inherited one) — and
`--json` reports all of it as one object, the argument vector and environment included. `--dry-run`
converts and verifies without writing anything, `--target-home DIR` keeps a demo or a test away from a
real Codex or Claude Code home, and `--target-binary PATH` names the harness executable when it is not
the target's default name (`codex`, `claude`, `pi`, `omp`, `opencode`) on `PATH` (for `--to opencode`
this is not optional in effect: the harness's own importer *is* the installation); the Claude Code, Pi
and Oh My Pi and OpenCode resume commands name the path the probe found — the program the conversion
asked and the verification ran — or the spelling given here when it found none, while the Codex commands
keep the spelling given here (or `codex`), because only the invocations that change into the session's
working directory need a path that cannot be re-resolved from there.

A conversion is a snapshot of the source at one moment, and it says where that moment is: the result
names the boundary it covers — the source file's `sha256` and size, whether the file is still the size
the read's own snapshot recorded (a size match, not a byte-identity check), and, for a transcript whose
records are lines, where its line-feed-terminated records end — so two continuations of one session that
kept growing can be told apart and the newer one chosen.
Converting the same source again after it has grown is the ordinary way to carry the rest: it reads the
source as it now is and writes **another** new session beside the earlier one — never a merge into a
session an agent may already have continued, and never a rewrite of the source. A read whose own snapshot
recorded a different size than the file has says so (`source.boundary.read_coverage: size-mismatch`)
instead of attributing the conversation to bytes it is not shown to have read. A file whose last bytes hold
no line feed is reported as what it is — a record the source had not terminated, which it may still be writing
(`boundary.unterminated_tail_bytes`, a warning, and a `records:` line in the human block) — and whether
the read carried it is left to the read's own answer rather than claimed either way; converting again once
the file is written reads it again.

`--select ORDINALS` makes the carried window explicit: with
`ccc continue claude:<id> --to codex --select 0-4,9` the read keeps the records whose own ordinal values
the specification names and the conversion writes that window as a fresh target session — the report's
`selection.ordinal_space` block states the space the ordinals are values in, and an ordinal outside it,
an inverted or duplicated region and a non-numeric value are refused by name with the usage exit code
before anything is installed. A window that would separate a recorded call from its recorded result
carries the counterpart as a declared necessity (the report's `selection.necessities_added`) — but only
where the counterpart stands on the line the read's own last record answers and the route's own writer can
state it truthfully; where it does not, the whole selection is refused by name (the pair and both
ordinals) rather than activating a branch the source's conversation left or leaving a declared counterpart
unpublished — and
`--strict-selection` refuses a separated window by name instead. The report (human and `--json`) names the
transformation per class — included, excluded, necessities added, and the calls the source itself left
undischarged (with the part of them whose records did not travel) — and accounts for every kept record by
its ordinal and the items it became. The option is
a capability of the **reference pair alone** (a Claude Code source → a Codex target); every other pair,
and `--strict-selection` without `--select`, is refused by name before the source is opened, and the
strict flag counts by its **presence**: an explicit `--strict-selection=false` is still the flag. A run
without the flag is byte-identical to the same run before the option existed.

The continuation engine itself lives in [`internal/ccc`](internal/ccc) and is callable without the
CLI: `SourceReader` reads a native session — an OpenCode one included — into a typed, transient
Conversation document,
`Archive`/`Ingestor` cover the inherited read and write paths, `targets.go` routes `Continue` to the
target writer, `ConvertToCodexRollout`/`ConvertToClaudeTranscript`/`ConvertToPiSession`/`ConvertToOMPSession`/
`ConvertToOpenCodeSession`
convert into the five target formats (a Codex rollout, a Claude Code transcript, the Pi family's two: a
Pi session and an Oh My Pi session, and OpenCode's own import document), and `Inspect` is what the
`inspect` command calls. Further targets
(GitHub Copilot CLI, …) arrive with the goals that specify them.

- Design and scope: [`docs/core-design/README.md`](docs/core-design/README.md)
- Outcome contracts: [`docs/goals/README.md`](docs/goals/README.md)
- What is carried, and how it is proven unchanged: [`docs/upstream.md`](docs/upstream.md)
- Gate, knobs and evidence rules: [`docs/harness.md`](docs/harness.md), [`docs/conventions.md`](docs/conventions.md)

Inherited AgentsView code stays under upstream's MIT licence (see [`LICENSE`](LICENSE), carried with
it); the code this project adds is under the same licence, with the upstream copyright notice
preserved.
