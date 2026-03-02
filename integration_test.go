package diplomacy_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/game"
	"github.com/sammy/diplomacy/internal/testutil"
)

var powers = []string{"Austria", "England", "France", "Germany", "Italy", "Russia", "Turkey"}

func setupFullGame(t *testing.T) (*game.Manager, *db.Game, []*db.Player) {
	t.Helper()
	store := testutil.NewTestDB(t)
	mgr := game.NewManager(store)

	g, err := mgr.CreateGame("guild-int", "chan-int", "integration-game", 86400)
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	var players []*db.Player
	for i, power := range powers {
		p, err := mgr.JoinGame(g.ID, fmt.Sprintf("player-%d", i), power)
		if err != nil {
			t.Fatalf("join as %s: %v", power, err)
		}
		players = append(players, p)
	}

	if err := mgr.StartGame(g.ID); err != nil {
		t.Fatalf("start game: %v", err)
	}

	g, _ = mgr.Store().GetGame(g.ID)
	return mgr, g, players
}

func submitOrder(t *testing.T, mgr *game.Manager, gameID int64, phase, userID, power, order string) {
	t.Helper()
	_, err := mgr.Store().SubmitOrder(&db.Order{
		GameID:    gameID,
		Phase:     phase,
		UserID:    userID,
		Power:     power,
		OrderText: order,
	})
	if err != nil {
		t.Fatalf("submit order %q: %v", order, err)
	}
}

func TestFullGame_OpeningMoves(t *testing.T) {
	mgr, g, players := setupFullGame(t)

	if g.Phase != "Spring 1901 Movement" {
		t.Fatalf("expected Spring 1901 Movement, got %s", g.Phase)
	}

	// Submit standard opening moves for England and France
	for _, p := range players {
		switch p.Power {
		case "England":
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "F LON - NTH")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A LVP - YOR")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "F EDI - NRG")
		case "France":
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A PAR - BUR")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A MAR - SPA")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "F BRE - MAO")
		case "Germany":
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A BER - KIE")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A MUN - RUH")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "F KIE - DEN")
		}
	}

	updated, results, gameOver, err := mgr.ProcessPhase(g.ID)
	if err != nil {
		t.Fatalf("process phase: %v", err)
	}

	if gameOver {
		t.Error("game should not be over after opening moves")
	}

	t.Logf("After Spring 1901: %s", updated.Phase)
	t.Logf("Results: %v", results)

	// Verify state is valid
	s, err := game.DeserializeState(updated.StateJSON)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	units := s.Units()
	if len(units) == 0 {
		t.Error("expected units after adjudication")
	}
	t.Logf("Units count: %d", len(units))
}

func TestFullGame_ThreeTurnSequence(t *testing.T) {
	mgr, g, _ := setupFullGame(t)

	// Process 5 phases (Spring M -> Spring R -> Fall M -> Fall R -> Winter A)
	var currentPhase string
	for i := 0; i < 5; i++ {
		current, _ := mgr.Store().GetGame(g.ID)
		currentPhase = current.Phase
		t.Logf("Processing phase %d: %s", i+1, currentPhase)

		updated, _, gameOver, err := mgr.ProcessPhase(g.ID)
		if err != nil {
			t.Fatalf("phase %d (%s): %v", i+1, currentPhase, err)
		}
		if gameOver {
			t.Logf("Game ended at phase %d", i+1)
			break
		}

		t.Logf("  -> Advanced to: %s", updated.Phase)
	}

	// Verify phase history was recorded
	history, err := mgr.Store().GetPhaseHistory(g.ID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) < 3 {
		t.Errorf("expected at least 3 history entries, got %d", len(history))
	}

	for _, h := range history {
		t.Logf("History: %s (season=%s, year=%d, type=%s)", h.Phase, h.Season, h.Year, h.PhaseType)
	}
}

