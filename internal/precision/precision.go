package precision

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	DefaultMultiplierPPM = int64(1_000_000)
	multiplierScale      = int32(6)
	priceScale           = int32(6)  // micros
	pricePerTokenShift   = int32(12) // USD/token -> micros per million tokens
)

type DecimalString string

func (d *DecimalString) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*d = ""
		return nil
	}

	if raw[0] == '"' {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		*d = DecimalString(strings.TrimSpace(decoded))
		return nil
	}

	*d = DecimalString(raw)
	return nil
}

func (d DecimalString) String() string {
	return strings.TrimSpace(string(d))
}

func (d DecimalString) Empty() bool {
	return d.String() == ""
}

func ParseMultiplierToPPM(input DecimalString) (int64, error) {
	raw := input.String()
	if raw == "" {
		return DefaultMultiplierPPM, nil
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid multiplier %q", raw)
	}
	if value.IsNegative() {
		return 0, fmt.Errorf("multiplier must be non-negative")
	}
	return value.Shift(multiplierScale).Round(0).IntPart(), nil
}

func MultiplierPPMToFloat64(ppm int64) float64 {
	if ppm <= 0 {
		ppm = DefaultMultiplierPPM
	}
	value := decimal.NewFromInt(ppm).Shift(-multiplierScale)
	out, _ := value.Float64()
	return out
}

func MultiplierPPMToString(ppm int64) string {
	if ppm <= 0 {
		ppm = DefaultMultiplierPPM
	}
	return decimal.NewFromInt(ppm).Shift(-multiplierScale).String()
}

func FloatMultiplierToPPM(value float64) int64 {
	if value <= 0 {
		return DefaultMultiplierPPM
	}
	return decimal.NewFromFloat(value).Shift(multiplierScale).Round(0).IntPart()
}

func ParseUSDToMicros(input DecimalString) (int64, error) {
	raw := input.String()
	if raw == "" {
		return 0, nil
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid usd amount %q", raw)
	}
	return value.Shift(priceScale).Round(0).IntPart(), nil
}

func ParseUSDToCents(input DecimalString) (int64, error) {
	raw := input.String()
	if raw == "" {
		return 0, nil
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid usd amount %q", raw)
	}
	return value.Shift(2).Round(0).IntPart(), nil
}

func CentsToUSDString(cents int64) string {
	return decimal.NewFromInt(cents).Shift(-2).StringFixed(2)
}

func MicrosToUSDString(micros int64) string {
	return decimal.NewFromInt(micros).Shift(-priceScale).StringFixed(6)
}

func CostPerTokenToMicrosPerMillion(value float64) int64 {
	if value <= 0 {
		return 0
	}
	return decimal.NewFromFloat(value).Shift(pricePerTokenShift).Round(0).IntPart()
}

func MicrosPerMillionToCostPerToken(value int64) float64 {
	if value <= 0 {
		return 0
	}
	out, _ := decimal.NewFromInt(value).Shift(-pricePerTokenShift).Float64()
	return out
}

func ParseUSDPerMillionToMicros(input DecimalString) (int64, error) {
	raw := input.String()
	if raw == "" {
		return 0, nil
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid price per million %q", raw)
	}
	if value.IsNegative() {
		return 0, fmt.Errorf("price per million must be non-negative")
	}
	return value.Shift(priceScale).Round(0).IntPart(), nil
}

func MicrosPerMillionToUSDString(value int64) string {
	if value == 0 {
		return "0"
	}
	return decimal.NewFromInt(value).Shift(-priceScale).String()
}
