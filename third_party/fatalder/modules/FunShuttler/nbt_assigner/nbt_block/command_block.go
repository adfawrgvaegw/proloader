package nbt_block

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_parser_block "github.com/Yeah114/FunShuttler/nbt_parser/block"
)

// 命令方块
type CommandBlock struct {
	console *nbt_console.Console
	data    nbt_parser_block.CommandBlock
}

func (CommandBlock) Offset() protocol.BlockPos {
	return protocol.BlockPos{0, 0, 0}
}

func (c *CommandBlock) Make() error {
	var mode uint32 = packet.CommandBlockImpulse
	api := c.console.API()

	if c.data.BlockName() == "minecraft:chain_command_block" {
		mode = packet.CommandBlockChain
	}
	if c.data.BlockName() == "minecraft:repeating_command_block" {
		mode = packet.CommandBlockRepeating
	}

	err := c.console.API().SetBlock().SetBlock(c.console.Center(), c.data.BlockName(), c.data.BlockStatesString())
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}
	c.console.UseHelperBlock(nbt_console.RequesterUser, nbt_console.ConsoleIndexCenterBlock, block_helper.ComplexBlock{
		KnownStates: true,
		Name:        c.data.BlockName(),
		States:      c.data.BlockStates(),
	})

	err = c.console.CanReachOrMove(c.console.Center())
	if err != nil {
		return fmt.Errorf("Make: %v", err)
	}

	err = api.Resources().WritePacket(&packet.CommandBlockUpdate{
		Block:              true,
		Position:           c.console.Center(),
		Mode:               mode,
		NeedsRedstone:      c.data.NBT.Auto == 0,
		Conditional:        c.data.NBT.ConditionalMode == 1,
		Command:            c.data.NBT.Command,
		Name:               c.data.NBT.CustomName,
		ShouldTrackOutput:  c.data.NBT.TrackOutput == 1,
		TickDelay:          c.data.NBT.TickDelay,
		ExecuteOnFirstTick: c.data.NBT.ExecuteOnFirstTick == 1,
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
