package viz_test

import (
	"testing"

	"github.com/justintout/go-sqlite-graph/viz"
)

func TestPaletteCycling(t *testing.T) {
	p := viz.NewPalette(nil)
	first := viz.PaletteColorFor(p, 0)
	wrapped := viz.PaletteColorFor(p, 10)
	if first != wrapped {
		t.Errorf("expected color at index 10 to wrap to index 0: got %q and %q", first, wrapped)
	}

	second := viz.PaletteColorFor(p, 1)
	if first == second {
		t.Error("expected different colors for different indices")
	}
}

func TestCustomPalette(t *testing.T) {
	colors := []string{"#111", "#222", "#333"}
	p := viz.NewPalette(colors)
	if got := viz.PaletteColorFor(p, 0); got != "#111" {
		t.Errorf("expected #111, got %s", got)
	}
	if got := viz.PaletteColorFor(p, 3); got != "#111" {
		t.Errorf("expected wrap to #111, got %s", got)
	}
}
