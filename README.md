# call/ccc

**call-with-current-conversation-continuation**

A focused CLI for continuing coding-agent conversations across native session formats.

Today the repository carries the **actual AgentsView source tree** at a pinned revision
([`docs/upstream.md`](docs/upstream.md)) and ships one working capability on top of it: inspecting an
existing agent session through AgentsView's own discovery, parsers and archive read path.

```bash
go build -tags fts5 -o ccc ./cmd/ccc      # CGO_ENABLED=1; -tags fts5 is what upstream ships

ccc inspect claude:9f2c1d47-9c3a-4f8f-9a4a-7b1e5c2d6f10
ccc inspect codex:/path/to/rollout-2026-05-04T09-15-00-0c8e5f21-3d6a-4b7c-9f10-2ab4cd6ef812.jsonl --json
```

`ccc inspect` locates a session with the provider's own reader, parses it, and reports its identity,
agent, source, conversation shape (counts, tool calls, compaction boundaries, malformed lines) and —
when a data directory holds one — what the SQLite archive holds for the same session. It never writes:
the source transcript and any archive are read only.

The continuation engine itself lives in [`internal/ccc`](internal/ccc) and is callable without the
CLI: `SourceReader` reads a native session into a typed, transient Conversation document,
`Archive`/`Ingestor` cover the inherited read and write paths, and `Inspect` is what the command calls.
Target writers and `ccc continue` arrive with the goals that specify them.

- Design and scope: [`docs/core-design/README.md`](docs/core-design/README.md)
- Outcome contracts: [`docs/goals/README.md`](docs/goals/README.md)
- What is carried, and how it is proven unchanged: [`docs/upstream.md`](docs/upstream.md)
- Gate, knobs and evidence rules: [`docs/harness.md`](docs/harness.md), [`docs/conventions.md`](docs/conventions.md)

Inherited AgentsView code stays under upstream's MIT licence (see [`LICENSE`](LICENSE), carried with
it); the code this project adds is under the same licence, with the upstream copyright notice
preserved.
