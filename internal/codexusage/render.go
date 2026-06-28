package codexusage

import (
	"fmt"
	"strings"
	"time"
)

func RenderText(report *Report) string {
	var b strings.Builder

	line(&b, "Codex Usage")
	line(&b, "")

	line(&b, "Session")
	lineKV(&b, "  id", report.SessionID)
	lineKV(&b, "  file", report.SessionFile)
	lineKV(&b, "  status", string(report.Status))
	lineKV(&b, "  started", formatTime(report.StartedAt))
	lineKV(&b, "  last event", formatTime(report.LastEventAt))
	lineKV(&b, "  duration", zeroFallback(report.Duration))

	line(&b, "")
	line(&b, "Runtime")
	lineKV(&b, "  model", firstNonEmpty(report.TurnContext.Model, report.SessionMeta.ConfiguredModel))
	lineKV(&b, "  provider", report.SessionMeta.ModelProvider)
	lineKV(&b, "  effort", firstNonEmpty(report.TurnContext.Effort, report.SessionMeta.ConfiguredEffort))
	lineKV(&b, "  cli version", report.SessionMeta.CLIVersion)
	lineKV(&b, "  originator", report.SessionMeta.Originator)

	line(&b, "")
	line(&b, "Workspace")
	lineKV(&b, "  cwd", firstNonEmpty(report.TurnContext.CWD, report.SessionMeta.CWD))
	lineKV(&b, "  approval", report.TurnContext.ApprovalPolicy)
	lineKV(&b, "  sandbox", report.TurnContext.SandboxType)
	lineKV(&b, "  network", report.TurnContext.NetworkAccess)
	lineKV(&b, "  timezone", report.TurnContext.Timezone)
	if len(report.TurnContext.WorkspaceRoots) > 0 {
		lineKV(&b, "  roots", strings.Join(report.TurnContext.WorkspaceRoots, ", "))
	}

	line(&b, "")
	line(&b, "Turn")
	lineKV(&b, "  id", report.CurrentTurn.ID)
	lineKV(&b, "  active", yesNo(report.CurrentTurn.Active))
	lineKV(&b, "  started", formatTime(report.CurrentTurn.StartedAt))
	lineKV(&b, "  completed", formatTime(report.CurrentTurn.CompletedAt))
	lineKV(&b, "  duration", zeroFallback(report.CurrentTurn.Duration))
	if report.CurrentTurn.TimeToFirstTokenMS > 0 {
		lineKV(&b, "  first token", fmt.Sprintf("%d ms", report.CurrentTurn.TimeToFirstTokenMS))
	}

	line(&b, "")
	line(&b, "Activity")
	lineKV(&b, "  user messages", itoa(report.Counters.UserMessages))
	lineKV(&b, "  agent messages", itoa(report.Counters.AgentMessages))
	lineKV(&b, "  commentary", itoa(report.Counters.CommentaryMessages))
	lineKV(&b, "  task started", itoa(report.Counters.TaskStarted))
	lineKV(&b, "  task completed", itoa(report.Counters.TaskCompleted))
	lineKV(&b, "  tool calls", itoa(report.Counters.ToolCalls))
	lineKV(&b, "  tool outputs", itoa(report.Counters.ToolCallOutputs))
	lineKV(&b, "  reasoning items", itoa(report.Counters.ReasoningItems))
	lineKV(&b, "  response messages", itoa(report.Counters.ResponseMessages))

	line(&b, "")
	line(&b, "Tokens")
	if report.LatestTokenSnapshot == nil {
		line(&b, "  no token snapshot found")
	} else {
		renderUsage(&b, "  total", report.LatestTokenSnapshot.Total)
		renderUsage(&b, "  last turn", report.LatestTokenSnapshot.Last)
		lineKV(&b, "  context window", itoa64(report.LatestTokenSnapshot.ModelContextWindow))
		lineKV(&b, "  context used", fmt.Sprintf("%.1f%%", report.LatestTokenSnapshot.ContextUsedPercent))
	}

	line(&b, "")
	line(&b, "Limits")
	if len(report.RateLimits) == 0 {
		line(&b, "  no rate limit snapshot found")
	} else {
		for _, limit := range report.RateLimits {
			title := firstNonEmpty(limit.LimitID, limit.PlanType, "unknown")
			line(&b, "  "+title)
			lineKV(&b, "    plan", limit.PlanType)
			lineKV(&b, "    observed", formatTime(limit.ObservedAt))
			if limit.Primary != nil {
				lineKV(&b, "    primary", formatRateWindow(limit.Primary))
			}
			if limit.Secondary != nil {
				lineKV(&b, "    secondary", formatRateWindow(limit.Secondary))
			}
			if limit.Credits != nil {
				lineKV(&b, "    credits", formatCredits(limit.Credits))
			}
			if limit.RateLimitReachedType != "" {
				lineKV(&b, "    reached type", limit.RateLimitReachedType)
			}
		}
	}

	if len(report.Warnings) > 0 {
		line(&b, "")
		line(&b, "Warnings")
		for _, warning := range report.Warnings {
			line(&b, "  - "+warning)
		}
	}

	return b.String()
}

