package structure

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/utils"
	"github.com/Yeah114/bdump/command"
	bdumptypes "github.com/Yeah114/bdump/types"
	"github.com/Yeah114/blocks"

	"github.com/andybalholm/brotli"
)

var CommandBlockNames = []string{
	"command_block",
	"repeating_command_block",
	"chain_command_block",
}

type BDX struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	minPos       define.BlockPos
	cmdNum       uint64

	runtimeBlockPoolID uint8
	constantStrings    map[uint16]string
	Author             string
	BlockNBT           map[define.BlockPos]map[string]any
}

func (k *BDX) ID() uint8 {
	return IDBDX
}

func (k *BDX) Name() string {
	return NameBDX
}

func (b *BDX) FromFile(file *os.File) (err error) {
	b.file = file
	b.size = &define.Size{}
	b.originalSize = &define.Size{}
	b.offsetPos = define.Offset{}
	b.minPos = define.BlockPos{}
	b.constantStrings = make(map[uint16]string)
	b.BlockNBT = make(map[define.BlockPos]map[string]any)

	if err := b.parseHeader(file); err != nil {
		return err
	}

	brw := brotli.NewReader(file)
	if err := b.parseMetadata(brw); err != nil {
		return err
	}

	return b.parseCommands(brw)
}

func (b *BDX) parseMetadata(brw *brotli.Reader) error {
	header := make([]byte, 3)
	if _, err := brw.Read(header); err != nil {
		return err
	}
	if string(header) != "BDX" {
		return ErrInvalidFile
	}

	author := make([]byte, 0)
	for {
		bs := make([]byte, 1)
		if _, err := brw.Read(bs); err != nil {
			return err
		}
		if bs[0] == 0 {
			break
		}
		author = append(author, bs[0])
	}
	b.Author = string(author)

	if _, err := brw.Read(make([]byte, 1)); err != nil {
		return err
	}

	return nil
}

func (b *BDX) parseCommands(brw *brotli.Reader) error {
	constantStringID := uint16(0)
	pos := [3]int32{0, 0, 0}
	size := [3]int32{0, 0, 0}
	minPos := [3]int32{0, 0, 0}
	cmdNum := uint64(0)

	for {
		cmd, err := command.ReadCommand(brw)
		if err == io.EOF {
			break
		}
		if err != nil {
			//return err
			continue
		}
		cmdNum++

		advancePos(cmd, &pos)

		switch c := cmd.(type) {
		case *command.CreateConstantString:
			b.constantStrings[constantStringID] = c.ConstantString
			constantStringID++
			continue
		case *command.UseRuntimeIDPool:
			b.runtimeBlockPoolID = c.PoolID
			continue
		case *command.Terminate:
			break
		}

		if nbtMap := buildNBTFromCommand(cmd); nbtMap != nil {
			runtimeBlockPool := BDXRuntimeBlockPools[b.runtimeBlockPoolID]
			if blockRuntimeID := b.getBlockRuntimeID(cmd, runtimeBlockPool); blockRuntimeID != 0 {
				blockName, _, found := block.RuntimeIDToState(blockRuntimeID)
				if found {
					id := chestBlockNameToID(blockName)
					if id != "" {
						nbtMap["id"] = id
					}
				}
			}
			b.BlockNBT[define.BlockPos{pos[0], pos[1], pos[2]}] = nbtMap
		}

		if pos[0] > size[0] {
			size[0] = pos[0]
		}
		if pos[1] > size[1] {
			size[1] = pos[1]
		}
		if pos[2] > size[2] {
			size[2] = pos[2]
		}
		if pos[0] < minPos[0] {
			minPos[0] = pos[0]
		}
		if pos[1] < minPos[1] {
			minPos[1] = pos[1]
		}
		if pos[2] < minPos[2] {
			minPos[2] = pos[2]
		}

		if _, isTerminate := cmd.(*command.Terminate); isTerminate {
			break
		}
	}

	b.minPos = define.BlockPos{minPos[0], minPos[1], minPos[2]}
	b.cmdNum = cmdNum
	b.size.Width = int(size[0]-minPos[0]) + 1
	b.size.Height = int(size[1]-minPos[1]) + 1
	b.size.Length = int(size[2]-minPos[2]) + 1
	b.originalSize.Width = b.size.Width
	b.originalSize.Height = b.size.Height
	b.originalSize.Length = b.size.Length
	return nil
}

