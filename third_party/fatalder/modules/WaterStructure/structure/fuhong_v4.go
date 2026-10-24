package structure

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/blocks"
)

type FuHongV4 struct {
	BaseReader
	file         *os.File
	size         *define.Size
	originalSize *define.Size
	offsetPos    define.Offset
	origin       define.Origin

	palette      []string
	paletteCache map[string]uint32
	blocks       []fuHongV4Block

	nonAirBlocks int
}

type fuHongV4Block struct {
	LocalX    int
	LocalY    int
	LocalZ    int
	RuntimeID uint32
	NBT       map[string]any
}

func (f *FuHongV4) ID() uint8 {
	return IDFuHongV4
}

func (f *FuHongV4) Name() string {
	return NameFuHongV4
}

func (f *FuHongV4) FromFile(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	var root struct {
		FuHongBuild []map[string]any `json:"FuHongBuild"`
		BlocksList  []string         `json:"BlocksList"`
	}

	if err := json.NewDecoder(file).Decode(&root); err != nil {
		return fmt.Errorf("解析 FuHong V4 的 JSON 失败: %w", err)
	}

	if len(root.BlocksList) == 0 {
		return ErrInvalidFile
	}

	f.file = file
	f.palette = root.BlocksList
	return f.populateFromBuild(root.FuHongBuild)
}

func (f *FuHongV4) populateFromBuild(chunks []map[string]any) error {
	f.paletteCache = make(map[string]uint32)
	f.blocks = nil
	f.nonAirBlocks = 0

	minX, minY, minZ := math.MaxInt, math.MaxInt, math.MaxInt
	maxX, maxY, maxZ := math.MinInt, math.MinInt, math.MinInt

	accum := make(map[[3]int]*fuHongV4Block)

	for chunkIdx, chunk := range chunks {
		blockEntries, ok := chunk["block"].([]any)
		if !ok {
			return fmt.Errorf("区块 %d: 方块列表缺失或无效", chunkIdx)
		}

		for entryIdx, rawEntry := range blockEntries {
			tuple, ok := rawEntry.([]any)
			if !ok || len(tuple) < 5 {
				return fmt.Errorf("区块 %d 条目 %d: 元组无效", chunkIdx, entryIdx)
			}

			if _, isString := tuple[0].(string); isString {
				continue // entity tuple – not handled currently
			}

			paletteIndex, err := toInt(tuple[0])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: 调色板索引: %w", chunkIdx, entryIdx, err)
			}
			if paletteIndex < 0 || paletteIndex >= len(f.palette) {
				return fmt.Errorf("区块 %d 条目 %d: 调色板索引 %d 越界", chunkIdx, entryIdx, paletteIndex)
			}

			aux, err := toInt(tuple[1])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: aux: %w", chunkIdx, entryIdx, err)
			}

			xs, err := toIntSlice(tuple[2])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: xs: %w", chunkIdx, entryIdx, err)
			}
			ys, err := toIntSlice(tuple[3])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: ys: %w", chunkIdx, entryIdx, err)
			}
			zs, err := toIntSlice(tuple[4])
			if err != nil {
				return fmt.Errorf("区块 %d 条目 %d: zs: %w", chunkIdx, entryIdx, err)
			}

			if len(xs) != len(ys) || len(xs) != len(zs) {
				return fmt.Errorf("区块 %d 条目 %d: 坐标数组长度不匹配", chunkIdx, entryIdx)
			}

			var extras []any
			if len(tuple) >= 6 {
				if extraSlice, ok := tuple[5].([]any); ok {
					extras = extraSlice
				}
			}

			blockName := f.palette[paletteIndex]
			runtimeID := f.runtimeIDFor(blockName, aux)

			for i := 0; i < len(xs); i++ {
				worldX := xs[i]
				worldY := ys[i]
				worldZ := zs[i]

				var nbt map[string]any
				if i < len(extras) {
					nbt = f.buildExtraData(blockName, extras[i])
				}

				key := [3]int{worldX, worldY, worldZ}
				if existing, ok := accum[key]; ok {
					existing.RuntimeID = runtimeID
					if existing.NBT == nil && nbt != nil {
						existing.NBT = nbt
					}
				} else {
					accum[key] = &fuHongV4Block{
						LocalX:    worldX,
						LocalY:    worldY,
						LocalZ:    worldZ,
						RuntimeID: runtimeID,
						NBT:       nbt,
					}
				}

				if worldX < minX {
					minX = worldX
				}
				if worldY < minY {
					minY = worldY
				}
				if worldZ < minZ {
					minZ = worldZ
				}
				if worldX > maxX {
					maxX = worldX
				}
				if worldY > maxY {
					maxY = worldY
				}
				if worldZ > maxZ {
					maxZ = worldZ
				}
			}
		}
	}

	if len(accum) == 0 {
		return ErrInvalidFile
	}

	width := maxX - minX + 1
	height := maxY - minY + 1
	length := maxZ - minZ + 1

	f.origin = define.Origin{int32(minX), int32(minY), int32(minZ)}
	f.size = &define.Size{Width: width, Height: height, Length: length}
	f.originalSize = &define.Size{Width: width, Height: height, Length: length}

	keys := make([][3]int, 0, len(accum))
	for key := range accum {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		if keys[i][2] != keys[j][2] {
			return keys[i][2] < keys[j][2]
		}
		return keys[i][0] < keys[j][0]
	})

	f.blocks = make([]fuHongV4Block, 0, len(accum))
	f.nonAirBlocks = 0

	for _, key := range keys {
		rec := accum[key]
		blk := fuHongV4Block{
			LocalX:    rec.LocalX - int(f.origin.X()),
			LocalY:    rec.LocalY - int(f.origin.Y()),
			LocalZ:    rec.LocalZ - int(f.origin.Z()),
			RuntimeID: rec.RuntimeID,
			NBT:       rec.NBT,
		}
		f.blocks = append(f.blocks, blk)
		if blk.RuntimeID != block.AirRuntimeID {
			f.nonAirBlocks++
		}
	}

	return nil
}