func RenderReconcile(report *ReconcileReport) string {
	var b strings.Builder

	line(&b, "Codex Usage — Reconcile (locale vs server)")
	line(&b, "")
	lineKV(&b, "sessions dir", report.SessionsDir)
	lineKV(&b, "sessioni analizzate", itoa(report.SessionsScanned))
	lineKV(&b, "finestre weekly", itoa(len(report.Windows)))

	for _, w := range report.Windows {
		line(&b, "")
		title := "Finestra weekly"
		if w.IsCurrent {
			title += " [CORRENTE]"
		}
		line(&b, title)
		lineKV(&b, "  piano", w.PlanType)
		lineKV(&b, "  da", formatTime(w.StartsAt))
		lineKV(&b, "  reset", formatTime(w.ResetsAt))
		lineKV(&b, "  usato (server)", fmt.Sprintf("%.0f%%", w.FinalPercent))
		lineKV(&b, "  rimane", fmt.Sprintf("%.0f%%", w.RemainingPercent))
		lineKV(&b, "  spiegato da sessioni locali", fmt.Sprintf("%.0f%%", w.ExplainedPercent))
		lineKV(&b, "  NON spiegato (altrove/cloud/?)", fmt.Sprintf("%.0f%%", w.UnexplainedPercent))

		line(&b, "  sessioni:")
		if len(w.Sessions) == 0 {
			line(&b, "    nessuna")
		}
		for _, s := range w.Sessions {
			line(&b, "    "+shortID(s.SessionID))
			lineKV(&b, "      quando", formatTime(s.StartedAt))
			lineKV(&b, "      prompt", itoa(s.Prompts))
			lineKV(&b, "      weekly", fmt.Sprintf("%.0f%% -> %.0f%% (Δ%+.0f%%)", s.WeeklyStartPercent, s.WeeklyEndPercent, s.WeeklyDeltaPercent))
			lineKV(&b, "      token committati", itoa64(s.CommittedTokens))
			if s.RolledBackTokensEst > 0 {
				lineKV(&b, "      token rolled-back (stima, pagati ma non contati)", itoa64(s.RolledBackTokensEst))
			}
			if s.Aborts > 0 || s.Rollbacks > 0 {
				lineKV(&b, "      abort/rollback", fmt.Sprintf("%d / %d", s.Aborts, s.Rollbacks))
			}
		}

		if len(w.Gaps) > 0 {
			line(&b, "  ⚠ consumo senza sessione locale:")
			for _, g := range w.Gaps {
				where := "prima della prima sessione"
				if g.AfterSession != "" {
					where = "tra " + shortID(g.AfterSession) + " e " + shortID(g.BeforeSession)
				}
				lineKV(&b, "    "+where, fmt.Sprintf("+%.0f%% (%s -> %s)", g.DeltaPercent, formatTime(g.From), formatTime(g.To)))
			}
		}
	}

	if len(report.Warnings) > 0 {
		line(&b, "")
		line(&b, "Warnings")
		for _, warning := range report.Warnings {
			line(&b, "  - "+warning)
		}
	}

	return b.String()
}

