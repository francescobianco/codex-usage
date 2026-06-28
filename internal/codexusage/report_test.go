package codexusage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReportFromExplicitSessionPath(t *testing.T) {
	tmp := t.TempDir()
	sessionPath := filepath.Join(tmp, "session.jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-06-28T22:46:44.541Z","type":"session_meta","payload":{"session_id":"abc123","id":"abc123","timestamp":"2026-06-28T22:45:02.077Z","cwd":"/repo","originator":"codex-tui","cli_version":"0.142.3","source":"cli","thread_source":"user","model_provider":"openai"}}`,
		`{"timestamp":"2026-06-28T22:46:44.570Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/repo","workspace_roots":["/repo"],"current_date":"2026-06-29","timezone":"Europe/Rome","approval_policy":"on-request","sandbox_policy":{"type":"workspace-write","network_access":false},"model":"gpt-5.4","personality":"pragmatic","effort":"medium"}}`,
		`{"timestamp":"2026-06-28T22:46:44.586Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1","started_at":1782686804}}`,
		`{"timestamp":"2026-06-28T22:46:44.600Z","type":"event_msg","payload":{"type":"user_message","message":"ciao"}}`,
		`{"timestamp":"2026-06-28T22:46:44.700Z","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}`,
		`{"timestamp":"2026-06-28T22:46:44.800Z","type":"response_item","payload":{"type":"function_call_output"}}`,
		`{"timestamp":"2026-06-28T22:46:45.000Z","type":"event_msg","payload":{"type":"agent_message","message":"status","phase":"commentary"}}`,
		`{"timestamp":"2026-06-28T22:46:45.100Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":250,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1050},"last_token_usage":{"input_tokens":200,"cached_input_tokens":50,"output_tokens":20,"reasoning_output_tokens":5,"total_tokens":220},"model_context_window":2000},"rate_limits":{"limit_id":"codex","plan_type":"plus","primary":{"used_percent":12.5,"window_minutes":300,"resets_at":1782704805},"secondary":{"used_percent":33,"window_minutes":10080,"resets_at":1783013605},"credits":null,"individual_limit":null,"rate_limit_reached_type":null}}}`,
		`{"timestamp":"2026-06-28T22:46:46.000Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-1","completed_at":1782686806,"duration_ms":1414,"time_to_first_token_ms":222}}`,
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write session fixture: %v", err)
	}

	report, err := LoadReport(tmp, sessionPath)
	if err != nil {
		t.Fatalf("LoadReport returned error: %v", err)
	}

	if report.SessionID != "abc123" {
		t.Fatalf("unexpected session id: %s", report.SessionID)
	}
	if report.Status != StatusComplete {
		t.Fatalf("unexpected status: %s", report.Status)
	}
	if report.Counters.ToolCalls != 1 || report.Counters.ToolCallOutputs != 1 {
		t.Fatalf("unexpected tool counters: %+v", report.Counters)
	}
	if report.LatestTokenSnapshot == nil || report.LatestTokenSnapshot.Total.TotalTokens != 1050 {
		t.Fatalf("unexpected token snapshot: %+v", report.LatestTokenSnapshot)
	}
	if len(report.RateLimits) != 1 || report.RateLimits[0].LimitID != "codex" {
		t.Fatalf("unexpected rate limits: %+v", report.RateLimits)
	}
}

func TestRenderTextIncludesCoreSections(t *testing.T) {
	report := &Report{
		SessionID:   "abc123",
		SessionFile: "/tmp/session.jsonl",
		Status:      StatusActive,
		SessionMeta: SessionMeta{
			ModelProvider:   "openai",
			CLIVersion:      "0.142.3",
			ConfiguredModel: "gpt-5.4",
		},
		TurnContext: TurnContext{
			CWD:            "/repo",
			ApprovalPolicy: "on-request",
			SandboxType:    "workspace-write",
			NetworkAccess:  "restricted",
		},
		CurrentTurn: CurrentTurn{
			ID:     "turn-1",
			Active: true,
		},
		Counters: Counters{
			UserMessages:  1,
			AgentMessages: 2,
		},
		LatestTokenSnapshot: &TokenSnapshot{
			Total:              TokenUsage{TotalTokens: 1000, InputTokens: 800, CachedInputTokens: 100, OutputTokens: 100},
			Last:               TokenUsage{TotalTokens: 250, InputTokens: 200, CachedInputTokens: 25, OutputTokens: 25},
			ModelContextWindow: 2000,
			ContextUsedPercent: 50,
		},
	}

	rendered := RenderText(report)
	for _, expected := range []string{"Codex Usage", "Session", "Runtime", "Workspace", "Turn", "Tokens", "context used: 50.0%"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("missing %q in output:\n%s", expected, rendered)
		}
	}
}
