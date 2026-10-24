package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// CratferNBT ..
type CratferNBT struct {
	ContainerInfo ContainerNBT
	DisabledSlots int16
}

// 合成器
type Crafter struct {
	DefaultBlock
	NBT CratferNBT
}

func (c *Crafter) AsContainer() *Container {
	return &Container{
		DefaultBlock: c.DefaultBlock,
		NBT:          c.NBT.ContainerInfo,
	}
}

func (c *Crafter) NeedSpecialHandle() bool {
	if c.NBT.DisabledSlots != 0 {
		return true
	}
	if c.AsContainer().NeedSpecialHandle() {
		return true
	}
	return false
}

func (c Crafter) NeedCheckCompletely() bool {
	return true
}

func (c Crafter) formatNBT(prefix string) string {
	result := ""

	if c.NBT.DisabledSlots != 0 {
		disableSlots := make([]int, 0)
		for index := range 9 {
			if c.NBT.DisabledSlots&int16(1<<index) != 0 {
				disableSlots = append(disableSlots, index+1)
			}
		}
		result += prefix + fmt.Sprintf("禁用的物品栏: %v\n", disableSlots)
	}

	result += c.AsContainer().formatNBT(prefix)
	return result
}

func (c *Crafter) Format(prefix string) string {
	result := c.DefaultBlock.Format(prefix)
	if c.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += c.formatNBT(prefix + "\t")
	}
	return result
}

func (c *Crafter) Parse(nbtMap map[string]any) error {
	container := c.AsContainer()
	err := container.Parse(nbtMap)
	if err != nil {
		return fmt.Errorf("Parse: %v", err)
	}

	c.NBT.DisabledSlots, _ = nbtMap["disabled_slots"].(int16)
	c.NBT.ContainerInfo = container.NBT

	return nil
}

func (c *Crafter) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)
	stableBytes := c.AsContainer().NBTStableBytes()

	w.Int16(&c.NBT.DisabledSlots)
	w.ByteSlice(&stableBytes)

	return buf.Bytes()
}

func (c *Crafter) FullStableBytes() []byte {
	return append(c.DefaultBlock.FullStableBytes(), c.NBTStableBytes()...)
}
