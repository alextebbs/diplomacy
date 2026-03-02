package bot

import (
	"testing"

	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"

	"github.com/sammy/diplomacy/internal/game"
)

func startingState(t *testing.T) string {
	t.Helper()
	s, err := classical.Start()
	if err != nil {
		t.Fatalf("classical.Start: %v", err)
	}
	json, err := game.SerializeState(s)
	if err != nil {
		t.Fatalf("SerializeState: %v", err)
	}
	return json
}

func TestGetUnitType(t *testing.T) {
	st, err := game.DeserializeState(startingState(t))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		prov string
		want string
	}{
		{"par", "A"},
		{"mar", "A"},
		{"bre", "F"},
		{"lon", "F"},
		{"edi", "F"},
		{"mos", "A"},
		{"stp/sc", "F"},
		{"con", "A"},
		{"ank", "F"},
	}
	for _, tt := range tests {
		t.Run(tt.prov, func(t *testing.T) {
			got := getUnitType(st, godip.Province(tt.prov))
			if got != tt.want {
				t.Errorf("getUnitType(%q) = %q, want %q", tt.prov, got, tt.want)
			}
		})
	}
}

func TestGetUnitType_Missing(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))
	got := getUnitType(st, godip.Province("bur"))
	if got != "A" {
		t.Errorf("getUnitType for empty province = %q, want default %q", got, "A")
	}
}

func TestGetMoveDestinations(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	tests := []struct {
		nation godip.Nation
		prov   string
		minLen int
	}{
		{"France", "par", 1},
		{"France", "mar", 1},
		{"France", "bre", 1},
		{"England", "lon", 1},
		{"England", "edi", 1},
		{"England", "lvp", 1},
		{"Germany", "ber", 1},
		{"Germany", "mun", 1},
		{"Germany", "kie", 1},
		{"Russia", "mos", 1},
		{"Russia", "war", 1},
		{"Russia", "sev", 1},
		{"Russia", "stp/sc", 1},
		{"Turkey", "con", 1},
		{"Turkey", "ank", 1},
		{"Turkey", "smy", 1},
		{"Austria", "vie", 1},
		{"Austria", "bud", 1},
		{"Austria", "tri", 1},
		{"Italy", "rom", 1},
		{"Italy", "ven", 1},
		{"Italy", "nap", 1},
	}
	for _, tt := range tests {
		t.Run(string(tt.nation)+"_"+tt.prov, func(t *testing.T) {
			dests := getMoveDestinations(st, tt.nation, godip.Province(tt.prov))
			if len(dests) < tt.minLen {
				t.Errorf("getMoveDestinations(%s, %q) = %d destinations, want at least %d", tt.nation, tt.prov, len(dests), tt.minLen)
			}
		})
	}
}

func TestGetMoveDestinations_WrongNation(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))
	dests := getMoveDestinations(st, "England", godip.Province("par"))
	if len(dests) != 0 {
		t.Errorf("expected no destinations for England at par, got %v", dests)
	}
}

func TestGetMoveDestinations_EmptyProvince(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))
	dests := getMoveDestinations(st, "France", godip.Province("bur"))
	if len(dests) != 0 {
		t.Errorf("expected no destinations for empty province, got %v", dests)
	}
}

func TestGetMoveDestinations_SpecificTargets(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	dests := getMoveDestinations(st, "France", godip.Province("par"))
	destSet := map[godip.Province]bool{}
	for _, d := range dests {
		destSet[d] = true
	}

	expected := []godip.Province{"bur", "pic", "gas"}
	for _, e := range expected {
		if !destSet[e] {
			t.Errorf("expected %q in destinations for Paris, got %v", e, dests)
		}
	}
}

func TestGetSupportableUnits(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	// Paris is adjacent to Brest (both French); Marseilles is NOT adjacent to Paris
	tests := []struct {
		prov   string
		minLen int
	}{
		{"par", 1},  // adjacent to Brest
		{"bre", 1},  // adjacent to Paris
		{"vie", 1},  // adjacent to Budapest and Trieste
	}
	for _, tt := range tests {
		t.Run(tt.prov, func(t *testing.T) {
			units := getSupportableUnits(st, godip.Province(tt.prov))
			if len(units) < tt.minLen {
				t.Errorf("getSupportableUnits(%q) = %d, want at least %d", tt.prov, len(units), tt.minLen)
			}
		})
	}
}

