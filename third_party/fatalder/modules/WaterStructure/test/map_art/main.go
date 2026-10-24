package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/world"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsmapart "github.com/Yeah114/WaterStructure/utils/map_art"
	"github.com/disintegration/imaging"
)

func main() {
	var (
		imagePath = flag.String("img", defaultImagePath(), "input image path")
		worldPath = flag.String("world", "test/map_art/world", "output Bedrock world directory")
		mapW      = flag.Int("w", 2, "map width in maps (1 map = 128px)")
		mapH      = flag.Int("h", 2, "map height in maps (1 map = 128px)")
		noRef     = flag.Bool("no-ref", true, "disable the reference column at Z-1")
		force2D   = flag.Bool("2d", false, "force the map art to be generated as a flat 2D plane")
		max3D     = flag.Int("max3d", 384, "max 3D height span in blocks (0 uses default 384)")
		startX    = flag.Int("sx", 4, "start subchunk X")
		startY    = flag.Int("sy", -4, "start subchunk Y")
		startZ    = flag.Int("sz", -4, "start subchunk Z")
	)
	flag.Parse()

	img, err := imaging.Open(*imagePath)
	if err != nil {
		fatalf("open image %q: %v", *imagePath, err)
	}

	bw, err := world.Open(*worldPath, nil)
	if err != nil {
		fatalf("open world %q: %v", *worldPath, err)
	}
	defer func() {
		_ = bw.CloseWorld()
	}()

	start := wsdefine.SubChunkPos{int32(*startX), int32(*startY), int32(*startZ)}
	minPos, maxPos, err := wsmapart.GenerateMapArtToWorld(bw, img, &wsmapart.Options{
		StartSubChunkPos:       start,
		MapWidth:               *mapW,
		MapHeight:              *mapH,
		Resample:               nil,
		DisableReferenceColumn: *noRef,
		Force2D:                *force2D,
		Max3DHeight:            int32(*max3D),
	})
	if err != nil {
		fatalf("GenerateMapArtToWorld: %v", err)
	}

	fmt.Printf("ok: wrote map art to %q (maps=%dx%d startSubChunk=%v)\n", *worldPath, *mapW, *mapH, start)
	fmt.Printf("bounds: min=%v max=%v\n", minPos, maxPos)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func defaultImagePath() string {
	const abs = "/root/WaterStructure/test/map_art/test.jpg"
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	return "test/map_art/test.jpg"
}
