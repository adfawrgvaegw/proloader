package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
)

// LecternNBT ..
type LecternNBT struct {
	CustomName string
	HaveBook   bool
	Book       nbt_parser_interface.Item
}

// 讲台
type Lectern struct {
	DefaultBlock
	NBT LecternNBT
}

func (l Lectern) NeedSpecialHandle() bool {
	if len(l.NBT.CustomName) > 0 {
		return true
	}
	if l.NBT.HaveBook {
		return true
	}
	return false
}

func (Lectern) NeedCheckCompletely() bool {
	return true
}

func (l Lectern) formatNBT(prefix string) string {
	result := ""

	if len(l.NBT.CustomName) > 0 {
		result += prefix + fmt.Sprintf("自定义名称: %s\n", l.NBT.CustomName)
	}
	if l.NBT.HaveBook {
		result += prefix + "书籍数据: \n"
		result += l.NBT.Book.Format(prefix + "\t")
	}

	return result
}

func (l *Lectern) Format(prefix string) string {
	result := l.DefaultBlock.Format(prefix)
	if l.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += l.formatNBT(prefix + "\t")
	}
	return result
}

func (l *Lectern) Parse(nbtMap map[string]any) error {
	l.NBT.CustomName, _ = nbtMap["CustomName"].(string)
	bookMap, ok := nbtMap["book"].(map[string]any)
	if ok {
		book, canGetByCommand, err := nbt_parser_interface.ParseItemNormal(l.NameChecker, bookMap)
		if err != nil {
			return fmt.Errorf("Parse: %v", err)
		}
		if canGetByCommand {
			l.NBT.HaveBook = true
			l.NBT.Book = book
		}
	}
	return nil
}

func (l Lectern) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	w.String(&l.NBT.CustomName)
	w.Bool(&l.NBT.HaveBook)
	if l.NBT.HaveBook {
		bookStableBytes := l.NBT.Book.TypeStableBytes()
		w.ByteSlice(&bookStableBytes)
	}

	return buf.Bytes()
}

func (l *Lectern) FullStableBytes() []byte {
	return append(l.DefaultBlock.FullStableBytes(), l.NBTStableBytes()...)
}
