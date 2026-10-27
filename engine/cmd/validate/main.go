package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/eval"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/sim"
)

const (
	hrFromS = 12
	rrFromS = 27
)

func main() {
	seeds := flag.Int("seeds", 5, "runs per scenario, each with its own seed")
	only := flag.String("scenario", "", "run only scenarios whose name contains this text")
	jsonOut := flag.String("json", "", "also write the per-scenario means as JSON to this file")
	recordings := flag.String("replay", "", "compare the skin regions of these lab recordings (JSONL, comma-separated) "+
		"against their reference heart rate, instead of running the simulation")
	flag.Parse()

	if *recordings != "" {
		out, err := replayTable(strings.Split(*recordings, ","))
		if err == nil {
			_, err = io.WriteString(os.Stdout, out)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	results := runSuite(*seeds, *only)
	if _, err := io.WriteString(os.Stdout, table(results)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, results); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func runSuite(seeds int, only string) []eval.Metrics {
	var results []eval.Metrics
	for _, sc := range sim.Suite() {
		if !strings.Contains(sc.Name, only) {
			continue
		}
		runs := make([]eval.Metrics, 0, seeds)
		for seed := 1; seed <= seeds; seed++ {
			sc.Seed = uint64(seed)
			m := eval.Summarize(eval.RunScenario(sc, session.DefaultConfig()), hrFromS, rrFromS)
			m.Scenario = sc.Name
			runs = append(runs, m)
		}
		results = append(results, eval.Mean(runs))
	}
	return results
}

func table(results []eval.Metrics) string {
	var b strings.Builder
	b.WriteString("| scenario | HR cov | HR MAE | HR P95 | HR false conf | 1st HR s | RR cov | RR MAE | phase calls | phase agree |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, m := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", m.Scenario,
			pct(m.HRCoverage), num(m.HRMAE), num(m.HRP95), pct(m.HRFalseConfident), num(m.FirstHRS),
			pct(m.RRCoverage), num(m.RRMAE), pct(m.PhaseCallRate), pct(m.PhaseAgreement))
	}
	return b.String()
}

func num(v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*v)
}

func writeJSON(path string, results []eval.Metrics) error {
	rows := make([]map[string]any, 0, len(results))
	for _, m := range results {
		rows = append(rows, map[string]any{
			"scenario": m.Scenario, "hrCoverage": finite(m.HRCoverage), "hrMae": finite(m.HRMAE),
			"hrP95": finite(m.HRP95), "hrFalseConfident": finite(m.HRFalseConfident), "firstHrS": finite(m.FirstHRS),
			"rrCoverage": finite(m.RRCoverage), "rrMae": finite(m.RRMAE),
			"phaseCallRate": finite(m.PhaseCallRate), "phaseAgreement": finite(m.PhaseAgreement),
		})
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func finite(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}
