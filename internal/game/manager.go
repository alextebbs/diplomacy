package game

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"
)

type Manager struct {
	store *db.Store
}

func NewManager(store *db.Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) Store() *db.Store {
	return m.store
}

func (m *Manager) CreateGame(guildID, channelID, name string, turnDurationSecs int64) (*db.Game, error) {
	existing, err := m.store.GetGameByName(guildID, name)
	if err != nil {
		return nil, fmt.Errorf("check existing game: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("a game named %q already exists in this server", name)
	}

	state, err := classical.Start()
	if err != nil {
		return nil, fmt.Errorf("initialize classical game: %w", err)
	}

	stateJSON, err := SerializeState(state)
	if err != nil {
		return nil, fmt.Errorf("serialize initial state: %w", err)
	}

	g := &db.Game{
		GuildID:      guildID,
		ChannelID:    channelID,
		Name:         name,
		Variant:      "Classical",
		Phase:        StatePhaseString(state),
		StateJSON:    stateJSON,
		TurnDuration: turnDurationSecs,
		Status:       db.GameStatusPending,
	}

	id, err := m.store.CreateGame(g)
	if err != nil {
		return nil, fmt.Errorf("create game: %w", err)
	}
	g.ID = id
	return g, nil
}

func (m *Manager) JoinGame(gameID int64, userID, power string) (*db.Player, error) {
	g, err := m.store.GetGame(gameID)
	if err != nil {
		return nil, fmt.Errorf("get game: %w", err)
	}
	if g == nil {
		return nil, fmt.Errorf("game not found")
	}
	if g.Status != db.GameStatusPending {
		return nil, fmt.Errorf("game has already started")
	}

	existing, err := m.store.GetPlayerByUserAndGame(gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("check existing player: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("you have already joined this game as %s", existing.Power)
	}

	if power == "" {
		available, err := m.availablePowers(gameID)
		if err != nil {
			return nil, err
		}
		if len(available) == 0 {
			return nil, fmt.Errorf("all powers are taken")
		}
		power = available[rand.Intn(len(available))]
	} else {
		if !isValidPower(power) {
			return nil, fmt.Errorf("invalid power: %s (valid: Austria, England, France, Germany, Italy, Russia, Turkey)", power)
		}
	}

	p := &db.Player{
		GameID: gameID,
		UserID: userID,
		Power:  power,
	}
	id, err := m.store.AddPlayer(p)
	if err != nil {
		return nil, fmt.Errorf("add player (power may already be taken): %w", err)
	}
	p.ID = id
	return p, nil
}

func (m *Manager) StartGame(gameID int64) error {
	g, err := m.store.GetGame(gameID)
	if err != nil {
		return fmt.Errorf("get game: %w", err)
	}
	if g == nil {
		return fmt.Errorf("game not found")
	}
	if g.Status != db.GameStatusPending {
		return fmt.Errorf("game has already started")
	}

	count, err := m.store.CountPlayers(gameID)
	if err != nil {
		return fmt.Errorf("count players: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("no players have joined")
	}

	deadline := time.Now().Add(time.Duration(g.TurnDuration) * time.Second)
	return m.store.UpdateGameState(gameID, g.Phase, g.StateJSON, &deadline, db.GameStatusActive)
}

func (m *Manager) ResolveGame(guildID, userID, name string) (*db.Game, error) {
	if name != "" {
		g, err := m.store.GetGameByName(guildID, name)
		if err != nil {
			return nil, fmt.Errorf("get game: %w", err)
		}
		if g == nil {
			return nil, fmt.Errorf("game %q not found", name)
		}
		return g, nil
	}

	games, err := m.store.GetActiveGamesForUser(guildID, userID)
	if err != nil {
		return nil, fmt.Errorf("get active games: %w", err)
	}

	switch len(games) {
	case 0:
		return nil, fmt.Errorf("you are not in any active games; join one first")
	case 1:
		return games[0], nil
	default:
		names := make([]string, len(games))
		for i, g := range games {
			names[i] = g.Name
		}
		return nil, fmt.Errorf("you are in multiple games, please specify which one: %v", names)
	}
}

func (m *Manager) GetGameStatus(g *db.Game) (string, error) {
	players, err := m.store.GetPlayersByGame(g.ID)
	if err != nil {
		return "", err
	}

	var ps PhaseState
	if err := json.Unmarshal([]byte(g.StateJSON), &ps); err != nil {
		return "", err
	}

	centerCounts := map[godip.Nation]int{}
	for _, nation := range ps.SupplyCenters {
		centerCounts[nation]++
	}

	status := fmt.Sprintf("**%s** - %s\n", g.Name, g.Phase)
	status += fmt.Sprintf("Status: %s\n", g.Status)
	if g.NextDeadline != nil {
		status += fmt.Sprintf("Deadline: <t:%d:R>\n", g.NextDeadline.Unix())
	}
	status += "\n**Supply Centers:**\n"
	for _, p := range players {
		nation := godip.Nation(p.Power)
		count := centerCounts[nation]
		readyMark := ""
		if p.IsReady {
			readyMark = " [READY]"
		}
		status += fmt.Sprintf("  %s (<@%s>): %d SCs%s\n", p.Power, p.UserID, count, readyMark)
	}

	return status, nil
}

func (m *Manager) ExportState(g *db.Game) ([]byte, error) {
	var ps PhaseState
	if err := json.Unmarshal([]byte(g.StateJSON), &ps); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}
	return data, nil
}

func (m *Manager) ImportGame(guildID, channelID, name string, turnDurationSecs int64, stateData []byte) (*db.Game, error) {
	existing, err := m.store.GetGameByName(guildID, name)
	if err != nil {
		return nil, fmt.Errorf("check existing game: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("a game named %q already exists in this server", name)
	}

	var ps PhaseState
	if err := json.Unmarshal(stateData, &ps); err != nil {
		return nil, fmt.Errorf("invalid state JSON: %w", err)
	}

	// Validate by attempting to create a godip state
	if _, err := ps.ToState(); err != nil {
		return nil, fmt.Errorf("invalid game state: %w", err)
	}

	stateJSON, err := json.Marshal(ps)
	if err != nil {
		return nil, fmt.Errorf("re-serialize state: %w", err)
	}

	phaseName := FormatPhaseName(ps.Season, ps.Year, ps.Type)

	g := &db.Game{
		GuildID:      guildID,
		ChannelID:    channelID,
		Name:         name,
		Variant:      "Classical",
		Phase:        phaseName,
		StateJSON:    string(stateJSON),
		TurnDuration: turnDurationSecs,
		Status:       db.GameStatusPending,
	}

	id, err := m.store.CreateGame(g)
	if err != nil {
		return nil, fmt.Errorf("create game: %w", err)
	}
	g.ID = id
	return g, nil
}

func (m *Manager) availablePowers(gameID int64) ([]string, error) {
	players, err := m.store.GetPlayersByGame(gameID)
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	for _, p := range players {
		taken[p.Power] = true
	}
	var available []string
	for _, nation := range classical.Nations {
		name := string(nation)
		if !taken[name] {
			available = append(available, name)
		}
	}
	return available, nil
}

func isValidPower(name string) bool {
	for _, n := range classical.Nations {
		if string(n) == name {
			return true
		}
	}
	return false
}
