package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

var testDBURL string

func TestMain(m *testing.M) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		testDBURL = url
		os.Exit(m.Run())
	}

	runtimeDir := filepath.Join(os.TempDir(), "ep-db-test")

	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(15433).
			Database("diplomacy_db_test").
			RuntimePath(runtimeDir),
	)
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start embedded postgres: %v\n", err)
		os.Exit(1)
	}

	testDBURL = "postgres://postgres:postgres@localhost:15433/diplomacy_db_test?sslmode=disable"

	code := m.Run()
	pg.Stop()
	os.RemoveAll(runtimeDir)
	os.Exit(code)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(testDBURL)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS phase_history CASCADE",
		"DROP TABLE IF EXISTS orders CASCADE",
		"DROP TABLE IF EXISTS players CASCADE",
		"DROP TABLE IF EXISTS games CASCADE",
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatalf("drop: %v", err)
		}
	}
	if err := s.Migrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrate(t *testing.T) {
	s := newTestStore(t)

	tables := []string{"games", "players", "orders", "phase_history"}
	for _, table := range tables {
		var exists bool
		err := s.db.QueryRow(
			`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`, table,
		).Scan(&exists)
		if err != nil {
			t.Errorf("table check %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s not found", table)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s := newTestStore(t)

	if err := s.Migrate(); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
}

func TestUniqueGameNamePerGuild(t *testing.T) {
	s := newTestStore(t)

	g := &Game{
		GuildID:      "guild1",
		Name:         "game1",
		Variant:      "Classical",
		Phase:        "Spring 1901 Movement",
		StateJSON:    "{}",
		TurnDuration: 86400,
		Status:       GameStatusPending,
	}

	_, err := s.CreateGame(g)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = s.CreateGame(g)
	if err == nil {
		t.Error("expected error for duplicate game name in same guild")
	}

	g.GuildID = "guild2"
	_, err = s.CreateGame(g)
	if err != nil {
		t.Errorf("different guild should allow same name: %v", err)
	}
}

func TestUniquePlayerPerGame(t *testing.T) {
	s := newTestStore(t)

	g := &Game{
		GuildID: "guild1", Name: "game1", Variant: "Classical",
		Phase: "Spring 1901 Movement", StateJSON: "{}", TurnDuration: 86400, Status: GameStatusPending,
	}
	gameID, _ := s.CreateGame(g)

	p := &Player{GameID: gameID, UserID: "user1", Power: "England"}
	_, err := s.AddPlayer(p)
	if err != nil {
		t.Fatalf("first player: %v", err)
	}

	p2 := &Player{GameID: gameID, UserID: "user1", Power: "France"}
	_, err = s.AddPlayer(p2)
	if err == nil {
		t.Error("expected error for duplicate user in same game")
	}

	p3 := &Player{GameID: gameID, UserID: "user2", Power: "England"}
	_, err = s.AddPlayer(p3)
	if err == nil {
		t.Error("expected error for duplicate power in same game")
	}

	p4 := &Player{GameID: gameID, UserID: "user2", Power: "France"}
	_, err = s.AddPlayer(p4)
	if err != nil {
		t.Errorf("different user and power should work: %v", err)
	}
}
