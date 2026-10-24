package nbt_block

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_assigner_interface "github.com/Yeah114/FunShuttler/nbt_assigner/interface"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
	nbt_hash "github.com/Yeah114/FunShuttler/nbt_parser/hash"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"

	"github.com/google/uuid"
)

// 在放置 NBT 方块以后会进行完整性检查，
// 如果完整性检查不提供，那么这个 NBT
// 方块会被重复制作。
//
// MaxRetryPlaceNBTBlock 便是指示重复
// 制作这个 NBT 方块的最大次数
const MaxRetryPlaceNBTBlock = 7

func init() {
	nbt_assigner_interface.NBTBlockIsSupported = NBTBlockIsSupported
	nbt_assigner_interface.PlaceNBTBlock = PlaceNBTBlock
}

// NBTBlockIsSupported 检查 block 是否是受支持的 NBT 方块
func NBTBlockIsSupported(block nbt_parser_interface.Block) bool {
	switch block.(type) {
	case *nbt_parser_block.CommandBlock:
	case *nbt_parser_block.Sign:
	case *nbt_parser_block.StructureBlock:
	case *nbt_parser_block.Container:
	case *nbt_parser_block.Banner:
	case *nbt_parser_block.Frame:
	case *nbt_parser_block.Lectern:
	case *nbt_parser_block.JukeBox:
	case *nbt_parser_block.BrewingStand:
	default:
		return false
	}
	return true
}

// PlaceNBTBlock 根据传入的操作台和缓存命中系统，
// 在操作台的中心方块处制作一个 NBT 方块 nbtBlock。
//
// canFast 指示目标方块是否可以直接通过 setblock 放置。
//
// 如果不能通过 setblock 放置，那么 uniqueID 指示目标
// 方块所在结构的唯一标识，并且 offset 指示其相邻的可能
// 的方块，例如床的尾方块相对于头方块的偏移
func PlaceNBTBlock(
	console *nbt_console.Console,
	cache *nbt_cache.NBTCacheSystem,
	nbtBlock nbt_parser_interface.Block,
) (
	canFast bool,
	uniqueID uuid.UUID,
	offset protocol.BlockPos,
	err error,
) {
	return placeNBTBlock(console, cache, nbtBlock, 0)
}

// placeNBTBlock ..
func placeNBTBlock(
	console *nbt_console.Console,
	cache *nbt_cache.NBTCacheSystem,
	nbtBlock nbt_parser_interface.Block,
	repeatCount uint8,
) (
	canFast bool,
	uniqueID uuid.UUID,
	offset protocol.BlockPos,
	err error,
) {
	// 初始化
	var method nbt_assigner_interface.Block
	hashNumber := nbt_hash.CompletelyHashNumber{
		HashNumber:    nbt_hash.NBTBlockFullHash(nbtBlock),
		SetHashNumber: nbt_hash.ContainerSetHash(nbtBlock),
	}

	// 检查是否可以快速放置
	if !nbtBlock.NeedSpecialHandle() {
		return true, uuid.UUID{}, protocol.BlockPos{}, nil
	}

	// 检查 NBT 缓存命中系统
	structure, hit, partHit := cache.NBTBlockCache().CheckCache(hashNumber)
	if hit && !partHit {
		return false, structure.UniqueID, structure.Offset, nil
	}

	// 取得相应 NBT 方块的放置方法
	switch block := nbtBlock.(type) {
	case *nbt_parser_block.CommandBlock:
		method = &CommandBlock{
			console: console,
			data:    *block,
		}
	case *nbt_parser_block.Sign:
		method = &Sign{
			console: console,
			data:    *block,
		}
	case *nbt_parser_block.StructureBlock:
		method = &StructrueBlock{
			console: console,
			data:    *block,
		}
	case *nbt_parser_block.Container:
		method = &Container{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.Banner:
		method = &Banner{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.Frame:
		method = &Frame{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.Lectern:
		method = &Lectern{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.JukeBox:
		method = &JukeBox{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.BrewingStand:
		method = &BrewingStand{
			console: console,
			cache:   cache,
			data:    *block,
		}
	case *nbt_parser_block.Crafter:
		method = &Crafter{
			console: console,
			cache:   cache,
			data:    *block,
		}
	}

	// 放置相应方块
	err = method.Make()
	if err != nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
	}

	// 检查完整性，如果需要的话
	if nbtBlock.NeedCheckCompletely() {
		nbtMap, err := simpleStructureGetter(console)
		if err != nil {
			return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
		}

		newBlock, err := nbt_parser_interface.ParseBlock(nil, nbtBlock.BlockName(), nbtBlock.BlockStates(), nbtMap)
		if err != nil {
			return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
		}

		if hashNumber.HashNumber != nbt_hash.NBTBlockFullHash(newBlock) {
			nextCount := repeatCount + 1
			if nextCount > MaxRetryPlaceNBTBlock {
				return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf(
					""+
						"PlaceNBTBlock: Self loop when place NBT block, "+
						"and result in invalid user input data, "+
						"and need to correct; "+
						"nbtBlock = %#v; newBlock = %#v; "+
						"nbtBlock.Format(\"\") = %#v; newBlock.Format(\"\") = %#v",
					nbtBlock, newBlock,
					nbtBlock.Format(""), newBlock.Format(""),
				)
			}
			return placeNBTBlock(console, cache, nbtBlock, nextCount)
		}
	}

	// 保存缓存
	err = cache.NBTBlockCache().StoreCache(nbtBlock, method.Offset())
	if err != nil {
		return false, uuid.UUID{}, protocol.BlockPos{}, fmt.Errorf("PlaceNBTBlock: %v", err)
	}

	// 下次调用时将直接返回缓存
	return placeNBTBlock(console, cache, nbtBlock, repeatCount)
}
