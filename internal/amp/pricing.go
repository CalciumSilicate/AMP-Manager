package amp

import (
	"context"
	"strings"

	"ampmanager/internal/model"

	"github.com/tidwall/gjson"
)

const (
	fastSpecialRateMultiplier = 2.0
	fastSpecialRateReason     = "FAST"
)

type PricingBreakdown struct {
	GroupMultiplier   float64
	ChannelMultiplier float64
	SpecialMultiplier float64
	SpecialReason     string
	TotalMultiplier   float64
}

func computePricingBreakdown(ctx context.Context, channel *model.Channel, requestBody []byte) PricingBreakdown {
	groupMultiplier := resolveGroupMultiplier(GetProxyConfig(ctx))

	channelMultiplier := 1.0
	if channel != nil && channel.RateMultiplier > 0 {
		channelMultiplier = channel.RateMultiplier
	}

	specialMultiplier := 1.0
	specialReason := ""
	if channel != nil &&
		channel.Type == model.ChannelTypeOpenAI &&
		channel.Endpoint == model.ChannelEndpointResponses &&
		strings.EqualFold(strings.TrimSpace(gjson.GetBytes(requestBody, "service_tier").String()), "priority") {
		specialMultiplier = fastSpecialRateMultiplier
		specialReason = fastSpecialRateReason
	}

	return PricingBreakdown{
		GroupMultiplier:   groupMultiplier,
		ChannelMultiplier: channelMultiplier,
		SpecialMultiplier: specialMultiplier,
		SpecialReason:     specialReason,
		TotalMultiplier:   groupMultiplier * channelMultiplier * specialMultiplier,
	}
}

func resolveGroupMultiplier(cfg *ProxyConfig) float64 {
	if cfg == nil {
		return 1
	}
	if cfg.GroupRateMultiplier != 0 || cfg.RateMultiplier == 0 {
		return cfg.GroupRateMultiplier
	}
	return cfg.RateMultiplier
}

func traceMultiplier(trace *RequestTrace) float64 {
	if trace == nil {
		return 1
	}
	if trace.GroupRateMultiplier == 0 && trace.ChannelRateMultiplier == 0 && trace.SpecialRateMultiplier == 0 {
		if trace.RateMultiplier == 0 {
			return 1
		}
		return trace.RateMultiplier
	}
	return trace.RateMultiplier
}
