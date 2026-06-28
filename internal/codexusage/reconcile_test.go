package codexusage

import (
	"os"
	"path/filepath"
	"testing"
)

// tokenCountLine builds a token_count event_msg JSONL line.
func tokenCountLine(ts string, total, last int64, weekly float64, resetsAt int64) string {
	return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":` +
		itoa64(total) + `},"last_token_usage":{"total_tokens":` + itoa64(last) +
		`},"model_context_window":258400},"rate_limits":{"limit_id":"codex","plan_type":"plus","primary":{"used_percent":1,"window_minutes":300,"resets_at":1782704805},"secondary":{"used_percent":` +
		itoaFloat(weekly) + `,"window_minutes":10080,"resets_at":` + itoa64(resetsAt) + `}}}}`
}

func itoaFloat(v float64) string {
	return itoa(int(v))
}

func writeSession(t *testing.T, dir, name string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := ""
	for i, l := range lines {
		if i > 0 {
			content += "\n"
		}
		content += l
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestReconcileFlagsUnexplainedGap(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	const reset int64 = 1783013605

	// One session in the window that takes weekly from 17% -> 33%, but starts at
	// 17% — meaning 17% was consumed before any local session in this window.
	writeSession(t, sessions, "s1.jsonl", []string{
		`{"timestamp":"2026-06-27T15:50:00.000Z","type":"session_meta","payload":{"session_id":"sess-1","id":"sess-1"}}`,
		`{"timestamp":"2026-06-27T15:50:01.000Z","type":"event_msg","payload":{"type":"user_message","message":"go"}}`,
		tokenCountLine("2026-06-27T15:50:02.000Z", 100, 100, 17, reset),
		tokenCountLine("2026-06-27T16:43:00.000Z", 5000, 4900, 33, reset),
	})

	report, err := LoadReconcile(home)
	if err != nil {
		t.Fatalf("LoadReconcile: %v", err)
	}
	if len(report.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(report.Windows))
	}
	w := report.Windows[0]
	if w.FinalPercent != 33 {
		t.Fatalf("final percent = %.0f, want 33", w.FinalPercent)
	}
	if w.ExplainedPercent != 16 {
		t.Fatalf("explained = %.0f, want 16", w.ExplainedPercent)
	}
	if w.UnexplainedPercent != 17 {
		t.Fatalf("unexplained = %.0f, want 17", w.UnexplainedPercent)
	}
	if len(w.Gaps) != 1 || w.Gaps[0].DeltaPercent != 17 {
		t.Fatalf("expected one 17%% gap, got %+v", w.Gaps)
	}
}

func TestReconcileCountsRolledBackTokens(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	const reset int64 = 1783013605

	// Commit reaches 446131 (last 65829). Then aborts: total stays flat while two
	// distinct rolled-back turns of 63855 are processed by the server.
	writeSession(t, sessions, "s.jsonl", []string{
		`{"timestamp":"2026-06-28T22:46:00.000Z","type":"session_meta","payload":{"session_id":"sess-x","id":"sess-x"}}`,
		tokenCountLine("2026-06-28T22:55:06.000Z", 446131, 65829, 33, reset),
		`{"timestamp":"2026-06-28T23:00:10.000Z","type":"event_msg","payload":{"type":"turn_aborted","reason":"interrupted"}}`,
		// repeat of the committed turn's last value -> must NOT be counted
		tokenCountLine("2026-06-28T23:00:11.000Z", 446131, 65829, 34, reset),
		tokenCountLine("2026-06-28T23:00:56.000Z", 446131, 63855, 34, reset),
		`{"timestamp":"2026-06-28T23:00:57.000Z","type":"event_msg","payload":{"type":"thread_rolled_back"}}`,
		tokenCountLine("2026-06-28T23:01:47.000Z", 446131, 63855, 34, reset),
	})

	report, err := LoadReconcile(home)
	if err != nil {
		t.Fatalf("LoadReconcile: %v", err)
	}
	s := report.Windows[0].Sessions[0]
	if s.CommittedTokens != 446131 {
		t.Fatalf("committed = %d, want 446131", s.CommittedTokens)
	}
	if s.RolledBackTokensEst != 2*63855 {
		t.Fatalf("rolled back = %d, want %d", s.RolledBackTokensEst, 2*63855)
	}
}
