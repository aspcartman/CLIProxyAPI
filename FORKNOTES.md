# Fork notes (aspcartman/CLIProxyAPI)

Remotes: `origin` = upstream `router-for-me/CLIProxyAPI`, `aspcartman` = this fork.

## Divergences from upstream

- **Claude BM25 tool search for Codex MCP tools** (`upstream.claude.claude-tool-search`, legacy top-level `claude-tool-search`; default off).
  After Responses→Claude translation and before cloaking, untyped `mcp__*` tools are marked `defer_loading` and
  `tool_search_tool_bm25_20251119` is appended when there are 10+ candidates. Core, cache-marked, `tool_choice`-forced and
  history-used tools stay loaded; Haiku and bodies that already use tool search / `defer_loading` are left untouched.
  Caller-owned header mode also sends the advanced-tool-use beta when the body needs it.
  Code: `applyClaudeToolSearch` in `internal/runtime/executor/claude_executor_request.go`; tests in `*_fork_tool_search_test.go`.
- **Interaction with upstream `upstream.claude.tool-search`** (default off): upstream's switch makes the Codex catalog
  advertise `supports_search_tool` for Claude and translates Codex's client `tool_search` into native deferred tools +
  `tool_reference`. When it applies, the fork's feature yields (sees `defer_loading`) — covered by
  `TestClaudeToolSearch_YieldsToUpstreamNativeToolSearch`. Enable one or the other, not both.
- **Codex CPA compaction capsules are unsealed before sending to OpenAI** (`internal/runtime/executor/codex_executor_compaction.go`).
- **Test fix**: `TestClaudeExecutor_ToolSearchDisabledMatchesDevGolden` masks the cloaking-injected current date (upstream golden is date-dependent).

## Update procedure

1. `git fetch --all`; `git checkout -b codex/upstream-YYYY-MM-DD main`; `git merge --no-ff origin/main`.
2. Resolve only mechanical conflicts; re-check the tool-search interaction above when upstream touches Claude tool handling.
3. `go build ./...`, `go vet ./...`, `go test ./...` (`TestPionMediaRelayBridgesAudioAndDataChannel` is flaky — re-run alone).
4. `git checkout main && git merge --no-ff codex/upstream-YYYY-MM-DD`; `git push aspcartman main` (never push to `origin`).
5. Build with `-X main.Version=$(git describe --tags --always)-aspcartman`, back up `~/.local/bin/cliproxyapi` to
   `~/.cli-proxy-api/backups/`, install via temp file + `mv`, then restart with exactly
   `launchctl kickstart -k gui/$(id -u)/com.aspcartman.cliproxyapi` and check port 8317.
   Never `bootout`/`unload` + `bootstrap`/`load`: the agent running this procedure talks through this proxy, so it dies
   after the bootout and the daemon is never started again.