func (f *FuHongV4) buildExtraData(blockName string, payload any) map[string]any {
	if payload == nil {
		return nil
	}

	lowerName := strings.ToLower(blockName)
	if strings.Contains(lowerName, "command_block") {
		if arr, ok := payload.([]any); ok {
			return buildFuHongCommandNBT(arr)
		}
		return nil
	}

	if chestBlockNameToID(strings.TrimSpace(blockName)) != "" {
		return buildFuHongContainerNBT(blockName, payload)
	}

	if strings.Contains(lowerName, "sign") {
		return buildFuHongSignNBT(blockName, payload)
	}

	return nil
}

func (f *FuHongV4) runtimeIDFor(name string, aux int) uint32 {
	cacheKey := fmt.Sprintf("%s|%d", name, aux)
	if runtimeID, ok := f.paletteCache[cacheKey]; ok {
		return runtimeID
	}

	runtimeID, found := blocks.LegacyBlockToRuntimeID(name, uint16(aux))
	if !found {
		runtimeID, found = blocks.BlockStrToRuntimeID(name)
	}
	if !found && !strings.Contains(name, ":") {
		prefixed := "minecraft:" + name
		runtimeID, found = blocks.LegacyBlockToRuntimeID(prefixed, uint16(aux))
		if !found {
			runtimeID, found = blocks.BlockStrToRuntimeID(prefixed)
		}
	}
	if !found {
		runtimeID = UnknownBlockRuntimeID
	}
	baseName, properties, found := blocks.RuntimeIDToState(runtimeID)
	if !found {
		runtimeID = UnknownBlockRuntimeID
	} else {
		runtimeID, found = block.StateToRuntimeID(baseName, properties)
		if !found {
			runtimeID = UnknownBlockRuntimeID
		}
	}

	f.paletteCache[cacheKey] = runtimeID
	return runtimeID
}

func (f *FuHongV4) GetOffsetPos() define.Offset {
	return f.offsetPos
}

func (f *FuHongV4) SetOffsetPos(offset define.Offset) {
	f.offsetPos = offset
	f.size.Width = f.originalSize.Width + int(math.Abs(float64(offset.X())))
	f.size.Length = f.originalSize.Length + int(math.Abs(float64(offset.Z())))
	f.size.Height = f.originalSize.Height + int(math.Abs(float64(offset.Y())))
}

func (f *FuHongV4) GetSize() define.Size {
	return *f.size
}

