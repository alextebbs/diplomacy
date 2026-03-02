package game

import (
	"fmt"
	"strings"

	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"
)

// ParseRawOrder parses a human-readable order string like "A PAR - BUR" into
// godip tokens suitable for classical.Parser.Parse.
func ParseRawOrder(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty order")
	}

	input = strings.ToUpper(input)

	// Normalize "VIA" at end for convoy moves
	hasVia := strings.HasSuffix(input, " VIA")
	if hasVia {
		input = strings.TrimSuffix(input, " VIA")
	}

	parts := strings.Fields(input)
	if len(parts) < 2 {
		return nil, fmt.Errorf("order too short: %q", input)
	}

	unitType := parts[0]
	if unitType != "A" && unitType != "F" {
		return nil, fmt.Errorf("invalid unit type %q (use A or F)", unitType)
	}

	province := normalizeProvince(parts[1])

	if len(parts) == 2 {
		return nil, fmt.Errorf("missing action in order %q", input)
	}

	action := parts[2]

	switch action {
	case "H", "HOLD":
		return []string{province, "Hold"}, nil

	case "-", "->", "MOVE":
		if len(parts) < 4 {
			return nil, fmt.Errorf("move order missing destination: %q", input)
		}
		dest := normalizeProvince(parts[3])
		if hasVia {
			return []string{province, "MoveViaConvoy", dest}, nil
		}
		return []string{province, "Move", dest}, nil

	case "S", "SUPPORT":
		return parseSupportOrder(province, parts[3:])

	case "C", "CONVOY":
		return parseConvoyOrder(province, parts[3:])

	case "B", "BUILD":
		return []string{province, "Build", unitTypeToGodip(unitType)}, nil

	case "D", "DISBAND":
		return []string{province, "Disband"}, nil

	case "R", "RETREAT":
		if len(parts) < 4 {
			return nil, fmt.Errorf("retreat order missing destination: %q", input)
		}
		dest := normalizeProvince(parts[3])
		return []string{province, "Move", dest}, nil

	default:
		return nil, fmt.Errorf("unknown action %q in order %q", action, input)
	}
}

func parseSupportOrder(src string, rest []string) ([]string, error) {
	if len(rest) < 2 {
		return nil, fmt.Errorf("support order too short")
	}

	// rest[0] is unit type (A/F), rest[1] is province
	supportedProv := normalizeProvince(rest[1])

	// Support hold: "A MAR S A PAR" (no more tokens)
	if len(rest) == 2 {
		return []string{src, "Support", supportedProv}, nil
	}

	// Support move: "A MAR S A PAR - BUR"
	if rest[2] == "-" || rest[2] == "->" || rest[2] == "MOVE" {
		if len(rest) < 4 {
			return nil, fmt.Errorf("support move order missing destination")
		}
		dest := normalizeProvince(rest[3])
		return []string{src, "Support", supportedProv, dest}, nil
	}

	return nil, fmt.Errorf("malformed support order")
}

func parseConvoyOrder(src string, rest []string) ([]string, error) {
	if len(rest) < 4 {
		return nil, fmt.Errorf("convoy order too short (expect: F SEA C A SRC - DST)")
	}

	// rest: A SRC - DST
	convoyedProv := normalizeProvince(rest[1])

	if rest[2] != "-" && rest[2] != "->" && rest[2] != "MOVE" {
		return nil, fmt.Errorf("malformed convoy order, expected '-' got %q", rest[2])
	}

	dest := normalizeProvince(rest[3])
	return []string{src, "Convoy", convoyedProv, dest}, nil
}

func normalizeProvince(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "/", "/")
	return s
}

func unitTypeToGodip(ut string) string {
	if ut == "A" {
		return string(godip.Army)
	}
	return string(godip.Fleet)
}

// ParseAndValidateOrder parses a raw order string and validates it using the
// classical parser. Returns the godip tokens on success.
func ParseAndValidateOrder(input string) ([]string, error) {
	tokens, err := ParseRawOrder(input)
	if err != nil {
		return nil, err
	}

	// Validate through godip's parser
	_, err = classical.Parser.Parse(tokens)
	if err != nil {
		return nil, fmt.Errorf("invalid order %q: %w", input, err)
	}

	return tokens, nil
}

// ParseMultipleOrders splits a comma-separated order string and parses each one.
func ParseMultipleOrders(input string) ([][]string, error) {
	parts := strings.Split(input, ",")
	var results [][]string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens, err := ParseAndValidateOrder(part)
		if err != nil {
			return nil, err
		}
		results = append(results, tokens)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no orders provided")
	}
	return results, nil
}