func advancePos(cmd command.Command, pos *[3]int32) {
	switch c := cmd.(type) {
	case *command.AddXValue:
		pos[0]++
	case *command.AddYValue:
		pos[1]++
	case *command.AddZValue:
		pos[2]++
	case *command.AddZValue0:
		pos[2]++
	case *command.AddInt8XValue:
		pos[0] += int32(c.Value)
	case *command.AddInt8YValue:
		pos[1] += int32(c.Value)
	case *command.AddInt8ZValue:
		pos[2] += int32(c.Value)
	case *command.AddInt16XValue:
		pos[0] += int32(c.Value)
	case *command.AddInt16YValue:
		pos[1] += int32(c.Value)
	case *command.AddInt16ZValue:
		pos[2] += int32(c.Value)
	case *command.AddInt16ZValue0:
		pos[2] += int32(c.Value)
	case *command.AddInt32XValue:
		pos[0] += int32(c.Value)
	case *command.AddInt32YValue:
		pos[1] += int32(c.Value)
	case *command.AddInt32ZValue:
		pos[2] += int32(c.Value)
	case *command.AddInt32ZValue0:
		pos[2] += int32(c.Value)
	case *command.SubtractXValue:
		pos[0]--
	case *command.SubtractYValue:
		pos[1]--
	case *command.SubtractZValue:
		pos[2]--
	}
}

func bdxFloorDiv(value, divisor int) int {
	if divisor == 0 {
		return 0
	}
	result := value / divisor
	if value < 0 && value%divisor != 0 {
		result--
	}
	return result
}

func bdxFloorMod(value, divisor int) int {
	if divisor == 0 {
		return 0
	}
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}
	return remainder
}

func buildNBT[T any](data T) map[string]any {
	switch v := any(data).(type) {
	case *bdumptypes.CommandBlockData:
		n := make(map[string]any)
		n["Command"] = v.Command
		n["CustomName"] = v.CustomName
		n["LastOutput"] = v.LastOutput
		n["TickDelay"] = v.TickDelay
		if v.ExecuteOnFirstTick {
			n["ExecuteOnFirstTick"] = byte(1)
		} else {
			n["ExecuteOnFirstTick"] = byte(0)
		}
		if v.TrackOutput {
			n["TrackOutput"] = byte(1)
		} else {
			n["TrackOutput"] = byte(0)
		}
		if v.Conditional {
			n["conditionalMode"] = byte(1)
		} else {
			n["conditionalMode"] = byte(0)
		}
		if v.NeedsRedstone {
			n["auto"] = byte(0)
		} else {
			n["auto"] = byte(1)
		}
		n["id"] = "CommandBlock"
		return n
	case []bdumptypes.ChestSlot:
		n := make(map[string]any)
		items := make([]map[string]any, 0, len(v))
		for _, slot := range v {
			item := map[string]any{
				"Slot":   byte(slot.Slot),
				"Name":   slot.Name,
				"Count":  slot.Count,
				"Damage": int16(slot.Damage),
			}
			items = append(items, item)
		}
		n["Items"] = items
		return n
	default:
		return nil
	}
}

func buildNBTFromCommand[T command.Command](cmd T) map[string]any {
	switch c := any(cmd).(type) {
	case *command.PlaceBlockWithNBTData:
		return c.BlockNBT
	case *command.PlaceBlockWithCommandBlockData:
		return buildNBT(c.CommandBlockData)
	case *command.PlaceCommandBlockWithCommandBlockData:
		return buildNBT(c.CommandBlockData)
	case *command.PlaceRuntimeBlockWithCommandBlockData:
		return buildNBT(c.CommandBlockData)
	case *command.PlaceRuntimeBlockWithCommandBlockDataAndUint32RuntimeID:
		return buildNBT(c.CommandBlockData)
	case *command.SetCommandBlockData:
		return buildNBT(c.CommandBlockData)
	case *command.PlaceBlockWithChestData:
		return buildNBT(c.ChestSlots)
	case *command.PlaceRuntimeBlockWithChestData:
		return buildNBT(c.ChestSlots)
	case *command.PlaceRuntimeBlockWithChestDataAndUint32RuntimeID:
		return buildNBT(c.ChestSlots)
	default:
		return nil
	}
}

