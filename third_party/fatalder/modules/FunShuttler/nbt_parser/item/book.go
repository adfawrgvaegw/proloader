package nbt_parser_item

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// BookNBT ..
type BookNBT struct {
	Pages  []string
	Author string
	Title  string
}

// 成书
type Book struct {
	DefaultItem
	NBT BookNBT
}

func (b Book) formatNBT(prefix string) string {
	result := ""

	if len(b.NBT.Title) > 0 {
		result += prefix + fmt.Sprintf("标题名: %s\n", b.NBT.Title)
	}
	if len(b.NBT.Author) > 0 {
		result += prefix + fmt.Sprintf("创造主: %s\n", b.NBT.Author)
	}
	result += prefix + fmt.Sprintf("页数: %d\n", len(b.NBT.Pages))

	return result
}

func (b *Book) Format(prefix string) string {
	result := b.DefaultItem.Format(prefix)
	if b.IsComplex() {
		result += prefix + "附加数据: \n"
		result += b.formatNBT(prefix + "\t")
	}
	return result
}

// parse ..
func (b *Book) parse(tag map[string]any) {
	b.DefaultItem.Basic.Metadata = 0
	b.DefaultItem.Enhance.ItemComponent.LockInInventory = false
	b.DefaultItem.Enhance.ItemComponent.LockInSlot = false
	b.DefaultItem.Enhance.EnchList = nil
	b.DefaultItem.Block = ItemBlockData{}

	if len(tag) == 0 {
		return
	}

	pages, _ := tag["pages"].([]any)
	for _, page := range pages {
		pageMap, ok := page.(map[string]any)
		if !ok {
			continue
		}
		content, ok := pageMap["text"].(string)
		if !ok {
			continue
		}
		b.NBT.Pages = append(b.NBT.Pages, content)
	}

	b.NBT.Author, _ = tag["author"].(string)
	b.NBT.Title, _ = tag["title"].(string)
}

func (b *Book) ParseNormal(nbtMap map[string]any) error {
	tag, _ := nbtMap["tag"].(map[string]any)
	b.parse(tag)
	return nil
}

func (b *Book) ParseNetwork(item protocol.ItemStack, itemName string) error {
	b.parse(item.NBTData)
	return nil
}

func (b *Book) IsComplex() bool {
	if b.ItemName() == "minecraft:written_book" {
		return true
	}
	if len(b.NBT.Author) > 0 {
		return true
	}
	if len(b.NBT.Pages) > 0 {
		return true
	}
	return false
}

func (b Book) complexFieldsOnly() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	protocol.FuncSliceUint16Length(w, &b.NBT.Pages, w.String)
	w.String(&b.NBT.Author)
	w.String(&b.NBT.Title)

	return buf.Bytes()
}

func (b *Book) NBTStableBytes() []byte {
	return append(b.DefaultItem.NBTStableBytes(), b.complexFieldsOnly()...)
}

func (b *Book) TypeStableBytes() []byte {
	return append(b.DefaultItem.TypeStableBytes(), b.complexFieldsOnly()...)
}

func (b *Book) FullStableBytes() []byte {
	return append(b.TypeStableBytes(), b.Basic.Count)
}
