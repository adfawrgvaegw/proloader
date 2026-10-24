package chunk_fill

import (
	"fmt"
	"math"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
)

const maxFillVolume = 32768

type blockInfo struct {
	name  string
	state string
	isAir bool
}

// GenerateChunkCommand emits a stream of fill/setblock commands that rebuild the
// provided chunk starting at startPos. Adjacent blocks with the same
// runtime ID are merged into fill commands subject to the 32k fill limit.
func GenerateChunkCommand(
	c *chunk.Chunk,
	startPos defines.BlockPos,
) <-chan string {
	return GenerateChunksCommand(map[defines.ChunkPos]*chunk.Chunk{
		{0, 0}: c,
	}, startPos)
}

// GenerateChunksCommand 会遍历提供的区块集合，按照它们的全局坐标生成能够跨区块合并的
// fill/setblock 命令流。该算法在单次 fill 命令的 32k 限制内尽可能扩大填充体积。
// startPos 起点。
func GenerateChunksCommand(chunks map[defines.ChunkPos]*chunk.Chunk, startPos defines.BlockPos) <-chan string {
	ch := make(chan string)

	go func() {
		defer close(ch)

		if len(chunks) == 0 {
			return
		}

		type chunkEntry struct {
			pos   defines.ChunkPos
			chunk *chunk.Chunk
		}

		entries := make([]chunkEntry, 0, len(chunks))
		for pos, c := range chunks {
			if c == nil {
				continue
			}
			subChunks := c.Sub()
			if len(subChunks) == 0 {
				continue
			}
			entries = append(entries, chunkEntry{pos: pos, chunk: c})
		}

		if len(entries) == 0 {
			return
		}

		minChunkX := entries[0].pos.X()
		maxChunkX := minChunkX
		minChunkZ := entries[0].pos.Z()
		maxChunkZ := minChunkZ
		minY := entries[0].chunk.Range().Min()
		maxY := entries[0].chunk.Range().Min() + len(entries[0].chunk.Sub())*16 - 1

		for _, entry := range entries[1:] {
			pos := entry.pos
			c := entry.chunk
			if pos.X() < minChunkX {
				minChunkX = pos.X()
			}
			if pos.X() > maxChunkX {
				maxChunkX = pos.X()
			}
			if pos.Z() < minChunkZ {
				minChunkZ = pos.Z()
			}
			if pos.Z() > maxChunkZ {
				maxChunkZ = pos.Z()
			}

			chunkMinY := c.Range().Min()
			chunkMaxY := chunkMinY + len(c.Sub())*16 - 1
			if chunkMinY < minY {
				minY = chunkMinY
			}
			if chunkMaxY > maxY {
				maxY = chunkMaxY
			}
		}

		sizeX := int(maxChunkX-minChunkX+1) * 16
		sizeZ := int(maxChunkZ-minChunkZ+1) * 16
		sizeY := maxY - minY + 1
		if sizeX <= 0 || sizeY <= 0 || sizeZ <= 0 {
			return
		}

		total := sizeX * sizeY * sizeZ
		blockIDs := make([]uint32, total)
		canSet := make([]bool, total)

		getIndex := func(x, y, z int) int {
			return ((x*sizeY)+y)*sizeZ + z
		}

		for _, entry := range entries {
			pos := entry.pos
			c := entry.chunk
			chunkMinY := c.Range().Min()
			subChunks := c.Sub()
			height := len(subChunks) * 16
			if height == 0 {
				continue
			}

			offsetX := int(pos.X()-minChunkX) * 16
			offsetZ := int(pos.Z()-minChunkZ) * 16
			offsetY := chunkMinY - minY

			for x := 0; x < 16; x++ {
				for y := 0; y < height; y++ {
					worldY := chunkMinY + y
					gy := offsetY + y
					if gy < 0 || gy >= sizeY {
						continue
					}
					for z := 0; z < 16; z++ {
						gx := offsetX + x
						gz := offsetZ + z
						if gx < 0 || gx >= sizeX || gz < 0 || gz >= sizeZ {
							continue
						}
						idx := getIndex(gx, gy, gz)
						blockIDs[idx] = c.Block(uint8(x), int16(worldY), uint8(z), 0)
						canSet[idx] = true
					}
				}
			}
		}

		cache := make(map[uint32]blockInfo)
		baseX := int(startPos.X())
		baseY := int(startPos.Y())
		baseZ := int(startPos.Z())

		ceilDiv := func(a, b int) int {
			if b <= 0 {
				return a
			}
			if a <= 0 {
				return 0
			}
			return (a + b - 1) / b
		}

		splitCuboid := func(l, h, w int) (int, int, int) {
			if l <= 0 {
				l = 1
			}
			if h <= 0 {
				h = 1
			}
			if w <= 0 {
				w = 1
			}

			a1 := int(math.Ceil(math.Sqrt(float64(maxFillVolume) / float64(h))))
			if a1 < 1 {
				a1 = 1
			}

			splitL := max(l/ceilDiv(l, a1), 1)
			splitW := max(w/ceilDiv(w, a1), 1)

			for {
				canSpreadX := false
				canSpreadZ := false

				if splitL < l && h*(splitL+1)*splitW <= maxFillVolume {
					splitL++
					canSpreadX = true
				}

				if splitW < w && h*splitL*(splitW+1) <= maxFillVolume {
					splitW++
					canSpreadZ = true
				}

				if !canSpreadX && !canSpreadZ {
					break
				}
			}

			return splitL, h, splitW
		}

		getBlockInfo := func(id uint32) blockInfo {
			if info, ok := cache[id]; ok {
				return info
			}

			if id == block.AirRuntimeID {
				info := blockInfo{name: "minecraft:air", state: "[]", isAir: true}
				cache[id] = info
				return info
			}

			name, properties, found := block.RuntimeIDToState(id)
			state := utils.PropertiesToStateStr(properties)
			if !found {
				name = "minecraft:air"
				state = "[]"
			}

			info := blockInfo{name: name, state: state, isAir: id == block.AirRuntimeID || name == "minecraft:air"}
			cache[id] = info
			return info
		}

		canFill := func(x, y, z int, id uint32) bool {
			if x < 0 || x >= sizeX || y < 0 || y >= sizeY || z < 0 || z >= sizeZ {
				return false
			}
			idx := getIndex(x, y, z)
			return canSet[idx] && blockIDs[idx] == id
		}

		sendFill := func(x1, y1, z1, x2, y2, z2 int, info blockInfo) {
			cmd := fmt.Sprintf(
				"fill %d %d %d %d %d %d %s %s\n",
				baseX+x1,
				baseY+y1,
				baseZ+z1,
				baseX+x2,
				baseY+y2,
				baseZ+z2,
				info.name,
				info.state,
			)
			ch <- cmd
		}

		sendSetBlock := func(x, y, z int, info blockInfo) {
			cmd := fmt.Sprintf(
				"setblock %d %d %d %s %s\n",
				baseX+x,
				baseY+y,
				baseZ+z,
				info.name,
				info.state,
			)
			ch <- cmd
		}

		for x := 0; x < sizeX; x++ {
			for y := 0; y < sizeY; y++ {
				for z := 0; z < sizeZ; z++ {
					idx := getIndex(x, y, z)
					if !canSet[idx] {
						continue
					}

					blockID := blockIDs[idx]
					info := getBlockInfo(blockID)
					if info.isAir {
						canSet[idx] = false
						continue
					}

					ex := x + 1
					for ex < sizeX && canFill(ex, y, z, blockID) {
						ex++
					}

					ez := z + 1
					for ez < sizeZ {
						allMatch := true
						for xx := x; xx < ex; xx++ {
							if !canFill(xx, y, ez, blockID) {
								allMatch = false
								break
							}
						}
						if !allMatch {
							break
						}
						ez++
					}

					ey := y + 1
					for ey < sizeY {
						allMatch := true
						for xx := x; xx < ex && allMatch; xx++ {
							for zz := z; zz < ez; zz++ {
								if !canFill(xx, ey, zz, blockID) {
									allMatch = false
									break
								}
							}
						}
						if !allMatch {
							break
						}
						ey++
					}

					for xx := x; xx < ex; xx++ {
						for yy := y; yy < ey; yy++ {
							for zz := z; zz < ez; zz++ {
								canSet[getIndex(xx, yy, zz)] = false
							}
						}
					}

					ex--
					ey--
					ez--

					if x == ex && y == ey && z == ez {
						sendSetBlock(x, y, z, info)
						continue
					}

					volume := (ex - x + 1) * (ey - y + 1) * (ez - z + 1)
					if volume <= maxFillVolume {
						sendFill(x, y, z, ex, ey, ez, info)
						continue
					}

					splitX, splitY, splitZ := splitCuboid(ex-x+1, ey-y+1, ez-z+1)
					if splitX <= 0 {
						splitX = ex - x + 1
					}
					if splitY <= 0 {
						splitY = ey - y + 1
					}
					if splitZ <= 0 {
						splitZ = ez - z + 1
					}

					for xx := x; xx <= ex; xx += splitX {
						maxX := xx + splitX - 1
						if maxX > ex {
							maxX = ex
						}
						for yy := y; yy <= ey; yy += splitY {
							maxY := yy + splitY - 1
							if maxY > ey {
								maxY = ey
							}
							for zz := z; zz <= ez; zz += splitZ {
								maxZ := zz + splitZ - 1
								if maxZ > ez {
									maxZ = ez
								}
								sendFill(xx, yy, zz, maxX, maxY, maxZ, info)
							}
						}
					}
				}
			}
		}
	}()

	return ch
}
