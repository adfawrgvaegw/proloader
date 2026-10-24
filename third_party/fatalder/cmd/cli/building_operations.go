package main

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/Fatalder/cmd/cli/ui"
	"github.com/disintegration/imaging"
	"github.com/mholt/archiver/v3"
	"github.com/schollz/progressbar/v3"

	l10n "github.com/Yeah114/Fatalder/utils/localization"
	wsdefine "github.com/Yeah114/WaterStructure/define"
	wsstructure "github.com/Yeah114/WaterStructure/structure"
	wsmapart "github.com/Yeah114/WaterStructure/utils/map_art"
	wsnetease "github.com/Yeah114/WaterStructure/utils/netease_world"
)

var selectionRegex = regexp.MustCompile(`@\[(-?\d+),(-?\d+),(-?\d+)\]~\[(-?\d+),(-?\d+),(-?\d+)\]`)

func handleBuildingOperations() {
	for {
		fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildingOpsTitle))

		options := []string{
			l10n.T(l10n.KeyBuildingOpsConvertMCWorldSlow),
			l10n.T(l10n.KeyBuildingOpsConvertMCWorldFast),
			l10n.T(l10n.KeyBuildingOpsAnyToAny),
			l10n.T(l10n.KeyBuildingOpsNeteaseEncrypt),
			l10n.T(l10n.KeyBuildingOpsNeteaseDecrypt),
			l10n.T(l10n.KeyBuildingOpsMapArt),
			l10n.T(l10n.KeyReturn),
		}

		index, _ := ui.Select(l10n.T(l10n.KeyUISelectPrompt), options)
		var err error

		switch index {
		case 0:
			err = buildingOpsConvertStructureToMCWorld(false)
		case 1:
			err = buildingOpsConvertStructureToMCWorld(true)
		case 2:
			err = buildingOpsAnyToAny()
		case 3:
			fmt.Println("You are not authorized to use this feature")
			//err = buildingOpsNeteaseCrypt(true)
		case 4:
			fmt.Println("You are not authorized to use this feature")
			//err = buildingOpsNeteaseCrypt(false)
		case 5:
			fmt.Println("You are not authorized to use this feature")
			//err = buildingOpsMapArt()
		case 6:
			return
		default:
			continue
		}

		if err != nil {
			fmt.Printf("%s: %v\n", l10n.T(l10n.KeyOperationFailed), err)
		}

		ui.Input(fmt.Sprintf("\n%s...", l10n.T(l10n.KeyPressEnter)))
	}
}

func buildingOpsConvertStructureToMCWorld(useFast bool) error {
	titleKey := l10n.KeyBuildingOpsConvertMCWorldSlow
	if useFast {
		titleKey = l10n.KeyBuildingOpsConvertMCWorldFast
	}
	fmt.Printf("\n=== %s ===\n", l10n.T(titleKey))

	src := strings.TrimSpace(ui.Input(l10n.T(l10n.KeyBuildingOpsPromptStructurePath)))
	if src == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}

	defaultOut := strings.TrimSuffix(src, filepath.Ext(src)) + ".mcworld"
	dest := strings.TrimSpace(ui.InputWithDefault(l10n.T(l10n.KeyBuildingOpsPromptOutputMCWorldPath), defaultOut))
	if dest == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".mcworld") {
		dest += ".mcworld"
	}

	fmt.Println(l10n.T(l10n.KeyBuildConvertToMCWorldStart))
	var bar *progressbar.ProgressBar
	startCB := func(total int) {
		if total <= 0 {
			bar = nil
			return
		}
		bar = progressbar.NewOptions(total,
			progressbar.OptionSetDescription(l10n.T(l10n.KeyBuildConvertToMCWorldProgress)),
			progressbar.OptionShowCount(),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowElapsedTimeOnFinish(),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionOnCompletion(func() { fmt.Println() }),
		)
	}
	progressCB := func() {
		if bar != nil {
			_ = bar.Add(1)
		}
	}

	var err error
	if useFast {
		err = convertStructureFileToMCWorldFast(src, dest, startCB, progressCB)
	} else {
		err = convertStructureFileToMCWorldSlow(src, dest, startCB, progressCB)
	}
	if err != nil {
		return err
	}
	if bar != nil {
		_ = bar.Finish()
	}
	fmt.Printf(l10n.T(l10n.KeyBuildConvertToMCWorldSuccess)+"\n", dest)
	return nil
}

