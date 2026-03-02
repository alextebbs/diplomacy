package game

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/zond/godip"
	"github.com/zond/godip/variants/classical"
)

var powerColors = map[godip.Nation]string{
	godip.Austria: "#C48F85",
	godip.England: "#7E9CC7",
	godip.France:  "#8FBCDB",
	godip.Germany: "#A0A0A0",
	godip.Italy:   "#7EAD7E",
	godip.Russia:  "#B399B0",
	godip.Turkey:  "#E6D77B",
	godip.Neutral:  "#E8D0A0",
}

var unitSymbols = map[godip.UnitType]string{
	godip.Army:  "A",
	godip.Fleet: "F",
}

// centerCoordRe extracts the first absolute coordinates from an SVG path "m x,y" or "M x,y"
var centerCoordRe = regexp.MustCompile(`(?i)[mM]\s*([-\d.]+)[,\s]+([-\d.]+)`)

// provincePathRe matches path elements with province IDs in the provinces layer
var provincePathRe = regexp.MustCompile(`<path[^>]*id="([a-z/]+)"[^>]*style="([^"]*)"[^>]*/?>`)

type Renderer struct {
	baseSVG     []byte
	centers     map[string][2]float64 // province -> (x, y)
	initialized bool
}

func NewRenderer() (*Renderer, error) {
	mapSVG, err := classical.ClassicalVariant.SVGMap()
	if err != nil {
		return nil, fmt.Errorf("load map SVG: %w", err)
	}

	r := &Renderer{
		baseSVG: normalizeFonts(mapSVG),
		centers: make(map[string][2]float64),
	}

	r.extractCenters()
	r.initialized = true
	return r, nil
}

// normalizeFonts fixes inconsistent font styling in the godip SVG template.
// Inkscape exported some text elements with font-family:LibreBaskerville-Bold
// but font-weight:normal, which causes rsvg-convert to render them as
// regular weight. This normalizes those to use standard CSS font-weight/style.
func normalizeFonts(svg []byte) []byte {
	s := string(svg)

	boldStyleRe := regexp.MustCompile(
		`font-weight:normal;([^"]*?)font-family:LibreBaskerville-Bold,\s*'Libre Baskerville'`,
	)
	s = boldStyleRe.ReplaceAllString(s,
		`font-weight:bold;${1}font-family:'Libre Baskerville'`,
	)

	boldStyleRe2 := regexp.MustCompile(
		`font-family:LibreBaskerville-Bold,\s*'Libre Baskerville';([^"]*?)font-weight:normal`,
	)
	s = boldStyleRe2.ReplaceAllString(s,
		`font-family:'Libre Baskerville';${1}font-weight:bold`,
	)

	s = strings.ReplaceAll(s,
		"font-family:LibreBaskerville-Bold, 'Libre Baskerville'",
		"font-family:'Libre Baskerville'",
	)

	italicStyleRe := regexp.MustCompile(
		`font-family:LibreBaskerville-Italic,\s*'Libre Baskerville'`,
	)
	s = italicStyleRe.ReplaceAllString(s,
		`font-family:'Libre Baskerville'`,
	)

	return []byte(s)
}

func (r *Renderer) extractCenters() {
	// Find all elements with id ending in "Center" to get province center coordinates
	re := regexp.MustCompile(`id="([a-z/]+)Center"[^>]*d="([^"]*)"`)
	matches := re.FindAllStringSubmatch(string(r.baseSVG), -1)
	for _, m := range matches {
		provID := m[1]
		pathData := m[2]
		if coords := extractFirstCoords(pathData); coords != nil {
			r.centers[provID] = *coords
		}
	}
}

