# call/ccc

**call-with-current-conversation-continuation**

A focused CLI for continuing coding-agent conversations across native session formats.

Today the repository carries the **actual AgentsView source tree** at a pinned revision
([`docs/upstream.md`](docs/upstream.md)) and ships three working capabilities on top of it: inspecting
an existing agent session through AgentsView's own discovery, parsers and archive read path, and
continuing a session in either direction — a Claude Code session as a new, natively resumable **Codex**
session, and a Codex session as a new, natively resumable **Claude Code** session.

```bash
go build -tags fts5 -o bin/ccc ./cmd/ccc   # CGO_ENABLED=1; -tags fts5 is what upstream ships

bin/ccc inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
bin/ccc inspect codex:/path/to/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl --json

bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex
bin/ccc continue codex:0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812 --to claude
# resume the Claude session this installed, from the directory its loader discovers it in:
cd <session working directory> && CLAUDE_CONFIG_DIR=<installed home> claude --resume <new session id>
```

`ccc inspect` locates a session with the provider's own reader, parses it, and reports its identity,
agent, source, conversation shape (counts, tool calls, compaction boundaries, malformed lines) and —
when a data directory holds one — what the SQLite archive holds for the same session. It never writes:
the source transcript and any archive are read only.

`ccc continue` reads that session through the same inherited reader, writes it as a **new** session of
the requested target (a Codex rollout under the target's own `sessions/` tree with `--to codex`, a
Claude Code transcript under `<config>/projects/<project-dir>/<session-id>.jsonl` with `--to claude`),
installs it without replacing anything,
and reports three separate states: what was converted (with every omission or narrowed meaning named
by category), whether the session is installed, and whether **the target harness itself** loaded it
(Codex's `codex app-server` `thread/resume`, or Claude Code's own loader for `--to claude`) or
continued from it (`--verify resume --resume-prompt TEXT`). It prints the target's own session id and
the command that resumes it — the data directory it installed into (`CODEX_HOME=…`, or for Claude Code
`CLAUDE_CONFIG_DIR=…` together with the working directory its loader discovers it from:
`cd <cwd> && CLAUDE_CONFIG_DIR=<home> claude --resume <id>`) plus the harness and the session id,
so the pasted command opens the home holding the session rather than the caller's default one — and
`--json` reports all of it as one object, the argument vector and environment included. `--dry-run`
converts and verifies without writing anything, `--target-home DIR` keeps a demo or a test away from a
real Codex or Claude Code home, and `--target-binary PATH` names the harness executable when it is not
the target's default name (`codex`, `claude`) on `PATH` (the name the printed resume commands use).

The continuation engine itself lives in [`internal/ccc`](internal/ccc) and is callable without the
CLI: `SourceReader` reads a native session into a typed, transient Conversation document,
`Archive`/`Ingestor` cover the inherited read and write paths, `targets.go` routes `Continue` to the
target writer, `ConvertToCodexRollout`/`ConvertToClaudeTranscript` convert into the two target formats,
and `Inspect` is what the `inspect` command calls. Further targets
(Pi, OpenCode, GitHub Copilot CLI, …) arrive with the goals that specify them.

- Design and scope: [`docs/core-design/README.md`](docs/core-design/README.md)
- Outcome contracts: [`docs/goals/README.md`](docs/goals/README.md)
- What is carried, and how it is proven unchanged: [`docs/upstream.md`](docs/upstream.md)
- Gate, knobs and evidence rules: [`docs/harness.md`](docs/harness.md), [`docs/conventions.md`](docs/conventions.md)

Inherited AgentsView code stays under upstream's MIT licence (see [`LICENSE`](LICENSE), carried with
it); the code this project adds is under the same licence, with the upstream copyright notice
preserved.
