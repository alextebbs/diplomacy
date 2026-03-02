package db

import "time"

type GameStatus string

const (
	GameStatusPending  GameStatus = "pending"
	GameStatusActive   GameStatus = "active"
	GameStatusFinished GameStatus = "finished"
)

type Game struct {
	ID           int64
	GuildID      string
	ChannelID    string
	Name         string
	Variant      string
	Phase        string
	StateJSON    string
	TurnDuration int64 // seconds
	NextDeadline *time.Time
	Status       GameStatus
	CreatedAt    time.Time
}

type Player struct {
	ID      int64
	GameID  int64
	UserID  string
	Power   string
	IsReady bool
}

type Order struct {
	ID        int64
	GameID    int64
	Phase     string
	UserID    string
	Power     string
	OrderText string
	CreatedAt time.Time
}

type PhaseHistory struct {
	ID          int64
	GameID      int64
	Phase       string
	Season      string
	Year        int
	PhaseType   string
	StateJSON   string
	OrdersJSON  *string
	ResultsJSON *string
	CreatedAt   time.Time
}