func extractFirstCoords(pathData string) *[2]float64 {
	match := centerCoordRe.FindStringSubmatch(pathData)
	if match == nil {
		return nil
	}
	x, err1 := strconv.ParseFloat(match[1], 64)
	y, err2 := strconv.ParseFloat(match[2], 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &[2]float64{x, y}
}

// RenderMap produces an SVG of the current board state.
func (r *Renderer) RenderMap(stateJSON string) ([]byte, error) {
	var ps PhaseState
	if err := json.Unmarshal([]byte(stateJSON), &ps); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	svg := string(r.baseSVG)

	// Color provinces by ownership
	svg = r.colorProvinces(svg, ps.SupplyCenters)

	// Make the provinces layer visible (replace display:none with display:inline
	// in any existing style attribute on the provinces group)
	provRe := regexp.MustCompile(`(id="provinces"[^>]*style=")([^"]*)(")`)
	if provRe.MatchString(svg) {
		svg = provRe.ReplaceAllStringFunc(svg, func(match string) string {
			return strings.ReplaceAll(match, "display:none", "display:inline")
		})
	}

	// Add unit markers
	svg = r.addUnitMarkers(svg, ps.Units)

	return []byte(svg), nil
}

// RenderMapWithOrders adds order overlay arrows for a single player's orders.
func (r *Renderer) RenderMapWithOrders(stateJSON string, playerOrders []string) ([]byte, error) {
	svgBytes, err := r.RenderMap(stateJSON)
	if err != nil {
		return nil, err
	}

	svg := string(svgBytes)

	orderLines := r.buildOrderOverlay(playerOrders)
	svg = r.injectIntoLayer(svg, "orders", orderLines)
	return []byte(svg), nil
}

// RenderHistoryMap renders a historical phase with all orders and their results.
func (r *Renderer) RenderHistoryMap(stateJSON, ordersJSON, resultsJSON string) ([]byte, error) {
	svgBytes, err := r.RenderMap(stateJSON)
	if err != nil {
		return nil, err
	}

	if ordersJSON == "" {
		return svgBytes, nil
	}

	var allOrders map[string][]string
	if err := json.Unmarshal([]byte(ordersJSON), &allOrders); err != nil {
		return svgBytes, nil
	}

	var results map[string]string
	if resultsJSON != "" {
		json.Unmarshal([]byte(resultsJSON), &results)
	}

	// Collect all order strings
	var orderStrings []string
	for _, orders := range allOrders {
		orderStrings = append(orderStrings, orders...)
	}

	svg := string(svgBytes)
	orderLines := r.buildOrderOverlay(orderStrings)
	svg = r.injectIntoLayer(svg, "orders", orderLines)
	return []byte(svg), nil
}

// injectIntoLayer makes a layer visible and injects content into it.
// Handles both self-closing (<g ... />) and open (<g ...>...</g>) group elements,
// and replaces display:none rather than adding a duplicate style attribute.
func (r *Renderer) injectIntoLayer(svg, layerID, content string) string {
	// Replace display:none with display:inline in the layer's existing style
	layerRe := regexp.MustCompile(fmt.Sprintf(`(id="%s"[^>]*style=")([^"]*)(")`, layerID))
	svg = layerRe.ReplaceAllStringFunc(svg, func(match string) string {
		return strings.ReplaceAll(match, "display:none", "display:inline")
	})

	// Convert self-closing tag to open tag and inject content before closing </g>
	selfClose := regexp.MustCompile(fmt.Sprintf(`(<g[^>]*id="%s"[^>]*)/\s*>`, layerID))
	if selfClose.MatchString(svg) {
		svg = selfClose.ReplaceAllString(svg, fmt.Sprintf(`$1>%s</g>`, content))
	} else {
		// Insert content right after the opening tag
		openTag := regexp.MustCompile(fmt.Sprintf(`(<g[^>]*id="%s"[^>]*>)`, layerID))
		svg = openTag.ReplaceAllString(svg, fmt.Sprintf(`$1%s`, content))
	}

	return svg
}

func (r *Renderer) colorProvinces(svg string, supplyCenters map[godip.Province]godip.Nation) string {
	for prov, nation := range supplyCenters {
		color := powerColors[nation]
		if color == "" {
			color = powerColors[godip.Neutral]
		}

		provStr := strings.ToLower(string(prov))
		// Match the province path and update its fill color
		idPattern := fmt.Sprintf(`id="%s"`, provStr)
		if strings.Contains(svg, idPattern) {
			// Find the style attribute for this province and update the fill
			re := regexp.MustCompile(fmt.Sprintf(`(id="%s"[^>]*style=")([^"]*)(")`, provStr))
			svg = re.ReplaceAllStringFunc(svg, func(match string) string {
				parts := re.FindStringSubmatch(match)
				if len(parts) < 4 {
					return match
				}
				style := parts[2]
				// Replace or add fill color
				if strings.Contains(style, "fill:") {
					style = regexp.MustCompile(`fill:[^;]+`).ReplaceAllString(style, "fill:"+color)
				} else {
					style = "fill:" + color + ";" + style
				}
				// Make sure it's visible
				style = strings.ReplaceAll(style, "display:none", "display:inline")
				return parts[1] + style + parts[3]
			})
		}
	}
	return svg
}

func (r *Renderer) addUnitMarkers(svg string, units map[godip.Province]godip.Unit) string {
	var markers strings.Builder
	markers.WriteString(`<g id="unit-markers" style="display:inline">`)

	for prov, unit := range units {
		provStr := strings.ToLower(string(prov.Super()))
		coords, ok := r.centers[provStr]
		if !ok {
			// Try with coast
			provStr = strings.ToLower(string(prov))
			coords, ok = r.centers[provStr]
			if !ok {
				continue
			}
		}

		x, y := coords[0], coords[1]
		color := powerColors[unit.Nation]
		symbol := unitSymbols[unit.Type]

		// Draw a colored circle with the unit letter
		markers.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="10" fill="%s" stroke="#000" stroke-width="1.5"/>`,
			x, y, color,
		))
		markers.WriteString(fmt.Sprintf(
			`<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" font-size="12" font-weight="bold" fill="#000">%s</text>`,
			x, y, symbol,
		))
	}

	markers.WriteString(`</g>`)

	// Insert before the closing </svg> tag
	svg = strings.Replace(svg, "</svg>", markers.String()+"</svg>", 1)
	return svg
}

