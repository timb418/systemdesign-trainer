# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A local system-design interview trainer: Go + HTMX server-rendered app, SQLite for session state, an embedded YAML task bank, and LLM agents (via OpenRouter, using the official `openai-go` SDK directly) that play interviewer/mentor/evaluator roles. Runs single-user on `127.0.0.1` only — no auth, no multi-tenant concerns. Product spec lives in `docs/SPEC.md` (Russian) — read it for the "why" behind modes, rubric, and scope decisions before making product-shape changes.

**UI strings, prompts, commit-adjacent user copy, and code comments in this repo are in Russian.** Match that when editing existing templates/prompts.

## Commands

```bash
go run ./cmd/trainer          # run the server (http://127.0.0.1:8080)
go build ./...
go vet ./...
go test ./...                 # all tests
go test ./internal/tasks/...  # single package
go test ./internal/web/... -run TestLearningModeProgressiveDisclosure  # single test
```

No lint config beyond `go vet`. No JS build step — HTMX/vanilla JS served as static files.

Optional: `./scripts/fetch-drawio.sh` downloads a self-hosted draw.io editor into `third_party/drawio` (gitignored) so the board doesn't depend on `embed.diagrams.net`. Without it, the board iframe falls back to the public embed.

## Architecture

**Request flow**: `cmd/trainer/main.go` wires `tasks.Bank` (embedded YAML) + `settings.Store` (config dir) + `store.Store` (SQLite) + `traineragent.Agents` into `web.Server`, then serves `srv.Handler()`. `web.ListenAndServe` refuses to bind anywhere but `127.0.0.1`/`localhost` — this is intentional (no auth layer), don't relax it.

**Packages** (`internal/`):
- `tasks` — loads and validates the task bank from embedded YAML (`internal/tasks/data/*.yaml`, embedded via `internal/tasks/embed.go`). `bank.go` has strict content-contract validation (`validateTask`) run at load time: every task needs ≥4 hidden functional/nonfunctional/scale facts, ≥3 reveal rules covering all three hidden fields, a gold diagram, ≥2 rubric overrides, etc. `learning.go` builds per-task `LearningBlueprint`s (6 fixed phases: orientation → requirements → scale → hld → deep_dive → reflection) from `learning.yaml` + per-task overrides. If you add/edit a task YAML, it must satisfy these invariants or the embedded bank fails to load (see `internal/tasks/validation_test.go` and `bank_test.go` for the contract).
- `agent` — talks to OpenRouter directly via the official `github.com/openai/openai-go/v3` Chat Completions client (`client.go`'s `newClient` sets the OpenRouter base URL, headers, and `provider`/`reasoning`/`usage` request extras via `option.WithJSONSet`) — no agent framework. Five agent flavors, each a fresh per-call request: interviewer (`Interview`), mentor for learning mode (`Mentor`), evaluator (`Evaluate`, structured rubric JSON output via `ResponseFormat`), compare-to-gold (`Compare`, structured JSON), and a context-summarizer (`Summarize`) used to compact long chat history — `Evaluate`/`Compare`/`Summarize` share the non-streaming `runOneShot` helper in `schema.go`. `Interview`/`Mentor` stream token-by-token and share `runToolLoop` (`stream.go`): it accumulates `ChatCompletionChunk` deltas via `openai.ChatCompletionAccumulator`, and if the model's `FinishReason` is `tool_calls`, executes the tool call(s), appends the results as tool messages, and re-issues the request — looping until the model returns plain text (capped by `maxToolTurns`). Any draft text streamed before a tool-call delta is discarded (`appendStreamText` resets the buffer), so only post-tool-call text reaches the candidate. Prompts are markdown files in `internal/agent/prompts/*.md` (embedded), templated with `{{placeholder}}` string replacement — not Go `text/template`. The interviewer has one tool, `reveal_facts` (`tool.go`), which the LLM calls to pull hidden card facts (scale/functional/nonfunctional) matched by keyword — this is how the "candidate must ask" mechanic works; don't leak hidden facts into the base prompt.
- `store` — SQLite (modernc.org/sqlite, pure Go, no cgo) session/message/diagram/rubric/learning-state persistence. Single connection (`SetMaxOpenConns(1)`), schema created idempotently in `migrate()` (no migration framework — additive `CREATE TABLE IF NOT EXISTS` only).
- `settings` — API key and model config stored outside the DB, in `$XDG_CONFIG_HOME/systemdesign-trainer/` (`settings.json` + `openrouter.key`, both 0600). Session data lives separately in `$XDG_DATA_HOME/systemdesign-trainer/sessions.db`.
- `diagram` — parses draw.io `mxfile` XML into a canonical `Topology` (nodes/edges) and a human-readable text dump. This canonical dump, not the raw XML, is what gets shown to the LLM agents when the candidate "shows the board" — keep `Human()`/`Parse()` changes in sync with what prompts expect.
- `web` — HTTP handlers (`server.go`), SSE streaming for chat token-by-token responses (`startSSE`/`streamConversation`), learning-mode-specific routes (`learning.go`), board-share detection heuristics (`boardshare.go`), conversation history compaction before hitting the LLM (`context.go` — keeps a rolling raw tail + summarized older messages + latest board share), and eval/compare payload building (`payload.go`). Templates are `html/template`, embedded from `web/templates/` via `web/embed.go`; `server.go`'s `New()` parses `base.html` + each named page together.

**Session modes** (`store.Mode`): `full_mock`, `drill` (pattern practice), `requirements_only`, `compare_gold`, `learning`. Each changes interviewer prompt framing and/or which routes are reachable — see `interviewerInstruction` in `agent/prompt.go` and the mode checks scattered through `web/server.go` (e.g. `compare_gold` requires a prior completed attempt; `learning` gates the gold diagram behind reflection completion via `learningGoldUnlocked`).

**Diagram canvas**: default is a blank draw.io board (`canvas: blank`); some tasks start from a pre-built `sketch` with a `starter_diagram` (`canvas: sketch`) — validated at load time that sketch tasks have one and blank tasks don't.

**Testing conventions**: table-driven, no mocking framework — `store` tests use a real temp-file SQLite; `tasks` tests use `testing/fstest.MapFS` to build fake task banks; `web` tests build a real `Server` against temp stores. `internal/tasks/bank_test.go` (`TestEmbeddedBankCoverage`, `TestLearningBlueprintCoversEntireBank`) enforce that the *actual* embedded content — not just the schema — satisfies coverage invariants (e.g. every task type is represented, every task has a learning blueprint), so editing `internal/tasks/data/*.yaml` can break tests even without touching Go code.
