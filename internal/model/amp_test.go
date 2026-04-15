package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestLogJSONOmitsHiddenBillingAndStreamingFields(t *testing.T) {
	billingGap := int64(7)
	logEntry := RequestLog{
		ID:                        "req-1",
		StatusCode:                200,
		LatencyMs:                 123,
		IsStreaming:               true,
		BillingStatus:             "settled",
		ChargedSubscriptionMicros: 10,
		ChargedBalanceMicros:      5,
		BillingGapMicros:          &billingGap,
	}

	payload, err := json.Marshal(logEntry)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	result := string(payload)
	for _, field := range []string{
		`"isStreaming"`,
		`"billingStatus"`,
		`"chargedSubscriptionMicros"`,
		`"chargedBalanceMicros"`,
		`"billingGapMicros"`,
	} {
		if strings.Contains(result, field) {
			t.Fatalf("expected %s to be omitted from request log JSON: %s", field, result)
		}
	}
}
