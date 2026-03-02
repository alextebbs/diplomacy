package game

import (
	"testing"
)

func TestParseRawOrder(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		// Valid move orders
		{"A PAR - BUR", []string{"par", "Move", "bur"}, false},
		{"F BRE - ENG", []string{"bre", "Move", "eng"}, false},
		{"a par - bur", []string{"par", "Move", "bur"}, false},

		// Valid hold orders
		{"A PAR H", []string{"par", "Hold"}, false},
		{"A PAR HOLD", []string{"par", "Hold"}, false},

		// Valid support orders
		{"A MAR S A PAR", []string{"mar", "Support", "par"}, false},
		{"A MAR S A PAR - BUR", []string{"mar", "Support", "par", "bur"}, false},

		// Valid convoy orders
		{"F NTH C A LON - NWY", []string{"nth", "Convoy", "lon", "nwy"}, false},

		// Valid convoy move (VIA)
		{"A LON - NWY VIA", []string{"lon", "MoveViaConvoy", "nwy"}, false},

		// Coast notation
		{"F STP/SC - BOT", []string{"stp/sc", "Move", "bot"}, false},

		// Disband
		{"A PAR D", []string{"par", "Disband"}, false},

		// Errors
		{"", nil, true},
		{"gibberish", nil, true},
		{"X PAR - BUR", nil, true},
		{"A", nil, true},
		{"A PAR", nil, true},
		{"A PAR -", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRawOrder(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRawOrder(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParseRawOrder(%q) = %v, want %v", tt.input, got, tt.want)
					return
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("ParseRawOrder(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestParseMultipleOrders(t *testing.T) {
	// Multiple orders separated by commas
	result, err := ParseMultipleOrders("A PAR - BUR, F BRE - ENG")
	if err != nil {
		t.Fatalf("parse multiple: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d orders, want 2", len(result))
	}

	// Single order
	result, err = ParseMultipleOrders("A PAR H")
	if err != nil {
		t.Fatalf("parse single: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("got %d orders, want 1", len(result))
	}

	// Empty
	_, err = ParseMultipleOrders("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseAndValidateOrder(t *testing.T) {
	// Valid order through godip parser
	tokens, err := ParseAndValidateOrder("A PAR - BUR")
	if err != nil {
		t.Fatalf("validate A PAR - BUR: %v", err)
	}
	if len(tokens) == 0 {
		t.Error("expected non-empty tokens")
	}

	// Valid hold
	tokens, err = ParseAndValidateOrder("A PAR H")
	if err != nil {
		t.Fatalf("validate A PAR H: %v", err)
	}
	if tokens[1] != "Hold" {
		t.Errorf("action = %q, want Hold", tokens[1])
	}
}
