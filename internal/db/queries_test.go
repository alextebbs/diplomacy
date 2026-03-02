package db

import (
	"testing"
	"time"
)

func createTestGame(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	g := &Game{
		GuildID: "guild1", ChannelID: "chan1", Name: name, Variant: "Classical",
		Phase: "Spring 1901 Movement", StateJSON: `{"Season":"Spring","Year":1901,"Type":"Movement"}`,
		TurnDuration: 86400, Status: GameStatusPending,
	}
	id, err := s.CreateGame(g)
	if err != nil {
		t.Fatalf("create game %s: %v", name, err)
	}
	return id
}

func TestCreateAndGetGame(t *testing.T) {
	s := newTestStore(t)
	id := createTestGame(t, s, "test-game")

	g, err := s.GetGame(id)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if g == nil {
		t.Fatal("game not found")
	}
	if g.Name != "test-game" {
		t.Errorf("name = %q, want %q", g.Name, "test-game")
	}
	if g.Status != GameStatusPending {
		t.Errorf("status = %q, want %q", g.Status, GameStatusPending)
	}
	if g.Variant != "Classical" {
		t.Errorf("variant = %q, want %q", g.Variant, "Classical")
	}
}

func TestGetGameByName(t *testing.T) {
	s := newTestStore(t)
	createTestGame(t, s, "my-game")

	g, err := s.GetGameByName("guild1", "my-game")
	if err != nil {
		t.Fatalf("get game by name: %v", err)
	}
	if g == nil {
		t.Fatal("game not found")
	}
	if g.Name != "my-game" {
		t.Errorf("name = %q, want %q", g.Name, "my-game")
	}

	// Nonexistent
	g, err = s.GetGameByName("guild1", "nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g != nil {
		t.Error("expected nil for nonexistent game")
	}
}

func TestGetGame_NotFound(t *testing.T) {
	s := newTestStore(t)
	g, err := s.GetGame(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g != nil {
		t.Error("expected nil for nonexistent game")
	}
}

func TestListGamesByGuild(t *testing.T) {
	s := newTestStore(t)
	createTestGame(t, s, "game-a")
	createTestGame(t, s, "game-b")

	// Different guild
	g := &Game{
		GuildID: "guild2", Name: "other-game", Variant: "Classical",
		Phase: "Spring 1901 Movement", StateJSON: "{}", TurnDuration: 86400, Status: GameStatusPending,
	}
	s.CreateGame(g)

	games, err := s.ListGamesByGuild("guild1")
	if err != nil {
		t.Fatalf("list games: %v", err)
	}
	if len(games) != 2 {
		t.Errorf("got %d games, want 2", len(games))
	}
}

func TestListActiveGamesByGuild(t *testing.T) {
	s := newTestStore(t)
	id1 := createTestGame(t, s, "active-game")
	createTestGame(t, s, "pending-game")

	// Make one active
	s.UpdateGameState(id1, "Spring 1901 Movement", "{}", nil, GameStatusActive)

	games, err := s.ListActiveGamesByGuild("guild1")
	if err != nil {
		t.Fatalf("list active games: %v", err)
	}
	// Both pending and active are in the "active" list per query
	if len(games) != 2 {
		t.Errorf("got %d games, want 2 (pending + active)", len(games))
	}
}

func TestUpdateGameState(t *testing.T) {
	s := newTestStore(t)
	id := createTestGame(t, s, "update-test")

	deadline := time.Now().Add(24 * time.Hour)
	err := s.UpdateGameState(id, "Fall 1901 Movement", `{"new":"state"}`, &deadline, GameStatusActive)
	if err != nil {
		t.Fatalf("update game state: %v", err)
	}

	g, _ := s.GetGame(id)
	if g.Phase != "Fall 1901 Movement" {
		t.Errorf("phase = %q, want %q", g.Phase, "Fall 1901 Movement")
	}
	if g.Status != GameStatusActive {
		t.Errorf("status = %q, want %q", g.Status, GameStatusActive)
	}
	if g.NextDeadline == nil {
		t.Error("expected non-nil deadline")
	}
}

func TestUpdateGameSettings(t *testing.T) {
	s := newTestStore(t)
	id := createTestGame(t, s, "settings-test")

	err := s.UpdateGameSettings(id, 3600)
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}

	g, _ := s.GetGame(id)
	if g.TurnDuration != 3600 {
		t.Errorf("turn_duration = %d, want 3600", g.TurnDuration)
	}
}

// Player tests

func TestAddAndGetPlayers(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "player-test")

	p := &Player{GameID: gameID, UserID: "user1", Power: "England"}
	pid, err := s.AddPlayer(p)
	if err != nil {
		t.Fatalf("add player: %v", err)
	}
	if pid == 0 {
		t.Error("expected nonzero player ID")
	}

	players, err := s.GetPlayersByGame(gameID)
	if err != nil {
		t.Fatalf("get players: %v", err)
	}
	if len(players) != 1 {
		t.Fatalf("got %d players, want 1", len(players))
	}
	if players[0].Power != "England" {
		t.Errorf("power = %q, want England", players[0].Power)
	}
}

