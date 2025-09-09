# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: CLI that spawns multiple named MCP servers. Creates per-server files under `.mcpio/` and mirrors output per the `-print` flag. Contains the process launch/wiring logic.
- `go.mod`: Module metadata (`module mcpio`).

## Build, Test, and Development Commands
- Build: `go build -o mcpio .` — produces the binary.
- Run: `go run . [-dir DIR] [-print] -- <name1> <cmd1> [args...] -- <name2> <cmd2> [args...]`
  - Example: `go run . -dir . -print -- codex node codex.js -- playwright npx playwright mcp`
- Format: `go fmt ./...` — enforce standard Go formatting.
- Lint/Vet: `go vet ./...` (and `staticcheck ./...` if available).
- Clean: `rm -f mcpio .mcpio/*.log .mcpio/*.fifo`

## Coding Style & Naming Conventions
- Idiomatic Go; code must be `gofmt`-clean. Small, composable functions with clear names.
- CLI flags use kebab-case (e.g., `-print`, `-dir`).
- Errors include context (`fmt.Errorf("...: %w", err)`) and actionable messages.

## Testing Guidelines
- Run tests: `go test ./...`.
- For process wiring, spin up short-lived commands via small helpers in tests (e.g., `exec.Command("sh", "-c", "echo hi; echo err 1>&2")`).
- For integration around FIFOs/logs, use `t.TempDir()` and assert files created in `.mcpio/`.
- Prefer table-driven tests and avoid flakiness (no network).

## Commit & Pull Request Guidelines
- Commits: imperative and scoped (e.g., `bridge: add -print mirroring`). Keep diffs focused.
- PRs: include description, linked issues, repro steps, and before/after behavior (sample console lines `[name:out] ...`). Update docs when adding flags or changing paths.
- Ensure `go build`, `go vet`, and `go test` pass before review.

## Usage Notes & Security
- On start, the CLI prints a single line per server listing files relative to `-dir`:
  - `[name:files]: .mcpio/<name>.in.fifo .mcpio/<name>.out.log`
- `-print` mirrors all in/out/err lines to stdout. Without it, stderr is mirrored only until the first input is sent via the FIFO (so startup errors are visible). Logs always append to per-server files. Do not commit logs or FIFOs.
