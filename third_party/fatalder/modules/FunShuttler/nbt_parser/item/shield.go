package nbt_parser_item

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/mapping"
	nbt_parser_general "github.com/Yeah114/FunShuttler/nbt_parser/general"

	"github.com/mitchellh/mapstructure"
)

// ShieldNBT ..
type ShieldNBT struct {
	HaveBase bool
	Base     int32
	Patterns []nbt_parser_general.BannerPattern
}

// 盾牌
type Shield struct {
	DefaultItem
	NBT ShieldNBT
}

func (s Shield) formatNBT(prefix string) string {
	result := prefix + fmt.Sprintf("盾牌基色: %s\n", mapping.ColorFormat[s.NBT.Base])

	if patternCount := len(s.NBT.Patterns); patternCount > 0 {
		result += prefix + fmt.Sprintf("旗帜图案 (合计 %d 个图案): \n", patternCount)
	}
	for _, pattern := range s.NBT.Patterns {
		result += pattern.Format(prefix + "\t- ")
	}

	return result
}

func (s *Shield) Format(prefix string) string {
	result := s.DefaultItem.Format(prefix)
	if s.IsComplex() {
		result += prefix + "附加数据: \n"
		result += s.formatNBT(prefix + "\t")
	}
	return result
}

// parse ..
func (s *Shield) parse(tag map[string]any) error {
	var isOminousShield bool

	s.DefaultItem.Enhance.ItemComponent.LockInInventory = false
	s.DefaultItem.Enhance.ItemComponent.LockInSlot = false
	s.DefaultItem.Block = ItemBlockData{}

	if len(tag) == 0 {
		return nil
	}

	patterns, _ := tag["Patterns"].([]any)
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
		if pattern.Pattern == mapping.BannerPatternOminous {
			isOminousShield = true
			s.NBT.Patterns = []nbt_parser_general.BannerPattern{
				pattern,
			}
			break
		}

		s.NBT.Patterns = append(s.NBT.Patterns, pattern)
	}

	s.NBT.Base, s.NBT.HaveBase = tag["Base"].(int32)
	if len(s.NBT.Patterns) > 0 {
		s.NBT.HaveBase = true
	}

	if isOminousShield {
		s.DefaultItem.Basic.Metadata = 0
		s.DefaultItem.Enhance.ItemComponent.KeepOnDeath = false
		s.NBT.Base = 15
	}

	return nil
}

func (s *Shield) ParseNormal(nbtMap map[string]any) error {
	tag, _ := nbtMap["tag"].(map[string]any)
	err := s.parse(tag)
	if err != nil {
		return fmt.Errorf("ParseNormal: %v", err)
	}
	return nil
}

func (s *Shield) ParseNetwork(item protocol.ItemStack, itemName string) error {
	err := s.parse(item.NBTData)
	if err != nil {
		return fmt.Errorf("ParseNetwork: %v", err)
	}
	return nil
}

func (s Shield) IsComplex() bool {
	if len(s.NBT.Patterns) > 0 {
		return true
	}
	if s.NBT.HaveBase {
		return true
	}
	return false
}

func (s Shield) complexFieldsOnly() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	protocol.SliceUint16Length(w, &s.NBT.Patterns)
	w.Bool(&s.NBT.HaveBase)
	w.Varint32(&s.NBT.Base)

	return buf.Bytes()
}

func (s *Shield) NBTStableBytes() []byte {
	return append(s.DefaultItem.NBTStableBytes(), s.complexFieldsOnly()...)
}

func (s *Shield) TypeStableBytes() []byte {
	return append(s.DefaultItem.TypeStableBytes(), s.complexFieldsOnly()...)
}

func (s *Shield) FullStableBytes() []byte {
	return append(s.TypeStableBytes(), s.Basic.Count)
}
