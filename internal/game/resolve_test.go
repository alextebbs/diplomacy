package game

import (
	"testing"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/testutil"
)

func TestResolveGame_ExplicitName(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	mgr.CreateGame("guild1", "chan1", "my-game", 86400)

	g, err := mgr.ResolveGame("guild1", "user1", "my-game")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if g.Name != "my-game" {
		t.Errorf("name = %q, want my-game", g.Name)
	}
}

func TestResolveGame_ExplicitName_NotFound(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	_, err := mgr.ResolveGame("guild1", "user1", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent game")
	}
}

func TestResolveGame_SingleGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	created, _ := mgr.CreateGame("guild1", "chan1", "solo-game", 86400)
	mgr.JoinGame(created.ID, "user1", "England")
	store.UpdateGameState(created.ID, "Spring 1901 Movement", created.StateJSON, nil, db.GameStatusActive)

	g, err := mgr.ResolveGame("guild1", "user1", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if g.Name != "solo-game" {
		t.Errorf("name = %q, want solo-game", g.Name)
	}
}

func TestResolveGame_MultipleGames(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	g1, _ := mgr.CreateGame("guild1", "chan1", "game-1", 86400)
	g2, _ := mgr.CreateGame("guild1", "chan1", "game-2", 86400)
	mgr.JoinGame(g1.ID, "user1", "England")
	mgr.JoinGame(g2.ID, "user1", "France")
	store.UpdateGameState(g1.ID, "Spring 1901 Movement", g1.StateJSON, nil, db.GameStatusActive)
	store.UpdateGameState(g2.ID, "Spring 1901 Movement", g2.StateJSON, nil, db.GameStatusActive)

	_, err := mgr.ResolveGame("guild1", "user1", "")
	if err == nil {
		t.Error("expected error when in multiple games")
	}
}

func TestResolveGame_NoGames(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	_, err := mgr.ResolveGame("guild1", "user1", "")
	if err == nil {
		t.Error("expected error when in no games")
	}
}

func TestResolveGame_WrongGuild(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	created, _ := mgr.CreateGame("guild1", "chan1", "guild1-game", 86400)
	mgr.JoinGame(created.ID, "user1", "England")
	store.UpdateGameState(created.ID, "Spring 1901 Movement", created.StateJSON, nil, db.GameStatusActive)

	// User is in guild1 game, but looking in guild2
	_, err := mgr.ResolveGame("guild2", "user1", "")
	if err == nil {
		t.Error("expected error when game is in different guild")
	}
}

func TestResolveGame_FinishedGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	created, _ := mgr.CreateGame("guild1", "chan1", "finished-game", 86400)
	mgr.JoinGame(created.ID, "user1", "England")
	store.UpdateGameState(created.ID, "Spring 1901 Movement", created.StateJSON, nil, db.GameStatusFinished)

	// Finished games should not auto-resolve
	_, err := mgr.ResolveGame("guild1", "user1", "")
	if err == nil {
		t.Error("expected error when only game is finished")
	}
}
