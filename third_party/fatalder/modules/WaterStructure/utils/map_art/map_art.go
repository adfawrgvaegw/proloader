package map_art

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsutils "github.com/Yeah114/WaterStructure/utils"
	"github.com/disintegration/imaging"
)

const (
	mapPixelSize = 128

	heightModeHigher uint8 = iota
	heightModeMiddle
	heightModeLower

	int16Min = -1 << 15
	int16Max = 1<<15 - 1
)

// Options controls how map art is generated and placed into the world.
type Options struct {
	StartSubChunkPos wsdefine.SubChunkPos
	MapWidth         int
	MapHeight        int
	Resample         *imaging.ResampleFilter // nil means imaging.Lanczos
	// DisableReferenceColumn disables placing the extra reference column at Z-1.
	// By default the reference column is enabled.
	DisableReferenceColumn bool
	// Force2D forces the map art to be generated as a flat 2D plane (all blocks share the same Y).
	// In this mode, the palette matching prefers the "middle" shade for each block, because height-based
	// shading is disabled.
	Force2D bool
	// Max3DHeight limits the vertical span (in blocks) used by the 3D height algorithm.
	// 0 means default (384), which matches the Overworld build height [-64..319].
	// This option is ignored when Force2D is true.
	Max3DHeight int32
}

//go:embed colors.json
var colorsJSON []byte

type blockMode struct {
	runtimeID  uint32
	heightMode uint8
}

type palette struct {
	modesByRGB   map[uint32]blockMode
	mappingRGB   [][3]uint8
	modesByRGB2D map[uint32]blockMode
	mappingRGB2D [][3]uint8
}

var (
	paletteOnce sync.Once
	paletteVal  palette
	paletteErr  error
)

