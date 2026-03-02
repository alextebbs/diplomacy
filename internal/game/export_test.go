package game

import (
	"encoding/json"
	"testing"

	"github.com/sammy/diplomacy/internal/testutil"
	"github.com/zond/godip/variants/classical"
)

func TestExportState(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	created, _ := mgr.CreateGame("guild1", "chan1", "export-test", 86400)
	g, _ := store.GetGame(created.ID)

	data, err := mgr.ExportState(g)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty export data")
	}

	// Verify it's valid JSON with expected fields
	var ps PhaseState
	if err := json.Unmarshal(data, &ps); err != nil {
		t.Fatalf("unmarshal exported data: %v", err)
	}
	if ps.Season != "Spring" {
		t.Errorf("season = %q, want Spring", ps.Season)
	}
	if ps.Year != 1901 {
		t.Errorf("year = %d, want 1901", ps.Year)
	}
	if ps.Type != "Movement" {
		t.Errorf("type = %q, want Movement", ps.Type)
	}
	if len(ps.Units) == 0 {
		t.Error("expected units in export")
	}
	if len(ps.SupplyCenters) == 0 {
		t.Error("expected supply centers in export")
	}
}

func TestImportState(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	// Create a valid state JSON
	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	g, err := mgr.ImportGame("guild1", "chan1", "imported", 86400, []byte(stateJSON))
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if g.Name != "imported" {
		t.Errorf("name = %q, want imported", g.Name)
	}
	if g.Phase != "Spring 1901 Movement" {
		t.Errorf("phase = %q, want Spring 1901 Movement", g.Phase)
	}
}

func TestRoundTrip(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	// Create and export a game
	created, _ := mgr.CreateGame("guild1", "chan1", "original", 86400)
	g, _ := store.GetGame(created.ID)
	exported, _ := mgr.ExportState(g)

	// Import it
	imported, err := mgr.ImportGame("guild1", "chan1", "copy", 86400, exported)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	// Export the copy
	importedGame, _ := store.GetGame(imported.ID)
	reExported, _ := mgr.ExportState(importedGame)

	// The state JSON should match
	var ps1, ps2 PhaseState
	json.Unmarshal(exported, &ps1)
	json.Unmarshal(reExported, &ps2)

	if ps1.Season != ps2.Season || ps1.Year != ps2.Year || ps1.Type != ps2.Type {
		t.Errorf("round-trip mismatch: %v vs %v", ps1, ps2)
	}
	if len(ps1.Units) != len(ps2.Units) {
		t.Errorf("units count: %d vs %d", len(ps1.Units), len(ps2.Units))
	}
}

func TestImportInvalidJSON(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	_, err := mgr.ImportGame("guild1", "chan1", "bad", 86400, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestImportDuplicateName(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	mgr.CreateGame("guild1", "chan1", "existing", 86400)

	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	_, err := mgr.ImportGame("guild1", "chan1", "existing", 86400, []byte(stateJSON))
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestImportMidGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	// Build a mid-game state by taking the real starting state and adjusting the phase
	s, _ := classical.Start()
	ps := NewPhaseState(s)
	ps.Season = "Fall"
	ps.Year = 1903
	ps.Type = "Movement"

	data, _ := json.Marshal(ps)

	g, err := mgr.ImportGame("guild1", "chan1", "midgame", 86400, data)
	if err != nil {
		t.Fatalf("import mid-game: %v", err)
	}
	if g.Phase != "Fall 1903 Movement" {
		t.Errorf("phase = %q, want Fall 1903 Movement", g.Phase)
	}
}
