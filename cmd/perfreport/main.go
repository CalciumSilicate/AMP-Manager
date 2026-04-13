package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ampmanager/internal/perf/report"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: perfreport <evaluate-step|write-report> [flags]")
	}

	switch os.Args[1] {
	case "evaluate-step":
		runEvaluateStep(os.Args[2:])
	case "write-report":
		runWriteReport(os.Args[2:])
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func runEvaluateStep(args []string) {
	fs := flag.NewFlagSet("evaluate-step", flag.ExitOnError)
	scenario := fs.String("scenario", "", "scenario name: smoke, nonstream, stream, mixed")
	label := fs.String("label", "", "human-readable load step label")
	summaryPath := fs.String("summary", "", "path to the k6 summary export json")
	outputPath := fs.String("output", "", "path to the evaluation json output")
	fs.Parse(args)

	if *scenario == "" || *summaryPath == "" || *outputPath == "" {
		log.Fatal("scenario, summary, and output are required")
	}

	evaluation, err := report.EvaluateStep(*scenario, *label, *summaryPath)
	if err != nil {
		log.Fatalf("evaluate step: %v", err)
	}
	if err := report.WriteJSON(*outputPath, evaluation); err != nil {
		log.Fatalf("write evaluation: %v", err)
	}

	fmt.Printf("%s %s stable=%t error_rate=%.4f transport_error_rate=%.4f nonstream_p95=%.2f stream_first_byte_p95=%.2f\n",
		evaluation.Scenario,
		evaluation.Label,
		evaluation.Stable,
		evaluation.ErrorRate,
		evaluation.TransportErrorRate,
		evaluation.NonStreamP95Ms,
		evaluation.StreamFirstByteP95Ms,
	)

	if evaluation.Breakpoint {
		os.Exit(report.BreakpointExitCode)
	}
}

func runWriteReport(args []string) {
	fs := flag.NewFlagSet("write-report", flag.ExitOnError)
	resultsDir := fs.String("results-dir", "", "result directory root")
	outputPath := fs.String("output", "", "markdown output path")
	fs.Parse(args)

	if *resultsDir == "" || *outputPath == "" {
		log.Fatal("results-dir and output are required")
	}

	metaPath := filepath.Join(*resultsDir, "meta.json")
	meta, err := report.LoadMeta(metaPath)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("load meta: %v", err)
	}

	evaluations, err := report.LoadEvaluations(filepath.Join(*resultsDir, "evaluations"))
	if err != nil {
		log.Fatalf("load evaluations: %v", err)
	}

	markdown := report.BuildMarkdownReport(meta, evaluations, *resultsDir)
	if err := os.WriteFile(*outputPath, []byte(markdown), 0o644); err != nil {
		log.Fatalf("write report: %v", err)
	}
}