func (f *FuHongV4) GetChunks(posList []define.ChunkPos) (map[define.ChunkPos]*chunk.Chunk, error) {
	chunks := make(map[define.ChunkPos]*chunk.Chunk, len(posList))
	height := f.size.Height
	if height <= 0 {
		height = 1
	}
	for _, pos := range posList {
		if _, exists := chunks[pos]; !exists {
			chunks[pos] = chunk.NewChunk(block.AirRuntimeID, MCWorldOverworldRange)
		}
	}

	if len(chunks) == 0 {
		return chunks, nil
	}

	offsetX := int(f.offsetPos.X())
	offsetY := int(f.offsetPos.Y())
	offsetZ := int(f.offsetPos.Z())

	for _, blk := range f.blocks {
		newX := blk.LocalX + offsetX
		newY := blk.LocalY + offsetY
		newZ := blk.LocalZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

		c, exists := chunks[chunkPos]
		if !exists {
			continue
		}

		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		c.SetBlock(uint8(localX), int16(newY)-64, uint8(localZ), 0, blk.RuntimeID)
	}

	return chunks, nil
}

func (f *FuHongV4) GetChunksNBT(posList []define.ChunkPos) (map[define.ChunkPos]map[define.BlockPos]map[string]any, error) {
	result := make(map[define.ChunkPos]map[define.BlockPos]map[string]any, len(posList))
	for _, pos := range posList {
		if _, exists := result[pos]; !exists {
			result[pos] = make(map[define.BlockPos]map[string]any)
		}
	}

	if len(result) == 0 {
		return result, nil
	}

	offsetX := int(f.offsetPos.X())
	offsetY := int(f.offsetPos.Y())
	offsetZ := int(f.offsetPos.Z())

	for _, blk := range f.blocks {
		if blk.NBT == nil {
			continue
		}

		newX := blk.LocalX + offsetX
		newY := blk.LocalY + offsetY
		newZ := blk.LocalZ + offsetZ

		chunkX := floorDiv(newX, 16)
		chunkZ := floorDiv(newZ, 16)
		chunkPos := define.ChunkPos{int32(chunkX), int32(chunkZ)}

		chunkNBT, exists := result[chunkPos]
		if !exists {
			continue
		}

		localX := newX - chunkX*16
		localZ := newZ - chunkZ*16
		blockPos := define.BlockPos{int32(localX), chunkLocalYFromWorld(newY), int32(localZ)}
		chunkNBT[blockPos] = blk.NBT
	}

	return result, nil
}

func (f *FuHongV4) CountNonAirBlocks() (int, error) {
	return f.nonAirBlocks, nil
}

func (f *FuHongV4) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if _, err := target.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置目标文件指针失败: %w", err)
	}
	if err := target.Truncate(0); err != nil {
		return fmt.Errorf("清空目标文件失败: %w", err)
	}

	return f.WriteTo(world, target, point1BlockPos, point2BlockPos, startCallback, progressCallback)
}

func (f *FuHongV4) Close() error {
	return nil
}

type fuHongV4ExportKey struct {
	Palette string
	Aux     int
}

type fuHongV4ExportGroup struct {
	palette   string
	aux       int
	xs        []int
	ys        []int
	zs        []int
	extras    []any
	hasExtras bool
}

type fuHongV4ResolvedPalette struct {
	paletteName string
	aux         int
}

