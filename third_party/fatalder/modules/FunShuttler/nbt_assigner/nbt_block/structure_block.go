package nbt_block

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"

	"github.com/go-gl/mathgl/mgl32"
)

// 结构方块
type StructrueBlock struct {
	console *nbt_console.Console
	data    nbt_parser_block.StructureBlock
}

func (StructrueBlock) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

func (s *StructrueBlock) Make() error {
	api := s.console.API()

	err := s.console.API().SetBlock().SetBlock(s.console.Center(), s.data.BlockName(), s.data.BlockStatesString())
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	s.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
		KnownStates: true,
		Name:        s.data.BlockName(),
		States:      s.data.BlockStates(),
	})

	err = s.console.CanReachOrMove(s.console.Center())
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	err = api.Resources().WritePacket(&packet.StructureBlockUpdate{
		Position:           s.console.Center(),
		StructureName:      s.data.NBT.StructureName,
		DataField:          s.data.NBT.DataField,
		IncludePlayers:     s.data.NBT.IncludePlayers == 1,
		ShowBoundingBox:    s.data.NBT.ShowBoundingBox == 1,
		StructureBlockType: s.data.NBT.Data,
		Settings: protocol.StructureSettings{
			PaletteName:           "default",
			IgnoreEntities:        s.data.NBT.IgnoreEntities == 1,
			IgnoreBlocks:          s.data.NBT.RemoveBlocks == 1,
			AllowNonTickingChunks: true,
			Size: [3]int32{
				s.data.NBT.XStructureSize,
				s.data.NBT.YStructureSize,
				s.data.NBT.ZStructureSize,
			},
			Offset: [3]int32{
				s.data.NBT.XStructureOffset,
				s.data.NBT.YStructureOffset,
				s.data.NBT.ZStructureOffset,
			},
			LastEditingPlayerUniqueID: s.console.API().GetBotInfo().EntityUniqueID,
			Rotation:                  s.data.NBT.Rotation,
			Mirror:                    s.data.NBT.Mirror,
			AnimationMode:             s.data.NBT.AnimationMode,
			AnimationDuration:         s.data.NBT.AnimationSeconds,
			Integrity:                 s.data.NBT.Integrity,
			Seed:                      uint32(s.data.NBT.Seed),
			Pivot: mgl32.Vec3{
				(float32(s.data.NBT.XStructureSize) - 1) / 2,
				(float32(s.data.NBT.YStructureSize) - 1) / 2,
				(float32(s.data.NBT.ZStructureSize) - 1) / 2,
			},
		},
		RedstoneSaveMode: s.data.NBT.RedstoneSaveMode,
		ShouldTrigger:    false,
		Waterlogged:      false,
	})
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	err = api.Commands().AwaitChangesGeneral()
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	return nil
}
