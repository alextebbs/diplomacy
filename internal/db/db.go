package db

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db *sql.DB
}

// Open connects to a PostgreSQL database using a connection URL.
// Example: "postgres://user:pass@localhost:5432/diplomacy?sslmode=disable"
func Open(connURL string) (*Store, error) {
	d, err := sql.Open("pgx", connURL)
	if err != nil {
		return nil, err
	}
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	return &Store{db: d}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS games (
			id            BIGSERIAL PRIMARY KEY,
			guild_id      TEXT NOT NULL,
			channel_id    TEXT NOT NULL DEFAULT '',
			name          TEXT NOT NULL,
			variant       TEXT NOT NULL DEFAULT 'Classical',
			phase         TEXT NOT NULL,
			state_json    TEXT NOT NULL,
			turn_duration BIGINT NOT NULL,
			next_deadline TIMESTAMPTZ,
			status        TEXT NOT NULL DEFAULT 'pending',
			created_at    TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(guild_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS players (
			id        BIGSERIAL PRIMARY KEY,
			game_id   BIGINT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
			user_id   TEXT NOT NULL,
			power     TEXT NOT NULL,
			is_ready  BOOLEAN DEFAULT FALSE,
			UNIQUE(game_id, user_id),
			UNIQUE(game_id, power)
		)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id         BIGSERIAL PRIMARY KEY,
			game_id    BIGINT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
			phase      TEXT NOT NULL,
			user_id    TEXT NOT NULL,
			power      TEXT NOT NULL,
			order_text TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS phase_history (
			id           BIGSERIAL PRIMARY KEY,
			game_id      BIGINT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
			phase        TEXT NOT NULL,
			season       TEXT NOT NULL,
			year         INTEGER NOT NULL,
			phase_type   TEXT NOT NULL,
			state_json   TEXT NOT NULL,
			orders_json  TEXT,
			results_json TEXT,
			created_at   TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_players_game ON players(game_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_game_phase ON orders(game_id, phase)`,
		`CREATE INDEX IF NOT EXISTS idx_phase_history_game ON phase_history(game_id)`,
		`CREATE INDEX IF NOT EXISTS idx_games_guild ON games(guild_id)`,
		`CREATE INDEX IF NOT EXISTS idx_games_status ON games(status)`,
	}

	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
