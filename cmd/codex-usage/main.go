package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yafb/codex-usage/internal/codexusage"
)

func main() {
	var (
		codexHome   = flag.String("codex-home", "", "Path to Codex home directory (default: $CODEX_HOME or ~/.codex)")
		session     = flag.String("session", "", "Session file path or session id")
		asJSON      = flag.Bool("json", false, "Render status as JSON")
		reconcile   = flag.Bool("reconcile", false, "Reconcile every local session against the server weekly counter")
		probe       = flag.Bool("probe", false, "Drive `codex exec` with probe prompts and measure how counters move (spends real quota)")
		probePrompt = flag.String("probe-prompt", "", "Prompt to send for each probe (default: a minimal one)")
		probeCount  = flag.Int("probe-count", 1, "How many probe turns to run")
		probeModel  = flag.String("probe-model", "", "Model to use for probes (default: Codex default)")
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *reconcile {
		rec, err := codexusage.LoadReconcile(*codexHome)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rec); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		fmt.Print(codexusage.RenderReconcile(rec))
		return
	}

	if *probe {
		rec, err := codexusage.RunProbes(codexusage.ProbeOptions{
			CodexHome: *codexHome,
			Prompt:    *probePrompt,
			Count:     *probeCount,
			Model:     *probeModel,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rec); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		fmt.Print(codexusage.RenderProbe(rec))
		return
	}

	report, err := codexusage.LoadReport(*codexHome, *session)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(codexusage.RenderText(report))
}
