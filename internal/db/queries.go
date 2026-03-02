package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) CreateGame(g *Game) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO games (guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		g.GuildID, g.ChannelID, g.Name, g.Variant, g.Phase, g.StateJSON, g.TurnDuration, g.NextDeadline, g.Status,
	).Scan(&id)
	return id, err
}

func (s *Store) GetGame(id int64) (*Game, error) {
	g := &Game{}
	err := s.db.QueryRow(
		`SELECT id, guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status, created_at
		 FROM games WHERE id = $1`, id,
	).Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

func (s *Store) GetGameByName(guildID, name string) (*Game, error) {
	g := &Game{}
	err := s.db.QueryRow(
		`SELECT id, guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status, created_at
		 FROM games WHERE guild_id = $1 AND name = $2`, guildID, name,
	).Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return g, err
}

func (s *Store) ListGamesByGuild(guildID string) ([]*Game, error) {
	rows, err := s.db.Query(
		`SELECT id, guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status, created_at
		 FROM games WHERE guild_id = $1 ORDER BY created_at DESC`, guildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		g := &Game{}
		if err := rows.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

func (s *Store) ListActiveGamesByGuild(guildID string) ([]*Game, error) {
	rows, err := s.db.Query(
		`SELECT id, guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status, created_at
		 FROM games WHERE guild_id = $1 AND status IN ('pending', 'active') ORDER BY created_at DESC`, guildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		g := &Game{}
		if err := rows.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

func (s *Store) UpdateGameState(id int64, phase, stateJSON string, nextDeadline *time.Time, status GameStatus) error {
	_, err := s.db.Exec(
		`UPDATE games SET phase = $1, state_json = $2, next_deadline = $3, status = $4 WHERE id = $5`,
		phase, stateJSON, nextDeadline, status, id,
	)
	return err
}

func (s *Store) UpdateGameSettings(id int64, turnDuration int64) error {
	_, err := s.db.Exec(
		`UPDATE games SET turn_duration = $1 WHERE id = $2`,
		turnDuration, id,
	)
	return err
}

func (s *Store) UpdateGameChannel(id int64, channelID string) error {
	_, err := s.db.Exec(
		`UPDATE games SET channel_id = $1 WHERE id = $2`,
		channelID, id,
	)
	return err
}

// Player queries

func (s *Store) AddPlayer(p *Player) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO players (game_id, user_id, power, is_ready) VALUES ($1, $2, $3, $4) RETURNING id`,
		p.GameID, p.UserID, p.Power, p.IsReady,
	).Scan(&id)
	return id, err
}

func (s *Store) GetPlayersByGame(gameID int64) ([]*Player, error) {
	rows, err := s.db.Query(
		`SELECT id, game_id, user_id, power, is_ready FROM players WHERE game_id = $1 ORDER BY power`, gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []*Player
	for rows.Next() {
		p := &Player{}
		if err := rows.Scan(&p.ID, &p.GameID, &p.UserID, &p.Power, &p.IsReady); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

func (s *Store) GetPlayerByUserAndGame(gameID int64, userID string) (*Player, error) {
	p := &Player{}
	err := s.db.QueryRow(
		`SELECT id, game_id, user_id, power, is_ready FROM players WHERE game_id = $1 AND user_id = $2`,
		gameID, userID,
	).Scan(&p.ID, &p.GameID, &p.UserID, &p.Power, &p.IsReady)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Store) GetActiveGamesForUser(guildID, userID string) ([]*Game, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.guild_id, g.channel_id, g.name, g.variant, g.phase, g.state_json, g.turn_duration, g.next_deadline, g.status, g.created_at
		 FROM games g
		 JOIN players p ON p.game_id = g.id
		 WHERE g.guild_id = $1 AND p.user_id = $2 AND g.status = 'active'
		 ORDER BY g.created_at DESC`, guildID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		g := &Game{}
		if err := rows.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

func (s *Store) SetPlayerReady(gameID int64, userID string, ready bool) error {
	res, err := s.db.Exec(
		`UPDATE players SET is_ready = $1 WHERE game_id = $2 AND user_id = $3`,
		ready, gameID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("player not found in game")
	}
	return nil
}

func (s *Store) ResetAllReady(gameID int64) error {
	_, err := s.db.Exec(`UPDATE players SET is_ready = FALSE WHERE game_id = $1`, gameID)
	return err
}

func (s *Store) AllPlayersReady(gameID int64) (bool, error) {
	var notReady int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM players WHERE game_id = $1 AND is_ready = FALSE`, gameID,
	).Scan(&notReady)
	if err != nil {
		return false, err
	}
	return notReady == 0, nil
}

func (s *Store) CountPlayers(gameID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM players WHERE game_id = $1`, gameID).Scan(&count)
	return count, err
}

// Order queries

func (s *Store) SubmitOrder(o *Order) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO orders (game_id, phase, user_id, power, order_text) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		o.GameID, o.Phase, o.UserID, o.Power, o.OrderText,
	).Scan(&id)
	return id, err
}

func (s *Store) GetOrdersForPhase(gameID int64, phase string) ([]*Order, error) {
	rows, err := s.db.Query(
		`SELECT id, game_id, phase, user_id, power, order_text, created_at
		 FROM orders WHERE game_id = $1 AND phase = $2 ORDER BY created_at`, gameID, phase,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		o := &Order{}
		if err := rows.Scan(&o.ID, &o.GameID, &o.Phase, &o.UserID, &o.Power, &o.OrderText, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (s *Store) GetOrdersForPlayerPhase(gameID int64, phase, userID string) ([]*Order, error) {
	rows, err := s.db.Query(
		`SELECT id, game_id, phase, user_id, power, order_text, created_at
		 FROM orders WHERE game_id = $1 AND phase = $2 AND user_id = $3 ORDER BY created_at`, gameID, phase, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*Order
	for rows.Next() {
		o := &Order{}
		if err := rows.Scan(&o.ID, &o.GameID, &o.Phase, &o.UserID, &o.Power, &o.OrderText, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

func (s *Store) ClearOrdersForPlayerPhase(gameID int64, phase, userID string) error {
	_, err := s.db.Exec(
		`DELETE FROM orders WHERE game_id = $1 AND phase = $2 AND user_id = $3`,
		gameID, phase, userID,
	)
	return err
}

func (s *Store) ClearOrdersForPhase(gameID int64, phase string) error {
	_, err := s.db.Exec(`DELETE FROM orders WHERE game_id = $1 AND phase = $2`, gameID, phase)
	return err
}

func (s *Store) DeleteOrder(id int64) error {
	_, err := s.db.Exec(`DELETE FROM orders WHERE id = $1`, id)
	return err
}

// Phase history queries

func (s *Store) SavePhaseHistory(h *PhaseHistory) (int64, error) {
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO phase_history (game_id, phase, season, year, phase_type, state_json, orders_json, results_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		h.GameID, h.Phase, h.Season, h.Year, h.PhaseType, h.StateJSON, h.OrdersJSON, h.ResultsJSON,
	).Scan(&id)
	return id, err
}

func (s *Store) GetPhaseHistory(gameID int64) ([]*PhaseHistory, error) {
	rows, err := s.db.Query(
		`SELECT id, game_id, phase, season, year, phase_type, state_json, orders_json, results_json, created_at
		 FROM phase_history WHERE game_id = $1 ORDER BY year,
		 CASE season WHEN 'Spring' THEN 1 WHEN 'Fall' THEN 2 WHEN 'Winter' THEN 3 END,
		 CASE phase_type WHEN 'Movement' THEN 1 WHEN 'Retreat' THEN 2 WHEN 'Adjustment' THEN 3 END`,
		gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*PhaseHistory
	for rows.Next() {
		h := &PhaseHistory{}
		if err := rows.Scan(&h.ID, &h.GameID, &h.Phase, &h.Season, &h.Year, &h.PhaseType, &h.StateJSON, &h.OrdersJSON, &h.ResultsJSON, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

func (s *Store) GetPhaseBySeasonYearType(gameID int64, season string, year int, phaseType string) (*PhaseHistory, error) {
	h := &PhaseHistory{}
	err := s.db.QueryRow(
		`SELECT id, game_id, phase, season, year, phase_type, state_json, orders_json, results_json, created_at
		 FROM phase_history WHERE game_id = $1 AND season = $2 AND year = $3 AND phase_type = $4`,
		gameID, season, year, phaseType,
	).Scan(&h.ID, &h.GameID, &h.Phase, &h.Season, &h.Year, &h.PhaseType, &h.StateJSON, &h.OrdersJSON, &h.ResultsJSON, &h.CreatedAt)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (s *Store) GetPhaseBySeasonYear(gameID int64, season string, year int) ([]*PhaseHistory, error) {
	rows, err := s.db.Query(
		`SELECT id, game_id, phase, season, year, phase_type, state_json, orders_json, results_json, created_at
		 FROM phase_history WHERE game_id = $1 AND season = $2 AND year = $3
		 ORDER BY CASE phase_type WHEN 'Movement' THEN 1 WHEN 'Retreat' THEN 2 WHEN 'Adjustment' THEN 3 END`,
		gameID, season, year,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*PhaseHistory
	for rows.Next() {
		h := &PhaseHistory{}
		if err := rows.Scan(&h.ID, &h.GameID, &h.Phase, &h.Season, &h.Year, &h.PhaseType, &h.StateJSON, &h.OrdersJSON, &h.ResultsJSON, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

func (s *Store) GetDueGames(now time.Time) ([]*Game, error) {
	rows, err := s.db.Query(
		`SELECT id, guild_id, channel_id, name, variant, phase, state_json, turn_duration, next_deadline, status, created_at
		 FROM games WHERE status = 'active' AND next_deadline IS NOT NULL AND next_deadline <= $1
		 ORDER BY next_deadline`, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		g := &Game{}
		if err := rows.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.Name, &g.Variant, &g.Phase, &g.StateJSON, &g.TurnDuration, &g.NextDeadline, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}