func buildingOpsAnyToAny() error {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildingOpsAnyToAny))

	src := strings.TrimSpace(ui.Input(l10n.T(l10n.KeyBuildingOpsPromptStructurePath)))
	if src == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcStruct, err := wsstructure.StructureFromFile(srcFile)
	if err != nil {
		return err
	}
	defer srcStruct.Close()
	fmt.Printf(l10n.T(l10n.KeyBuildingOpsDetectedStructureType)+"\n", srcStruct.Name())

	formatNames := supportedStructureFormatNames()
	if len(formatNames) == 0 {
		return errors.New("no supported formats found")
	}
	index, _ := ui.Select(l10n.T(l10n.KeyBuildingOpsSelectTargetFormat), formatNames)
	if index < 0 || index >= len(formatNames) {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	targetFormat := formatNames[index]
	targetFactory, ok := wsstructure.StructureNamePool[targetFormat]
	if !ok {
		return fmt.Errorf("unsupported format: %s", targetFormat)
	}

	defaultOut := strings.TrimSuffix(src, filepath.Ext(src)) + "." + strings.ToLower(targetFormat)
	dest := strings.TrimSpace(ui.InputWithDefault(l10n.T(l10n.KeyBuildingOpsPromptOutputStructurePath), defaultOut))
	if dest == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if handled, err := tryExportFromMCWorldSource(src, targetFormat, targetFactory, destFile); handled {
		if err != nil {
			return err
		}
		fmt.Printf(l10n.T(l10n.KeyBuildingOpsAnyToAnyDone)+"\n", dest)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "fatalder-any2any-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	worldDir := filepath.Join(tmpDir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return err
	}
	bedrockWorld, err := world.Open(worldDir, nil)
	if err != nil {
		return err
	}
	defer func() { _ = bedrockWorld.CloseWorld() }()

	startSubChunkPos := wsdefine.SubChunkPos{0, -4, 0}
	var writeBar *progressbar.ProgressBar
	if err := srcStruct.ToMCWorld(
		bedrockWorld,
		startSubChunkPos,
		func(total int) { writeBar = buildBuildingOpsBar(total, l10n.T(l10n.KeyBuildingOpsProgressWriteWorld)) },
		func() {
			if writeBar != nil {
				_ = writeBar.Add(1)
			}
		},
	); err != nil {
		return err
	}
	if writeBar != nil {
		_ = writeBar.Finish()
	}

	size := srcStruct.GetSize()
	startBlockPos := wsdefine.BlockPos{
		startSubChunkPos.X() * 16,
		startSubChunkPos.Y() * 16,
		startSubChunkPos.Z() * 16,
	}
	endBlockPos := wsdefine.BlockPos{
		startBlockPos.X() + int32(size.Width) - 1,
		startBlockPos.Y() + int32(size.Height) - 1,
		startBlockPos.Z() + int32(size.Length) - 1,
	}

	targetStruct := targetFactory()
	var exportBar *progressbar.ProgressBar
	if err := targetStruct.FromMCWorld(
		bedrockWorld,
		destFile,
		startBlockPos,
		endBlockPos,
		func(total int) {
			exportBar = buildBuildingOpsBar(total, l10n.T(l10n.KeyBuildingOpsProgressExportStructure))
		},
		func() {
			if exportBar != nil {
				_ = exportBar.Add(1)
			}
		},
	); err != nil {
		return err
	}
	if exportBar != nil {
		_ = exportBar.Finish()
	}

	fmt.Printf(l10n.T(l10n.KeyBuildingOpsAnyToAnyDone)+"\n", dest)
	return nil
}

