package amp

import "testing"

func TestDetectLocalToolQuery(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		wantTool string
		wantOK   bool
	}{
		{"empty", "", "", false},
		{"webSearch2", "webSearch2", webSearchQuery, true},
		{"new_prefix_webSearch2", "mcp__tools__mcp-websearch2", webSearchQuery, true},
		{"extract", "extractWebPageContent", extractWebPageContentQuery, true},
		{"new_prefix_extract", "mcp__tools__mcp-extractwebpagecontent", extractWebPageContentQuery, true},
		{"with_params", "foo=bar&webSearch2&x=y", webSearchQuery, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTool, gotOK := detectLocalToolQuery(tc.rawQuery)
			if gotOK != tc.wantOK {
				t.Fatalf("ok mismatch: got %v want %v", gotOK, tc.wantOK)
			}
			if gotTool != tc.wantTool {
				t.Fatalf("tool mismatch: got %q want %q", gotTool, tc.wantTool)
			}
		})
	}
}

func TestIsAdRequest(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		want     bool
	}{
		{"empty", "", false},
		{"normal query", "webSearch2", false},
		{"recordAdImpressionEnd", "recordAdImpressionEnd", true},
		{"recordAdImpressionStart", "recordAdImpressionStart", true},
		{"getCurrentAd", "getCurrentAd", true},
		{"ad with params", "foo=bar&getCurrentAd&x=y", true},
		{"not ad", "recordSomethingElse", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isAdRequest(tc.rawQuery)
			if got != tc.want {
				t.Fatalf("isAdRequest(%q) = %v, want %v", tc.rawQuery, got, tc.want)
			}
		})
	}
}

func TestDetectAdEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		want     string
	}{
		{"empty", "", ""},
		{"getCurrentAd", "getCurrentAd", "getCurrentAd"},
		{"recordAdImpressionStart", "recordAdImpressionStart", "recordAdImpressionStart"},
		{"recordAdImpressionEnd", "recordAdImpressionEnd", "recordAdImpressionEnd"},
		{"with other params", "foo=bar&recordAdImpressionEnd&x=y", "recordAdImpressionEnd"},
		{"not ad", "webSearch2", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectAdEndpoint(tc.rawQuery)
			if got != tc.want {
				t.Fatalf("detectAdEndpoint(%q) = %q, want %q", tc.rawQuery, got, tc.want)
			}
		})
	}
}

func TestBuildAdResponse(t *testing.T) {
	resp := buildAdResponse("getCurrentAd")
	if resp["ok"] != true {
		t.Fatal("getCurrentAd response should have ok=true")
	}
	if resp["result"] != nil {
		t.Fatal("getCurrentAd response should have result=nil (no ad available)")
	}

	resp = buildAdResponse("recordAdImpressionStart")
	if resp["ok"] != true {
		t.Fatal("recordAdImpressionStart response should have ok=true")
	}
	if resp["creditsConsumed"] != "0" {
		t.Fatal("recordAdImpressionStart response should have creditsConsumed=0")
	}

	resp = buildAdResponse("recordAdImpressionEnd")
	if resp["ok"] != true {
		t.Fatal("recordAdImpressionEnd response should have ok=true")
	}
	if resp["creditsConsumed"] != "0" {
		t.Fatal("recordAdImpressionEnd response should have creditsConsumed=0")
	}
}