func chestBlockNameToID(blockName string) string {
	blockNBTID := ""
	switch blockName {
	case "minecraft:blast_furnace", "minecraft:lit_blast_furnace":
		blockNBTID = "BlastFurnace"
	case "minecraft:furnace", "minecraft:lit_furnace":
		blockNBTID = "Furnace"
	case "minecraft:smoker", "minecraft:lit_smoker":
		blockNBTID = "Smoker"
	case "minecraft:chest", "minecraft:trapped_chest":
		blockNBTID = "Chest"
	case "minecraft:hopper":
		blockNBTID = "Hopper"
	case "minecraft:dispenser":
		blockNBTID = "Dispenser"
	case "minecraft:dropper":
		blockNBTID = "Dropper"
	case "minecraft:barrel":
		blockNBTID = "Barrel"
	case "minecraft:crafter":
		blockNBTID = "Crafter"
	}
	if strings.Contains(blockName, "shulker_box") {
		blockNBTID = "ShulkerBox"
	}
	return blockNBTID
}

func (b *BDX) GetOffsetPos() define.Offset {
	return b.offsetPos
}

func (b *BDX) SetOffsetPos(offset define.Offset) {
	b.offsetPos = offset
	b.size.Width = b.originalSize.Width + int(math.Abs(float64(offset.X())))
	b.size.Length = b.originalSize.Length + int(math.Abs(float64(offset.Z())))
	b.size.Height = b.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (b *BDX) GetSize() define.Size {
	return *b.size
}

func (b *BDX) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk)
	for _, pos := range posList {
		chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if err := b.parseHeader(file); err != nil {
		return nil, err
	}

	brw := brotli.NewReader(file)
	if err := b.skipMetadata(brw); err != nil {
		return nil, err
	}

	return b.parseCommandsToChunks(brw, chunks)
}

func (b *BDX) parseHeader(file *os.File) error {
	header := make([]byte, 3)
	if _, err := file.Read(header); err != nil {
		return err
	}
	if string(header) != "BD@" {
		return ErrInvalidFile
	}
	return nil
}

func (b *BDX) skipMetadata(brw *brotli.Reader) error {
	header := make([]byte, 3)
	if _, err := brw.Read(header); err != nil {
		return err
	}
	if string(header) != "BDX" {
		return ErrInvalidFile
	}

	for {
		bs := make([]byte, 1)
		if _, err := brw.Read(bs); err != nil {
			return err
		}
		if bs[0] == 0 {
			break
		}
	}

	if _, err := brw.Read(make([]byte, 1)); err != nil {
		return err
	}

	return nil
}

func (b *BDX) parseCommandsToChunks(brw *brotli.Reader, chunks map[define.ChunkPos]*chunk.Chunk) (map[define.ChunkPos]*chunk.Chunk, error) {
	pos := [3]int32{0, 0, 0}
	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	minX := int(b.minPos.X())
	minY := int(b.minPos.Y())
	minZ := int(b.minPos.Z())
	runtimeBlockPool := BDXRuntimeBlockPools[b.runtimeBlockPoolID]

	for {
		cmd, err := command.ReadCommand(brw)
		if err == io.EOF {
			break
		}
		if err != nil {
			//return nil, err
			continue
		}

		advancePos(cmd, &pos)

		if _, isTerminate := cmd.(*command.Terminate); isTerminate {
			break
		}

		if b.isMetadataCommand(cmd) {
			continue
		}

		relativeX := int(pos[0]) - minX
		relativeY := int(pos[1]) - minY
		relativeZ := int(pos[2]) - minZ
		worldX := relativeX + offsetX
		worldY := relativeY + offsetY
		worldZ := relativeZ + offsetZ

		if worldY < 0 || worldY >= b.size.Height {
			continue
		}

		chunkPos := define.ChunkPos{int32(bdxFloorDiv(worldX, 16)), int32(bdxFloorDiv(worldZ, 16))}
		targetChunk, exists := chunks[chunkPos]
		if !exists {
			continue
		}

		localX := uint8(bdxFloorMod(worldX, 16))
		localY := int16(worldY)
		localZ := uint8(bdxFloorMod(worldZ, 16))

		if blockRuntimeID := b.getBlockRuntimeID(cmd, runtimeBlockPool); blockRuntimeID != 0 {
			targetChunk.SetBlock(localX, localY-64, localZ, 0, blockRuntimeID)
		}
	}

	return chunks, nil
}

