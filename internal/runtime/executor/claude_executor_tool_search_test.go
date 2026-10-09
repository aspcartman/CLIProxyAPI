package executor

import (
	"fmt"
	"strings"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	"github.com/tidwall/gjson"
)

// toolSearchBody builds a Claude body with core tools plus n mcp__ tools.
func toolSearchBody(model string, n int, extra string) []byte {
	tools := []string{
		`{"name":"exec_command","input_schema":{"type":"object"}}`,
		`{"name":"apply_patch","input_schema":{"type":"object"}}`,
	}
	for i := 0; i < n; i++ {
		tools = append(tools, fmt.Sprintf(`{"name":"mcp__github__tool_%d","input_schema":{"type":"object"}}`, i))
	}
	return []byte(`{"model":"` + model + `","tools":[` + strings.Join(tools, ",") + `]` + extra + `}`)
}

func deferredNames(body []byte) map[string]bool {
	out := map[string]bool{}
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		if tool.Get("defer_loading").Bool() {
			out[tool.Get("name").String()] = true
		}
		return true
	})
	return out
}

func countSearchTools(body []byte) int {
	n := 0
	gjson.GetBytes(body, "tools").ForEach(func(_, tool gjson.Result) bool {
		if strings.HasPrefix(tool.Get("type").String(), "tool_search_tool_") {
			n++
		}
		return true
	})
	return n
}

func TestApplyClaudeToolSearch_NoChange(t *testing.T) {
	for name, body := range map[string][]byte{
		"fewer than 10":        toolSearchBody("claude-opus-5-5", 9, ""),
		"haiku":                toolSearchBody("claude-haiku-4-5", 20, ""),
		"no tools":             []byte(`{"model":"claude-opus-5-5"}`),
		"existing search tool": toolSearchBody("claude-opus-5-5", 20, `,"x":1`),
		"existing defer":       []byte(strings.Replace(string(toolSearchBody("claude-opus-5-5", 20, "")), `"name":"exec_command",`, `"name":"exec_command","defer_loading":false,`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "existing search tool" {
				body = []byte(strings.Replace(string(body), `"tools":[`, `"tools":[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"},`, 1))
			}
			if got := applyClaudeToolSearch(body); string(got) != string(body) {
				t.Fatalf("body changed:\n%s", got)
			}
		})
	}
}

func TestApplyClaudeToolSearch_DefersOnlyMCPTools(t *testing.T) {
	body := applyClaudeToolSearch(toolSearchBody("claude-opus-5-5", 12, ""))
	deferred := deferredNames(body)
	if len(deferred) != 12 || deferred["exec_command"] || deferred["apply_patch"] {
		t.Fatalf("deferred = %v", deferred)
	}
	if countSearchTools(body) != 1 || gjson.GetBytes(body, "tools.#(type==\"tool_search_tool_bm25_20251119\").name").String() != "tool_search_tool_bm25" {
		t.Fatalf("search tool missing or duplicated: %s", gjson.GetBytes(body, "tools").Raw)
	}
	// Re-applying is a no-op: the body now manages tool search itself.
	if again := applyClaudeToolSearch(body); string(again) != string(body) {
		t.Fatal("second application changed body")
	}
}

func TestApplyClaudeToolSearch_KeepsUsedForcedAndCachedTools(t *testing.T) {
	body := toolSearchBody("claude-opus-5-5", 14, `,"tool_choice":{"type":"tool","name":"mcp__github__tool_1"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__github__tool_0","input":{}}]}]`)
	body = []byte(strings.Replace(string(body), `{"name":"mcp__github__tool_2",`, `{"name":"mcp__github__tool_2","cache_control":{"type":"ephemeral"},`, 1))
	deferred := deferredNames(applyClaudeToolSearch(body))
	if len(deferred) != 11 || deferred["mcp__github__tool_0"] || deferred["mcp__github__tool_1"] || deferred["mcp__github__tool_2"] {
		t.Fatalf("deferred = %v", deferred)
	}
}

// The executor's beta and OAuth renaming paths must accept the rewritten body.
func TestApplyClaudeToolSearch_BetaAndRenaming(t *testing.T) {
	body := applyClaudeToolSearch(toolSearchBody("claude-opus-5-5", 12, ""))
	if betas := claudeCodeCLIBetas(body, nil, true); !strings.Contains(betas, claudeAdvancedToolUseBeta) {
		t.Fatalf("betas missing %s: %s", claudeAdvancedToolUseBeta, betas)
	}
	renamed, _ := remapOAuthToolNames(body)
	if got := len(deferredNames(renamed)); got != 12 {
		t.Fatalf("deferred after renaming = %d, want 12: %s", got, gjson.GetBytes(renamed, "tools").Raw)
	}
	if got := gjson.GetBytes(renamed, "tools.#(type==\"tool_search_tool_bm25_20251119\").name").String(); got != "tool_search_tool_bm25" {
		t.Fatalf("search tool renamed to %q", got)
	}
}

// End to end: a Codex Responses request reaches Anthropic with deferred MCP
// tools, the search tool, and the advanced-tool-use beta.
func TestClaudeExecutor_ToolSearchForCodexRequests(t *testing.T) {
	children := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		children = append(children, fmt.Sprintf(`{"type":"function","name":"tool_%d","parameters":{"type":"object","properties":{}}}`, i))
	}
	payload := []byte(`{"model":"claude-opus-5-5","input":[{"type":"message","role":"user","content":"hi"}],"tools":[` +
		`{"type":"function","name":"exec_command","parameters":{"type":"object","properties":{}}},` +
		`{"type":"namespace","name":"mcp__codex_apps__github","tools":[` + strings.Join(children, ",") + `]}]}`)
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			cfg := midSystemConfig()
			cfg.ClaudeToolSearch = enabled
			upstream := &midSystemUpstream{}
			_, err := NewClaudeExecutor(cfg).Execute(upstream.context(t, nil), midSystemAuth(), cliproxyexecutor.Request{
				Model: "claude-opus-5-5", Payload: payload,
			}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse})
			if err != nil {
				t.Fatal(err)
			}
			deferred, search := len(deferredNames(upstream.body)), countSearchTools(upstream.body)
			beta := strings.Contains(strings.Join(helps.HeaderValuesCaseInsensitive(upstream.headers, "Anthropic-Beta"), ","), claudeAdvancedToolUseBeta)
			if enabled && (deferred != 12 || search != 1 || !beta) {
				t.Fatalf("deferred=%d search=%d beta=%v", deferred, search, beta)
			}
			if !enabled && (deferred != 0 || search != 0) {
				t.Fatalf("disabled setting changed tools: %s", gjson.GetBytes(upstream.body, "tools").Raw)
			}
		})
	}
}

// Caller-owned fingerprint mode (plain API key, no cloak) must still send the
// beta, since tool search was added by the proxy rather than the caller.
func TestApplyClaudeHeaders_ToolSearchBetaInCallerOwnedMode(t *testing.T) {
	body := applyClaudeToolSearch(toolSearchBody("claude-opus-5-5", 12, ""))
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "key-ts", "cloak_mode": "never"}}
	req := newClaudeHeaderTestRequest(t, nil)
	if err := applyClaudeHeaders(req, auth, "key-ts", false, nil, body, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(helps.HeaderValuesCaseInsensitive(req.Header, "Anthropic-Beta"), ","); !strings.Contains(got, claudeAdvancedToolUseBeta) {
		t.Fatalf("Anthropic-Beta = %q, want %s", got, claudeAdvancedToolUseBeta)
	}
}
