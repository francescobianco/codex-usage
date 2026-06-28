package codexusage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Report struct {
	SessionID           string              `json:"session_id"`
	SessionFile         string              `json:"session_file"`
	Status              SessionStatus       `json:"status"`
	StartedAt           *time.Time          `json:"started_at,omitempty"`
	LastEventAt         *time.Time          `json:"last_event_at,omitempty"`
	Duration            string              `json:"duration"`
	SessionMeta         SessionMeta         `json:"session_meta"`
	TurnContext         TurnContext         `json:"turn_context"`
	CurrentTurn         CurrentTurn         `json:"current_turn"`
	Counters            Counters            `json:"counters"`
	LatestTokenSnapshot *TokenSnapshot      `json:"latest_token_snapshot,omitempty"`
	RateLimits          []RateLimitSnapshot `json:"rate_limits"`
	Warnings            []string            `json:"warnings,omitempty"`
}

type SessionStatus string

const (
	StatusActive   SessionStatus = "active"
	StatusIdle     SessionStatus = "idle"
	StatusComplete SessionStatus = "complete"
)

type SessionMeta struct {
	CWD              string `json:"cwd,omitempty"`
	Originator       string `json:"originator,omitempty"`
	CLIType          string `json:"source,omitempty"`
	CLIPathSource    string `json:"thread_source,omitempty"`
	CLIVersion       string `json:"cli_version,omitempty"`
	ModelProvider    string `json:"model_provider,omitempty"`
	ConfiguredModel  string `json:"configured_model,omitempty"`
	ConfiguredEffort string `json:"configured_effort,omitempty"`
	RawTimestamp     string `json:"raw_timestamp,omitempty"`
}

type TurnContext struct {
	CWD                string   `json:"cwd,omitempty"`
	WorkspaceRoots     []string `json:"workspace_roots,omitempty"`
	CurrentDate        string   `json:"current_date,omitempty"`
	Timezone           string   `json:"timezone,omitempty"`
	ApprovalPolicy     string   `json:"approval_policy,omitempty"`
	SandboxType        string   `json:"sandbox_type,omitempty"`
	NetworkAccess      string   `json:"network_access,omitempty"`
	Model              string   `json:"model,omitempty"`
	Personality        string   `json:"personality,omitempty"`
	Effort             string   `json:"effort,omitempty"`
	ModelContextWindow int64    `json:"model_context_window,omitempty"`
}

type CurrentTurn struct {
	ID                 string     `json:"id,omitempty"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	Duration           string     `json:"duration,omitempty"`
	TimeToFirstTokenMS int64      `json:"time_to_first_token_ms,omitempty"`
	Active             bool       `json:"active"`
}

type Counters struct {
	UserMessages       int `json:"user_messages"`
	AgentMessages      int `json:"agent_messages"`
	CommentaryMessages int `json:"commentary_messages"`
	TaskStarted        int `json:"task_started"`
	TaskCompleted      int `json:"task_completed"`
	ToolCalls          int `json:"tool_calls"`
	ToolCallOutputs    int `json:"tool_call_outputs"`
	ReasoningItems     int `json:"reasoning_items"`
	ResponseMessages   int `json:"response_messages"`
	TokenSnapshots     int `json:"token_snapshots"`
}

type TokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

type TokenSnapshot struct {
	Total              TokenUsage `json:"total"`
	Last               TokenUsage `json:"last"`
	ModelContextWindow int64      `json:"model_context_window"`
	ContextUsedPercent float64    `json:"context_used_percent"`
}

type RateLimitWindow struct {
	UsedPercent   *float64   `json:"used_percent,omitempty"`
	WindowMinutes *int64     `json:"window_minutes,omitempty"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
}

type Credits struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance,omitempty"`
}

type RateLimitSnapshot struct {
	LimitID              string           `json:"limit_id,omitempty"`
	LimitName            string           `json:"limit_name,omitempty"`
	PlanType             string           `json:"plan_type,omitempty"`
	RateLimitReachedType string           `json:"rate_limit_reached_type,omitempty"`
	Primary              *RateLimitWindow `json:"primary,omitempty"`
	Secondary            *RateLimitWindow `json:"secondary,omitempty"`
	Credits              *Credits         `json:"credits,omitempty"`
	IndividualLimit      json.RawMessage  `json:"individual_limit,omitempty"`
	ObservedAt           *time.Time       `json:"observed_at,omitempty"`
}

type rawEnvelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type rawSessionMeta struct {
	SessionID     string `json:"session_id"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	Originator    string `json:"originator"`
	CLIVersion    string `json:"cli_version"`
	Source        string `json:"source"`
	ThreadSource  string `json:"thread_source"`
	ModelProvider string `json:"model_provider"`
}

type rawTurnContext struct {
	TurnID         string   `json:"turn_id"`
	CWD            string   `json:"cwd"`
	WorkspaceRoots []string `json:"workspace_roots"`
	CurrentDate    string   `json:"current_date"`
	Timezone       string   `json:"timezone"`
	ApprovalPolicy string   `json:"approval_policy"`
	Model          string   `json:"model"`
	Personality    string   `json:"personality"`
	Effort         string   `json:"effort"`
	SandboxPolicy  struct {
		Type          string `json:"type"`
		NetworkAccess bool   `json:"network_access"`
	} `json:"sandbox_policy"`
}

type rawEvent struct {
	Type               string        `json:"type"`
	TurnID             string        `json:"turn_id"`
	StartedAt          int64         `json:"started_at"`
	CompletedAt        int64         `json:"completed_at"`
	DurationMS         int64         `json:"duration_ms"`
	TimeToFirstTokenMS int64         `json:"time_to_first_token_ms"`
	Message            string        `json:"message"`
	Phase              string        `json:"phase"`
	Info               rawTokenInfo  `json:"info"`
	RateLimits         rawRateLimits `json:"rate_limits"`
}

type rawResponseItem struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type rawTokenInfo struct {
	TotalTokenUsage    TokenUsage `json:"total_token_usage"`
	LastTokenUsage     TokenUsage `json:"last_token_usage"`
	ModelContextWindow int64      `json:"model_context_window"`
}

type rawRateLimits struct {
	LimitID              string          `json:"limit_id"`
	LimitName            string          `json:"limit_name"`
	PlanType             string          `json:"plan_type"`
	RateLimitReachedType string          `json:"rate_limit_reached_type"`
	Primary              *rawRateWindow  `json:"primary"`
	Secondary            *rawRateWindow  `json:"secondary"`
	Credits              *Credits        `json:"credits"`
	IndividualLimit      json.RawMessage `json:"individual_limit"`
}

type rawRateWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes *int64   `json:"window_minutes"`
	ResetsAt      *int64   `json:"resets_at"`
}

type parserState struct {
	report     Report
	activeTurn CurrentTurn
	latestTurn CurrentTurn
	limitsByID map[string]RateLimitSnapshot
}

func LoadReport(codexHome, session string) (*Report, error) {
	root, err := resolveCodexHome(codexHome)
	if err != nil {
		return nil, err
	}

	sessionPath, err := resolveSessionPath(root, session)
	if err != nil {
		return nil, err
	}

	report, err := parseSessionFile(sessionPath)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func resolveCodexHome(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if fromEnv := os.Getenv("CODEX_HOME"); fromEnv != "" {
		return fromEnv, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func resolveSessionPath(codexHome, session string) (string, error) {
	if session != "" {
		if strings.HasSuffix(session, ".jsonl") {
			return session, nil
		}
		return findSessionByID(filepath.Join(codexHome, "sessions"), session)
	}
	return findLatestSession(filepath.Join(codexHome, "sessions"))
}

func findLatestSession(root string) (string, error) {
	var latest string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if latest == "" || path > latest {
			latest = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan sessions: %w", err)
	}
	if latest == "" {
		return "", errors.New("no session files found")
	}
	return latest, nil
}

func findSessionByID(root, sessionID string) (string, error) {
	var match string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if strings.Contains(path, sessionID) {
			match = path
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("scan sessions: %w", err)
	}
	if match == "" {
		return "", fmt.Errorf("session %q not found under %s", sessionID, root)
	}
	return match, nil
}

func parseSessionFile(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	state := parserState{
		report:     Report{SessionFile: path},
		limitsByID: make(map[string]RateLimitSnapshot),
	}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var env rawEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			state.report.Warnings = append(state.report.Warnings, fmt.Sprintf("skip invalid JSONL line: %v", err))
			continue
		}

		ts := parseRFC3339(env.Timestamp)
		if ts != nil {
			state.report.LastEventAt = ts
		}

		switch env.Type {
		case "session_meta":
			if err := state.handleSessionMeta(env.Payload); err != nil {
				state.report.Warnings = append(state.report.Warnings, err.Error())
			}
		case "turn_context":
			if err := state.handleTurnContext(env.Payload); err != nil {
				state.report.Warnings = append(state.report.Warnings, err.Error())
			}
		case "event_msg":
			if err := state.handleEvent(env.Payload, ts); err != nil {
				state.report.Warnings = append(state.report.Warnings, err.Error())
			}
		case "response_item":
			if err := state.handleResponseItem(env.Payload); err != nil {
				state.report.Warnings = append(state.report.Warnings, err.Error())
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}

	state.finalize()
	return &state.report, nil
}

func (s *parserState) handleSessionMeta(payload json.RawMessage) error {
	var meta rawSessionMeta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return fmt.Errorf("decode session_meta: %w", err)
	}
	s.report.SessionID = firstNonEmpty(meta.SessionID, meta.ID)
	s.report.SessionMeta = SessionMeta{
		CWD:           meta.CWD,
		Originator:    meta.Originator,
		CLIType:       meta.Source,
		CLIPathSource: meta.ThreadSource,
		CLIVersion:    meta.CLIVersion,
		ModelProvider: meta.ModelProvider,
		RawTimestamp:  meta.Timestamp,
	}
	s.report.StartedAt = parseRFC3339(meta.Timestamp)
	return nil
}

func (s *parserState) handleTurnContext(payload json.RawMessage) error {
	var ctx rawTurnContext
	if err := json.Unmarshal(payload, &ctx); err != nil {
		return fmt.Errorf("decode turn_context: %w", err)
	}
	s.report.TurnContext = TurnContext{
		CWD:            ctx.CWD,
		WorkspaceRoots: ctx.WorkspaceRoots,
		CurrentDate:    ctx.CurrentDate,
		Timezone:       ctx.Timezone,
		ApprovalPolicy: ctx.ApprovalPolicy,
		SandboxType:    ctx.SandboxPolicy.Type,
		NetworkAccess:  boolString(ctx.SandboxPolicy.NetworkAccess),
		Model:          ctx.Model,
		Personality:    ctx.Personality,
		Effort:         ctx.Effort,
	}
	s.report.SessionMeta.ConfiguredModel = ctx.Model
	s.report.SessionMeta.ConfiguredEffort = ctx.Effort
	return nil
}

func (s *parserState) handleEvent(payload json.RawMessage, observedAt *time.Time) error {
	var ev rawEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decode event_msg: %w", err)
	}
	switch ev.Type {
	case "user_message":
		s.report.Counters.UserMessages++
	case "agent_message":
		s.report.Counters.AgentMessages++
		if ev.Phase == "commentary" {
			s.report.Counters.CommentaryMessages++
		}
	case "task_started":
		s.report.Counters.TaskStarted++
		startedAt := unixSeconds(ev.StartedAt)
		s.activeTurn = CurrentTurn{ID: ev.TurnID, StartedAt: startedAt, Active: true}
		s.latestTurn = s.activeTurn
	case "task_complete":
		s.report.Counters.TaskCompleted++
		completedAt := unixSeconds(ev.CompletedAt)
		if s.activeTurn.ID == ev.TurnID {
			s.activeTurn.CompletedAt = completedAt
			s.activeTurn.TimeToFirstTokenMS = ev.TimeToFirstTokenMS
			s.activeTurn.Duration = formatDurationMS(ev.DurationMS)
			s.activeTurn.Active = false
			s.latestTurn = s.activeTurn
			s.activeTurn = CurrentTurn{}
		} else {
			s.latestTurn = CurrentTurn{ID: ev.TurnID, CompletedAt: completedAt, Duration: formatDurationMS(ev.DurationMS), TimeToFirstTokenMS: ev.TimeToFirstTokenMS}
		}
	case "token_count":
		s.report.Counters.TokenSnapshots++
		s.report.LatestTokenSnapshot = buildTokenSnapshot(ev.Info)
		if s.report.LatestTokenSnapshot != nil {
			s.report.TurnContext.ModelContextWindow = s.report.LatestTokenSnapshot.ModelContextWindow
		}
		limit := buildRateLimitSnapshot(ev.RateLimits, observedAt)
		if limit.LimitID != "" || limit.PlanType != "" || limit.Credits != nil {
			key := firstNonEmpty(limit.LimitID, limit.PlanType, "unknown")
			s.limitsByID[key] = limit
		}
	}
	return nil
}

func (s *parserState) handleResponseItem(payload json.RawMessage) error {
	var item rawResponseItem
	if err := json.Unmarshal(payload, &item); err != nil {
		return fmt.Errorf("decode response_item: %w", err)
	}
	switch item.Type {
	case "function_call":
		s.report.Counters.ToolCalls++
	case "function_call_output":
		s.report.Counters.ToolCallOutputs++
	case "reasoning":
		s.report.Counters.ReasoningItems++
	case "message":
		s.report.Counters.ResponseMessages++
	}
	return nil
}

func (s *parserState) finalize() {
	if s.activeTurn.Active {
		s.report.CurrentTurn = s.activeTurn
		s.report.Status = StatusActive
		if s.activeTurn.StartedAt != nil {
			s.report.CurrentTurn.Duration = time.Since(*s.activeTurn.StartedAt).Round(time.Second).String()
		}
	} else {
		s.report.CurrentTurn = s.latestTurn
		if s.report.Counters.TaskCompleted > 0 {
			s.report.Status = StatusComplete
		} else {
			s.report.Status = StatusIdle
		}
	}

	if s.report.StartedAt != nil && s.report.LastEventAt != nil {
		s.report.Duration = s.report.LastEventAt.Sub(*s.report.StartedAt).Round(time.Second).String()
	}

	for _, limit := range s.limitsByID {
		s.report.RateLimits = append(s.report.RateLimits, limit)
	}
	sort.Slice(s.report.RateLimits, func(i, j int) bool {
		return s.report.RateLimits[i].LimitID < s.report.RateLimits[j].LimitID
	})
}

func buildTokenSnapshot(info rawTokenInfo) *TokenSnapshot {
	snapshot := &TokenSnapshot{Total: info.TotalTokenUsage, Last: info.LastTokenUsage, ModelContextWindow: info.ModelContextWindow}
	if info.ModelContextWindow > 0 {
		snapshot.ContextUsedPercent = (float64(info.TotalTokenUsage.TotalTokens) / float64(info.ModelContextWindow)) * 100
	}
	return snapshot
}

func buildRateLimitSnapshot(raw rawRateLimits, observedAt *time.Time) RateLimitSnapshot {
	return RateLimitSnapshot{
		LimitID:              raw.LimitID,
		LimitName:            raw.LimitName,
		PlanType:             raw.PlanType,
		RateLimitReachedType: raw.RateLimitReachedType,
		Primary:              buildRateWindow(raw.Primary),
		Secondary:            buildRateWindow(raw.Secondary),
		Credits:              raw.Credits,
		IndividualLimit:      raw.IndividualLimit,
		ObservedAt:           observedAt,
	}
}

func buildRateWindow(raw *rawRateWindow) *RateLimitWindow {
	if raw == nil {
		return nil
	}
	window := &RateLimitWindow{UsedPercent: raw.UsedPercent, WindowMinutes: raw.WindowMinutes}
	if raw.ResetsAt != nil {
		window.ResetsAt = unixSeconds(*raw.ResetsAt)
	}
	return window
}

func parseRFC3339(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil
	}
	return &t
}

func unixSeconds(v int64) *time.Time {
	if v == 0 {
		return nil
	}
	t := time.Unix(v, 0).UTC()
	return &t
}

func boolString(v bool) string {
	if v {
		return "enabled"
	}
	return "restricted"
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return (time.Duration(ms) * time.Millisecond).Round(time.Second).String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