func (b *BDX) isMetadataCommand(cmd command.Command) bool {
	switch cmd.(type) {
	case *command.CreateConstantString, *command.UseRuntimeIDPool:
		return true
	default:
		return false
	}
}

func (b *BDX) getBlockRuntimeID(cmd command.Command, runtimeBlockPool []uint32) uint32 {
	switch c := cmd.(type) {
	case *command.PlaceBlock:
		return b.processLegacyBlock(c.BlockConstantStringID, c.BlockData)
	case *command.PlaceBlockWithBlockStates:
		return b.processBlockWithStates(c.BlockConstantStringID, c.BlockStatesConstantStringID)
	case *command.PlaceBlockWithBlockStatesDeprecated:
		return b.processBlockWithStatesString(c.BlockConstantStringID, c.BlockStatesString)
	case *command.PlaceBlockWithChestData:
		return b.processLegacyBlock(c.BlockConstantStringID, c.BlockData)
	case *command.PlaceBlockWithCommandBlockData:
		return b.processLegacyBlock(c.BlockConstantStringID, c.BlockData)
	case *command.PlaceBlockWithNBTData:
		return b.processBlockWithStates(c.BlockConstantStringID, c.BlockStatesConstantStringID)
	case *command.PlaceCommandBlockWithCommandBlockData:
		blockName := CommandBlockNames[c.CommandBlockData.Mode]
		return b.processLegacyBlockByName(blockName, c.BlockData)
	case *command.PlaceRuntimeBlock:
		return runtimeBlockPool[c.BlockRuntimeID]
	case *command.PlaceRuntimeBlockWithChestData:
		return runtimeBlockPool[c.BlockRuntimeID]
	case *command.PlaceRuntimeBlockWithChestDataAndUint32RuntimeID:
		return runtimeBlockPool[c.BlockRuntimeID]
	case *command.PlaceRuntimeBlockWithCommandBlockData:
		return runtimeBlockPool[c.BlockRuntimeID]
	case *command.PlaceRuntimeBlockWithCommandBlockDataAndUint32RuntimeID:
		return runtimeBlockPool[c.BlockRuntimeID]
	default:
		return 0
	}
}

func (b *BDX) processLegacyBlock(blockConstantStringID uint16, blockData uint16) uint32 {
	blockName, ok := b.constantStrings[blockConstantStringID]
	if !ok {
		return 0
	}
	return b.processLegacyBlockByName(blockName, blockData)
}

func (b *BDX) processLegacyBlockByName(blockName string, blockData uint16) uint32 {
	runtimeID, found := blocks.LegacyBlockToRuntimeID(blockName, blockData)
	if !found {
		return 0
	}
	name, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		return 0
	}
	blockRuntimeID, found := block.StateToRuntimeID(name, properties)
	if !found {
		return 0
	}
	return blockRuntimeID
}

func (b *BDX) processBlockWithStates(blockConstantStringID, blockStatesConstantStringID uint16) uint32 {
	blockName, ok := b.constantStrings[blockConstantStringID]
	if !ok {
		return 0
	}
	blockStates, ok := b.constantStrings[blockStatesConstantStringID]
	if !ok {
		return 0
	}

	runtimeID, found := blocks.BlockNameAndStateStrToRuntimeID(blockName, blockStates)
	if !found {
		return 0
	}
	name, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		return 0
	}
	blockRuntimeID, found := block.StateToRuntimeID(name, properties)
	if !found {
		return 0
	}
	return blockRuntimeID
}

func (b *BDX) processBlockWithStatesString(blockConstantStringID uint16, blockStates string) uint32 {
	blockName, ok := b.constantStrings[blockConstantStringID]
	if !ok {
		return 0
	}
	runtimeID, found := blocks.BlockNameAndStateStrToRuntimeID(blockName, blockStates)
	if !found {
		return 0
	}
	name, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		return 0
	}
	blockRuntimeID, found := block.StateToRuntimeID(name, properties)
	if !found {
		return 0
	}
	return blockRuntimeID
}

