package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const BreakpointExitCode = 10

type SummaryExport struct {
	Metrics map[string]MetricSummary `json:"metrics"`
}

type MetricSummary struct {
	Values map[string]float64 `json:"values"`
	Legacy map[string]float64 `json:"-"`
}

func (metric *MetricSummary) UnmarshalJSON(data []byte) error {
	type alias MetricSummary
	var withValues struct {
		Values map[string]float64 `json:"values"`
	}
	if err := json.Unmarshal(data, &withValues); err == nil && withValues.Values != nil {
		metric.Values = withValues.Values
		return nil
	}

	legacy := make(map[string]float64)
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	metric.Legacy = legacy
	return nil
}

type StepEvaluation struct {
	GeneratedAt          time.Time `json:"generated_at"`
	Scenario             string    `json:"scenario"`
	Label                string    `json:"label"`
	SummaryPath          string    `json:"summary_path"`
	RequestCount         float64   `json:"request_count"`
	ErrorRate            float64   `json:"error_rate"`
	TransportErrorRate   float64   `json:"transport_error_rate"`
	NonStreamP95Ms       float64   `json:"nonstream_p95_ms,omitempty"`
	StreamFirstByteP95Ms float64   `json:"stream_first_byte_p95_ms,omitempty"`
	Stable               bool      `json:"stable"`
	Breakpoint           bool      `json:"breakpoint"`
	Reasons              []string  `json:"reasons"`
}

type SessionMeta struct {
	StartedAt            time.Time `json:"started_at"`
	Profile              string    `json:"profile"`
	UserCount            int       `json:"user_count"`
	ActiveKeyPool        int       `json:"active_key_pool"`
	AppURL               string    `json:"app_url"`
	UpstreamURL          string    `json:"upstream_url"`
	RequestDetailEnabled bool      `json:"request_detail_enabled"`
}

func EvaluateStep(scenario, label, summaryPath string) (*StepEvaluation, error) {
	summary, err := LoadSummary(summaryPath)
	if err != nil {
		return nil, err
	}

	evaluation := &StepEvaluation{
		GeneratedAt:        time.Now().UTC(),
		Scenario:           scenario,
		Label:              label,
		SummaryPath:        summaryPath,
		RequestCount:       metricValue(summary, "http_reqs", "count"),
		ErrorRate:          maxMetric(summary, []metricRef{{"scenario_error_rate", "rate"}, {"checks", "rate"}}),
		TransportErrorRate: metricValue(summary, "scenario_transport_error_rate", "rate"),
		Stable:             true,
	}

	switch scenario {
	case "smoke":
		evaluation.NonStreamP95Ms = metricValue(summary, "amp_req_latency_ms", "p(95)")
		if evaluation.ErrorRate > 0 || evaluation.TransportErrorRate > 0 {
			evaluation.Stable = false
			evaluation.Breakpoint = true
			evaluation.Reasons = append(evaluation.Reasons, "smoke scenario observed request failures")
		}
	case "nonstream":
		evaluation.NonStreamP95Ms = metricValue(summary, "amp_req_latency_ms", "p(95)")
		evaluation.applyCommonThresholds()
		if evaluation.NonStreamP95Ms > 5000 {
			evaluation.markBreakpoint(fmt.Sprintf("non-stream p95 exceeded 5000ms (%.2fms)", evaluation.NonStreamP95Ms))
		}
	case "stream":
		evaluation.StreamFirstByteP95Ms = metricValue(summary, "stream_first_byte_ms", "p(95)")
		evaluation.applyCommonThresholds()
		if evaluation.StreamFirstByteP95Ms > 3000 {
			evaluation.markBreakpoint(fmt.Sprintf("stream first-byte p95 exceeded 3000ms (%.2fms)", evaluation.StreamFirstByteP95Ms))
		}
	case "mixed":
		evaluation.NonStreamP95Ms = metricValue(summary, "mixed_nonstream_latency_ms", "p(95)")
		evaluation.StreamFirstByteP95Ms = metricValue(summary, "mixed_stream_first_byte_ms", "p(95)")
		evaluation.applyCommonThresholds()
		if evaluation.NonStreamP95Ms > 5000 {
			evaluation.markBreakpoint(fmt.Sprintf("mixed non-stream p95 exceeded 5000ms (%.2fms)", evaluation.NonStreamP95Ms))
		}
		if evaluation.StreamFirstByteP95Ms > 3000 {
			evaluation.markBreakpoint(fmt.Sprintf("mixed stream first-byte p95 exceeded 3000ms (%.2fms)", evaluation.StreamFirstByteP95Ms))
		}
	default:
		return nil, fmt.Errorf("unsupported scenario %q", scenario)
	}

	if evaluation.Stable && len(evaluation.Reasons) == 0 {
		evaluation.Reasons = []string{"within configured breakpoint thresholds"}
	}
	return evaluation, nil
}

func (e *StepEvaluation) applyCommonThresholds() {
	if e.ErrorRate >= 0.01 {
		e.markBreakpoint(fmt.Sprintf("error rate exceeded 1%% (%.4f)", e.ErrorRate))
	}
	if e.TransportErrorRate >= 0.005 {
		e.markBreakpoint(fmt.Sprintf("transport error rate exceeded 0.5%% (%.4f)", e.TransportErrorRate))
	}
}

func (e *StepEvaluation) markBreakpoint(reason string) {
	e.Stable = false
	e.Breakpoint = true
	e.Reasons = append(e.Reasons, reason)
}