// GenerateMapArtToWorld resizes img to (mapWidth*128)x(mapHeight*128), converts it into map art blocks,
// and places them into bedrockWorld starting at opts.StartSubChunkPos.
//
// opts.MapWidth/opts.MapHeight are counts in maps (1 map == 128x128 pixels).
//
// The generated structure includes a single extra "reference" column at Z-1 (north of the art)
// to stabilize brightness for the first row, matching the behavior of the reference implementation.
//
// If opts.Resample is nil, imaging.Lanczos is used.
//
// It returns the inclusive min/max block coordinates that were written, which is useful for selection.
func GenerateMapArtToWorld(bedrockWorld *world.BedrockWorld, img image.Image, opts *Options) (minPos [3]int32, maxPos [3]int32, err error) {
	if bedrockWorld == nil {
		return [3]int32{}, [3]int32{}, errors.New("bedrockWorld is nil")
	}
	if img == nil {
		return [3]int32{}, [3]int32{}, errors.New("img is nil")
	}
	if opts == nil {
		return [3]int32{}, [3]int32{}, errors.New("opts is nil")
	}
	if opts.MapWidth <= 0 || opts.MapHeight <= 0 {
		return [3]int32{}, [3]int32{}, fmt.Errorf("invalid map size: %dx%d", opts.MapWidth, opts.MapHeight)
	}

	if err := loadPalette(); err != nil {
		return [3]int32{}, [3]int32{}, err
	}

	targetW := opts.MapWidth * mapPixelSize
	targetH := opts.MapHeight * mapPixelSize

	filter := imaging.Lanczos
	if opts.Resample != nil {
		filter = *opts.Resample
	}
	resized := imaging.Resize(img, targetW, targetH, filter)

	// Prepare per-pixel block mode (runtime id + height mode).
	pixelModes, err := mapPixelsToModes(resized, paletteVal, opts.Force2D)
	if err != nil {
		return [3]int32{}, [3]int32{}, err
	}

	includeReferenceColumn := !opts.DisableReferenceColumn

	baseX := opts.StartSubChunkPos.X() * 16
	baseY := opts.StartSubChunkPos.Y() * 16
	baseZ := opts.StartSubChunkPos.Z() * 16

	emeraldID, ok := block.StateToRuntimeID("minecraft:emerald_block", nil)
	if !ok {
		return [3]int32{}, [3]int32{}, errors.New(`unknown block runtime id: "minecraft:emerald_block"`)
	}

	mcWorld, err := wsutils.NewMCWorld(bedrockWorld, context.Background())
	if err != nil {
		return [3]int32{}, [3]int32{}, err
	}

	minX := baseX
	maxX := baseX + int32(targetW-1)
	minZ := baseZ
	if includeReferenceColumn {
		minZ = baseZ - 1
	}
	maxZ := baseZ + int32(targetH-1)
	minY := int32(1<<31 - 1)
	maxY := int32(-1 << 31)

	// zIndex: 0 is reference column at Z-1, 1..targetH are pixels at Z..Z+targetH-1.
	if opts.Force2D {
		worldY32 := baseY
		if worldY32 < int16Min || worldY32 > int16Max {
			return [3]int32{}, [3]int32{}, fmt.Errorf("computed Y out of int16 range: %d", worldY32)
		}
		worldY := int16(worldY32)

		if includeReferenceColumn {
			for x := 0; x < targetW; x++ {
				worldX := baseX + int32(x)
				if err := mcWorld.SetBlock(worldX, worldY, baseZ-1, emeraldID); err != nil {
					return [3]int32{}, [3]int32{}, err
				}
			}
		}
		for x := 0; x < targetW; x++ {
			worldX := baseX + int32(x)
			for z := 0; z < targetH; z++ {
				worldZ := baseZ + int32(z)
				runtimeID := pixelModes.runtimeID[x*targetH+z]
				if err := mcWorld.SetBlock(worldX, worldY, worldZ, runtimeID); err != nil {
					return [3]int32{}, [3]int32{}, err
				}
			}
		}

		minY, maxY = worldY32, worldY32
		mcWorld.Flush()
		return [3]int32{minX, minY, minZ}, [3]int32{maxX, maxY, maxZ}, nil
	}

	// Compute Y for the reference column and all pixels.
	heights, err := computeHeights(targetW, targetH, pixelModes.heightMode, includeReferenceColumn)
	if err != nil {
		return [3]int32{}, [3]int32{}, err
	}
	max3DHeight := opts.Max3DHeight
	if max3DHeight == 0 {
		max3DHeight = 384
	}
	if max3DHeight < 1 {
		return [3]int32{}, [3]int32{}, fmt.Errorf("invalid Max3DHeight: %d", opts.Max3DHeight)
	}
	limitHeights(heights, targetW, targetH, includeReferenceColumn, max3DHeight)

	if includeReferenceColumn {
		for x := 0; x < targetW; x++ {
			worldX := baseX + int32(x)
			for zIndex := 0; zIndex <= targetH; zIndex++ {
				worldZ := baseZ + int32(zIndex) - 1
				worldY32 := heights[x*(targetH+1)+zIndex] + baseY
				if worldY32 < int16Min || worldY32 > int16Max {
					return [3]int32{}, [3]int32{}, fmt.Errorf("computed Y out of int16 range: %d", worldY32)
				}
				worldY := int16(worldY32)

				var runtimeID uint32
				if zIndex == 0 {
					runtimeID = emeraldID
				} else {
					runtimeID = pixelModes.runtimeID[x*targetH+(zIndex-1)]
				}
				if err := mcWorld.SetBlock(worldX, worldY, worldZ, runtimeID); err != nil {
					return [3]int32{}, [3]int32{}, err
				}
				if worldY32 < minY {
					minY = worldY32
				}
				if worldY32 > maxY {
					maxY = worldY32
				}
			}
		}
	} else {
		for x := 0; x < targetW; x++ {
			worldX := baseX + int32(x)
			for zIndex := 1; zIndex <= targetH; zIndex++ {
				worldZ := baseZ + int32(zIndex) - 1
				worldY32 := heights[x*(targetH+1)+zIndex] + baseY
				if worldY32 < int16Min || worldY32 > int16Max {
					return [3]int32{}, [3]int32{}, fmt.Errorf("computed Y out of int16 range: %d", worldY32)
				}
				worldY := int16(worldY32)

				var runtimeID uint32
				runtimeID = pixelModes.runtimeID[x*targetH+(zIndex-1)]
				if err := mcWorld.SetBlock(worldX, worldY, worldZ, runtimeID); err != nil {
					return [3]int32{}, [3]int32{}, err
				}
				if worldY32 < minY {
					minY = worldY32
				}
				if worldY32 > maxY {
					maxY = worldY32
				}
			}
		}
	}

	mcWorld.Flush()
	return [3]int32{minX, minY, minZ}, [3]int32{maxX, maxY, maxZ}, nil
}