func TestGetPlayerByUserAndGame(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "player-lookup")

	s.AddPlayer(&Player{GameID: gameID, UserID: "user-a", Power: "France"})

	p, err := s.GetPlayerByUserAndGame(gameID, "user-a")
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if p == nil {
		t.Fatal("player not found")
	}
	if p.Power != "France" {
		t.Errorf("power = %q, want France", p.Power)
	}

	// Nonexistent
	p, err = s.GetPlayerByUserAndGame(gameID, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil for nonexistent player")
	}
}

func TestSetPlayerReady(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "ready-test")

	s.AddPlayer(&Player{GameID: gameID, UserID: "user1", Power: "England"})

	err := s.SetPlayerReady(gameID, "user1", true)
	if err != nil {
		t.Fatalf("set ready: %v", err)
	}

	p, _ := s.GetPlayerByUserAndGame(gameID, "user1")
	if !p.IsReady {
		t.Error("expected player to be ready")
	}

	// Nonexistent player
	err = s.SetPlayerReady(gameID, "nobody", true)
	if err == nil {
		t.Error("expected error for nonexistent player")
	}
}

func TestAllPlayersReady(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "all-ready")

	s.AddPlayer(&Player{GameID: gameID, UserID: "u1", Power: "England"})
	s.AddPlayer(&Player{GameID: gameID, UserID: "u2", Power: "France"})

	allReady, _ := s.AllPlayersReady(gameID)
	if allReady {
		t.Error("should not be all ready initially")
	}

	s.SetPlayerReady(gameID, "u1", true)
	allReady, _ = s.AllPlayersReady(gameID)
	if allReady {
		t.Error("should not be all ready with one missing")
	}

	s.SetPlayerReady(gameID, "u2", true)
	allReady, _ = s.AllPlayersReady(gameID)
	if !allReady {
		t.Error("expected all ready")
	}
}

func TestResetAllReady(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "reset-ready")

	s.AddPlayer(&Player{GameID: gameID, UserID: "u1", Power: "England"})
	s.SetPlayerReady(gameID, "u1", true)

	err := s.ResetAllReady(gameID)
	if err != nil {
		t.Fatalf("reset ready: %v", err)
	}

	p, _ := s.GetPlayerByUserAndGame(gameID, "u1")
	if p.IsReady {
		t.Error("expected player to not be ready after reset")
	}
}

func TestCountPlayers(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "count-test")

	count, _ := s.CountPlayers(gameID)
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	s.AddPlayer(&Player{GameID: gameID, UserID: "u1", Power: "England"})
	s.AddPlayer(&Player{GameID: gameID, UserID: "u2", Power: "France"})

	count, _ = s.CountPlayers(gameID)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestGetActiveGamesForUser(t *testing.T) {
	s := newTestStore(t)
	id1 := createTestGame(t, s, "active-1")
	id2 := createTestGame(t, s, "active-2")

	s.AddPlayer(&Player{GameID: id1, UserID: "user1", Power: "England"})
	s.AddPlayer(&Player{GameID: id2, UserID: "user1", Power: "France"})

	s.UpdateGameState(id1, "Spring 1901 Movement", "{}", nil, GameStatusActive)
	s.UpdateGameState(id2, "Spring 1901 Movement", "{}", nil, GameStatusActive)

	games, err := s.GetActiveGamesForUser("guild1", "user1")
	if err != nil {
		t.Fatalf("get active games: %v", err)
	}
	if len(games) != 2 {
		t.Errorf("got %d games, want 2", len(games))
	}
}

// Order tests

func TestSubmitAndGetOrders(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "order-test")

	o := &Order{
		GameID: gameID, Phase: "Spring 1901 Movement",
		UserID: "user1", Power: "England", OrderText: "A LON - NTH",
	}
	oid, err := s.SubmitOrder(o)
	if err != nil {
		t.Fatalf("submit order: %v", err)
	}
	if oid == 0 {
		t.Error("expected nonzero order ID")
	}

	orders, err := s.GetOrdersForPhase(gameID, "Spring 1901 Movement")
	if err != nil {
		t.Fatalf("get orders: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("got %d orders, want 1", len(orders))
	}
	if orders[0].OrderText != "A LON - NTH" {
		t.Errorf("order text = %q", orders[0].OrderText)
	}
}

func TestGetOrdersForPlayerPhase(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "player-orders")

	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u1", Power: "England", OrderText: "A LON - NTH"})
	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u2", Power: "France", OrderText: "A PAR - BUR"})

	orders, err := s.GetOrdersForPlayerPhase(gameID, "Spring 1901 Movement", "u1")
	if err != nil {
		t.Fatalf("get player orders: %v", err)
	}
	if len(orders) != 1 {
		t.Errorf("got %d orders, want 1", len(orders))
	}
}