func WriteJSON(path string, value any) error {
	directory := filepath.Dir(path)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func LoadSummary(path string) (*SummaryExport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read k6 summary: %w", err)
	}

	var summary SummaryExport
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, fmt.Errorf("decode k6 summary: %w", err)
	}
	if len(summary.Metrics) == 0 {
		return nil, fmt.Errorf("k6 summary %s has no metrics", path)
	}
	return &summary, nil
}

func LoadMeta(path string) (*SessionMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func LoadEvaluations(directory string) ([]StepEvaluation, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	evaluations := make([]StepEvaluation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var evaluation StepEvaluation
		if err := json.Unmarshal(raw, &evaluation); err != nil {
			return nil, err
		}
		evaluations = append(evaluations, evaluation)
	}

	sort.Slice(evaluations, func(i, j int) bool {
		if evaluations[i].Scenario == evaluations[j].Scenario {
			return evaluations[i].Label < evaluations[j].Label
		}
		return evaluations[i].Scenario < evaluations[j].Scenario
	})
	return evaluations, nil
}

func BuildMarkdownReport(meta *SessionMeta, evaluations []StepEvaluation, resultsDir string) string {
	var builder strings.Builder

	builder.WriteString("# Performance Load Test Report\n\n")
	if meta != nil {
		builder.WriteString("## Environment\n\n")
		builder.WriteString(fmt.Sprintf("- Started at: `%s`\n", meta.StartedAt.Format(time.RFC3339)))
		builder.WriteString(fmt.Sprintf("- Profile: `%s`\n", meta.Profile))
		builder.WriteString(fmt.Sprintf("- App URL: `%s`\n", meta.AppURL))
		builder.WriteString(fmt.Sprintf("- Upstream URL: `%s`\n", meta.UpstreamURL))
		builder.WriteString(fmt.Sprintf("- Seeded users: `%d`\n", meta.UserCount))
		builder.WriteString(fmt.Sprintf("- Active key pool: `%d`\n", meta.ActiveKeyPool))
		builder.WriteString(fmt.Sprintf("- Request detail enabled: `%t`\n\n", meta.RequestDetailEnabled))
	}

	builder.WriteString("## Stable Ceilings\n\n")
	ceilings := stableCeilings(evaluations)
	for _, scenario := range []string{"smoke", "nonstream", "stream", "mixed"} {
		value := ceilings[scenario]
		if value == "" {
			value = "none"
		}
		builder.WriteString(fmt.Sprintf("- %s: `%s`\n", scenario, value))
	}
	builder.WriteString("\n")

	builder.WriteString("## Step Results\n\n")
	for _, scenario := range []string{"smoke", "nonstream", "stream", "mixed"} {
		filtered := filterScenario(evaluations, scenario)
		if len(filtered) == 0 {
			continue
		}
		builder.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(scenario)))
		builder.WriteString("| Step | Stable | Error Rate | Transport Error Rate | Non-stream p95 | Stream first-byte p95 | Notes |\n")
		builder.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, evaluation := range filtered {
			notes := strings.Join(evaluation.Reasons, "; ")
			if notes == "" {
				notes = "n/a"
			}
			builder.WriteString(fmt.Sprintf(
				"| %s | %t | %.4f | %.4f | %.2fms | %.2fms | %s |\n",
				evaluation.Label,
				evaluation.Stable,
				evaluation.ErrorRate,
				evaluation.TransportErrorRate,
				evaluation.NonStreamP95Ms,
				evaluation.StreamFirstByteP95Ms,
				notes,
			))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Artifacts\n\n")
	builder.WriteString(fmt.Sprintf("- Result directory: `%s`\n", resultsDir))
	builder.WriteString(fmt.Sprintf("- k6 summaries: `%s`\n", filepath.Join(resultsDir, "k6")))
	builder.WriteString(fmt.Sprintf("- Evaluations: `%s`\n", filepath.Join(resultsDir, "evaluations")))
	builder.WriteString(fmt.Sprintf("- SQL snapshots: `%s`\n", filepath.Join(resultsDir, "sql")))
	builder.WriteString(fmt.Sprintf("- Container logs: `%s`\n", filepath.Join(resultsDir, "logs")))

	return builder.String()
}

type metricRef struct {
	name string
	key  string
}

func maxMetric(summary *SummaryExport, refs []metricRef) float64 {
	max := 0.0
	for _, ref := range refs {
		value := metricValue(summary, ref.name, ref.key)
		if value > max {
			max = value
		}
	}
	return max
}

func metricValue(summary *SummaryExport, name, key string) float64 {
	metric, ok := summary.Metrics[name]
	if !ok {
		return 0
	}
	if metric.Values == nil {
		return metric.Legacy[key]
	}
	return metric.Values[key]
}

func stableCeilings(evaluations []StepEvaluation) map[string]string {
	result := make(map[string]string)
	for _, scenario := range []string{"smoke", "nonstream", "stream", "mixed"} {
		for _, evaluation := range filterScenario(evaluations, scenario) {
			if evaluation.Stable {
				result[scenario] = evaluation.Label
				continue
			}
			break
		}
	}
	return result
}

func filterScenario(evaluations []StepEvaluation, scenario string) []StepEvaluation {
	filtered := make([]StepEvaluation, 0)
	for _, evaluation := range evaluations {
		if evaluation.Scenario == scenario {
			filtered = append(filtered, evaluation)
		}
	}
	return filtered
}
