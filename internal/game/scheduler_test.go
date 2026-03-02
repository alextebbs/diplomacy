package game

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/testutil"
)

func TestDeadlineTriggersAdjudication(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := testutil.StartedGame(t, store)

	// Set deadline to the past
	past := time.Now().Add(-1 * time.Hour)
	store.UpdateGameState(g.ID, g.Phase, g.StateJSON, &past, db.GameStatusActive)

	var mu sync.Mutex
	var callbackCalled bool
	scheduler := NewScheduler(mgr, store)
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		mu.Lock()
		callbackCalled = true
		mu.Unlock()
	})

	scheduler.Tick()

	mu.Lock()
	called := callbackCalled
	mu.Unlock()

	if !called {
		t.Error("expected callback to be called for due game")
	}

	// Verify phase advanced
	updated, _ := store.GetGame(g.ID)
	if updated.Phase == "Spring 1901 Movement" {
		t.Error("phase should have advanced")
	}
}

func TestDeadlineNotYetDue(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := testutil.StartedGame(t, store)

	// Set deadline to the future
	future := time.Now().Add(24 * time.Hour)
	store.UpdateGameState(g.ID, g.Phase, g.StateJSON, &future, db.GameStatusActive)

	var callbackCalled bool
	scheduler := NewScheduler(mgr, store)
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		callbackCalled = true
	})

	scheduler.Tick()

	if callbackCalled {
		t.Error("callback should not be called for game with future deadline")
	}
}

func TestAllReadyTriggersEarly(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, players := testutil.StartedGame(t, store)

	// Set future deadline
	future := time.Now().Add(24 * time.Hour)
	store.UpdateGameState(g.ID, g.Phase, g.StateJSON, &future, db.GameStatusActive)

	var callbackCalled bool
	scheduler := NewScheduler(mgr, store)
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		callbackCalled = true
	})

	// Mark all players ready
	for _, p := range players {
		store.SetPlayerReady(g.ID, p.UserID, true)
	}

	adjudicated, err := scheduler.CheckReady(g.ID)
	if err != nil {
		t.Fatalf("check ready: %v", err)
	}
	if !adjudicated {
		t.Error("expected early adjudication when all ready")
	}
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
}

func TestPartialReadyNoTrigger(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, players := testutil.StartedGame(t, store)

	future := time.Now().Add(24 * time.Hour)
	store.UpdateGameState(g.ID, g.Phase, g.StateJSON, &future, db.GameStatusActive)

	scheduler := NewScheduler(mgr, store)

	// Mark only some players ready
	for i, p := range players {
		if i < len(players)-1 {
			store.SetPlayerReady(g.ID, p.UserID, true)
		}
	}

	adjudicated, _ := scheduler.CheckReady(g.ID)
	if adjudicated {
		t.Error("should not adjudicate when not all ready")
	}
}

func TestReadyResetsAfterPhase(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, players := testutil.StartedGame(t, store)

	// Mark everyone ready
	for _, p := range players {
		store.SetPlayerReady(g.ID, p.UserID, true)
	}

	// Process phase
	mgr.ProcessPhase(g.ID)

	// Check ready flags are reset
	for _, p := range players {
		player, _ := store.GetPlayerByUserAndGame(g.ID, p.UserID)
		if player.IsReady {
			t.Errorf("player %s should not be ready after phase advance", p.Power)
		}
	}
}

func TestSchedulerSkipsFinishedGames(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := testutil.StartedGame(t, store)

	// Set game as finished with past deadline
	past := time.Now().Add(-1 * time.Hour)
	store.UpdateGameState(g.ID, g.Phase, g.StateJSON, &past, db.GameStatusFinished)

	var callbackCalled bool
	scheduler := NewScheduler(mgr, store)
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		callbackCalled = true
	})

	scheduler.Tick()

	if callbackCalled {
		t.Error("callback should not be called for finished games")
	}
}

func TestMultipleGamesIndependent(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	// Create two games
	g1, _ := testutil.StartedGame(t, store)

	g2, _ := mgr.CreateGame("guild-test", "chan-test", "test-game-2", 86400)
	for i, power := range testutil.ClassicalPowers {
		mgr.JoinGame(g2.ID, fmt.Sprintf("user-2-%d", i), power)
	}
	mgr.StartGame(g2.ID)
	g2, _ = store.GetGame(g2.ID)

	// Only first game is due
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(24 * time.Hour)
	store.UpdateGameState(g1.ID, g1.Phase, g1.StateJSON, &past, db.GameStatusActive)
	store.UpdateGameState(g2.ID, g2.Phase, g2.StateJSON, &future, db.GameStatusActive)

	var count int
	scheduler := NewScheduler(mgr, store)
	scheduler.SetCallback(func(g *db.Game, results map[string]string, gameOver bool) {
		count++
	})

	scheduler.Tick()

	if count != 1 {
		t.Errorf("expected exactly 1 callback, got %d", count)
	}
}
