package main

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	bwoDefine "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsutils "github.com/Yeah114/WaterStructure/utils"
	wsmapart "github.com/Yeah114/WaterStructure/utils/map_art"
)

func TestGenerateMapArtToWorld_MultiMap(t *testing.T) {
	dir := t.TempDir()
	bw, err := world.Open(dir, nil)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	defer func() { _ = bw.CloseWorld() }()

	src := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x * 37) % 255),
				G: uint8((y * 71) % 255),
				B: uint8(((x + y) * 53) % 255),
				A: 255,
			})
		}
	}

	start := wsdefine.SubChunkPos{0, -4, 0}
	minPos, maxPos, err := wsmapart.GenerateMapArtToWorld(bw, src, &wsmapart.Options{
		StartSubChunkPos: start,
		MapWidth:         2,
		MapHeight:        2,
		Resample:         nil,
		Force2D:          false,
		Max3DHeight:      384,
	})
	if err != nil {
		t.Fatalf("GenerateMapArtToWorld: %v", err)
	}
	if minPos[0] != 0 || minPos[2] != -1 {
		t.Fatalf("unexpected minPos: %v", minPos)
	}
	if maxPos[0] != 255 || maxPos[2] != 255 {
		t.Fatalf("unexpected maxPos: %v", maxPos)
	}

	// Verify chunks exist where the art should have written blocks:
	// x in [0..255], z in [-1..255] => chunkX [0..15], chunkZ [-1..15].
	for _, pos := range []bwoDefine.ChunkPos{
		{0, -1},
		{0, 0},
		{15, 15},
	} {
		_, exists, err := bw.LoadChunk(bwoDefine.DimensionIDOverworld, pos)
		if err != nil {
			t.Fatalf("LoadChunk %v: %v", pos, err)
		}
		if !exists {
			t.Fatalf("expected chunk %v to exist", pos)
		}
	}

	mcWorld, err := wsutils.NewMCWorld(bw, context.Background())
	if err != nil {
		t.Fatalf("NewMCWorld: %v", err)
	}

	emeraldID, ok := block.StateToRuntimeID("minecraft:emerald_block", nil)
	if !ok {
		t.Fatalf("StateToRuntimeID(emerald_block) not found")
	}

	// Reference column at Z=-1 must contain emerald blocks (at some computed Y).
	if _, found := findRuntimeIDInColumn(t, mcWorld, 0, -1, emeraldID); !found {
		t.Fatalf("expected emerald block in reference column at x=0 z=-1")
	}
	if _, found := findRuntimeIDInColumn(t, mcWorld, 200, -1, emeraldID); !found {
		t.Fatalf("expected emerald block in reference column at x=200 z=-1")
	}

	// Across map boundaries, pixels must still be placed (non-air) at some Y.
	assertNonAirAt(t, mcWorld, 127, 0)
	assertNonAirAt(t, mcWorld, 128, 0)
	assertNonAirAt(t, mcWorld, 0, 127)
	assertNonAirAt(t, mcWorld, 0, 128)
}

func TestGenerateMapArtToWorld_NoReferenceColumn(t *testing.T) {
	dir := t.TempDir()
	bw, err := world.Open(dir, nil)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	defer func() { _ = bw.CloseWorld() }()

	src := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}

	start := wsdefine.SubChunkPos{0, -4, 0}
	minPos, maxPos, err := wsmapart.GenerateMapArtToWorld(bw, src, &wsmapart.Options{
		StartSubChunkPos:       start,
		MapWidth:               1,
		MapHeight:              1,
		Resample:               nil,
		DisableReferenceColumn: true,
		Force2D:                false,
		Max3DHeight:            384,
	})
	if err != nil {
		t.Fatalf("GenerateMapArtToWorld: %v", err)
	}
	if minPos[2] != 0 || maxPos[2] != 127 {
		t.Fatalf("unexpected Z bounds: min=%v max=%v", minPos, maxPos)
	}

	mcWorld, err := wsutils.NewMCWorld(bw, context.Background())
	if err != nil {
		t.Fatalf("NewMCWorld: %v", err)
	}

	// At Z=-1 there should be no emerald reference blocks.
	emeraldID, ok := block.StateToRuntimeID("minecraft:emerald_block", nil)
	if !ok {
		t.Fatalf("StateToRuntimeID(emerald_block) not found")
	}
	if _, found := findRuntimeIDInColumn(t, mcWorld, 0, -1, emeraldID); found {
		t.Fatalf("did not expect emerald block in reference column when disabled")
	}

	// But the art itself should be placed at Z>=0.
	assertNonAirAt(t, mcWorld, 0, 0)
}