func tryExportFromMCWorldSource(
	structurePath string,
	targetFormat string,
	targetFactory wsstructure.StructureFunc,
	targetFile *os.File,
) (handled bool, err error) {
	ext := strings.ToLower(filepath.Ext(structurePath))
	if ext != ".mcworld" && ext != ".zip" {
		return false, nil
	}

	if targetFormat == wsstructure.NameMCWorld {
		src, err := os.Open(structurePath)
		if err != nil {
			return true, err
		}
		defer src.Close()
		if _, err := io.Copy(targetFile, src); err != nil {
			return true, err
		}
		return true, nil
	}

	extractDir, cleanup, err := unarchiveMCWorldToTempDir(structurePath)
	if err != nil {
		return false, nil
	}
	defer cleanup()

	bw, err := world.Open(extractDir, nil)
	if err != nil {
		return false, nil
	}
	defer func() {
		_ = bw.CloseWorld()
		_ = bw.Close()
	}()

	startPos, endPos, ok := parseSelectionBounds(structurePath)
	if !ok {
		startPos, endPos, ok = parseSelectionBounds(bw.LevelDat().LevelName)
	}
	if !ok {
		return true, fmt.Errorf("无法从文件名或世界名称中解析坐标信息")
	}

	var bar *progressbar.ProgressBar
	targetStruct := targetFactory()
	if err := targetStruct.FromMCWorld(
		bw,
		targetFile,
		startPos,
		endPos,
		func(total int) { bar = buildBuildingOpsBar(total, l10n.T(l10n.KeyBuildingOpsProgressExportStructure)) },
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	); err != nil {
		return true, err
	}
	if bar != nil {
		_ = bar.Finish()
	}
	return true, nil
}

func parseSelectionBounds(target string) (start wsdefine.BlockPos, end wsdefine.BlockPos, ok bool) {
	matches := selectionRegex.FindStringSubmatch(target)
	if len(matches) != 7 {
		return wsdefine.BlockPos{}, wsdefine.BlockPos{}, false
	}

	parse := func(s string) (int32, bool) {
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, false
		}
		return int32(v), true
	}

	sx, ok1 := parse(matches[1])
	sy, ok2 := parse(matches[2])
	sz, ok3 := parse(matches[3])
	ex, ok4 := parse(matches[4])
	ey, ok5 := parse(matches[5])
	ez, ok6 := parse(matches[6])
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) {
		return wsdefine.BlockPos{}, wsdefine.BlockPos{}, false
	}

	minPos := wsdefine.BlockPos{minInt32(sx, ex), minInt32(sy, ey), minInt32(sz, ez)}
	maxPos := wsdefine.BlockPos{maxInt32(sx, ex), maxInt32(sy, ey), maxInt32(sz, ez)}
	return minPos, maxPos, true
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func buildingOpsNeteaseCrypt(encrypt bool) error {
	titleKey := l10n.KeyBuildingOpsNeteaseDecrypt
	if encrypt {
		titleKey = l10n.KeyBuildingOpsNeteaseEncrypt
	}
	fmt.Printf("\n=== %s ===\n", l10n.T(titleKey))

	input := strings.TrimSpace(ui.Input(l10n.T(l10n.KeyBuildingOpsPromptWorldOrDBPath)))
	if input == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	info, err := os.Stat(input)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		// treat as mcworld file
		worldDir, cleanup, err := unarchiveMCWorldToTempDir(input)
		if err != nil {
			return err
		}
		defer cleanup()

		dbDir := filepath.Join(worldDir, "db")
		if _, err := os.Stat(dbDir); err != nil {
			return fmt.Errorf("missing db dir: %w", err)
		}

		if encrypt {
			err = wsnetease.Encrypt(dbDir, nil)
		} else {
			err = wsnetease.Decrypt(dbDir, nil)
		}
		if err != nil {
			return err
		}

		defaultOut := strings.TrimSuffix(input, filepath.Ext(input))
		if encrypt {
			defaultOut += ".encrypted.mcworld"
		} else {
			defaultOut += ".decrypted.mcworld"
		}
		out := strings.TrimSpace(ui.InputWithDefault(l10n.T(l10n.KeyBuildingOpsPromptOutputMCWorldPath), defaultOut))
		if out == "" {
			return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
		}
		if !strings.HasSuffix(strings.ToLower(out), ".mcworld") {
			out += ".mcworld"
		}

		if err := archiveDirAsMCWorld(worldDir, out); err != nil {
			return err
		}
		fmt.Printf(l10n.T(l10n.KeyBuildingOpsNeteaseDone)+"\n", out)
		return nil
	}

	dbDir := input
	if filepath.Base(dbDir) != "db" {
		dbDir = filepath.Join(input, "db")
	}
	if _, err := os.Stat(filepath.Join(dbDir, "CURRENT")); err != nil {
		return fmt.Errorf("invalid db dir: %w", err)
	}

	if !ui.Confirm(fmt.Sprintf(l10n.T(l10n.KeyBuildingOpsConfirmModifyDir), dbDir)) {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}

	if encrypt {
		if err := wsnetease.Encrypt(dbDir, nil); err != nil {
			return err
		}
	} else {
		if err := wsnetease.Decrypt(dbDir, nil); err != nil {
			return err
		}
	}
	fmt.Printf(l10n.T(l10n.KeyBuildingOpsNeteaseDone)+"\n", dbDir)
	return nil
}