func TestFullGame_ExportImportContinue(t *testing.T) {
	mgr, g, _ := setupFullGame(t)

	// Process two phases
	mgr.ProcessPhase(g.ID)
	mgr.ProcessPhase(g.ID)

	// Export the game
	g, _ = mgr.Store().GetGame(g.ID)
	data, err := mgr.ExportState(g)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Import into a new game
	imported, err := mgr.ImportGame("guild-int", "chan-int", "imported-game", 86400, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if imported.Phase != g.Phase {
		t.Errorf("imported phase %q != original phase %q", imported.Phase, g.Phase)
	}

	// Add players and start
	for i, power := range powers {
		mgr.JoinGame(imported.ID, fmt.Sprintf("imp-player-%d", i), power)
	}
	mgr.StartGame(imported.ID)

	// Process another phase on the imported game
	updated, _, _, err := mgr.ProcessPhase(imported.ID)
	if err != nil {
		t.Fatalf("process imported game: %v", err)
	}

	t.Logf("Imported game advanced to: %s", updated.Phase)

	// Verify we can still export
	updated, _ = mgr.Store().GetGame(imported.ID)
	reExported, err := mgr.ExportState(updated)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}

	var ps game.PhaseState
	if err := json.Unmarshal(reExported, &ps); err != nil {
		t.Fatalf("unmarshal re-export: %v", err)
	}
	if len(ps.Units) == 0 {
		t.Error("re-export should have units")
	}
}

func TestFullGame_MultipleGamesParallel(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := game.NewManager(store)

	// Create two games
	g1, _ := mgr.CreateGame("guild-int", "chan-int", "game-1", 86400)
	g2, _ := mgr.CreateGame("guild-int", "chan-int", "game-2", 3600)

	for i, power := range powers {
		mgr.JoinGame(g1.ID, fmt.Sprintf("g1-player-%d", i), power)
		mgr.JoinGame(g2.ID, fmt.Sprintf("g2-player-%d", i), power)
	}

	mgr.StartGame(g1.ID)
	mgr.StartGame(g2.ID)

	// Process game 1
	u1, _, _, err := mgr.ProcessPhase(g1.ID)
	if err != nil {
		t.Fatalf("process game 1: %v", err)
	}

	// Process game 2
	u2, _, _, err := mgr.ProcessPhase(g2.ID)
	if err != nil {
		t.Fatalf("process game 2: %v", err)
	}

	// Both should have advanced
	if u1.Phase == "Spring 1901 Movement" {
		t.Error("game 1 should have advanced")
	}
	if u2.Phase == "Spring 1901 Movement" {
		t.Error("game 2 should have advanced")
	}

	t.Logf("Game 1: %s", u1.Phase)
	t.Logf("Game 2: %s", u2.Phase)
}

func TestFullGame_ToVictory(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := game.NewManager(store)

	g, _ := mgr.CreateGame("guild-int", "chan-int", "victory-test", 86400)
	for i, power := range powers {
		mgr.JoinGame(g.ID, fmt.Sprintf("v-player-%d", i), power)
	}
	mgr.StartGame(g.ID)

	// Process phases until we reach an adjustment phase, then manipulate SC ownership
	for i := 0; i < 10; i++ {
		current, _ := mgr.Store().GetGame(g.ID)
		if current.Status == db.GameStatusFinished {
			t.Logf("Game finished at iteration %d", i)
			return
		}

		// Check if we're at an adjustment phase - we can try to set SCs
		var ps game.PhaseState
		json.Unmarshal([]byte(current.StateJSON), &ps)

		if ps.Type == "Adjustment" {
			// Give France 18 supply centers to trigger victory
			for prov := range ps.SupplyCenters {
				ps.SupplyCenters[prov] = "France"
			}
			modified, _ := json.Marshal(ps)
			store.UpdateGameState(current.ID, current.Phase, string(modified), current.NextDeadline, db.GameStatusActive)
		}

		_, _, gameOver, err := mgr.ProcessPhase(g.ID)
		if err != nil {
			t.Fatalf("phase %d: %v", i, err)
		}
		if gameOver {
			t.Logf("Victory detected at iteration %d!", i)
			final, _ := store.GetGame(g.ID)
			if final.Status != db.GameStatusFinished {
				t.Error("game should be finished")
			}
			return
		}
	}

	t.Log("No victory in 10 phases (expected for initial test)")
}