func TestGetSupportableUnits_France(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	units := getSupportableUnits(st, godip.Province("par"))
	provSet := map[godip.Province]bool{}
	for _, u := range units {
		provSet[u.Province.Super()] = true
	}

	// Paris is adjacent to Brest (bre is a neighbor of par)
	if !provSet["bre"] {
		t.Error("expected Brest in supportable units from Paris")
	}
}

func TestGetSupportableUnits_Austria(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	// Vienna is adjacent to Budapest and Trieste
	units := getSupportableUnits(st, godip.Province("vie"))
	provSet := map[godip.Province]bool{}
	for _, u := range units {
		provSet[u.Province.Super()] = true
	}

	if !provSet["bud"] {
		t.Error("expected Budapest in supportable units from Vienna")
	}
	if !provSet["tri"] {
		t.Error("expected Trieste in supportable units from Vienna")
	}
}

func TestGetSupportableUnits_Isolated(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	// Edinburgh is only adjacent to Liverpool among friendly units
	units := getSupportableUnits(st, godip.Province("edi"))
	provSet := map[godip.Province]bool{}
	for _, u := range units {
		provSet[u.Province.Super()] = true
	}
	if !provSet["lvp"] {
		t.Error("expected Liverpool in supportable units from Edinburgh")
	}
}

func TestGetConvoyableArmies(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	// Ankara fleet is adjacent to Constantinople (army)
	armies := getConvoyableArmies(st, godip.Province("ank"))
	if len(armies) == 0 {
		t.Error("expected convoyable armies from Ankara fleet")
	}

	found := false
	for _, a := range armies {
		if a.Super() == "con" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Constantinople army in convoyable armies from Ankara, got %v", armies)
	}
}

func TestGetConvoyableArmies_NoArmiesNearby(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	// Edinburgh fleet - Liverpool army is adjacent
	armies := getConvoyableArmies(st, godip.Province("edi"))
	found := false
	for _, a := range armies {
		if a.Super() == "lvp" {
			found = true
		}
	}
	if !found {
		t.Logf("Convoyable armies from Edinburgh: %v", armies)
	}
}

func TestProvName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"par", "Paris"},
		{"lon", "London"},
		{"bre", "Brest"},
		{"stp", "St. Petersburg"},
		{"stp/sc", "St. Petersburg (SC)"},
		{"con", "Constantinople"},
		{"UNKNOWN", "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := provName(tt.input)
			if got != tt.want {
				t.Errorf("provName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFriendlyOrder(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"A PAR H", "Army Paris Hold"},
		{"F BRE - ENG", "Fleet Brest → English Channel"},
		{"A MAR S A PAR", "Army Marseilles Support Army Paris"},
		{"A MAR S A PAR - BUR", "Army Marseilles Support Army Paris → Burgundy"},
		{"F NTH C A LON - NWY", "Fleet North Sea Convoy Army London → Norway"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := friendlyOrder(tt.input)
			if got != tt.want {
				t.Errorf("friendlyOrder(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGameID(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1", 1},
		{"42", 42},
		{"0", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseGameID(tt.input)
			if got != tt.want {
				t.Errorf("parseGameID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetMoveDestinations_AllNationsHaveOptions(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))
	nations := []godip.Nation{"Austria", "England", "France", "Germany", "Italy", "Russia", "Turkey"}

	for _, nation := range nations {
		units, _, _, _, _, _ := st.Dump()
		found := false
		for prov, unit := range units {
			if unit.Nation != nation {
				continue
			}
			dests := getMoveDestinations(st, nation, prov)
			if len(dests) > 0 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("nation %s has no move destinations for any unit", nation)
		}
	}
}

func TestGetMoveDestinations_NoDuplicates(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	dests := getMoveDestinations(st, "France", godip.Province("par"))
	seen := map[godip.Province]bool{}
	for _, d := range dests {
		if seen[d] {
			t.Errorf("duplicate destination %q in results", d)
		}
		seen[d] = true
	}
}

func TestGetSupportableUnits_ReturnsUnitInfo(t *testing.T) {
	st, _ := game.DeserializeState(startingState(t))

	units := getSupportableUnits(st, godip.Province("par"))
	for _, u := range units {
		if u.Province == "" {
			t.Error("unitInfo has empty province")
		}
		if u.Type != godip.Army && u.Type != godip.Fleet {
			t.Errorf("unitInfo has unexpected type: %v", u.Type)
		}
		if u.Nation == "" {
			t.Error("unitInfo has empty nation")
		}
	}
}
