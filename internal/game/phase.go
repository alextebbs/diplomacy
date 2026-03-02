package game

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/sammy/diplomacy/internal/db"
	"github.com/zond/godip"
	"github.com/zond/godip/orders"
	"github.com/zond/godip/state"
	"github.com/zond/godip/variants/classical"
)

// ProcessPhase adjudicates the current phase of a game and advances to the next.
// Returns the updated game, adjudication results summary, and whether the game ended.
func (m *Manager) ProcessPhase(gameID int64) (*db.Game, map[string]string, bool, error) {
	g, err := m.store.GetGame(gameID)
	if err != nil {
		return nil, nil, false, fmt.Errorf("get game: %w", err)
	}
	if g == nil {
		return nil, nil, false, fmt.Errorf("game not found")
	}
	if g.Status != db.GameStatusActive {
		return nil, nil, false, fmt.Errorf("game is not active")
	}

	s, err := DeserializeState(g.StateJSON)
	if err != nil {
		return nil, nil, false, fmt.Errorf("deserialize state: %w", err)
	}

	dbOrders, err := m.store.GetOrdersForPhase(g.ID, g.Phase)
	if err != nil {
		return nil, nil, false, fmt.Errorf("get orders: %w", err)
	}

	if err := applyOrders(s, dbOrders); err != nil {
		log.Printf("Warning: error applying some orders for game %d: %v", gameID, err)
	}

	prePhase := s.Phase()
	preStateJSON, _ := SerializeState(s)

	if err := s.Next(); err != nil {
		return nil, nil, false, fmt.Errorf("adjudicate: %w", err)
	}

	// Collect resolutions from the pre-adjudication state
	_, _, _, _, _, resolutions := func() (
		map[godip.Province]godip.Unit,
		map[godip.Province]godip.Nation,
		map[godip.Province]godip.Unit,
		map[godip.Province]godip.Province,
		map[godip.Province]map[godip.Province]bool,
		map[godip.Province]error,
	) {
		// We need to get resolutions, but Next() already advanced.
		// Re-parse the pre-state and re-adjudicate to capture resolutions.
		preState, _ := DeserializeState(preStateJSON)
		applyOrders(preState, dbOrders)

		// Run adjudication again to get resolutions
		preState.Next()
		return preState.Dump()
	}()

	results := map[string]string{}
	for prov, err := range resolutions {
		if err == nil {
			results[string(prov)] = "OK"
		} else {
			results[string(prov)] = err.Error()
		}
	}

	// Save phase history
	ordersMap := buildOrdersMap(dbOrders)
	ordersJSON, _ := json.Marshal(ordersMap)
	resultsJSON, _ := json.Marshal(results)

	ordersJSONStr := string(ordersJSON)
	resultsJSONStr := string(resultsJSON)

	history := &db.PhaseHistory{
		GameID:      g.ID,
		Phase:       g.Phase,
		Season:      string(prePhase.Season()),
		Year:        prePhase.Year(),
		PhaseType:   string(prePhase.Type()),
		StateJSON:   preStateJSON,
		OrdersJSON:  &ordersJSONStr,
		ResultsJSON: &resultsJSONStr,
	}
	if _, err := m.store.SavePhaseHistory(history); err != nil {
		return nil, nil, false, fmt.Errorf("save phase history: %w", err)
	}

	// Check for victory
	newPhase := s.Phase()
	newStateJSON, err := SerializeState(s)
	if err != nil {
		return nil, nil, false, fmt.Errorf("serialize new state: %w", err)
	}

	gameOver := false
	newStatus := db.GameStatusActive

	if newPhase.Type() == godip.Adjustment {
		winner := checkVictory(s)
		if winner != "" {
			gameOver = true
			newStatus = db.GameStatusFinished
		}
	}

	newPhaseName := StatePhaseString(s)
	var nextDeadline *time.Time
	if !gameOver {
		d := time.Now().Add(time.Duration(g.TurnDuration) * time.Second)
		nextDeadline = &d
	}

	if err := m.store.UpdateGameState(g.ID, newPhaseName, newStateJSON, nextDeadline, newStatus); err != nil {
		return nil, nil, false, fmt.Errorf("update game state: %w", err)
	}

	// Clear orders and ready flags for the new phase
	if err := m.store.ClearOrdersForPhase(g.ID, g.Phase); err != nil {
		log.Printf("Warning: failed to clear orders: %v", err)
	}
	if err := m.store.ResetAllReady(g.ID); err != nil {
		log.Printf("Warning: failed to reset ready flags: %v", err)
	}

	// Reload game to return updated version
	g, _ = m.store.GetGame(gameID)
	return g, results, gameOver, nil
}

func applyOrders(s *state.State, dbOrders []*db.Order) error {
	var firstErr error
	for _, o := range dbOrders {
		tokens, err := ParseRawOrder(o.OrderText)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("parse order %q: %w", o.OrderText, err)
			}
			continue
		}
		adj, err := classical.Parser.Parse(tokens)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("validate order %q: %w", o.OrderText, err)
			}
			continue
		}
		prov := godip.Province(tokens[0])
		if err := s.SetOrder(prov, adj); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("set order at %s: %w", prov, err)
			}
		}
	}
	return firstErr
}

func buildOrdersMap(dbOrders []*db.Order) map[string][]string {
	result := map[string][]string{}
	for _, o := range dbOrders {
		result[o.Power] = append(result[o.Power], o.OrderText)
	}
	return result
}

func checkVictory(s *state.State) godip.Nation {
	_, supplyCenters, _, _, _, _ := s.Dump()
	counts := map[godip.Nation]int{}
	for _, nation := range supplyCenters {
		counts[nation]++
	}
	for nation, count := range counts {
		if nation != godip.Neutral && count >= 18 {
			return nation
		}
	}
	return ""
}

// GetPossibleOrders returns all valid orders for a nation in the current phase.
func GetPossibleOrders(s *state.State, nation godip.Nation) godip.Options {
	return s.Phase().Options(s, nation)
}

// BuildOrder creates a godip order from type and parameters.
func BuildOrder(orderType string, params ...godip.Province) (godip.Adjudicator, error) {
	switch orderType {
	case "Hold":
		if len(params) < 1 {
			return nil, fmt.Errorf("hold requires province")
		}
		return orders.Hold(params[0]), nil
	case "Move":
		if len(params) < 2 {
			return nil, fmt.Errorf("move requires source and destination")
		}
		return orders.Move(params[0], params[1]), nil
	case "MoveViaConvoy":
		if len(params) < 2 {
			return nil, fmt.Errorf("convoy move requires source and destination")
		}
		return orders.Move(params[0], params[1]).ViaConvoy(), nil
	case "Support":
		if len(params) == 2 {
			return orders.SupportHold(params[0], params[1]), nil
		}
		if len(params) == 3 {
			return orders.SupportMove(params[0], params[1], params[2]), nil
		}
		return nil, fmt.Errorf("support requires 2 or 3 provinces")
	case "Convoy":
		if len(params) < 3 {
			return nil, fmt.Errorf("convoy requires source, convoyed, and destination")
		}
		return orders.Convoy(params[0], params[1], params[2]), nil
	case "Build":
		if len(params) < 1 {
			return nil, fmt.Errorf("build requires province")
		}
		// Build is actually handled differently in godip; it requires unit type
		return nil, fmt.Errorf("use ParseRawOrder for build orders")
	case "Disband":
		if len(params) < 1 {
			return nil, fmt.Errorf("disband requires province")
		}
		return orders.Disband(params[0], time.Now()), nil
	default:
		return nil, fmt.Errorf("unknown order type: %s", orderType)
	}
}