func (b *BDX) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any)
	for _, pos := range posList {
		result[pos] = make(map[define.BlockPos]map[string]any)
	}

	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	minX := int(b.minPos.X())
	minY := int(b.minPos.Y())
	minZ := int(b.minPos.Z())

	for blockPos, nbtData := range b.BlockNBT {
		relativeX := int(blockPos.X()) - minX
		relativeY := int(blockPos.Y()) - minY
		relativeZ := int(blockPos.Z()) - minZ

		worldX := relativeX + offsetX
		worldY := relativeY + offsetY
		worldZ := relativeZ + offsetZ

		if worldY < 0 || worldY >= b.size.Height {
			continue
		}

		chunkPos := define.ChunkPos{int32(bdxFloorDiv(worldX, 16)), int32(bdxFloorDiv(worldZ, 16))}
		if chunkNBT, exists := result[chunkPos]; exists {
			localX := int32(bdxFloorMod(worldX, 16))
			localZ := int32(bdxFloorMod(worldZ, 16))
			localBlockPos := define.BlockPos{localX, int32(worldY) - 64, localZ}
			chunkNBT[localBlockPos] = nbtData
		}
	}

	return result, nil
}

func (b *BDX) CountNonAirBlocks() (int, error) {
	nonAirBlocks := 0

	file, err := os.Open(b.file.Name())
	if err != nil {
		return nonAirBlocks, fmt.Errorf("failed to reopen file: %w", err)
	}
	defer file.Close()

	if err := b.parseHeader(file); err != nil {
		return nonAirBlocks, err
	}

	brw := brotli.NewReader(file)
	if err := b.skipMetadata(brw); err != nil {
		return nonAirBlocks, err
	}

	runtimeBlockPool := BDXRuntimeBlockPools[b.runtimeBlockPoolID]

	for {
		cmd, err := command.ReadCommand(brw)
		if err == io.EOF {
			break
		}
		if err != nil {
			//return nonAirBlocks, err
			continue
		}

		if _, isTerminate := cmd.(*command.Terminate); isTerminate {
			break
		}

		if b.isMetadataCommand(cmd) {
			continue
		}

		if blockRuntimeID := b.getBlockRuntimeID(cmd, runtimeBlockPool); blockRuntimeID != 0 {
			nonAirBlocks++
		}
	}

	return nonAirBlocks, nil
}

func (b *BDX) Close() error {
	return nil
}

/*
func (b *BDX) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	return convertReaderToMCWorld(b, bedrockWorld, startSubChunkPos, startCallback, progressCallback)
}
*/

