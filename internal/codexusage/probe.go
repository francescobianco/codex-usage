package codexusage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ProbeReport is the result of driving Codex with one or more probe prompts and
// measuring how the server counters and token usage moved for each.
type ProbeReport struct {
	Model       string     `json:"model,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	Runs        []ProbeRun `json:"runs"`
	TotalTokens int64      `json:"total_tokens"`
	WeeklyDelta float64    `json:"weekly_delta_percent"`
	Warnings    []string   `json:"warnings,omitempty"`
}

// ProbeRun is a single probe turn and what it cost.
type ProbeRun struct {
	Index           int     `json:"index"`
	Prompt          string  `json:"prompt"`
	SessionID       string  `json:"session_id,omitempty"`
	File            string  `json:"file,omitempty"`
	DurationSeconds float64 `json:"duration_seconds"`
	CommittedTokens int64   `json:"committed_tokens"`
	Primary5hStart  float64 `json:"primary_5h_start_percent"`
	Primary5hEnd    float64 `json:"primary_5h_end_percent"`
	Primary5hDelta  float64 `json:"primary_5h_delta_percent"`
	WeeklyStart     float64 `json:"weekly_start_percent"`
	WeeklyEnd       float64 `json:"weekly_end_percent"`
	WeeklyDelta     float64 `json:"weekly_delta_percent"`
	Error           string  `json:"error,omitempty"`
}

// ProbeOptions configures a probe run.
type ProbeOptions struct {
	CodexHome string
	Prompt    string
	Count     int
	Model     string
	// CodexBin is the codex executable (default "codex").
	CodexBin string
}

// RunProbes drives `codex exec` Count times with the same prompt and reports,
// for each turn, the exact tokens spent and how much the server 5h and weekly
// counters moved. Every probe consumes real quota.
func RunProbes(opts ProbeOptions) (*ProbeReport, error) {
	root, err := resolveCodexHome(opts.CodexHome)
	if err != nil {
		return nil, err
	}
	sessionsDir := filepath.Join(root, "sessions")
	bin := opts.CodexBin
	if bin == "" {
		bin = "codex"
	}
	if opts.Count < 1 {
		opts.Count = 1
	}
	if opts.Prompt == "" {
		opts.Prompt = "Rispondi solo con la parola: OK"
	}

	report := &ProbeReport{Model: opts.Model, StartedAt: time.Now()}

	for i := 1; i <= opts.Count; i++ {
		run := ProbeRun{Index: i, Prompt: opts.Prompt}
		before := time.Now()

		newest, _ := newestSessionModTime(sessionsDir)

		args := []string{"exec", "--skip-git-repo-check", "-s", "read-only"}
		if opts.Model != "" {
			args = append(args, "-m", opts.Model)
		}
		args = append(args, opts.Prompt)

		cmd := exec.Command(bin, args...)
		cmd.Dir = os.TempDir()
		if opts.CodexHome != "" {
			cmd.Env = append(os.Environ(), "CODEX_HOME="+opts.CodexHome)
		}
		out, runErr := cmd.CombinedOutput()
		run.DurationSeconds = time.Since(before).Round(time.Second).Seconds()
		if runErr != nil {
			run.Error = fmt.Sprintf("%v: %s", runErr, tail(string(out), 200))
			report.Runs = append(report.Runs, run)
			continue
		}

		file, ferr := findSessionAfter(sessionsDir, newest)
		if ferr != nil || file == "" {
			run.Error = "session file della sonda non trovato"
			report.Runs = append(report.Runs, run)
			continue
		}
		scan, serr := scanSession(file)
		if serr != nil {
			run.Error = serr.Error()
			report.Runs = append(report.Runs, run)
			continue
		}

		run.SessionID = scan.id
		run.File = file
		run.CommittedTokens = scan.committed
		run.Primary5hStart = scan.primaryStart
		run.Primary5hEnd = scan.primaryEnd
		run.Primary5hDelta = scan.primaryEnd - scan.primaryStart
		run.WeeklyStart = scan.weeklyStart
		run.WeeklyEnd = scan.weeklyEnd
		run.WeeklyDelta = scan.weeklyEnd - scan.weeklyStart

		report.TotalTokens += run.CommittedTokens
		report.WeeklyDelta += run.WeeklyDelta
		report.Runs = append(report.Runs, run)
	}

	return report, nil
}

func newestSessionModTime(dir string) (time.Time, error) {
	var newest time.Time
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err
}

// findSessionAfter returns the most recently modified session file whose mtime
// is strictly newer than the given reference time.
func findSessionAfter(dir string, after time.Time) (string, error) {
	var (
		best     string
		bestTime time.Time
	)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mt := info.ModTime()
		if mt.After(after) && mt.After(bestTime) {
			best = path
			bestTime = mt
		}
		return nil
	})
	return best, err
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
