package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/utils"

	"github.com/mitchellh/mapstructure"
)

// CommandBlockNBT ..
type CommandBlockNBT struct {
	Command            string `mapstructure:"Command"`
	CustomName         string `mapstructure:"CustomName"`
	TickDelay          int32  `mapstructure:"TickDelay"`
	ExecuteOnFirstTick byte   `mapstructure:"ExecuteOnFirstTick"`
	TrackOutput        byte   `mapstructure:"TrackOutput"`
	ConditionalMode    byte   `mapstructure:"conditionalMode"`
	Auto               byte   `mapstructure:"auto"`
}

// 命令方块
type CommandBlock struct {
	DefaultBlock
	NBT CommandBlockNBT
}

func (c *CommandBlock) NeedSpecialHandle() bool {
	conditionalBit, _ := c.BlockStates()["conditional_bit"].(byte)

	if len(c.NBT.Command) > 0 || len(c.NBT.CustomName) > 0 {
		return true
	}
	if conditionalBit != c.NBT.ConditionalMode {
		return true
	}
	if c.NBT.TickDelay != 0 {
		return true
	}

	switch c.BlockName() {
	case "minecraft:repeating_command_block":
		if c.NBT.ExecuteOnFirstTick == 0 {
			return true
		}
	default:
		if c.NBT.ExecuteOnFirstTick == 1 {
			return true
		}
	}

	switch c.BlockName() {
	case "minecraft:chain_command_block":
		if c.NBT.Auto == 0 {
			return true
		}
	default:
		if c.NBT.Auto == 1 {
			return true
		}
	}

	return false
}

func (c CommandBlock) NeedCheckCompletely() bool {
	return false
}

func (c *CommandBlock) formatNBT(prefix string) string {
	result := ""

	if len(c.NBT.CustomName) > 0 {
		result += prefix + fmt.Sprintf("悬浮文本: %s\n", c.NBT.CustomName)
	}
	if len(c.NBT.Command) > 0 {
		result += prefix + fmt.Sprintf("控制台命令: %s\n", c.NBT.Command)
	}

	result += prefix + fmt.Sprintf("有条件的: %s\n", utils.FormatByte(c.NBT.ConditionalMode))
	result += prefix + fmt.Sprintf("需要红石: %s\n", utils.FormatBool(c.NBT.Auto == 0))
	result += prefix + fmt.Sprintf("已选项中的延迟: %d\n", c.NBT.TickDelay)
	result += prefix + fmt.Sprintf("执行第一个已选项: %s\n", utils.FormatByte(c.NBT.ExecuteOnFirstTick))

	return result
}

func (c *CommandBlock) Format(prefix string) string {
	result := c.DefaultBlock.Format(prefix)
	if c.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += c.formatNBT(prefix + "\t")
	}
	return result
}

func (c *CommandBlock) Parse(nbtMap map[string]any) error {
	var result CommandBlockNBT

	err := mapstructure.Decode(&nbtMap, &result)
	if err != nil {
		return fmt.Errorf("Parse: %v", err)
	}
	c.NBT = result

	conditionalBit, _ := c.BlockStates()["conditional_bit"].(byte)
	if conditionalBit == 1 {
		c.NBT.ConditionalMode = 1
	}

	return nil
}

func (c CommandBlock) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	w.String(&c.NBT.Command)
	w.String(&c.NBT.CustomName)
	w.Varint32(&c.NBT.TickDelay)
	w.Uint8(&c.NBT.ExecuteOnFirstTick)
	w.Uint8(&c.NBT.ConditionalMode)
	w.Uint8(&c.NBT.Auto)

	return buf.Bytes()
}

func (c *CommandBlock) FullStableBytes() []byte {
	return append(c.DefaultBlock.FullStableBytes(), c.NBTStableBytes()...)
}
