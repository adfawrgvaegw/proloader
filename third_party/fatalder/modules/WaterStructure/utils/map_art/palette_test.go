package map_art

import "testing"

func TestPaletteLoads(t *testing.T) {
	if err := loadPalette(); err != nil {
		t.Fatalf("loadPalette: %v", err)
	}
}
