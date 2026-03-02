package game

import (
	"testing"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/testutil"
)

func TestCreateGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	g, err := mgr.CreateGame("guild1", "chan1", "test-game", 86400)
	if err != nil {
		t.Fatalf("create game: %v", err)
	}

	if g.Name != "test-game" {
		t.Errorf("name = %q, want test-game", g.Name)
	}
	if g.Variant != "Classical" {
		t.Errorf("variant = %q, want Classical", g.Variant)
	}
	if g.Status != db.GameStatusPending {
		t.Errorf("status = %q, want pending", g.Status)
	}
	if g.Phase != "Spring 1901 Movement" {
		t.Errorf("phase = %q, want Spring 1901 Movement", g.Phase)
	}
}

func TestCreateGame_DuplicateName(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	_, err := mgr.CreateGame("guild1", "chan1", "dupe", 86400)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = mgr.CreateGame("guild1", "chan1", "dupe", 86400)
	if err == nil {
		t.Error("expected error for duplicate game name")
	}
}

func TestJoinGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "join-test", 86400)

	p, err := mgr.JoinGame(g.ID, "user1", "England")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
	if p.Power != "England" {
		t.Errorf("power = %q, want England", p.Power)
	}
}

func TestJoinGame_RandomPower(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "random-test", 86400)

	p, err := mgr.JoinGame(g.ID, "user1", "")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
	if p.Power == "" {
		t.Error("expected a power to be assigned")
	}
}

func TestJoinGame_DuplicateUser(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "dup-user", 86400)

	mgr.JoinGame(g.ID, "user1", "England")

	_, err := mgr.JoinGame(g.ID, "user1", "France")
	if err == nil {
		t.Error("expected error for duplicate user")
	}
}

func TestJoinGame_DuplicatePower(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "dup-power", 86400)

	mgr.JoinGame(g.ID, "user1", "England")

	_, err := mgr.JoinGame(g.ID, "user2", "England")
	if err == nil {
		t.Error("expected error for duplicate power")
	}
}

func TestJoinGame_InvalidPower(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "invalid-power", 86400)

	_, err := mgr.JoinGame(g.ID, "user1", "Mordor")
	if err == nil {
		t.Error("expected error for invalid power")
	}
}

func TestJoinGame_GameAlreadyStarted(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "started-game", 86400)
	mgr.JoinGame(g.ID, "user1", "England")
	mgr.StartGame(g.ID)

	_, err := mgr.JoinGame(g.ID, "user2", "France")
	if err == nil {
		t.Error("expected error for joining started game")
	}
}

func TestStartGame(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "start-test", 86400)
	mgr.JoinGame(g.ID, "user1", "England")

	err := mgr.StartGame(g.ID)
	if err != nil {
		t.Fatalf("start game: %v", err)
	}

	updated, _ := store.GetGame(g.ID)
	if updated.Status != db.GameStatusActive {
		t.Errorf("status = %q, want active", updated.Status)
	}
	if updated.NextDeadline == nil {
		t.Error("expected deadline to be set")
	}
}

func TestStartGame_NoPlayers(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "no-players", 86400)

	err := mgr.StartGame(g.ID)
	if err == nil {
		t.Error("expected error for starting with no players")
	}
}

func TestStartGame_AlreadyStarted(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "double-start", 86400)
	mgr.JoinGame(g.ID, "user1", "England")
	mgr.StartGame(g.ID)

	err := mgr.StartGame(g.ID)
	if err == nil {
		t.Error("expected error for double start")
	}
}

func TestGetGameStatus(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)
	g, _ := mgr.CreateGame("guild1", "chan1", "status-test", 86400)
	mgr.JoinGame(g.ID, "user1", "England")

	g, _ = store.GetGame(g.ID)
	status, err := mgr.GetGameStatus(g)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status == "" {
		t.Error("expected non-empty status")
	}
}

func TestExportAndImportState(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	g, _ := mgr.CreateGame("guild1", "chan1", "export-test", 86400)
	g, _ = store.GetGame(g.ID)

	data, err := mgr.ExportState(g)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty export data")
	}

	imported, err := mgr.ImportGame("guild1", "chan1", "imported-game", 86400, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Name != "imported-game" {
		t.Errorf("imported name = %q, want imported-game", imported.Name)
	}
	if imported.Phase != g.Phase {
		t.Errorf("imported phase = %q, want %q", imported.Phase, g.Phase)
	}
}

func TestImportGame_InvalidJSON(t *testing.T) {
	store := testutil.NewTestDB(t)
	mgr := NewManager(store)

	_, err := mgr.ImportGame("guild1", "chan1", "bad-import", 86400, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