func limitHeights(heights []int32, width int, height int, includeReferenceColumn bool, maxHeight int32) {
	if maxHeight <= 0 {
		return
	}
	maxAllowed := maxHeight - 1
	if maxAllowed < 0 {
		maxAllowed = 0
	}

	maxRel := int32(0)
	for x := 0; x < width; x++ {
		startZ := 0
		if !includeReferenceColumn {
			startZ = 1
		}
		for zIndex := startZ; zIndex <= height; zIndex++ {
			v := heights[x*(height+1)+zIndex]
			if v > maxRel {
				maxRel = v
			}
		}
	}
	if maxRel <= maxAllowed || maxRel == 0 {
		return
	}

	for x := 0; x < width; x++ {
		startZ := 0
		if !includeReferenceColumn {
			startZ = 1
		}
		for zIndex := startZ; zIndex <= height; zIndex++ {
			i := x*(height+1) + zIndex
			heights[i] = heights[i] * maxAllowed / maxRel
		}
	}
}

type pixelModeBuffers struct {
	runtimeID  []uint32
	heightMode []uint8
}

func mapPixelsToModes(img *image.NRGBA, pal palette, force2D bool) (pixelModeBuffers, error) {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	runtimeIDs := make([]uint32, w*h)
	heightModes := make([]uint8, w*h)

	modesByRGB := pal.modesByRGB
	mappingRGB := pal.mappingRGB
	if force2D {
		modesByRGB = pal.modesByRGB2D
		mappingRGB = pal.mappingRGB2D
		if len(modesByRGB) == 0 || len(mappingRGB) == 0 {
			return pixelModeBuffers{}, errors.New("2D palette is empty")
		}
	}

	// Cache: input RGB -> closest palette RGB.
	closestCache := make(map[uint32]uint32, 4096)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := img.NRGBAAt(x, y)
			packed := packRGB(px)

			closestPacked, ok := closestCache[packed]
			if !ok {
				best := searchClosestRGB([3]uint8{px.R, px.G, px.B}, mappingRGB)
				closestPacked = uint32(best[0])<<16 | uint32(best[1])<<8 | uint32(best[2])
				closestCache[packed] = closestPacked
			}

			mode, ok := modesByRGB[closestPacked]
			if !ok {
				return pixelModeBuffers{}, fmt.Errorf("internal palette lookup failed for rgb=%06x", closestPacked)
			}
			idx := x*h + y
			runtimeIDs[idx] = mode.runtimeID
			heightModes[idx] = mode.heightMode
		}
	}

	return pixelModeBuffers{
		runtimeID:  runtimeIDs,
		heightMode: heightModes,
	}, nil
}

