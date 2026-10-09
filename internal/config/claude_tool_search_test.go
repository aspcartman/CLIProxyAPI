package config

import "testing"

// claude-tool-search loads from the canonical upstream.claude path and the legacy top level.
func TestClaudeToolSearchConfigPaths(t *testing.T) {
	for name, raw := range map[string]string{
		"upstream": "upstream:\n  claude:\n    claude-tool-search: true\n",
		"legacy":   "claude-tool-search: true\n",
	} {
		cfg, err := ParseConfigBytes([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !cfg.ClaudeToolSearch {
			t.Fatalf("%s: ClaudeToolSearch = false", name)
		}
	}
	cfg, err := ParseConfigBytes([]byte("upstream:\n  claude:\n    model-level-cooling: true\n"))
	if err != nil || cfg.ClaudeToolSearch {
		t.Fatalf("default must be false (err=%v)", err)
	}
}
