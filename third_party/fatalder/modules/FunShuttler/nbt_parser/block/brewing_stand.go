package nbt_parser_block

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
)

// BrewingStandNBT ..
type BrewingStandNBT struct {
	Items      []ItemWithSlot
	CustomName string
}

// 酿造台
type BrewingStand struct {
	DefaultBlock
	NBT BrewingStandNBT
}

func (b *BrewingStand) NeedSpecialHandle() bool {
	if len(b.NBT.Items) > 0 {
		return true
	}
	if len(b.NBT.CustomName) > 0 {
		return true
	}
	return false
}

func (BrewingStand) NeedCheckCompletely() bool {
	return true
}

func (b BrewingStand) formatNBT(prefix string) string {
	result := ""

	if len(b.NBT.CustomName) > 0 {
		result += prefix + fmt.Sprintf("自定义名称: %s\n", b.NBT.CustomName)
	}

	if itemCount := len(b.NBT.Items); itemCount > 0 {
		result += prefix + fmt.Sprintf("共装有 %d 个物品: \n", itemCount)
	} else {
		result += prefix + "无物品\n"
	}

	for _, item := range b.NBT.Items {
		result += item.Format(prefix + "\t")
	}

	return result
}

func (b *BrewingStand) Format(prefix string) string {
	result := b.DefaultBlock.Format(prefix)
	if b.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += b.formatNBT(prefix + "\t")
	}
	return result
}

func (b *BrewingStand) Parse(nbtMap map[string]any) error {
	itemsMap, _ := nbtMap["Items"].([]any)
	blockStates := map[string]any{
		"brewing_stand_slot_a_bit": byte(0),
		"brewing_stand_slot_b_bit": byte(0),
		"brewing_stand_slot_c_bit": byte(0),
	}

	for _, value := range itemsMap {
		itemMap, ok := value.(map[string]any)
		if !ok {
			continue
		}

		item, canGetByCommand, err := nbt_parser_interface.ParseItemNormal(b.NameChecker, itemMap)
		if err != nil {
			return fmt.Errorf("Parse: %v", err)
		}
		if !canGetByCommand {
			continue
		}

		slot, _ := itemMap["Slot"].(byte)
		switch slot {
		case 1:
			blockStates["brewing_stand_slot_a_bit"] = byte(1)
		case 2:
			blockStates["brewing_stand_slot_b_bit"] = byte(1)
		case 3:
			blockStates["brewing_stand_slot_c_bit"] = byte(1)
		}

		b.NBT.Items = append(b.NBT.Items, ItemWithSlot{
			Item: item,
			Slot: slot,
		})
	}

	b.States = blockStates
	b.NBT.CustomName, _ = nbtMap["CustomName"].(string)
	return nil
}

func (b BrewingStand) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)
	w.String(&b.NBT.CustomName)

	itemMapping := make(map[uint8]ItemWithSlot)
	slots := make([]uint8, 0)
	for _, value := range b.NBT.Items {
		itemMapping[value.Slot] = value
		slots = append(slots, value.Slot)
	}

	slices.SortStableFunc(slots, func(a uint8, b uint8) int {
		return cmp.Compare(a, b)
	})

	for _, slot := range slots {
		item := itemMapping[slot]
		stableItemBytes := append(item.Item.FullStableBytes(), item.Slot)
		w.ByteSlice(&stableItemBytes)
	}

	return buf.Bytes()
}

func (b *BrewingStand) FullStableBytes() []byte {
	return append(b.DefaultBlock.FullStableBytes(), b.NBTStableBytes()...)
}
