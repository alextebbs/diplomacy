package game

import (
	"encoding/json"
	"testing"

	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"
)

func TestSerializeDeserializeState(t *testing.T) {
	s, err := classical.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	serialized, err := SerializeState(s)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	if serialized == "" {
		t.Fatal("serialized state is empty")
	}

	// Verify JSON structure
	var ps PhaseState
	if err := json.Unmarshal([]byte(serialized), &ps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if ps.Season != godip.Spring {
		t.Errorf("season = %q, want Spring", ps.Season)
	}
	if ps.Year != 1901 {
		t.Errorf("year = %d, want 1901", ps.Year)
	}
	if ps.Type != godip.Movement {
		t.Errorf("type = %q, want Movement", ps.Type)
	}

	// Verify round-trip
	restored, err := DeserializeState(serialized)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	phase := restored.Phase()
	if phase.Season() != godip.Spring {
		t.Errorf("restored season = %q, want Spring", phase.Season())
	}
	if phase.Year() != 1901 {
		t.Errorf("restored year = %d, want 1901", phase.Year())
	}
}

func TestFormatPhaseName(t *testing.T) {
	tests := []struct {
		season    godip.Season
		year      int
		phaseType godip.PhaseType
		want      string
	}{
		{godip.Spring, 1901, godip.Movement, "Spring 1901 Movement"},
		{godip.Fall, 1902, godip.Retreat, "Fall 1902 Retreat"},
		{godip.Fall, 1901, godip.Adjustment, "Fall 1901 Adjustment"},
	}

	for _, tt := range tests {
		got := FormatPhaseName(tt.season, tt.year, tt.phaseType)
		if got != tt.want {
			t.Errorf("FormatPhaseName(%s, %d, %s) = %q, want %q", tt.season, tt.year, tt.phaseType, got, tt.want)
		}
	}
}

func TestNewPhaseState(t *testing.T) {
	s, _ := classical.Start()
	ps := NewPhaseState(s)

	if ps.Season != godip.Spring {
		t.Errorf("season = %q, want Spring", ps.Season)
	}
	if ps.Year != 1901 {
		t.Errorf("year = %d, want 1901", ps.Year)
	}
	if len(ps.Units) == 0 {
		t.Error("expected units in phase state")
	}
	if len(ps.SupplyCenters) == 0 {
		t.Error("expected supply centers in phase state")
	}
}

func TestPhaseStateToState(t *testing.T) {
	s, _ := classical.Start()
	ps := NewPhaseState(s)

	restored, err := ps.ToState()
	if err != nil {
		t.Fatalf("to state: %v", err)
	}

	phase := restored.Phase()
	if phase.Year() != 1901 {
		t.Errorf("year = %d, want 1901", phase.Year())
	}
}

func TestDeserializeInvalidJSON(t *testing.T) {
	_, err := DeserializeState("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
