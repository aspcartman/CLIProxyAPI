package responses

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// Claude tool search blocks must vanish; only the deferred tool's call surfaces,
// restored to its Codex namespace and name.
var toolSearchRequest = []byte(`{"tools":[{"type":"function","name":"exec_command"},{"type":"namespace","name":"mcp__codex_apps__github","tools":[{"type":"function","name":"fetch_pr","parameters":{"type":"object"}}]}]}`)

func toolSearchClaudeChunks() [][]byte {
	return [][]byte{
		[]byte(`data: {"type":"message_start","message":{"id":"msg_ts","usage":{"input_tokens":1,"output_tokens":0}}}`),
		[]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_ts","name":"tool_search_tool_bm25","input":{}}}`),
		[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"github pull request\"}"}}`),
		[]byte(`data: {"type":"content_block_stop","index":0}`),
		[]byte(`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_ts","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"mcp__codex_apps__github__fetch_pr"}]}}}`),
		[]byte(`data: {"type":"content_block_stop","index":1}`),
		[]byte(`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_pr","name":"mcp__codex_apps__github__fetch_pr","input":{}}}`),
		[]byte(`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"pr\":1}"}}`),
		[]byte(`data: {"type":"content_block_stop","index":2}`),
		[]byte(`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`),
		[]byte(`data: {"type":"message_stop"}`),
	}
}

func assertSingleDeferredCall(t *testing.T, output gjson.Result) {
	t.Helper()
	items := output.Array()
	if len(items) != 1 {
		t.Fatalf("output items = %d, want 1: %s", len(items), output.Raw)
	}
	item := items[0]
	if item.Get("type").String() != "function_call" || item.Get("name").String() != "fetch_pr" || item.Get("namespace").String() != "mcp__codex_apps__github" {
		t.Fatalf("unexpected item: %s", item.Raw)
	}
	if item.Get("arguments").String() != `{"pr":1}` {
		t.Fatalf("arguments = %q", item.Get("arguments").String())
	}
}

func TestClaudeToolSearchBlocksDroppedStream(t *testing.T) {
	var param any
	var completed gjson.Result
	added := 0
	for _, chunk := range toolSearchClaudeChunks() {
		for _, out := range ConvertClaudeResponseToOpenAIResponses(context.Background(), "claude-test", toolSearchRequest, nil, chunk, &param) {
			ev, data := parseClaudeResponsesSSEEvent(t, out)
			if ev == "response.output_item.added" {
				added++
			}
			if ev == "response.completed" {
				completed = data
			}
		}
	}
	if added != 1 {
		t.Fatalf("output_item.added events = %d, want 1", added)
	}
	assertSingleDeferredCall(t, completed.Get("response.output"))
}

func TestClaudeToolSearchBlocksDroppedNonStream(t *testing.T) {
	var lines []string
	for _, chunk := range toolSearchClaudeChunks() {
		lines = append(lines, string(chunk))
	}
	out := ConvertClaudeResponseToOpenAIResponsesNonStream(context.Background(), "claude-test", toolSearchRequest, nil, []byte(strings.Join(lines, "\n")), nil)
	assertSingleDeferredCall(t, gjson.GetBytes(out, "output"))
}