func (r *Renderer) buildOrderOverlay(orderStrings []string) string {
	var lines strings.Builder

	for _, orderStr := range orderStrings {
		tokens, err := ParseRawOrder(orderStr)
		if err != nil || len(tokens) < 2 {
			continue
		}

		src := tokens[0]
		srcCoords, ok := r.centers[src]
		if !ok {
			continue
		}

		switch tokens[1] {
		case "Move", "MoveViaConvoy":
			if len(tokens) >= 3 {
				dst := tokens[2]
				dstCoords, ok := r.centers[dst]
				if !ok {
					continue
				}
				lines.WriteString(fmt.Sprintf(
					`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>`,
					srcCoords[0], srcCoords[1], dstCoords[0], dstCoords[1],
				))
			}

		case "Support":
			if len(tokens) >= 3 {
				target := tokens[2]
				targetCoords, ok := r.centers[target]
				if !ok {
					continue
				}
				lines.WriteString(fmt.Sprintf(
					`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#666" stroke-width="1.5" stroke-dasharray="5,3"/>`,
					srcCoords[0], srcCoords[1], targetCoords[0], targetCoords[1],
				))
			}

		case "Convoy":
			if len(tokens) >= 4 {
				dst := tokens[3]
				dstCoords, ok := r.centers[dst]
				if !ok {
					continue
				}
				lines.WriteString(fmt.Sprintf(
					`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#999" stroke-width="1.5" stroke-dasharray="2,4"/>`,
					srcCoords[0], srcCoords[1], dstCoords[0], dstCoords[1],
				))
			}

		case "Hold":
			lines.WriteString(fmt.Sprintf(
				`<circle cx="%.1f" cy="%.1f" r="14" fill="none" stroke="#333" stroke-width="2"/>`,
				srcCoords[0], srcCoords[1],
			))
		}
	}

	return lines.String()
}

// ConvertSVGToPNG converts SVG data to PNG using rsvg-convert.
func ConvertSVGToPNG(svgData []byte) ([]byte, error) {
	cmd := exec.Command("rsvg-convert", "-f", "png", "-w", "2048")
	cmd.Stdin = bytes.NewReader(svgData)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rsvg-convert: %v: %s", err, errBuf.String())
	}
	return out.Bytes(), nil
}

// RenderMapPNG renders the map and converts to PNG. Falls back to SVG if rsvg-convert is unavailable.
func (r *Renderer) RenderMapPNG(stateJSON string) ([]byte, string, error) {
	svgData, err := r.RenderMap(stateJSON)
	if err != nil {
		return nil, "", err
	}

	pngData, err := ConvertSVGToPNG(svgData)
	if err != nil {
		log.Printf("PNG conversion failed (falling back to SVG): %v", err)
		return svgData, "map.svg", nil
	}
	return pngData, "map.png", nil
}
