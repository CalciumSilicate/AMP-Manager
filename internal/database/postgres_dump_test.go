package database

import "testing"

func TestCanUseDockerPostgresFallback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "", want: true},
		{host: "localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "::1", want: true},
		{host: "postgres", want: true},
		{host: "ampmanager-postgres", want: true},
		{host: "db.internal.example.com", want: false},
		{host: "10.0.0.15", want: false},
	}

	for _, tt := range tests {
		if got := canUseDockerPostgresFallback(tt.host); got != tt.want {
			t.Fatalf("canUseDockerPostgresFallback(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