func (f *FuHongV4) WriteTo(
	bw *world.BedrockWorld,
	w io.Writer,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if bw == nil {
		return fmt.Errorf("bedrock 世界为 nil")
	}
	if f.paletteCache == nil {
		f.paletteCache = make(map[string]uint32)
	}
	if w == nil {
		return fmt.Errorf("writer 为 nil")
	}

	startBlockPos := define.BlockPos{
		min(point1BlockPos.X(), point2BlockPos.X()),
		min(point1BlockPos.Y(), point2BlockPos.Y()),
		min(point1BlockPos.Z(), point2BlockPos.Z()),
	}
	endBlockPos := define.BlockPos{
		max(point1BlockPos.X(), point2BlockPos.X()),
		max(point1BlockPos.Y(), point2BlockPos.Y()),
		max(point1BlockPos.Z(), point2BlockPos.Z()),
	}

	startX := int(startBlockPos.X())
	startY := int(startBlockPos.Y())
	startZ := int(startBlockPos.Z())
	endX := int(endBlockPos.X())
	endY := int(endBlockPos.Y())
	endZ := int(endBlockPos.Z())

	minChunkX := floorDiv(startX, 16)
	maxChunkX := floorDiv(endX, 16)
	minChunkZ := floorDiv(startZ, 16)
	maxChunkZ := floorDiv(endZ, 16)
	minSubChunkY := floorDiv(startY, 16)
	maxSubChunkY := floorDiv(endY, 16)

	chunkXCount := maxChunkX - minChunkX + 1
	chunkZCount := maxChunkZ - minChunkZ + 1
	subChunkYCount := maxSubChunkY - minSubChunkY + 1
	totalSubChunks := chunkXCount * chunkZCount * subChunkYCount
	if startCallback != nil {
		startCallback(totalSubChunks)
	}

	blocksList := []string{"minecraft:air"}
	paletteIndex := map[string]int{"minecraft:air": 0}
	resolvedCache := make(map[uint32]fuHongV4ResolvedPalette)

	if _, err := io.WriteString(w, `{"Build_Info":{},"FuHongBuild":[`); err != nil {
		return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
	}
	_ = fuHongMaybeFlush(w)

	wroteAnyChunk := false

	for cz := minChunkZ; cz <= maxChunkZ; cz++ {
		chunkWorldZStart := cz * 16
		chunkWorldZEnd := chunkWorldZStart + 15
		effectiveWorldZStart := max(chunkWorldZStart, startZ)
		effectiveWorldZEnd := min(chunkWorldZEnd, endZ)

		for cx := minChunkX; cx <= maxChunkX; cx++ {
			chunkWorldXStart := cx * 16
			chunkWorldXEnd := chunkWorldXStart + 15
			effectiveWorldXStart := max(chunkWorldXStart, startX)
			effectiveWorldXEnd := min(chunkWorldXEnd, endX)

			chunkPos := bwo_define.ChunkPos{int32(cx), int32(cz)}
			nbtByWorldPos, err := fuHongLoadChunkNBT(bw, chunkPos, startX, startY, startZ, endX, endY, endZ)
			if err != nil {
				return err
			}

			groups := make(map[fuHongV4ExportKey]*fuHongV4ExportGroup)

			for subY := minSubChunkY; subY <= maxSubChunkY; subY++ {
				subChunkWorldYStart := subY * 16
				subChunkWorldYEnd := subChunkWorldYStart + 15
				effectiveWorldYStart := max(subChunkWorldYStart, startY)
				effectiveWorldYEnd := min(subChunkWorldYEnd, endY)

				if effectiveWorldXStart <= effectiveWorldXEnd && effectiveWorldZStart <= effectiveWorldZEnd && effectiveWorldYStart <= effectiveWorldYEnd {
					worldSubChunkPos := bwo_define.SubChunkPos{int32(cx), int32(subY), int32(cz)}
					subChunk := bw.LoadSubChunk(bwo_define.DimensionIDOverworld, worldSubChunkPos)
					if subChunk != nil {
						for wy := effectiveWorldYStart; wy <= effectiveWorldYEnd; wy++ {
							localY := byte(wy - subChunkWorldYStart)
							for wz := effectiveWorldZStart; wz <= effectiveWorldZEnd; wz++ {
								localZ := byte(wz - chunkWorldZStart)
								for wx := effectiveWorldXStart; wx <= effectiveWorldXEnd; wx++ {
									localX := byte(wx - chunkWorldXStart)
									runtimeID := subChunk.Block(localX, localY, localZ, 0)
									if runtimeID == block.AirRuntimeID {
										continue
									}

									resolved, ok := resolvedCache[runtimeID]
									if !ok {
										paletteName, aux := f.resolveFuHongPalette(runtimeID)
										resolved = fuHongV4ResolvedPalette{paletteName: paletteName, aux: aux}
										resolvedCache[runtimeID] = resolved
									}

									if _, ok := paletteIndex[resolved.paletteName]; !ok {
										idx := len(blocksList)
										blocksList = append(blocksList, resolved.paletteName)
										paletteIndex[resolved.paletteName] = idx
									}

									groupKey := fuHongV4ExportKey{Palette: resolved.paletteName, Aux: resolved.aux}
									group, ok := groups[groupKey]
									if !ok {
										group = &fuHongV4ExportGroup{
											palette: resolved.paletteName,
											aux:     resolved.aux,
										}
										groups[groupKey] = group
									}

									localExportX := wx - startX
									localExportY := wy - startY
									localExportZ := wz - startZ
									group.xs = append(group.xs, localExportX)
									group.ys = append(group.ys, localExportY)
									group.zs = append(group.zs, localExportZ)

									if fuHongNeedsExtras(resolved.paletteName) {
										extra := f.buildFuHongExtraPayload(resolved.paletteName, nbtByWorldPos[define.BlockPos{int32(wx), int32(wy), int32(wz)}])
										group.extras = append(group.extras, extra)
										group.hasExtras = true
									}
								}
							}
						}
					}
				}

				if progressCallback != nil {
					progressCallback()
				}
			}

			if len(groups) == 0 {
				continue
			}

			if wroteAnyChunk {
				if _, err := io.WriteString(w, ","); err != nil {
					return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
				}
			}

			localChunkBaseX := chunkWorldXStart - startX
			localChunkBaseZ := chunkWorldZStart - startZ

			if _, err := io.WriteString(w, fmt.Sprintf(`{"startX":%d,"startZ":%d,"block":[`, localChunkBaseX, localChunkBaseZ)); err != nil {
				return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
			}

			groupKeys := make([]fuHongV4ExportKey, 0, len(groups))
			for k := range groups {
				groupKeys = append(groupKeys, k)
			}
			sort.Slice(groupKeys, func(i, j int) bool {
				if groupKeys[i].Palette != groupKeys[j].Palette {
					return groupKeys[i].Palette < groupKeys[j].Palette
				}
				return groupKeys[i].Aux < groupKeys[j].Aux
			})

			for idx, gk := range groupKeys {
				if idx > 0 {
					if _, err := io.WriteString(w, ","); err != nil {
						return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
					}
				}
				g := groups[gk]
				pIdx := paletteIndex[g.palette]
				entry := []any{pIdx, g.aux, g.xs, g.ys, g.zs}
				if g.hasExtras {
					entry = append(entry, g.extras)
				}
				b, err := json.Marshal(entry)
				if err != nil {
					return fmt.Errorf("序列化 FuHong V4 block 条目失败: %w", err)
				}
				if _, err := w.Write(b); err != nil {
					return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
				}
			}

			if _, err := io.WriteString(w, `]}`); err != nil {
				return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
			}
			wroteAnyChunk = true
			_ = fuHongMaybeFlush(w)
		}
	}

	if !wroteAnyChunk {
		return fmt.Errorf("未导出任何方块")
	}

	if _, err := io.WriteString(w, `],"BlocksList":`); err != nil {
		return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
	}
	bl, err := json.Marshal(blocksList)
	if err != nil {
		return fmt.Errorf("序列化 FuHong V4 BlocksList 失败: %w", err)
	}
	if _, err := w.Write(bl); err != nil {
		return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
	}
	if _, err := io.WriteString(w, `,"TimeUsed":"0ms"}`); err != nil {
		return fmt.Errorf("写入 FuHong V4 JSON 失败: %w", err)
	}
	_ = fuHongMaybeFlush(w)

	return nil
}