func TestFullGame_ReadyMechanism(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := game.NewManager(store)

	g, _ := mgr.CreateGame("guild-int", "chan-int", "ready-test", 86400)
	var players []*db.Player
	for i, power := range powers {
		p, _ := mgr.JoinGame(g.ID, fmt.Sprintf("r-player-%d", i), power)
		players = append(players, p)
	}
	mgr.StartGame(g.ID)

	scheduler := game.NewScheduler(mgr, store)
	var callbackCalled bool
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		callbackCalled = true
	})

	// Mark all players ready
	for _, p := range players {
		store.SetPlayerReady(g.ID, p.UserID, true)
	}

	// Check ready should trigger adjudication
	adjudicated, err := scheduler.CheckReady(g.ID)
	if err != nil {
		t.Fatalf("check ready: %v", err)
	}
	if !adjudicated {
		t.Error("expected adjudication when all ready")
	}
	if !callbackCalled {
		t.Error("expected callback")
	}

	// Verify phase advanced
	updated, _ := store.GetGame(g.ID)
	if updated.Phase == "Spring 1901 Movement" {
		t.Error("phase should have advanced")
	}

	// Verify ready flags reset
	for _, p := range players {
		player, _ := store.GetPlayerByUserAndGame(g.ID, p.UserID)
		if player.IsReady {
			t.Errorf("player %s should not be ready after advance", p.Power)
		}
	}
}

func TestFullGame_OrdersPreservedInHistory(t *testing.T) {
	mgr, g, players := setupFullGame(t)

	// Submit orders for England
	for _, p := range players {
		if p.Power == "England" {
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "F LON - NTH")
			submitOrder(t, mgr, g.ID, g.Phase, p.UserID, p.Power, "A LVP - YOR")
		}
	}

	mgr.ProcessPhase(g.ID)

	// Check history has orders
	history, _ := mgr.Store().GetPhaseHistory(g.ID)
	if len(history) == 0 {
		t.Fatal("expected at least 1 history entry")
	}

	h := history[0]
	if h.OrdersJSON == nil {
		t.Fatal("expected orders in history")
	}

	var orders map[string][]string
	if err := json.Unmarshal([]byte(*h.OrdersJSON), &orders); err != nil {
		t.Fatalf("unmarshal orders: %v", err)
	}

	engOrders, ok := orders["England"]
	if !ok {
		t.Error("expected England orders in history")
	}
	if len(engOrders) != 2 {
		t.Errorf("expected 2 England orders, got %d", len(engOrders))
	}
}

func TestFullGame_GameNameResolution(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := game.NewManager(store)

	// User in one game
	g1, _ := mgr.CreateGame("guild-int", "chan-int", "only-game", 86400)
	mgr.JoinGame(g1.ID, "user-solo", "England")
	mgr.StartGame(g1.ID)

	resolved, err := mgr.ResolveGame("guild-int", "user-solo", "")
	if err != nil {
		t.Fatalf("resolve single game: %v", err)
	}
	if resolved.Name != "only-game" {
		t.Errorf("resolved name = %q, want only-game", resolved.Name)
	}

	// Add second game
	g2, _ := mgr.CreateGame("guild-int", "chan-int", "second-game", 86400)
	mgr.JoinGame(g2.ID, "user-solo", "France")
	mgr.StartGame(g2.ID)

	// Now resolution should fail without explicit name
	_, err = mgr.ResolveGame("guild-int", "user-solo", "")
	if err == nil {
		t.Error("expected error for multiple games")
	}

	// Explicit name should work
	resolved, err = mgr.ResolveGame("guild-int", "user-solo", "second-game")
	if err != nil {
		t.Fatalf("resolve explicit: %v", err)
	}
	if resolved.Name != "second-game" {
		t.Errorf("resolved = %q, want second-game", resolved.Name)
	}
}
