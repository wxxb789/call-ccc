# Harness — call/ccc

One command decides whether a tree is acceptable:

```bash
python scripts/gate.py      # must print GATE: PASS; CI runs exactly this
```

## What the gate does

| Step | Tool | What it proves |
| --- | --- | --- |
| `gofmt` | `gofmt -l` over tracked `*.go` | formatting is not drifting |
| `build` | `go build <targets>` with `CGO_ENABLED=1` | the tree compiles with the inherited SQLite driver |
| `vet` | `go vet <targets>` | no obvious static defects |
| `test` | `go test -count=1 <targets>` | the tests actually run and pass |
| `selftest` | `python -m unittest discover -s scripts/tests` | the harness's own rules still hold |
| `parity` | `scripts/upstream-parity.py` when present | inherited AgentsView source matches its pinned upstream revision |
| `secrets` | `gitleaks detect --no-git` when available | no committed credentials |

Exit code 0 means every step that **ran** passed. A step that could not run is printed as
`SKIP`/`BOUNDARY` with its reason; it is never silently counted as a pass, and the verdict line says
`GATE: PASS (partial: …)` so a partial run is visible in CI logs and in any transcript quoting it.

## Knobs (all in-repo, all auditable)

- `scripts/gate-targets.txt` — one Go package pattern per line (`#` comments allowed) for
  build/vet/test. Absent means `./...`. Narrowing it is a disclosure obligation: say why in this file
  and in the increment's report.
- `scripts/gate-required.txt` — one step name per line. A step listed here that is skipped,
  unavailable or reported as a platform boundary **fails** the gate: the verdict is computed from the
  effective status, so listing a step cannot silently become a footnote. This is how a check is
  promoted from "runs when it can" to "must run".
- `scripts/upstream-parity.py` — the source-side parity check. The gate runs it when it exists; the
  increment that introduces the pinned upstream revision is the one that adds it and lists `parity` in
  `scripts/gate-required.txt`.
- `CALLCCC_GATE_ROOT` — run the gate against another tree. Used by `scripts/tests/test_gate.py`, which
  pins the required-step rule against a throw-away tree instead of trusting the reader.

## Platform boundary

Linux, macOS and Windows are the intended platforms, subject to native harness availability. On Linux
and macOS a missing Go toolchain or a missing C compiler is a **failure**: the gate cannot silently
degrade where the project is expected to build fully. On Windows a missing C compiler is a disclosed
boundary — the gate runs what it can and says so — because CGO on that platform depends on the
toolchain being installed separately.

## CI

`.github/workflows/gate.yml` runs `python scripts/gate.py` on `ubuntu-latest`, `macos-latest` and
`windows-latest` for every pull request and every push to `main`. CI installs Go and gitleaks; a CI
job with fewer tools than another is the boundary above, not a different gate.

## Growth policy

The gate grows by **adding a step to `scripts/gate.py` and (when it must always run) a line to
`scripts/gate-required.txt`** — not by adding a second script that CI calls separately, and not by
adding a check that only runs on the author's machine. A check that exists in a shell history or a PR
description does not exist. When a new check is introduced, this file and `AGENTS.md` say so in the
same increment.