type fuHongFlusher interface {
	Flush() error
}

func fuHongMaybeFlush(w io.Writer) error {
	if f, ok := w.(fuHongFlusher); ok {
		return f.Flush()
	}
	return nil
}

func fuHongNeedsExtras(blockName string) bool {
	lowerName := strings.ToLower(blockName)
	if strings.Contains(lowerName, "command_block") {
		return true
	}
	if strings.Contains(lowerName, "sign") {
		return true
	}
	return chestBlockNameToID(strings.TrimSpace(blockName)) != ""
}

func fuHongLoadChunkNBT(
	bw *world.BedrockWorld,
	chunkPos bwo_define.ChunkPos,
	startX, startY, startZ int,
	endX, endY, endZ int,
) (map[define.BlockPos]map[string]any, error) {
	nbts, err := bw.LoadNBT(bwo_define.DimensionIDOverworld, chunkPos)
	if err != nil {
		return nil, fmt.Errorf("读取区块 NBT 失败 (%v): %w", chunkPos, err)
	}

	out := make(map[define.BlockPos]map[string]any, len(nbts))
	for _, nbt := range nbts {
		xv, okX := asInt32(nbt["x"])
		yv, okY := asInt32(nbt["y"])
		zv, okZ := asInt32(nbt["z"])
		if !okX || !okY || !okZ {
			continue
		}
		x := int(xv)
		y := int(yv)
		z := int(zv)
		if x < startX || x > endX || y < startY || y > endY || z < startZ || z > endZ {
			continue
		}
		out[define.BlockPos{xv, yv, zv}] = nbt
	}
	return out, nil
}

