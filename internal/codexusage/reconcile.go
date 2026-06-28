package codexusage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReconcileReport aggregates every local session against the server-reported
// weekly rate-limit counter, so the user can see whether the quota the server
// charged is fully explained by sessions recorded on this machine.
type ReconcileReport struct {
	CodexHome       string         `json:"codex_home"`
	SessionsDir     string         `json:"sessions_dir"`
	SessionsScanned int            `json:"sessions_scanned"`
	GeneratedAt     time.Time      `json:"generated_at"`
	Windows         []WeeklyWindow `json:"windows"`
	Warnings        []string       `json:"warnings,omitempty"`
}

// WeeklyWindow is one secondary rate-limit cycle (identified by its reset time)
// with the sessions that fall inside it.
type WeeklyWindow struct {
	PlanType           string           `json:"plan_type,omitempty"`
	WindowMinutes      int64            `json:"window_minutes"`
	StartsAt           *time.Time       `json:"starts_at,omitempty"`
	ResetsAt           *time.Time       `json:"resets_at,omitempty"`
	IsCurrent          bool             `json:"is_current"`
	FinalPercent       float64          `json:"final_percent"`
	RemainingPercent   float64          `json:"remaining_percent"`
	ExplainedPercent   float64          `json:"explained_percent"`
	UnexplainedPercent float64          `json:"unexplained_percent"`
	Sessions           []SessionSummary `json:"sessions"`
	Gaps               []UnexplainedGap `json:"unexplained_gaps,omitempty"`
}

