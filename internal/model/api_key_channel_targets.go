package model

import (
	"encoding/json"
	"strings"
)

type APIKeyChannelTarget struct {
	ChannelID string `json:"channelId"`
	Priority  int    `json:"priority"`
}

func NormalizeAPIKeyChannelTargets(targets []APIKeyChannelTarget) []APIKeyChannelTarget {
	if len(targets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(targets))
	result := make([]APIKeyChannelTarget, 0, len(targets))
	for _, target := range targets {
		channelID := strings.TrimSpace(target.ChannelID)
		if channelID == "" {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		priority := target.Priority
		if priority <= 0 {
			priority = 100
		}
		result = append(result, APIKeyChannelTarget{
			ChannelID: channelID,
			Priority:  priority,
		})
	}
	return result
}

func ParseAPIKeyChannelTargets(raw string) []APIKeyChannelTarget {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var targets []APIKeyChannelTarget
	if err := json.Unmarshal([]byte(trimmed), &targets); err != nil {
		return nil
	}
	return NormalizeAPIKeyChannelTargets(targets)
}

func MustMarshalAPIKeyChannelTargets(targets []APIKeyChannelTarget) string {
	normalized := NormalizeAPIKeyChannelTargets(targets)
	if len(normalized) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