func computeHeights(width int, height int, heightModes []uint8, includeReferenceColumn bool) ([]int32, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions: %dx%d", width, height)
	}
	if len(heightModes) != width*height {
		return nil, fmt.Errorf("invalid heightModes length: got %d want %d", len(heightModes), width*height)
	}

	// heights are stored as relative offsets; final world Y is baseY + heights[i].
	// Layout: [x*(height+1) + zIndex], where zIndex=0 is the reference column.
	out := make([]int32, width*(height+1))

	// For each X column, anchor the first pixel (zIndex=1) at 0 and set the reference column (zIndex=0)
	// based on the requested brightness mode for that first pixel.
	for x := 0; x < width; x++ {
		firstMode := heightModes[x*height+0]
		evenX := (x%2 == 0)

		switch firstMode {
		case heightModeHigher:
			if evenX {
				out[x*(height+1)+0] = -2
			} else {
				out[x*(height+1)+0] = -1
			}
		case heightModeMiddle:
			out[x*(height+1)+0] = 0
		case heightModeLower:
			if evenX {
				out[x*(height+1)+0] = 1
			} else {
				out[x*(height+1)+0] = 2
			}
		default:
			return nil, fmt.Errorf("invalid height mode: %d", firstMode)
		}
		out[x*(height+1)+1] = 0
	}

	for x := 0; x < width; x++ {
		evenX := (x%2 == 0)
		for zIndex := 2; zIndex <= height; zIndex++ {
			evenZ := (zIndex%2 == 0)

			lastMode := heightModes[x*height+(zIndex-2)]
			currentMode := heightModes[x*height+(zIndex-1)]

			lastY := out[x*(height+1)+(zIndex-1)]
			prevY := out[x*(height+1)+(zIndex-2)]

			switch lastMode {
			case heightModeHigher:
				switch currentMode {
				case heightModeHigher:
					if lastY-prevY >= 2 {
						out[x*(height+1)+zIndex] = lastY + 1
					} else {
						out[x*(height+1)+zIndex] = lastY + 2
					}
				case heightModeMiddle:
					out[x*(height+1)+zIndex] = lastY
				case heightModeLower:
					if (evenX && evenZ) || (!evenX && !evenZ) {
						out[x*(height+1)+zIndex] = lastY - 2
					} else {
						out[x*(height+1)+zIndex] = lastY - 1
					}
				default:
					return nil, fmt.Errorf("invalid height mode: %d", currentMode)
				}
			case heightModeMiddle:
				switch currentMode {
				case heightModeHigher:
					if (evenX && evenZ) || (!evenX && !evenZ) {
						out[x*(height+1)+zIndex] = lastY + 1
					} else {
						out[x*(height+1)+zIndex] = lastY + 2
					}
				case heightModeMiddle:
					out[x*(height+1)+zIndex] = lastY
				case heightModeLower:
					if (evenX && evenZ) || (!evenX && !evenZ) {
						out[x*(height+1)+zIndex] = lastY - 2
					} else {
						out[x*(height+1)+zIndex] = lastY - 1
					}
				default:
					return nil, fmt.Errorf("invalid height mode: %d", currentMode)
				}
			case heightModeLower:
				switch currentMode {
				case heightModeHigher:
					if (evenX && evenZ) || (!evenX && !evenZ) {
						out[x*(height+1)+zIndex] = lastY + 1
					} else {
						out[x*(height+1)+zIndex] = lastY + 2
					}
				case heightModeMiddle:
					out[x*(height+1)+zIndex] = lastY
				case heightModeLower:
					if prevY-lastY == 1 {
						out[x*(height+1)+zIndex] = lastY - 2
					} else {
						out[x*(height+1)+zIndex] = lastY - 1
					}
				default:
					return nil, fmt.Errorf("invalid height mode: %d", currentMode)
				}
			default:
				return nil, fmt.Errorf("invalid height mode: %d", lastMode)
			}
		}
	}

	minY := out[0]
	if !includeReferenceColumn {
		minY = out[1]
	}
	for x := 0; x < width; x++ {
		start := x * (height + 1)
		end := start + (height + 1)
		if !includeReferenceColumn {
			start++
		}
		for _, v := range out[start:end] {
			if v < minY {
				minY = v
			}
		}
	}
	for i := range out {
		out[i] -= minY
	}
	return out, nil
}

func searchClosestRGB(target [3]uint8, paletteRGB [][3]uint8) [3]uint8 {
	best := [3]uint8{}
	bestDist := ^uint32(0)
	tr, tg, tb := int32(target[0]), int32(target[1]), int32(target[2])
	for _, c := range paletteRGB {
		dr := tr - int32(c[0])
		dg := tg - int32(c[1])
		db := tb - int32(c[2])
		dist := uint32(dr*dr + dg*dg + db*db)
		if dist < bestDist {
			bestDist = dist
			best = c
		}
	}
	return best
}