func TestClearOrdersForPlayerPhase(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "clear-orders")

	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u1", Power: "England", OrderText: "A LON - NTH"})
	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u2", Power: "France", OrderText: "A PAR - BUR"})

	err := s.ClearOrdersForPlayerPhase(gameID, "Spring 1901 Movement", "u1")
	if err != nil {
		t.Fatalf("clear orders: %v", err)
	}

	orders, _ := s.GetOrdersForPhase(gameID, "Spring 1901 Movement")
	if len(orders) != 1 {
		t.Errorf("got %d orders after clear, want 1 (u2's order)", len(orders))
	}
}

func TestClearOrdersForPhase(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "clear-all")

	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u1", Power: "England", OrderText: "A LON - NTH"})
	s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u2", Power: "France", OrderText: "A PAR - BUR"})

	s.ClearOrdersForPhase(gameID, "Spring 1901 Movement")

	orders, _ := s.GetOrdersForPhase(gameID, "Spring 1901 Movement")
	if len(orders) != 0 {
		t.Errorf("got %d orders after clear all, want 0", len(orders))
	}
}

func TestDeleteOrder(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "delete-order")

	oid, _ := s.SubmitOrder(&Order{GameID: gameID, Phase: "Spring 1901 Movement", UserID: "u1", Power: "England", OrderText: "A LON H"})

	err := s.DeleteOrder(oid)
	if err != nil {
		t.Fatalf("delete order: %v", err)
	}

	orders, _ := s.GetOrdersForPhase(gameID, "Spring 1901 Movement")
	if len(orders) != 0 {
		t.Errorf("got %d orders after delete, want 0", len(orders))
	}
}

// Phase history tests

func TestSaveAndGetPhaseHistory(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "history-test")

	ordersJSON := `{"England":["A LON - NTH"]}`
	resultsJSON := `{"lon":"OK"}`
	h := &PhaseHistory{
		GameID:      gameID,
		Phase:       "Spring 1901 Movement",
		Season:      "Spring",
		Year:        1901,
		PhaseType:   "Movement",
		StateJSON:   `{"Season":"Spring","Year":1901}`,
		OrdersJSON:  &ordersJSON,
		ResultsJSON: &resultsJSON,
	}

	hid, err := s.SavePhaseHistory(h)
	if err != nil {
		t.Fatalf("save history: %v", err)
	}
	if hid == 0 {
		t.Error("expected nonzero history ID")
	}

	history, err := s.GetPhaseHistory(gameID)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("got %d history entries, want 1", len(history))
	}
	if history[0].Season != "Spring" {
		t.Errorf("season = %q, want Spring", history[0].Season)
	}
	if history[0].Year != 1901 {
		t.Errorf("year = %d, want 1901", history[0].Year)
	}
}

func TestGetPhaseBySeasonYear(t *testing.T) {
	s := newTestStore(t)
	gameID := createTestGame(t, s, "season-year")

	s.SavePhaseHistory(&PhaseHistory{
		GameID: gameID, Phase: "Spring 1901 Movement",
		Season: "Spring", Year: 1901, PhaseType: "Movement",
		StateJSON: "{}",
	})
	s.SavePhaseHistory(&PhaseHistory{
		GameID: gameID, Phase: "Fall 1901 Movement",
		Season: "Fall", Year: 1901, PhaseType: "Movement",
		StateJSON: "{}",
	})

	phases, err := s.GetPhaseBySeasonYear(gameID, "Spring", 1901)
	if err != nil {
		t.Fatalf("get by season/year: %v", err)
	}
	if len(phases) != 1 {
		t.Errorf("got %d phases, want 1", len(phases))
	}

	// No results
	phases, _ = s.GetPhaseBySeasonYear(gameID, "Winter", 1901)
	if len(phases) != 0 {
		t.Errorf("got %d phases, want 0 for Winter 1901", len(phases))
	}
}

func TestGetDueGames(t *testing.T) {
	s := newTestStore(t)
	id1 := createTestGame(t, s, "due-game")
	id2 := createTestGame(t, s, "not-due-game")

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	s.UpdateGameState(id1, "Spring 1901 Movement", "{}", &past, GameStatusActive)
	s.UpdateGameState(id2, "Spring 1901 Movement", "{}", &future, GameStatusActive)

	games, err := s.GetDueGames(time.Now())
	if err != nil {
		t.Fatalf("get due games: %v", err)
	}
	if len(games) != 1 {
		t.Errorf("got %d due games, want 1", len(games))
	}
	if games[0].Name != "due-game" {
		t.Errorf("due game = %q, want due-game", games[0].Name)
	}
}
