# mcpio

mcpio is a tiny CLI that bridges shell I/O to one or more MCP servers. It launches named server commands, exposes a per‑server FIFO for stdin, and logs stdout/stderr per server. This lets your shell act as a simple MCP client: write JSON lines to a FIFO and read responses from logs or mirrored console output.

## Features
- Multiple named servers using `--` groups.
- Per‑server files under `.mcpio/` inside the base directory:
  - `.mcpio/<name>.in.fifo` — write JSON lines to send to server stdin.
  - `.mcpio/<name>.out.log` — combined stdout+stderr log.
- Optional console mirroring with `[name:in]`, `[name:out]`, `[name:err]` prefixes.

## Install
- Build from source: `go build -o mcpio .`

## Usage
- Run two servers and mirror IO to console:
  - `go run . -dir . -print -- codex codex mcp -- gemini gemini --experimental-acp`
- On start, it prints file paths:
  - `[codex:files]: .mcpio/codex.in.fifo .mcpio/codex.out.log`
- Send input:
  - `echo '{"method":"ping", "id": 323}' > .mcpio/codex.in.fifo`

## Flags
- `-dir`: base directory for `.mcpio/` files (default `.`).
- `-print`: when present, mirrors all in/out/err lines to stdout. Without it, stderr is mirrored only until the first input is sent via the FIFO (startup errors are visible).


