package config

import "strings"

type ServerRole string

const (
	ServerRoleAll   ServerRole = "all"
	ServerRolePanel ServerRole = "panel"
	ServerRoleProxy ServerRole = "proxy"
)

func (c *Config) Role() ServerRole {
	if c == nil {
		return ServerRoleAll
	}
	return NormalizeServerRole(c.ServerRole)
}

func NormalizeServerRole(raw string) ServerRole {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ServerRolePanel):
		return ServerRolePanel
	case string(ServerRoleProxy):
		return ServerRoleProxy
	default:
		return ServerRoleAll
	}
}

func (r ServerRole) RunsPanel() bool {
	return r == ServerRoleAll || r == ServerRolePanel
}

func (r ServerRole) RunsProxy() bool {
	return r == ServerRoleAll || r == ServerRoleProxy
}
