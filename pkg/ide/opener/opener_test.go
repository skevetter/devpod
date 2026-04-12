package opener

import (
	"testing"
)

func TestParseAddressAndPort_Empty(t *testing.T) {
	addr, p, err := ParseAddressAndPort("", 10000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p < 10000 {
		t.Errorf("expected port >= 10000, got %d", p)
	}
	if addr == "" {
		t.Error("expected non-empty address")
	}
}

func TestParseAddressAndPort_Explicit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantAddr string
		wantPort int
	}{
		{"host:port", "127.0.0.1:8080", "127.0.0.1:8080", 8080},
		{"localhost:port", "localhost:3000", "localhost:3000", 3000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, p, err := ParseAddressAndPort(tt.input, 10000)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
			if p != tt.wantPort {
				t.Errorf("port = %d, want %d", p, tt.wantPort)
			}
		})
	}
}

func TestParseAddressAndPort_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"missing port", "127.0.0.1"},
		{"invalid format", "not:a:valid:address"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseAddressAndPort(tt.input, 10000)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