func packRGB(c color.NRGBA) uint32 {
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

func loadPalette() error {
	paletteOnce.Do(func() {
		var raw [][]any
		if err := json.Unmarshal(colorsJSON, &raw); err != nil {
			paletteErr = fmt.Errorf("failed to parse colors.json: %w", err)
			return
		}

		modes := make(map[uint32]blockMode, len(raw)*3)
		mapping := make([][3]uint8, 0, len(raw)*3)
		modes2D := make(map[uint32]blockMode, len(raw))
		mapping2D := make([][3]uint8, 0, len(raw))

		// Resolve runtime IDs once per block name.
		runtimeByBlock := make(map[string]uint32, len(raw))

		for _, entry := range raw {
			if len(entry) != 4 {
				paletteErr = fmt.Errorf("invalid palette entry length: %d", len(entry))
				return
			}

			blockName, ok := entry[3].(string)
			if !ok || blockName == "" {
				paletteErr = errors.New("invalid palette entry block name")
				return
			}
			blockName = normalizeBlockName(blockName)

			runtimeID, ok := runtimeByBlock[blockName]
			if !ok {
				id, found := block.StateToRuntimeID(blockName, nil)
				if !found {
					// Keep working even if the palette contains legacy/unknown names.
					// Skipping an entry slightly reduces color fidelity but preserves usability.
					continue
				}
				runtimeID = id
				runtimeByBlock[blockName] = runtimeID
			}

			higher, err := parseRGBA(entry[0])
			if err != nil {
				paletteErr = fmt.Errorf("parse higher color: %w", err)
				return
			}
			middle, err := parseRGBA(entry[1])
			if err != nil {
				paletteErr = fmt.Errorf("parse middle color: %w", err)
				return
			}
			lower, err := parseRGBA(entry[2])
			if err != nil {
				paletteErr = fmt.Errorf("parse lower color: %w", err)
				return
			}

			addMode := func(c color.RGBA, mode uint8) {
				key := uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
				modes[key] = blockMode{runtimeID: runtimeID, heightMode: mode}
				mapping = append(mapping, [3]uint8{c.R, c.G, c.B})
			}

			addMode(higher, heightModeHigher)
			addMode(middle, heightModeMiddle)
			addMode(lower, heightModeLower)

			// 2D uses the middle shade only.
			middleKey := uint32(middle.R)<<16 | uint32(middle.G)<<8 | uint32(middle.B)
			modes2D[middleKey] = blockMode{runtimeID: runtimeID, heightMode: heightModeMiddle}
			mapping2D = append(mapping2D, [3]uint8{middle.R, middle.G, middle.B})
		}

		paletteVal = palette{
			modesByRGB:   modes,
			mappingRGB:   mapping,
			modesByRGB2D: modes2D,
			mappingRGB2D: mapping2D,
		}

		if len(paletteVal.mappingRGB) == 0 || len(paletteVal.modesByRGB) == 0 {
			paletteErr = errors.New("palette is empty after filtering unknown blocks")
			return
		}
	})
	return paletteErr
}

func normalizeBlockName(name string) string {
	switch name {
	case "minecraft:stained_hardened_clay", "minecraft:hardened_clay":
		return "minecraft:terracotta"
	default:
		return name
	}
}

func parseRGBA(v any) (color.RGBA, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 4 {
		return color.RGBA{}, errors.New("invalid RGBA array")
	}
	toU8 := func(x any) (uint8, error) {
		f, ok := x.(float64)
		if !ok {
			return 0, errors.New("invalid number")
		}
		if f < 0 || f > 255 {
			return 0, errors.New("number out of range")
		}
		return uint8(f), nil
	}
	r, err := toU8(arr[0])
	if err != nil {
		return color.RGBA{}, err
	}
	g, err := toU8(arr[1])
	if err != nil {
		return color.RGBA{}, err
	}
	b, err := toU8(arr[2])
	if err != nil {
		return color.RGBA{}, err
	}
	a, err := toU8(arr[3])
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: r, G: g, B: b, A: a}, nil
}