func TestGenerateMapArtToWorld_Force2D(t *testing.T) {
	dir := t.TempDir()
	bw, err := world.Open(dir, nil)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	defer func() { _ = bw.CloseWorld() }()

	src := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 10), G: uint8(y * 10), B: 80, A: 255})
		}
	}

	start := wsdefine.SubChunkPos{0, -4, 0}
	minPos, maxPos, err := wsmapart.GenerateMapArtToWorld(bw, src, &wsmapart.Options{
		StartSubChunkPos: start,
		MapWidth:         1,
		MapHeight:        1,
		Resample:         nil,
		Force2D:          true,
	})
	if err != nil {
		t.Fatalf("GenerateMapArtToWorld: %v", err)
	}
	if minPos[1] != -64 || maxPos[1] != -64 {
		t.Fatalf("expected flat Y bounds at -64, got min=%v max=%v", minPos, maxPos)
	}

	mcWorld, err := wsutils.NewMCWorld(bw, context.Background())
	if err != nil {
		t.Fatalf("NewMCWorld: %v", err)
	}

	// Pixels should exist at y=-64, and nearby y should remain air.
	rid, err := mcWorld.LoadBlock(0, -64, 0)
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if rid == block.AirRuntimeID {
		t.Fatalf("expected non-air at y=-64 for a pixel")
	}
	above, err := mcWorld.LoadBlock(0, -63, 0)
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if above != block.AirRuntimeID {
		t.Fatalf("expected air above flat plane, got runtimeID=%d", above)
	}
}

func TestGenerateMapArtToWorld_Max3DHeight(t *testing.T) {
	dir := t.TempDir()
	bw, err := world.Open(dir, nil)
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	defer func() { _ = bw.CloseWorld() }()

	src := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			src.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x * 37) % 255),
				G: uint8((y * 71) % 255),
				B: uint8(((x + y) * 53) % 255),
				A: 255,
			})
		}
	}

	start := wsdefine.SubChunkPos{0, -4, 0} // baseY = -64
	minPos, maxPos, err := wsmapart.GenerateMapArtToWorld(bw, src, &wsmapart.Options{
		StartSubChunkPos: start,
		MapWidth:         1,
		MapHeight:        2,
		Resample:         nil,
		Force2D:          false,
		Max3DHeight:      8,
	})
	if err != nil {
		t.Fatalf("GenerateMapArtToWorld: %v", err)
	}
	if maxPos[1]-minPos[1] > 7 {
		t.Fatalf("expected Y span <= 7, got min=%v max=%v", minPos, maxPos)
	}
	if minPos[1] < -64 || maxPos[1] > 319 {
		t.Fatalf("expected within overworld range, got min=%v max=%v", minPos, maxPos)
	}
}

func assertNonAirAt(t *testing.T, mcWorld *wsutils.MCWorld, x int32, z int32) {
	t.Helper()
	if _, _, found := findAnyNonAirInColumn(t, mcWorld, x, z); !found {
		t.Fatalf("expected a non-air block at x=%d z=%d", x, z)
	}
}

func findAnyNonAirInColumn(t *testing.T, mcWorld *wsutils.MCWorld, x int32, z int32) (int16, uint32, bool) {
	t.Helper()
	for yi := -64; yi <= 320; yi++ {
		rid, err := mcWorld.LoadBlock(x, int16(yi), z)
		if err != nil {
			t.Fatalf("LoadBlock x=%d y=%d z=%d: %v", x, yi, z, err)
		}
		if rid != block.AirRuntimeID {
			return int16(yi), rid, true
		}
	}
	return 0, 0, false
}

func findRuntimeIDInColumn(t *testing.T, mcWorld *wsutils.MCWorld, x int32, z int32, want uint32) (int16, bool) {
	t.Helper()
	for yi := -64; yi <= 320; yi++ {
		rid, err := mcWorld.LoadBlock(x, int16(yi), z)
		if err != nil {
			t.Fatalf("LoadBlock x=%d y=%d z=%d: %v", x, yi, z, err)
		}
		if rid == want {
			return int16(yi), true
		}
	}
	return 0, false
}