// SessionSummary is the per-session view used inside a weekly window.
type SessionSummary struct {
	SessionID           string     `json:"session_id"`
	File                string     `json:"file"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	Prompts             int        `json:"prompts"`
	CommittedTokens     int64      `json:"committed_tokens"`
	RolledBackTokensEst int64      `json:"rolled_back_tokens_est"`
	Aborts              int        `json:"aborts"`
	Rollbacks           int        `json:"rollbacks"`
	WeeklyStartPercent  float64    `json:"weekly_start_percent"`
	WeeklyEndPercent    float64    `json:"weekly_end_percent"`
	WeeklyDeltaPercent  float64    `json:"weekly_delta_percent"`
}

// UnexplainedGap is a rise in the server weekly counter that no local session
// accounts for: either before the first session of a window, or between two
// sessions. This is the signal worth investigating (other device, web/cloud
// usage, or unauthorized access).
type UnexplainedGap struct {
	AfterSession  string     `json:"after_session,omitempty"`
	BeforeSession string     `json:"before_session,omitempty"`
	DeltaPercent  float64    `json:"delta_percent"`
	From          *time.Time `json:"from,omitempty"`
	To            *time.Time `json:"to,omitempty"`
}

// unexplainedThreshold ignores sub-percent noise (the server reports integer %).
const unexplainedThreshold = 0.5

// LoadReconcile scans every session under the Codex home and builds the
// reconciliation report.
func LoadReconcile(codexHome string) (*ReconcileReport, error) {
	root, err := resolveCodexHome(codexHome)
	if err != nil {
		return nil, err
	}
	sessionsDir := filepath.Join(root, "sessions")

	report := &ReconcileReport{
		CodexHome:   root,
		SessionsDir: sessionsDir,
		GeneratedAt: time.Now(),
	}

	var scans []sessionScan
	err = filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		report.SessionsScanned++
		scan, scanErr := scanSession(path)
		if scanErr != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", filepath.Base(path), scanErr))
			return nil
		}
		if scan.hasWeekly {
			scans = append(scans, scan)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan sessions: %w", err)
	}

	report.Windows = buildWindows(scans, report.GeneratedAt)
	return report, nil
}

// sessionScan holds the lightweight per-session facts reconcile needs.
type sessionScan struct {
	id            string
	file          string
	startedAt     *time.Time
	endedAt       *time.Time
	prompts       int
	committed     int64
	rolledBack    int64
	aborts        int
	rollbacks     int
	hasWeekly     bool
	weeklyStart   float64
	weeklyEnd     float64
	weeklyResetAt *time.Time
	windowMinutes int64
	planType      string
	// primary (5h) window, captured for probes (finer-grained than weekly)
	hasPrimary   bool
	primaryStart float64
	primaryEnd   float64
}

func scanSession(path string) (sessionScan, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionScan{}, err
	}
	defer f.Close()

	scan := sessionScan{file: path}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)

	var (
		abortedSinceAdvance bool
		committedTurnTokens int64
	)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env rawEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if ts := parseRFC3339(env.Timestamp); ts != nil {
			if scan.startedAt == nil {
				scan.startedAt = ts
			}
			scan.endedAt = ts
		}

		switch env.Type {
		case "session_meta":
			var meta rawSessionMeta
			if json.Unmarshal(env.Payload, &meta) == nil {
				scan.id = firstNonEmpty(meta.SessionID, meta.ID)
			}
		case "event_msg":
			var ev rawEvent
			if json.Unmarshal(env.Payload, &ev) != nil {
				continue
			}
			switch ev.Type {
			case "user_message":
				scan.prompts++
			case "turn_aborted":
				scan.aborts++
				abortedSinceAdvance = true
			case "thread_rolled_back":
				scan.rollbacks++
				abortedSinceAdvance = true
			case "token_count":
				total := ev.Info.TotalTokenUsage.TotalTokens
				last := ev.Info.LastTokenUsage.TotalTokens
				if total > scan.committed {
					scan.committed = total
					committedTurnTokens = last
					abortedSinceAdvance = false
				} else if abortedSinceAdvance && last > 0 && last != committedTurnTokens {
					// Server processed a turn that local rollback reverted: the
					// cumulative total did not move, but the work was charged.
					scan.rolledBack += last
				}
				if pri := ev.RateLimits.Primary; pri != nil && pri.UsedPercent != nil {
					if !scan.hasPrimary {
						scan.hasPrimary = true
						scan.primaryStart = *pri.UsedPercent
					}
					scan.primaryEnd = *pri.UsedPercent
				}
				if sec := ev.RateLimits.Secondary; sec != nil && sec.UsedPercent != nil {
					if !scan.hasWeekly {
						scan.hasWeekly = true
						scan.weeklyStart = *sec.UsedPercent
					}
					scan.weeklyEnd = *sec.UsedPercent
					if sec.WindowMinutes != nil {
						scan.windowMinutes = *sec.WindowMinutes
					}
					if sec.ResetsAt != nil {
						scan.weeklyResetAtSet(*sec.ResetsAt)
					}
					if ev.RateLimits.PlanType != "" {
						scan.planType = ev.RateLimits.PlanType
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return sessionScan{}, err
	}
	return scan, nil
}

func (s *sessionScan) weeklyResetAtSet(unix int64) {
	s.weeklyResetAt = unixSeconds(unix)
}

func buildWindows(scans []sessionScan, now time.Time) []WeeklyWindow {
	groups := make(map[int64][]sessionScan)
	for _, s := range scans {
		if s.weeklyResetAt == nil {
			continue
		}
		key := s.weeklyResetAt.Unix()
		groups[key] = append(groups[key], s)
	}

	windows := make([]WeeklyWindow, 0, len(groups))
	for key, members := range groups {
		sort.Slice(members, func(i, j int) bool {
			return timeLess(members[i].startedAt, members[j].startedAt)
		})

		resetsAt := time.Unix(key, 0).UTC()
		w := WeeklyWindow{
			ResetsAt:  &resetsAt,
			IsCurrent: resetsAt.After(now),
		}
		if len(members) > 0 {
			w.WindowMinutes = members[0].windowMinutes
			w.PlanType = members[0].planType
		}
		if w.WindowMinutes > 0 {
			start := resetsAt.Add(-time.Duration(w.WindowMinutes) * time.Minute)
			w.StartsAt = &start
		}

		var prevPercent float64
		var prevID string
		var prevEnd *time.Time
		for _, m := range members {
			summary := SessionSummary{
				SessionID:           m.id,
				File:                m.file,
				StartedAt:           m.startedAt,
				EndedAt:             m.endedAt,
				Prompts:             m.prompts,
				CommittedTokens:     m.committed,
				RolledBackTokensEst: m.rolledBack,
				Aborts:              m.aborts,
				Rollbacks:           m.rollbacks,
				WeeklyStartPercent:  m.weeklyStart,
				WeeklyEndPercent:    m.weeklyEnd,
				WeeklyDeltaPercent:  m.weeklyEnd - m.weeklyStart,
			}
			if gap := m.weeklyStart - prevPercent; gap > unexplainedThreshold {
				w.Gaps = append(w.Gaps, UnexplainedGap{
					AfterSession:  prevID,
					BeforeSession: m.id,
					DeltaPercent:  gap,
					From:          prevEnd,
					To:            m.startedAt,
				})
				w.UnexplainedPercent += gap
			}
			w.ExplainedPercent += summary.WeeklyDeltaPercent
			w.Sessions = append(w.Sessions, summary)
			prevPercent = m.weeklyEnd
			prevID = m.id
			prevEnd = m.endedAt
		}
		w.FinalPercent = prevPercent
		w.RemainingPercent = 100 - w.FinalPercent
		windows = append(windows, w)
	}

	sort.Slice(windows, func(i, j int) bool {
		return windows[i].ResetsAt.After(*windows[j].ResetsAt)
	})
	return windows
}

func timeLess(a, b *time.Time) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	default:
		return a.Before(*b)
	}
}