func RenderProbe(report *ProbeReport) string {
	var b strings.Builder

	line(&b, "Codex Usage — Probe (sonde live)")
	line(&b, "")
	lineKV(&b, "model", zeroFallback(report.Model))
	lineKV(&b, "avviato", report.StartedAt.Local().Format(time.RFC3339))
	lineKV(&b, "sonde", itoa(len(report.Runs)))
	line(&b, "")

	for _, r := range report.Runs {
		line(&b, fmt.Sprintf("Sonda #%d", r.Index))
		lineKV(&b, "  prompt", r.Prompt)
		if r.Error != "" {
			lineKV(&b, "  errore", r.Error)
			line(&b, "")
			continue
		}
		lineKV(&b, "  session", shortID(r.SessionID))
		lineKV(&b, "  durata", fmt.Sprintf("%.0fs", r.DurationSeconds))
		lineKV(&b, "  token (questa sonda)", itoa64(r.CommittedTokens))
		lineKV(&b, "  5h", fmt.Sprintf("%.0f%% -> %.0f%% (Δ%+.1f%%)", r.Primary5hStart, r.Primary5hEnd, r.Primary5hDelta))
		lineKV(&b, "  weekly", fmt.Sprintf("%.0f%% -> %.0f%% (Δ%+.1f%%)", r.WeeklyStart, r.WeeklyEnd, r.WeeklyDelta))
		line(&b, "")
	}

	line(&b, "Totale sonde")
	lineKV(&b, "  token spesi", itoa64(report.TotalTokens))
	lineKV(&b, "  weekly mosso", fmt.Sprintf("%+.1f%%", report.WeeklyDelta))

	if len(report.Warnings) > 0 {
		line(&b, "")
		line(&b, "Warnings")
		for _, w := range report.Warnings {
			line(&b, "  - "+w)
		}
	}

	return b.String()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	if id == "" {
		return "(senza id)"
	}
	return id
}

func renderUsage(b *strings.Builder, label string, usage TokenUsage) {
	lineKV(b, label, fmt.Sprintf("total=%s input=%s cached=%s output=%s reasoning=%s", itoa64(usage.TotalTokens), itoa64(usage.InputTokens), itoa64(usage.CachedInputTokens), itoa64(usage.OutputTokens), itoa64(usage.ReasoningOutputTokens)))
}

func formatRateWindow(window *RateLimitWindow) string {
	parts := make([]string, 0, 3)
	if window.UsedPercent != nil {
		parts = append(parts, fmt.Sprintf("used=%.1f%%", *window.UsedPercent))
	}
	if window.WindowMinutes != nil {
		parts = append(parts, fmt.Sprintf("window=%dm", *window.WindowMinutes))
	}
	if window.ResetsAt != nil {
		parts = append(parts, "reset="+formatTime(window.ResetsAt))
	}
	return strings.Join(parts, " ")
}

func formatCredits(credits *Credits) string {
	return fmt.Sprintf("has=%t unlimited=%t balance=%s", credits.HasCredits, credits.Unlimited, zeroFallback(credits.Balance))
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format(time.RFC3339)
}

func line(b *strings.Builder, value string) {
	b.WriteString(value)
	b.WriteByte('\n')
}

func lineKV(b *strings.Builder, key, value string) {
	line(b, fmt.Sprintf("%s: %s", key, zeroFallback(value)))
}

func zeroFallback(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

func itoa64(v int64) string {
	return fmt.Sprintf("%d", v)
}
