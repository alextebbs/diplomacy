package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/sammy/diplomacy/internal/db"
	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"
)

var (
	pgOnce  sync.Once
	pgDB    *embeddedpostgres.EmbeddedPostgres
	pgURL   string
	pgErr   error
	pgPort  uint32 = 15432
)

// EnsurePostgres starts a shared embedded PostgreSQL instance. The instance
// persists for the lifetime of the test process. Tests across packages that
// import testutil will share this instance (same binary), and each call to
// NewTestDB drops/recreates tables for isolation.
func EnsurePostgres(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}

	pgOnce.Do(func() {
		pgDB = embeddedpostgres.NewDatabase(
			embeddedpostgres.DefaultConfig().
				Port(pgPort).
				Database("diplomacy_test"),
		)
		pgErr = pgDB.Start()
		if pgErr == nil {
			pgURL = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/diplomacy_test?sslmode=disable", pgPort)
		}
	})

	if pgErr != nil {
		t.Fatalf("failed to start embedded postgres: %v", pgErr)
	}
	return pgURL
}

// NewTestDB returns a migrated Store connected to a test PostgreSQL.
// Each call drops and re-creates all tables for a clean slate.
func NewTestDB(t *testing.T) *db.Store {
	t.Helper()
	connURL := EnsurePostgres(t)

	store, err := db.Open(connURL)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	for _, stmt := range []string{
		"DROP TABLE IF EXISTS phase_history CASCADE",
		"DROP TABLE IF EXISTS orders CASCADE",
		"DROP TABLE IF EXISTS players CASCADE",
		"DROP TABLE IF EXISTS games CASCADE",
	} {
		if _, err := store.DB().Exec(stmt); err != nil {
			t.Fatalf("failed to drop tables: %v", err)
		}
	}

	if err := store.Migrate(); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

var ClassicalPowers = []string{"Austria", "England", "France", "Germany", "Italy", "Russia", "Turkey"}

func NewTestGame(t *testing.T, store *db.Store) (*db.Game, []*db.Player) {
	t.Helper()

	state, err := classical.Start()
	if err != nil {
		t.Fatalf("failed to start classical game: %v", err)
	}

	stateJSON, err := serializeState(state)
	if err != nil {
		t.Fatalf("failed to serialize state: %v", err)
	}

	phase := state.Phase()
	phaseName := fmt.Sprintf("%s %d %s", phase.Season(), phase.Year(), phase.Type())

	g := &db.Game{
		GuildID:      "guild-test",
		ChannelID:    "channel-test",
		Name:         "test-game",
		Variant:      "Classical",
		Phase:        phaseName,
		StateJSON:    stateJSON,
		TurnDuration: 86400,
		Status:       db.GameStatusPending,
	}

	id, err := store.CreateGame(g)
	if err != nil {
		t.Fatalf("failed to create test game: %v", err)
	}
	g.ID = id

	var players []*db.Player
	for i, power := range ClassicalPowers {
		p := &db.Player{
			GameID: id,
			UserID: fmt.Sprintf("user-%d", i),
			Power:  power,
		}
		pid, err := store.AddPlayer(p)
		if err != nil {
			t.Fatalf("failed to add player: %v", err)
		}
		p.ID = pid
		players = append(players, p)
	}

	return g, players
}

func StartedGame(t *testing.T, store *db.Store) (*db.Game, []*db.Player) {
	t.Helper()
	g, players := NewTestGame(t, store)

	if err := store.UpdateGameState(g.ID, g.Phase, g.StateJSON, nil, db.GameStatusActive); err != nil {
		t.Fatalf("failed to start game: %v", err)
	}
	g.Status = db.GameStatusActive
	return g, players
}

type phaseState struct {
	Season        godip.Season                              `json:"Season"`
	Year          int                                       `json:"Year"`
	Type          godip.PhaseType                           `json:"Type"`
	Units         map[godip.Province]godip.Unit             `json:"Units"`
	SupplyCenters map[godip.Province]godip.Nation            `json:"SupplyCenters"`
	Dislodgeds    map[godip.Province]godip.Unit             `json:"Dislodgeds"`
	Dislodgers    map[godip.Province]godip.Province          `json:"Dislodgers"`
	Bounces       map[godip.Province]map[godip.Province]bool `json:"Bounces"`
	Resolutions   map[godip.Province]string                 `json:"Resolutions"`
}

func serializeState(s interface {
	Phase() godip.Phase
	Dump() (map[godip.Province]godip.Unit, map[godip.Province]godip.Nation, map[godip.Province]godip.Unit, map[godip.Province]godip.Province, map[godip.Province]map[godip.Province]bool, map[godip.Province]error)
}) (string, error) {
	p := s.Phase()
	ps := &phaseState{
		Season:      p.Season(),
		Year:        p.Year(),
		Type:        p.Type(),
		Resolutions: map[godip.Province]string{},
	}
	var resolutions map[godip.Province]error
	ps.Units, ps.SupplyCenters, ps.Dislodgeds, ps.Dislodgers, ps.Bounces, resolutions = s.Dump()
	for prov, err := range resolutions {
		if err == nil {
			ps.Resolutions[prov] = "OK"
		} else {
			ps.Resolutions[prov] = err.Error()
		}
	}
	data, err := json.Marshal(ps)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