func buildingOpsMapArt() error {
	fmt.Printf("\n=== %s ===\n", l10n.T(l10n.KeyBuildingOpsMapArt))

	imgPath := strings.TrimSpace(ui.Input(l10n.T(l10n.KeyBuildingOpsPromptImagePath)))
	if imgPath == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	img, err := imaging.Open(imgPath)
	if err != nil {
		return err
	}

	input := strings.TrimSpace(ui.Input(l10n.T(l10n.KeyBuildingOpsPromptWorldOrDBPath)))
	if input == "" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}

	startX := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptStartSubChunkX), 0)
	startY := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptStartSubChunkY), -4)
	startZ := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptStartSubChunkZ), 0)
	mapW := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptMapWidth), 1)
	mapH := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptMapHeight), 1)
	force2D := ui.ConfirmWithDefault(l10n.T(l10n.KeyBuildingOpsPromptMapForce2D), false)
	disableRef := ui.ConfirmWithDefault(l10n.T(l10n.KeyBuildingOpsPromptMapDisableReference), false)
	max3DHeight := ui.InputIntWithDefault(l10n.T(l10n.KeyBuildingOpsPromptMapMax3DHeight), 0)

	opts := &wsmapart.Options{
		StartSubChunkPos:       wsdefine.SubChunkPos{int32(startX), int32(startY), int32(startZ)},
		MapWidth:               mapW,
		MapHeight:              mapH,
		DisableReferenceColumn: disableRef,
		Force2D:                force2D,
		Max3DHeight:            int32(max3DHeight),
	}

	info, err := os.Stat(input)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		worldDir, cleanup, err := unarchiveMCWorldToTempDir(input)
		if err != nil {
			return err
		}
		defer cleanup()

		minPos, maxPos, err := writeMapArtToWorldDir(worldDir, img, opts)
		if err != nil {
			return err
		}

		defaultOut := strings.TrimSuffix(input, filepath.Ext(input)) + ".mapart.mcworld"
		out := strings.TrimSpace(ui.InputWithDefault(l10n.T(l10n.KeyBuildingOpsPromptOutputMCWorldPath), defaultOut))
		if out == "" {
			return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
		}
		if !strings.HasSuffix(strings.ToLower(out), ".mcworld") {
			out += ".mcworld"
		}
		if err := archiveDirAsMCWorld(worldDir, out); err != nil {
			return err
		}
		fmt.Printf(l10n.T(l10n.KeyBuildingOpsMapArtRange)+"\n", minPos[0], minPos[1], minPos[2], maxPos[0], maxPos[1], maxPos[2])
		fmt.Printf(l10n.T(l10n.KeyBuildingOpsMapArtDone)+"\n", out)
		return nil
	}

	if filepath.Base(input) == "db" {
		return fmt.Errorf("%s", l10n.T(l10n.KeyBuildingOpsMapArtNeedsWorldDir))
	}

	if !ui.Confirm(fmt.Sprintf(l10n.T(l10n.KeyBuildingOpsConfirmModifyDir), input)) {
		return fmt.Errorf("%s", l10n.T(l10n.KeyOperationCancel))
	}
	minPos, maxPos, err := writeMapArtToWorldDir(input, img, opts)
	if err != nil {
		return err
	}
	fmt.Printf(l10n.T(l10n.KeyBuildingOpsMapArtRange)+"\n", minPos[0], minPos[1], minPos[2], maxPos[0], maxPos[1], maxPos[2])
	fmt.Printf(l10n.T(l10n.KeyBuildingOpsMapArtDone)+"\n", input)
	return nil
}

