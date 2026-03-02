package game

import (
	"encoding/json"
	"fmt"

	"github.com/zond/godip"
	"github.com/zond/godip/state"
	"github.com/zond/godip/variants/classical"
)

// PhaseState is the JSON-serializable snapshot of a godip game state.
// Mirrors the structure from godip's gae/common.go.
type PhaseState struct {
	Season        godip.Season                                `json:"Season"`
	Year          int                                         `json:"Year"`
	Type          godip.PhaseType                             `json:"Type"`
	Units         map[godip.Province]godip.Unit               `json:"Units"`
	SupplyCenters map[godip.Province]godip.Nation              `json:"SupplyCenters"`
	Dislodgeds    map[godip.Province]godip.Unit               `json:"Dislodgeds"`
	Dislodgers    map[godip.Province]godip.Province            `json:"Dislodgers"`
	Bounces       map[godip.Province]map[godip.Province]bool   `json:"Bounces"`
	Resolutions   map[godip.Province]string                   `json:"Resolutions"`
}

func NewPhaseState(s *state.State) *PhaseState {
	p := s.Phase()
	ps := &PhaseState{
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

	return ps
}

func (ps *PhaseState) ToState() (*state.State, error) {
	phase := classical.NewPhase(ps.Year, ps.Season, ps.Type)
	s := classical.Blank(phase)

	orders := map[godip.Province]godip.Adjudicator{}
	s.Load(
		ps.Units,
		ps.SupplyCenters,
		ps.Dislodgeds,
		ps.Dislodgers,
		ps.Bounces,
		orders,
	)
	return s, nil
}

func SerializeState(s *state.State) (string, error) {
	ps := NewPhaseState(s)
	data, err := json.Marshal(ps)
	if err != nil {
		return "", fmt.Errorf("marshal state: %w", err)
	}
	return string(data), nil
}

func DeserializeState(data string) (*state.State, error) {
	var ps PhaseState
	if err := json.Unmarshal([]byte(data), &ps); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return ps.ToState()
}

func FormatPhaseName(season godip.Season, year int, phaseType godip.PhaseType) string {
	return fmt.Sprintf("%s %d %s", season, year, phaseType)
}

func StatePhaseString(s *state.State) string {
	p := s.Phase()
	return FormatPhaseName(p.Season(), p.Year(), p.Type())
}
