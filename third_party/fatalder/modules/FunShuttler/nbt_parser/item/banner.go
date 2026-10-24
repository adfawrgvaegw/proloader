package nbt_parser_item

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/mapping"
	nbt_parser_general "github.com/Yeah114/FunShuttler/nbt_parser/general"

	"github.com/mitchellh/mapstructure"
)

// BannerNBT ..
type BannerNBT struct {
	Patterns []nbt_parser_general.BannerPattern
	Type     int32
}

// 旗帜
type Banner struct {
	DefaultItem
	NBT BannerNBT
}

func (b Banner) formatNBT(prefix string) string {
	result := prefix + fmt.Sprintf("旗帜基色: %s\n", mapping.ColorFormat[int32(b.ItemMetadata())])

	if b.NBT.Type == nbt_parser_general.BannerTypeOminous {
		result += prefix + "旗帜类型: 灾厄\n"
	} else {
		result += prefix + "旗帜类型: 普通\n"
	}

	if patternCount := len(b.NBT.Patterns); patternCount > 0 {
		result += prefix + fmt.Sprintf("旗帜图案 (合计 %d 个图案): \n", patternCount)
	}
	for _, pattern := range b.NBT.Patterns {
		result += pattern.Format(prefix + "\t- ")
	}

	return result
}

func (b *Banner) Format(prefix string) string {
	result := b.DefaultItem.Format(prefix)
	if b.IsComplex() {
		result += prefix + "附加数据: \n"
		result += b.formatNBT(prefix + "\t")
	}
	return result
}

// parse ..
func (b *Banner) parse(tag map[string]any) error {
	b.DefaultItem.Enhance.ItemComponent.LockInInventory = false
	b.DefaultItem.Enhance.ItemComponent.LockInSlot = false
	b.DefaultItem.Enhance.EnchList = nil
	b.DefaultItem.Block = ItemBlockData{}

	if len(tag) == 0 {
		return nil
	}

	patterns, _ := tag["Patterns"].([]any)
	if len(patterns) > 6 {
		patterns = patterns[0:6]
	}

	for _, value := range patterns {
		var pattern nbt_parser_general.BannerPattern

		val, ok := value.(map[string]any)
		if !ok {
			continue
		}

		err := mapstructure.Decode(&val, &pattern)
		if err != nil {
			return fmt.Errorf("parse: %v", err)
		}

		if mapping.BannerPatternUnsupported[pattern.Pattern] {
			continue
		}

		b.NBT.Patterns = append(b.NBT.Patterns, pattern)
	}

	b.NBT.Type, _ = tag["Type"].(int32)
	if b.NBT.Type == nbt_parser_general.BannerTypeOminous {
		b.NBT.Patterns = nil
	}

	return nil
}

func (b *Banner) ParseNormal(nbtMap map[string]any) error {
	tag, _ := nbtMap["tag"].(map[string]any)
	err := b.parse(tag)
	if err != nil {
		return fmt.Errorf("ParseNormal: %v", err)
	}
	return nil
}

func (b *Banner) ParseNetwork(item protocol.ItemStack, itemName string) error {
	err := b.parse(item.NBTData)
	if err != nil {
		return fmt.Errorf("ParseNetwork: %v", err)
	}
	return nil
}

func (b Banner) IsComplex() bool {
	if len(b.NBT.Patterns) > 0 {
		return true
	}
	if b.NBT.Type == nbt_parser_general.BannerTypeOminous {
		return true
	}
	return false
}

func (b Banner) complexFieldsOnly() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	protocol.SliceUint16Length(w, &b.NBT.Patterns)
	w.Varint32(&b.NBT.Type)

	return buf.Bytes()
}

func (b *Banner) NBTStableBytes() []byte {
	return append(b.DefaultItem.NBTStableBytes(), b.complexFieldsOnly()...)
}

func (b *Banner) TypeStableBytes() []byte {
	return append(b.DefaultItem.TypeStableBytes(), b.complexFieldsOnly()...)
}

func (b *Banner) FullStableBytes() []byte {
	return append(b.TypeStableBytes(), b.Basic.Count)
}
