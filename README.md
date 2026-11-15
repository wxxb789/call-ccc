# call/ccc

**call-with-current-conversation-continuation**

A focused CLI for continuing coding-agent conversations across native session formats.

Today the repository carries the **actual AgentsView source tree** at a pinned revision
([`docs/upstream.md`](docs/upstream.md)) and ships two working capabilities on top of it: inspecting an
existing agent session through AgentsView's own discovery, parsers and archive read path, and
continuing a Claude Code session as a new, natively resumable **Codex** session.

```bash
go build -tags fts5 -o bin/ccc ./cmd/ccc   # CGO_ENABLED=1; -tags fts5 is what upstream ships

bin/ccc inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
bin/ccc inspect codex:/path/to/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl --json

bin/ccc continue claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10 --to codex
```

`ccc inspect` locates a session with the provider's own reader, parses it, and reports its identity,
agent, source, conversation shape (counts, tool calls, compaction boundaries, malformed lines) and —
when a data directory holds one — what the SQLite archive holds for the same session. It never writes:
the source transcript and any archive are read only.

`ccc continue` reads that session through the same inherited reader, writes it as a **new** Codex
session (a rollout under the target's own `sessions/` tree), installs it without replacing anything,
and reports three separate states: what was converted (with every omission or narrowed meaning named
by category), whether the session is installed, and whether **Codex itself** loaded it
(`codex app-server`'s `thread/resume`) or continued from it (`codex exec resume`, with
`--verify resume --resume-prompt TEXT`). It prints the target's own session id and the command that
resumes it, and `--json` reports all of it as one object. `--dry-run` converts and verifies without
writing anything, and `--target-home DIR` keeps a demo or a test away from a real Codex home.

The continuation engine itself lives in [`internal/ccc`](internal/ccc) and is callable without the
CLI: `SourceReader` reads a native session into a typed, transient Conversation document,
`Archive`/`Ingestor` cover the inherited read and write paths, `ConvertToCodexRollout`/`Continue` add
the Codex target and the pipeline, and `Inspect` is what the `inspect` command calls. Further targets
(Pi, OpenCode, GitHub Copilot CLI, …) arrive with the goals that specify them.

- Design and scope: [`docs/core-design/README.md`](docs/core-design/README.md)
- Outcome contracts: [`docs/goals/README.md`](docs/goals/README.md)
- What is carried, and how it is proven unchanged: [`docs/upstream.md`](docs/upstream.md)
- Gate, knobs and evidence rules: [`docs/harness.md`](docs/harness.md), [`docs/conventions.md`](docs/conventions.md)

Inherited AgentsView code stays under upstream's MIT licence (see [`LICENSE`](LICENSE), carried with
it); the code this project adds is under the same licence, with the upstream copyright notice
preserved.
