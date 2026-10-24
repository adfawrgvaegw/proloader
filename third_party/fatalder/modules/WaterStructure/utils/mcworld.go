package utils

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
)

// BlockPos ..
type BlockPos [3]int32

// MCWorld ..
type MCWorld struct {
	mu *sync.Mutex

	givenCtx          context.Context
	internalCtx       context.Context
	internalCtxCancel context.CancelFunc
	closer            *sync.Once

	gameSaves    *world.BedrockWorld
	cachedChunks map[define.ChunkPos]*chunk.Chunk
	cachedNBTs   map[define.ChunkPos]map[BlockPos]map[string]any
}

// NewMCWorld ..
func NewMCWorld(bedrockWorld *world.BedrockWorld, ctx context.Context) (result *MCWorld, err error) {
	internalCtx, internalCtxCancel := context.WithCancel(context.Background())
	result = &MCWorld{
		mu:                new(sync.Mutex),
		givenCtx:          ctx,
		internalCtx:       internalCtx,
		internalCtxCancel: internalCtxCancel,
		closer:            new(sync.Once),
		gameSaves:         bedrockWorld,
		cachedChunks:      make(map[define.ChunkPos]*chunk.Chunk),
		cachedNBTs:        make(map[define.ChunkPos]map[BlockPos]map[string]any),
	}

	return result, nil
}

func (m *MCWorld) AutoFlush(d time.Duration) {
	go func() {
		ticker := time.NewTicker(d)
		defer func() {
			ticker.Stop()
		}()
		for {
			select {
			case <-ticker.C:
			case <-m.givenCtx.Done():
				return
			case <-m.internalCtx.Done():
				return
			}
			m.Flush()
		}
	}()
}

// flush ..
func (m *MCWorld) flush() {
	for cp, data := range m.cachedChunks {
		_ = m.gameSaves.SaveChunk(
			define.DimensionIDOverworld,
			cp, data,
		)
		m.cachedChunks[cp] = nil
	}
	m.cachedChunks = make(map[define.ChunkPos]*chunk.Chunk)

	for cp, data := range m.cachedNBTs {
		nbts := make([]map[string]any, 0)
		for _, value := range data {
			nbts = append(nbts, value)
		}
		_ = m.gameSaves.SaveNBT(
			define.DimensionIDOverworld,
			cp, nbts,
		)
		m.cachedNBTs[cp] = nil
	}
	m.cachedNBTs = make(map[define.ChunkPos]map[BlockPos]map[string]any)
}

// Flush ..
func (m *MCWorld) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flush()
}

// Close ..
func (m *MCWorld) Close() (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closer.Do(func() {
		m.flush()
		if err = m.gameSaves.CloseWorld(); err == nil {
			m.internalCtxCancel()
		}
	})
	if err != nil {
		m.closer = new(sync.Once)
		return fmt.Errorf("关闭世界失败: %v", err)
	}

	return nil
}

// LoadBlock ..
func (m *MCWorld) LoadBlock(x int32, y int16, z int32) (blockRuntimeID uint32, err error) {
	var exists bool

	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	c, ok := m.cachedChunks[chunkPos]
	if !ok {
		c, exists, err = m.gameSaves.LoadChunk(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return 0, fmt.Errorf("加载方块失败: %v", err)
		}
		if !exists {
			return block.AirRuntimeID, nil
		}
	}

	x -= chunkPos[0] << 4
	z -= chunkPos[1] << 4

	return c.Block(uint8(x), y, uint8(z), 0), nil
}

// SetBlock ..
func (m *MCWorld) SetBlock(x int32, y int16, z int32, blockRuntimeID uint32) error {
	var exists bool
	var err error

	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	c, ok := m.cachedChunks[chunkPos]
	if !ok {
		c, exists, err = m.gameSaves.LoadChunk(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return fmt.Errorf("设置方块失败: %v", err)
		}
		if !exists {
			c = chunk.NewChunk(block.AirRuntimeID, define.Dimension(define.DimensionIDOverworld).Range())
		}
		m.cachedChunks[chunkPos] = c
	}

	x -= chunkPos[0] << 4
	z -= chunkPos[1] << 4
	c.SetBlock(uint8(x), y, uint8(z), 0, blockRuntimeID)

	return nil
}

// SetBlockNBT ..
func (m *MCWorld) SetBlockNBT(x int32, y int32, z int32, blockNBT map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	chunkPos := define.ChunkPos{x >> 4, z >> 4}
	nbtMap, ok := m.cachedNBTs[chunkPos]
	if !ok {
		nbtMap = make(map[BlockPos]map[string]any)

		nbts, err := m.gameSaves.LoadNBT(define.DimensionIDOverworld, chunkPos)
		if err != nil {
			return fmt.Errorf("设置方块 NBT 失败: %v", err)
		}

		for _, value := range nbts {
			posX, ok := value["x"].(int32)
			if !ok {
				continue
			}
			posY, ok := value["y"].(int32)
			if !ok {
				continue
			}
			posZ, ok := value["z"].(int32)
			if !ok {
				continue
			}
			nbtMap[BlockPos{posX, posY, posZ}] = value
		}
	}

	nbtMap[BlockPos{x, int32(y), z}] = blockNBT
	m.cachedNBTs[chunkPos] = nbtMap

	return nil
}
