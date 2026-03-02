package bot

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantSec float64
	}{
		{"24h", false, 86400},
		{"12h", false, 43200},
		{"1h30m", false, 5400},
		{"48h", false, 172800},
		{"30s", true, 0},   // too short
		{"10s", true, 0},   // too short
		{"1m", false, 60},  // minimum
		{"invalid", true, 0},
		{"", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && d.Seconds() != tt.wantSec {
				t.Errorf("parseDuration(%q) = %v, want %vs", tt.input, d, tt.wantSec)
			}
		})
	}
}

func TestGetStringOpt(t *testing.T) {
	// Since we can't easily construct discordgo option objects, 
	// test with nil to confirm it returns empty string
	result := getStringOpt(nil, "test")
	if result != "" {
		t.Errorf("expected empty string for nil opts, got %q", result)
	}
}

func TestGetIntOpt(t *testing.T) {
	result := getIntOpt(nil, "test")
	if result != 0 {
		t.Errorf("expected 0 for nil opts, got %d", result)
	}
}

func TestBytesReader(t *testing.T) {
	data := []byte("hello world")
	r := bytesReader(data)
	if r == nil {
		t.Error("expected non-nil reader")
	}

	buf := make([]byte, len(data))
	n, _ := r.Read(buf)
	if n != len(data) {
		t.Errorf("read %d bytes, want %d", n, len(data))
	}
	if string(buf) != "hello world" {
		t.Errorf("read %q, want %q", buf, "hello world")
	}
}

func TestParseDuration_MinimumEnforced(t *testing.T) {
	_, err := parseDuration("30s")
	if err == nil {
		t.Error("expected error for duration under 1 minute")
	}

	d, err := parseDuration("1m")
	if err != nil {
		t.Errorf("1m should be valid: %v", err)
	}
	if d != 1*time.Minute {
		t.Errorf("1m = %v, want 1m0s", d)
	}
}
