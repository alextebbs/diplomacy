package game

import (
	"strings"
	"testing"

	"github.com/zond/godip/variants/classical"
)

func TestNewRenderer(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	if r == nil {
		t.Fatal("renderer is nil")
	}
	if len(r.baseSVG) == 0 {
		t.Error("base SVG is empty")
	}
	if len(r.centers) == 0 {
		t.Error("no province centers extracted")
	}
}

func TestRenderInitialMap(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	svgData, err := r.RenderMap(stateJSON)
	if err != nil {
		t.Fatalf("render map: %v", err)
	}

	if len(svgData) == 0 {
		t.Error("rendered SVG is empty")
	}

	svg := string(svgData)

	// Should contain SVG elements
	if !strings.Contains(svg, "<svg") {
		t.Error("output should contain SVG tag")
	}

	// Should contain unit markers
	if !strings.Contains(svg, "unit-markers") {
		t.Error("output should contain unit markers")
	}
}

func TestRenderMapWithOrders(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	orders := []string{"A PAR - BUR", "F BRE - ENG"}
	svgData, err := r.RenderMapWithOrders(stateJSON, orders)
	if err != nil {
		t.Fatalf("render with orders: %v", err)
	}

	if len(svgData) == 0 {
		t.Error("rendered SVG is empty")
	}
}

func TestRenderEmptyOrders(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	svgData, err := r.RenderMap(stateJSON)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	svg := string(svgData)
	// Should not contain any stray order overlay
	if strings.Contains(svg, "marker-end") {
		t.Error("clean map should not contain order arrows")
	}
}

func TestRenderHistoryMap(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	s, _ := classical.Start()
	stateJSON, _ := SerializeState(s)

	ordersJSON := `{"England":["F LON - NTH"],"France":["A PAR - BUR"]}`
	resultsJSON := `{"lon":"OK","par":"OK"}`

	svgData, err := r.RenderHistoryMap(stateJSON, ordersJSON, resultsJSON)
	if err != nil {
		t.Fatalf("render history: %v", err)
	}

	if len(svgData) == 0 {
		t.Error("rendered SVG is empty")
	}
}

func TestExtractFirstCoords(t *testing.T) {
	tests := []struct {
		name     string
		pathData string
		wantNil  bool
	}{
		{"absolute", "M 100.5,200.3 L 300,400", false},
		{"relative", "m 50.2,75.8 c 1,2,3,4", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFirstCoords(tt.pathData)
			if (result == nil) != tt.wantNil {
				t.Errorf("extractFirstCoords(%q) nil = %v, want %v", tt.pathData, result == nil, tt.wantNil)
			}
		})
	}
}
