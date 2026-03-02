package game

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/sammy/diplomacy/internal/db"
)

// PhaseCallback is called after a phase is processed. Used by the bot to post
// results to the appropriate Discord channel.
type PhaseCallback func(g *db.Game, results map[string]string, gameOver bool)

type Scheduler struct {
	mgr      *Manager
	store    *db.Store
	callback PhaseCallback
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
}

func NewScheduler(mgr *Manager, store *db.Store) *Scheduler {
	return &Scheduler{
		mgr:      mgr,
		store:    store,
		interval: 30 * time.Second,
	}
}

func (s *Scheduler) SetCallback(cb PhaseCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = cb
}

func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go s.run(ctx)
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	games, err := s.store.GetDueGames(time.Now())
	if err != nil {
		log.Printf("Scheduler: failed to get due games: %v", err)
		return
	}

	for _, g := range games {
		s.processGame(g)
	}
}

func (s *Scheduler) processGame(g *db.Game) {
	log.Printf("Scheduler: processing game %d (%s) - %s", g.ID, g.Name, g.Phase)

	updated, results, gameOver, err := s.mgr.ProcessPhase(g.ID)
	if err != nil {
		log.Printf("Scheduler: failed to process game %d: %v", g.ID, err)
		return
	}

	s.mu.Lock()
	cb := s.callback
	s.mu.Unlock()

	if cb != nil {
		cb(updated, results, gameOver)
	}
}

// CheckReady checks if all players in a game are ready and triggers adjudication if so.
func (s *Scheduler) CheckReady(gameID int64) (bool, error) {
	allReady, err := s.store.AllPlayersReady(gameID)
	if err != nil {
		return false, err
	}
	if !allReady {
		return false, nil
	}

	g, err := s.store.GetGame(gameID)
	if err != nil || g == nil {
		return false, err
	}
	if g.Status != db.GameStatusActive {
		return false, nil
	}

	log.Printf("Scheduler: all players ready for game %d (%s), adjudicating early", g.ID, g.Name)

	updated, results, gameOver, err := s.mgr.ProcessPhase(gameID)
	if err != nil {
		return false, err
	}

	s.mu.Lock()
	cb := s.callback
	s.mu.Unlock()

	if cb != nil {
		cb(updated, results, gameOver)
	}

	return true, nil
}

// Tick is exposed for testing to manually trigger a scheduler cycle.
func (s *Scheduler) Tick() {
	s.tick()
}
