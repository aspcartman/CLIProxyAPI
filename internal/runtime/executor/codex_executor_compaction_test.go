package executor

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// A Claude-session compaction capsule must reach OpenAI as plain context; native OpenAI ones stay as-is.
func TestExpandCodexCPACompaction(t *testing.T) {
	capsule, err := helps.SealAntigravityCompaction("we fixed the parser", "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"input":[{"type":"compaction","id":"cmp_ag_compact_1"},{"type":"compaction","id":"cmp_native","encrypted_content":"gAAAAnative"},{"type":"message","role":"user","content":"hi"}]}`)
	payload, _ = sjson.SetBytes(payload, "input.0.encrypted_content", capsule)
	req := cliproxyexecutor.Request{Payload: payload}
	opts := cliproxyexecutor.Options{OriginalRequest: payload}

	if err := expandCodexCPACompaction(&req, &opts); err != nil {
		t.Fatal(err)
	}
	for _, got := range [][]byte{req.Payload, opts.OriginalRequest} {
		input := gjson.GetBytes(got, "input").Array()
		if len(input) != 3 {
			t.Fatalf("want 3 items, got %d: %s", len(input), got)
		}
		if input[0].Get("role").String() != "developer" || !strings.Contains(input[0].Get("content.0.text").String(), "we fixed the parser") {
			t.Fatalf("CPA capsule not expanded: %s", input[0].Raw)
		}
		if input[1].Get("encrypted_content").String() != "gAAAAnative" {
			t.Fatalf("native compaction item changed: %s", input[1].Raw)
		}
		if input[2].Get("content").String() != "hi" {
			t.Fatalf("message changed: %s", input[2].Raw)
		}
	}

	// A corrupted CPA capsule fails clearly instead of reaching OpenAI.
	bad, _ := sjson.SetBytes([]byte(`{"input":[{"type":"compaction"}]}`), "input.0.encrypted_content", capsule[:len(capsule)-4])
	if err := expandCodexCPACompaction(&cliproxyexecutor.Request{Payload: bad}, &cliproxyexecutor.Options{}); err == nil {
		t.Fatal("want error for corrupted capsule")
	}
}