func writeMapArtToWorldDir(worldDir string, img image.Image, opts *wsmapart.Options) (minPos [3]int32, maxPos [3]int32, err error) {
	bedrockWorld, err := world.Open(worldDir, nil)
	if err != nil {
		return [3]int32{}, [3]int32{}, err
	}
	defer func() { _ = bedrockWorld.CloseWorld() }()

	return wsmapart.GenerateMapArtToWorld(bedrockWorld, img, opts)
}

func supportedStructureFormatNames() []string {
	var names []string
	for name := range wsstructure.StructureNamePool {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildBuildingOpsBar(total int, desc string) *progressbar.ProgressBar {
	if total <= 0 {
		return nil
	}
	return progressbar.NewOptions(
		total,
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)
}

func convertStructureFileToMCWorldSlow(srcPath, destPath string, startCallback func(int), progressCallback func()) error {
	if startCallback == nil {
		startCallback = func(int) {}
	}
	if progressCallback == nil {
		progressCallback = func() {}
	}

	tempDir, err := os.MkdirTemp("", "fatalder-mcworld-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	worldDir := filepath.Join(tempDir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return err
	}

	bedrockWorld, err := world.Open(worldDir, nil)
	if err != nil {
		return err
	}

	file, err := os.Open(srcPath)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}
	defer reader.Close()

	size := reader.GetSize()
	if err := reader.ToMCWorld(bedrockWorld, bwo_define.SubChunkPos{0, -4, 0}, startCallback, progressCallback); err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}

	structureName := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]",
		structureName,
		size.Width-1,
		size.Height-64-1,
		size.Length-1,
	)
	bedrockWorld.LevelDat().LevelName = worldName
	if err := bedrockWorld.CloseWorld(); err != nil {
		return err
	}

	return archiveDirAsMCWorld(worldDir, destPath)
}

func convertStructureFileToMCWorldFast(srcPath, destPath string, startCallback func(int), progressCallback func()) error {
	if startCallback == nil {
		startCallback = func(int) {}
	}
	if progressCallback == nil {
		progressCallback = func() {}
	}

	tempDir, err := os.MkdirTemp("", "fatalder-mcworld-fast-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	worldDir := filepath.Join(tempDir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return err
	}

	bedrockWorld, err := world.Open(worldDir, nil)
	if err != nil {
		return err
	}

	file, err := os.Open(srcPath)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}
	defer file.Close()

	reader, err := wsstructure.StructureFromFile(file)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}
	defer reader.Close()

	size := reader.GetSize()
	if err := convertReaderToMCWorldFast(reader, bedrockWorld, bwo_define.SubChunkPos{0, -4, 0}, startCallback, progressCallback); err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}

	structureName := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]",
		structureName,
		size.Width-1,
		size.Height-64-1,
		size.Length-1,
	)
	bedrockWorld.LevelDat().LevelName = worldName
	if err := bedrockWorld.CloseWorld(); err != nil {
		return err
	}

	return archiveDirAsMCWorld(worldDir, destPath)
}

