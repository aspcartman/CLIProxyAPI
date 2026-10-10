package executor

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// expandCodexCPACompaction unseals CPA-made compaction capsules (from Claude/Antigravity
// sessions) into plain developer context before the request reaches OpenAI, which cannot
// decrypt them. Native OpenAI compaction items pass through untouched.
func expandCodexCPACompaction(req *cliproxyexecutor.Request, opts *cliproxyexecutor.Options) error {
	for _, payload := range []*[]byte{&req.Payload, &opts.OriginalRequest} {
		input := gjson.GetBytes(*payload, "input")
		if !input.IsArray() {
			continue
		}
		changed := false
		items := make([]string, 0, len(input.Array()))
		for _, item := range input.Array() {
			encrypted := item.Get("encrypted_content").String()
			if item.Get("type").String() != "compaction" || !helps.RecognizedAntigravityCompactionCapsule(encrypted) {
				items = append(items, item.Raw)
				continue
			}
			summary, errUnseal := helps.UnsealAntigravityCompaction(strings.TrimSpace(encrypted))
			if errUnseal != nil {
				return errUnseal
			}
			// Same shape ExpandAntigravityCompactionCapsules produces for Claude/Antigravity.
			msg := []byte(`{"type":"message","role":"developer","content":[{"type":"input_text"}]}`)
			msg, _ = sjson.SetBytes(msg, "content.0.text", "Context summary from previous turns:\n"+summary)
			items = append(items, string(msg))
			changed = true
		}
		if changed {
			*payload, _ = sjson.SetRawBytes(*payload, "input", []byte("["+strings.Join(items, ",")+"]"))
		}
	}
	return nil
}