func (f *FuHongV4) resolveFuHongPalette(runtimeID uint32) (paletteName string, aux int) {
	name, properties, found := block.RuntimeIDToState(runtimeID)
	if !found || strings.TrimSpace(name) == "" {
		return "minecraft:unknown", 0
	}
	name = strings.TrimSpace(name)
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}

	// 优先尝试用 legacy aux 还原（FuHong 传统用法）。
	for candidateAux := 0; candidateAux <= 255; candidateAux++ {
		if f.runtimeIDFor(name, candidateAux) == runtimeID {
			return name, candidateAux
		}
	}

	// fallback：把 states 写进 BlocksList，使读取端走 BlockStrToRuntimeID 精确匹配。
	if len(properties) == 0 {
		return name, 0
	}
	return name + "[" + formatFuHongStates(properties) + "]", 0
}

func formatFuHongStates(properties map[string]any) string {
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(formatFuHongStateValue(properties[k]))
	}
	return b.String()
}

func formatFuHongStateValue(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case uint8:
		return strconv.FormatInt(int64(t), 10)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case uint16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint32:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

func (f *FuHongV4) buildFuHongExtraPayload(blockName string, nbt map[string]any) any {
	lowerName := strings.ToLower(blockName)
	if strings.Contains(lowerName, "command_block") {
		if nbt == nil {
			return []any{"", 0, 0, ""}
		}
		return buildFuHongCommandPayload(nbt)
	}

	if containerID := chestBlockNameToID(strings.TrimSpace(blockName)); containerID != "" {
		if nbt == nil {
			return []any{}
		}
		return buildFuHongContainerPayload(nbt)
	}

	if strings.Contains(lowerName, "sign") {
		if nbt == nil {
			return ""
		}
		if front, ok := nbt["FrontText"].(map[string]any); ok {
			if text, ok := front["Text"].(string); ok {
				return text
			}
		}
		if text, ok := nbt["Text"].(string); ok {
			return text
		}
	}

	return nil
}

func buildFuHongCommandPayload(nbt map[string]any) []any {
	cmd, _ := nbt["Command"].(string)

	delay := 0
	switch v := nbt["TickDelay"].(type) {
	case int32:
		delay = int(v)
	case int64:
		delay = int(v)
	case int:
		delay = v
	case uint8:
		delay = int(v)
	case uint16:
		delay = int(v)
	case uint32:
		delay = int(v)
	}

	auto := false
	switch v := nbt["auto"].(type) {
	case uint8:
		auto = v != 0
	case int8:
		auto = v != 0
	case int32:
		auto = v != 0
	case int:
		auto = v != 0
	}

	custom, _ := nbt["CustomName"].(string)
	autoInt := 0
	if auto {
		autoInt = 1
	}
	return []any{cmd, delay, autoInt, custom}
}

func buildFuHongContainerPayload(nbt map[string]any) []any {
	rawItems, ok := nbt["Items"]
	if !ok || rawItems == nil {
		return []any{}
	}

	itemsSlice, ok := rawItems.([]any)
	if !ok {
		if typed, ok := rawItems.([]map[string]any); ok {
			itemsSlice = make([]any, 0, len(typed))
			for _, it := range typed {
				itemsSlice = append(itemsSlice, it)
			}
		} else {
			return []any{}
		}
	}

	out := make([]any, 0, len(itemsSlice))
	for _, raw := range itemsSlice {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := itemMap["Name"].(string)
		if name == "" {
			continue
		}
		count, _ := toInt(itemMap["Count"])
		aux, _ := toInt(itemMap["Damage"])
		slot, _ := toInt(itemMap["Slot"])

		out = append(out, []any{name, aux, count, slot})
	}
	return out
}

func asInt32(v any) (int32, bool) {
	switch t := v.(type) {
	case int32:
		return t, true
	case int:
		return int32(t), true
	case int64:
		return int32(t), true
	case float64:
		return int32(t), true
	case uint32:
		return int32(t), true
	case uint8:
		return int32(t), true
	case int16:
		return int32(t), true
	default:
		return 0, false
	}
}