func convertReaderToMCWorldFast(reader wsstructure.Structure, bedrockWorld *world.BedrockWorld, startSubChunkPos bwo_define.SubChunkPos, startCallback func(int), progressCallback func()) error {
	if reader == nil {
		return errors.New("reader is nil")
	}
	if bedrockWorld == nil {
		return errors.New("bedrock world is nil")
	}

	size := reader.GetSize()
	xCount := size.GetChunkXCount()
	zCount := size.GetChunkZCount()
	totalChunks := xCount * zCount
	startCallback(totalChunks)
	if totalChunks == 0 {
		return nil
	}

	allChunkPos := make([]wsdefine.ChunkPos, 0, totalChunks)
	for x := 0; x < xCount; x++ {
		for z := 0; z < zCount; z++ {
			allChunkPos = append(allChunkPos, wsdefine.ChunkPos{int32(x), int32(z)})
		}
	}

	batchSize := 256
	if totalChunks < batchSize {
		batchSize = totalChunks
	}
	if batchSize <= 0 {
		batchSize = 1
	}

	type batchResult struct {
		positions []wsdefine.ChunkPos
		chunks    map[wsdefine.ChunkPos]*chunk.Chunk
		nbts      map[wsdefine.ChunkPos]map[wsdefine.BlockPos]map[string]any
		err       error
	}

	taskCh := make(chan []wsdefine.ChunkPos)
	resultCh := make(chan batchResult)

	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for positions := range taskCh {
				chunks, err := reader.GetChunks(positions)
				if err != nil {
					resultCh <- batchResult{positions: positions, err: err}
					continue
				}
				nbts, err := reader.GetChunksNBT(positions)
				if err != nil {
					resultCh <- batchResult{positions: positions, chunks: chunks, err: err}
					continue
				}
				resultCh <- batchResult{positions: positions, chunks: chunks, nbts: nbts}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	go func() {
		for i := 0; i < len(allChunkPos); i += batchSize {
			end := i + batchSize
			if end > len(allChunkPos) {
				end = len(allChunkPos)
			}
			taskCh <- allChunkPos[i:end]
		}
		close(taskCh)
	}()

	chunkOffsetX := startSubChunkPos.X()
	chunkOffsetZ := startSubChunkPos.Z()
	blockYOffset := startSubChunkPos.Y() * 16

	for res := range resultCh {
		if res.err != nil {
			return res.err
		}

		for _, pos := range res.positions {
			chunkData, ok := res.chunks[pos]
			if ok && chunkData != nil {
				chunkData.Compact()
				targetPos := bwo_define.ChunkPos{pos.X() + chunkOffsetX, pos.Z() + chunkOffsetZ}
				if err := bedrockWorld.SaveChunk(bwo_define.DimensionIDOverworld, targetPos, chunkData); err != nil {
					return err
				}
			}
			progressCallback()
		}

		for cpos, blockMap := range res.nbts {
			if len(blockMap) == 0 {
				continue
			}
			list := make([]map[string]any, 0, len(blockMap))
			absChunkX := (cpos.X() + chunkOffsetX) * 16
			absChunkZ := (cpos.Z() + chunkOffsetZ) * 16
			for bpos, n := range blockMap {
				if n == nil {
					continue
				}
				m := make(map[string]any, len(n)+3)
				for k, v := range n {
					m[k] = v
				}
				m["x"] = absChunkX + bpos.X()
				m["y"] = blockYOffset + bpos.Y() + 64
				m["z"] = absChunkZ + bpos.Z()
				list = append(list, m)
			}
			if len(list) == 0 {
				continue
			}
			targetPos := bwo_define.ChunkPos{cpos.X() + chunkOffsetX, cpos.Z() + chunkOffsetZ}
			if err := bedrockWorld.SaveNBT(bwo_define.DimensionIDOverworld, targetPos, list); err != nil {
				return err
			}
		}
	}

	return nil
}

func unarchiveMCWorldToTempDir(mcworldPath string) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "fatalder-mcworld-unzip-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	z := archiver.Zip{}
	if err := z.Unarchive(mcworldPath, tempDir); err != nil {
		cleanup()
		return "", nil, err
	}
	return tempDir, cleanup, nil
}

func archiveDirAsMCWorld(worldDir string, outPath string) error {
	entries, err := os.ReadDir(worldDir)
	if err != nil {
		return err
	}
	var inputs []string
	for _, entry := range entries {
		inputs = append(inputs, filepath.Join(worldDir, entry.Name()))
	}

	tmpDir, err := os.MkdirTemp("", "fatalder-mcworld-zip-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tmpZip := filepath.Join(tmpDir, "world.zip")
	z := archiver.Zip{}
	if err := z.Archive(inputs, tmpZip); err != nil {
		return err
	}

	if err := os.RemoveAll(outPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpZip, outPath); err != nil {
		return copyAndRemoveFile(tmpZip, outPath)
	}
	return nil
}

func copyAndRemoveFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	_ = dstFile.Sync()
	return os.Remove(src)
}
