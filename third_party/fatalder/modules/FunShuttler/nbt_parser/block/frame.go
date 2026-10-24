package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
)

// FrameNBT ..
type FrameNBT struct {
	ItemRotation float32
	HaveItem     bool
	Item         nbt_parser_interface.Item
}

// 物品展示框
type Frame struct {
	DefaultBlock
	NBT FrameNBT
}

func (f Frame) NeedSpecialHandle() bool {
	return f.NBT.HaveItem
}

func (f Frame) NeedCheckCompletely() bool {
	return true
}

func (f Frame) formatNBT(prefix string) string {
	result := prefix + fmt.Sprintf("旋转角度: %v 度\n", f.NBT.ItemRotation)
	result += prefix + "物品数据: \n"
	result += f.NBT.Item.Format(prefix + "\t")
	return result
}

func (f *Frame) Format(prefix string) string {
	result := f.DefaultBlock.Format(prefix)
	if f.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += f.formatNBT(prefix + "\t")
	}
	return result
}

func (f *Frame) Parse(nbtMap map[string]any) error {
	f.NBT.ItemRotation, _ = nbtMap["ItemRotation"].(float32)

	itemMap, ok := nbtMap["Item"].(map[string]any)
	if ok {
		item, canGetByCommand, err := nbt_parser_interface.ParseItemNormal(f.NameChecker, itemMap)
		if err != nil {
			return fmt.Errorf("Parse: %v", err)
		}
		if canGetByCommand && item.ItemName() != "minecraft:filled_map" {
			f.NBT.HaveItem = true
			f.NBT.Item = item
		}
	}

	return nil
}

func (f Frame) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	w.Bool(&f.NBT.HaveItem)
	if f.NBT.HaveItem {
		itemStableBytes := f.NBT.Item.TypeStableBytes()
		w.Float32(&f.NBT.ItemRotation)
		w.ByteSlice(&itemStableBytes)
	}

	return buf.Bytes()
}

func (f *Frame) FullStableBytes() []byte {
	return append(f.DefaultBlock.FullStableBytes(), f.NBTStableBytes()...)
}