func (b *BDX) ToMCWorld(
	bedrockWorld *world.BedrockWorld,
	startSubChunkPos bwo_define.SubChunkPos,
	startCallback func(int),
	progressCallback func(),
) error {
	startSubChunkPosX := startSubChunkPos.X()
	startSubChunkPosY := startSubChunkPos.Y()
	startSubChunkPosZ := startSubChunkPos.Z()
	startX := startSubChunkPosX * 16
	startY := startSubChunkPosY * 16
	startZ := startSubChunkPosZ * 16
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	mcworld, err := utils.NewMCWorld(bedrockWorld, ctx)
	if err != nil {
		return err
	}
	mcworld.AutoFlush(time.Second)

	// 1. 强制总进度为 100
	totalProgress := 100
	if startCallback != nil {
		startCallback(totalProgress)
	}

	file, err := os.Open(b.file.Name())
	if err != nil {
		return fmt.Errorf("重新打开文件失败: %w", err)
	}
	defer file.Close()

	if err := b.parseHeader(file); err != nil {
		return err
	}

	brw := brotli.NewReader(file)
	if err := b.skipMetadata(brw); err != nil {
		return err
	}

	pos := [3]int32{0, 0, 0}
	offsetX := int(b.offsetPos.X())
	offsetY := int(b.offsetPos.Y())
	offsetZ := int(b.offsetPos.Z())
	minX := int(b.minPos.X())
	minY := int(b.minPos.Y())
	minZ := int(b.minPos.Z())
	runtimeBlockPool := BDXRuntimeBlockPools[b.runtimeBlockPoolID]

	// 2. 计算实际总任务数
	totalItems := int(b.cmdNum) + len(b.BlockNBT)
	if totalItems <= 0 {
		totalItems = 1 // 避免除以0
	}
	currentItem := 0
	lastReportedProgress := -1

	// 处理命令
	for {
		cmd, err := command.ReadCommand(brw)
		if err == io.EOF {
			break
		}
		if err != nil {
			//return err
			continue
		}

		advancePos(cmd, &pos)

		if _, isTerminate := cmd.(*command.Terminate); isTerminate {
			break
		}

		if b.isMetadataCommand(cmd) {
			continue
		}

		relativeX := int(pos[0]) - minX
		relativeY := int(pos[1]) - minY
		relativeZ := int(pos[2]) - minZ
		worldX := relativeX + offsetX
		worldY := relativeY + offsetY
		worldZ := relativeZ + offsetZ

		if blockRuntimeID := b.getBlockRuntimeID(cmd, runtimeBlockPool); blockRuntimeID != 0 {
			x := startX + int32(worldX)
			y := int16(int(startY) + worldY)
			z := startZ + int32(worldZ)
			err = mcworld.SetBlock(x, y, z, blockRuntimeID)
			if err != nil {
				return err
			}
		}

		// 3. 更新进度
		currentItem++
		currentProgress := (currentItem * totalProgress) / totalItems
		if progressCallback != nil && currentProgress > lastReportedProgress {
			for i := lastReportedProgress + 1; i <= currentProgress; i++ {
				progressCallback()
			}
			lastReportedProgress = currentProgress
		}
	}

	mcworld.Flush()
	chunksNBTs := map[bwo_define.ChunkPos][]map[string]any{}

	// 处理 BlockNBT
	for pos, nbt := range b.BlockNBT {
		relativeX := int(pos[0]) - minX
		relativeY := int(pos[1]) - minY
		relativeZ := int(pos[2]) - minZ
		worldX := relativeX + offsetX
		worldY := relativeY + offsetY
		worldZ := relativeZ + offsetZ
		x := startX + int32(worldX)
		y := startY + int32(worldY)
		z := startZ + int32(worldZ)
		blockNBT := utils.DeepCopyNBT(nbt)
		blockNBT["x"] = x
		blockNBT["y"] = y
		blockNBT["z"] = z
		chunkPos := define.ChunkPos{int32(bdxFloorDiv(int(x), 16)), int32(bdxFloorDiv(int(z), 16))}
		nbts, ok := chunksNBTs[chunkPos]
		if !ok {
			chunksNBTs[chunkPos] = make([]map[string]any, 0)
			nbts = chunksNBTs[chunkPos]
		}
		chunksNBTs[chunkPos] = append(nbts, blockNBT)

		// 3. 更新进度
		currentItem++
		currentProgress := (currentItem * totalProgress) / totalItems
		if progressCallback != nil && currentProgress > lastReportedProgress {
			for i := lastReportedProgress + 1; i <= currentProgress; i++ {
				progressCallback()
			}
			lastReportedProgress = currentProgress
		}
	}

	// 确保进度达到 100
	if progressCallback != nil && lastReportedProgress < totalProgress {
		for i := lastReportedProgress + 1; i <= totalProgress; i++ {
			progressCallback()
		}
	}

	for chunkPos, nbts := range chunksNBTs {
		err := bedrockWorld.SaveNBT(bwo_define.Dimension(bwo_define.DimensionIDOverworld), chunkPos, nbts)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *BDX) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	startBlockPos := define.BlockPos{
		min(point1BlockPos.X(), point2BlockPos.X()),
		min(point1BlockPos.Y(), point2BlockPos.Y()),
		min(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	startBlockPosX := startBlockPos.X()
	startBlockPosY := startBlockPos.Y()
	startBlockPosZ := startBlockPos.Z()

	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPosX := endBlockPos.X()
	endBlockPosY := endBlockPos.Y()
	endBlockPosZ := endBlockPos.Z()

	startSubChunkPos := define.SubChunkPos{
		(startBlockPosX - mod(startBlockPosX, 16)) / 16,
		(startBlockPosY - mod(startBlockPosY, 16)) / 16,
		(startBlockPosZ - mod(startBlockPosZ, 16)) / 16,
	}

	endSubChunkPos := define.SubChunkPos{
		(endBlockPosX + mod(endBlockPosX, 16) + 15) / 16,
		(endBlockPosY + mod(endBlockPosY, 16) + 15) / 16,
		(endBlockPosZ + mod(endBlockPosZ, 16) + 15) / 16,
	}

	startSubChunkPosX := startSubChunkPos.X()
	startSubChunkPosY := startSubChunkPos.Y()
	startSubChunkPosZ := startSubChunkPos.Z()
	subChunkXNum := endSubChunkPos.X() - startSubChunkPosX + 1
	subChunkYNum := endSubChunkPos.Y() - startSubChunkPosY + 1
	subChunkZNum := endSubChunkPos.Z() - startSubChunkPosZ + 1
	subChunkNum := subChunkXNum * subChunkYNum * subChunkZNum

	_, err := target.Write([]byte("BD@"))
	if err != nil {
		return err
	}
	brotilWriter := brotli.NewWriter(target)
	defer brotilWriter.Close()
	_, err = brotilWriter.Write([]byte("BDX"))
	if err != nil {
		return err
	}
	_, err = brotilWriter.Write([]byte{0})
	if err != nil {
		return err
	}
	_, err = brotilWriter.Write([]byte{0})
	if err != nil {
		return err
	}

	if startCallback != nil {
		startCallback(int(subChunkNum))
	}
	pos := startSubChunkPos
	movePos := func(blockPos define.BlockPos) error {
		moveX := blockPos.X() - pos.X()
		moveY := blockPos.Y() - pos.Y()
		moveZ := blockPos.Z() - pos.Z()
		switch moveX {
		case 0:
			break
		case 1:
			err := command.WriteCommand(&command.AddXValue{}, brotilWriter)
			if err != nil {
				return err
			}
		case -1:
			err := command.WriteCommand(&command.SubtractXValue{}, brotilWriter)
			if err != nil {
				return err
			}
		default:
			if moveX >= math.MinInt8 && moveX <= math.MaxInt8 {
				err := command.WriteCommand(&command.AddInt8XValue{
					Value: int8(moveX),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else if moveX >= math.MinInt16 && moveX <= math.MaxInt16 {
				err := command.WriteCommand(&command.AddInt16XValue{
					Value: int16(moveX),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else {
				err := command.WriteCommand(&command.AddInt32XValue{
					Value: moveX,
				}, brotilWriter)
				if err != nil {
					return err
				}
			}
		}

		switch moveY {
		case 0:
			break
		case 1:
			err := command.WriteCommand(&command.AddYValue{}, brotilWriter)
			if err != nil {
				return err
			}
		case -1:
			err := command.WriteCommand(&command.SubtractYValue{}, brotilWriter)
			if err != nil {
				return err
			}
		default:
			if moveY >= math.MinInt8 && moveY <= math.MaxInt8 {
				err := command.WriteCommand(&command.AddInt8YValue{
					Value: int8(moveY),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else if moveY >= math.MinInt16 && moveY <= math.MaxInt16 {
				err := command.WriteCommand(&command.AddInt16YValue{
					Value: int16(moveY),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else {
				err := command.WriteCommand(&command.AddInt32YValue{
					Value: moveY,
				}, brotilWriter)
				if err != nil {
					return err
				}
			}
		}

		switch moveZ {
		case 0:
			break
		case 1:
			err := command.WriteCommand(&command.AddZValue{}, brotilWriter)
			if err != nil {
				return err
			}
		case -1:
			err := command.WriteCommand(&command.SubtractZValue{}, brotilWriter)
			if err != nil {
				return err
			}
		default:
			if moveZ >= math.MinInt8 && moveZ <= math.MaxInt8 {
				err := command.WriteCommand(&command.AddInt8ZValue{
					Value: int8(moveZ),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else if moveZ >= math.MinInt16 && moveZ <= math.MaxInt16 {
				err := command.WriteCommand(&command.AddInt16ZValue{
					Value: int16(moveZ),
				}, brotilWriter)
				if err != nil {
					return err
				}
			} else {
				err := command.WriteCommand(&command.AddInt32ZValue{
					Value: moveZ,
				}, brotilWriter)
				if err != nil {
					return err
				}
			}
		}

		pos[0] += moveX
		pos[1] += moveY
		pos[2] += moveZ
		return nil
	}

	palette := make(map[uint32][2]uint16)
	placeBlock := func(blockRuntimeID uint32, nbt map[string]any) error {
		if nbt == nil {
			ids, ok := palette[blockRuntimeID]
			var blockConstantStringID, blockStatesConstantStringID uint16
			if ok {
				blockConstantStringID = ids[0]
				blockStatesConstantStringID = ids[1]
			} else {
				blockName, blockStates, found := block.RuntimeIDToState(blockRuntimeID)
				if !found {
					blockName, blockStates, _ = block.RuntimeIDToState(UnknownBlockRuntimeID)
				}
				err := command.WriteCommand(&command.CreateConstantString{
					ConstantString: blockName,
				}, brotilWriter)
				if err != nil {
					return err
				}
				err = command.WriteCommand(&command.CreateConstantString{
					ConstantString: utils.PropertiesToStateStr(blockStates),
				}, brotilWriter)
				if err != nil {
					return err
				}
				blockConstantStringID = uint16(len(palette)) * 2
				blockStatesConstantStringID = blockConstantStringID + 1
				palette[blockRuntimeID] = [2]uint16{
					blockConstantStringID,
					blockStatesConstantStringID,
				}
			}
			err := command.WriteCommand(&command.PlaceBlockWithBlockStates{
				BlockConstantStringID:       blockConstantStringID,
				BlockStatesConstantStringID: blockStatesConstantStringID,
			}, brotilWriter)
			if err != nil {
				return err
			}
		}
		return nil
	}
	for subChunkZ := range subChunkZNum {
		for subChunkX := range subChunkXNum {
			for subChunkY := range subChunkYNum {
				worldSubChunkPos := bwo_define.SubChunkPos{
					startSubChunkPosX + subChunkX,
					startSubChunkPosY + subChunkY,
					startSubChunkPosZ + subChunkZ,
				}
				subChunk := world.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
				if subChunk == nil {
					continue
				}
				if subChunk.Empty() {
					continue
				}
				subChunkWorldXStart := worldSubChunkPos.X() * 16
				subChunkWorldXEnd := subChunkWorldXStart + 15
				subChunkWorldYStart := worldSubChunkPos.Y() * 16
				subChunkWorldYEnd := subChunkWorldYStart + 15
				subChunkWorldZStart := worldSubChunkPos.Z() * 16
				subChunkWorldZEnd := subChunkWorldZStart + 15

				effectiveWorldXStart := max(subChunkWorldXStart, startBlockPosX)
				effectiveWorldXEnd := min(subChunkWorldXEnd, endBlockPosX)
				effectiveWorldYStart := max(subChunkWorldYStart, startBlockPosY)
				effectiveWorldYEnd := min(subChunkWorldYEnd, endBlockPosY)
				effectiveWorldZStart := max(subChunkWorldZStart, startBlockPosZ)
				effectiveWorldZEnd := min(subChunkWorldZEnd, endBlockPosZ)

				if effectiveWorldXStart > effectiveWorldXEnd ||
					effectiveWorldYStart > effectiveWorldYEnd ||
					effectiveWorldZStart > effectiveWorldZEnd {
					continue
				}

				for x := byte(effectiveWorldXStart - subChunkWorldXStart); x <= byte(effectiveWorldXEnd-subChunkWorldXStart); x++ {
					for y := byte(effectiveWorldYStart - subChunkWorldYStart); y <= byte(effectiveWorldYEnd-subChunkWorldYStart); y++ {
						for z := byte(effectiveWorldZStart - subChunkWorldZStart); z <= byte(effectiveWorldZEnd-subChunkWorldZStart); z++ {
							blockRuntimeID := subChunk.Block(x, y, z, 0)
							if blockRuntimeID == block.AirRuntimeID {
								continue
							}
							blockPosX := worldSubChunkPos.X()*16 + int32(x)
							blockPosY := worldSubChunkPos.Y()*16 + int32(y)
							blockPosZ := worldSubChunkPos.Z()*16 + int32(z)
							err := movePos(define.BlockPos{
								blockPosX,
								blockPosY,
								blockPosZ,
							})
							if err != nil {
								return err
							}
							err = placeBlock(blockRuntimeID, nil)
							if err != nil {
								return err
							}
						}
					}
				}
				progressCallback()
			}
		}
	}

	err = command.WriteCommand(&command.Terminate{}, brotilWriter)
	if err != nil {
		return err
	}
	return nil
}
