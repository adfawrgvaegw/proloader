package nbt_parser_block

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	nbt_parser_interface "github.com/Yeah114/FunShuttler/nbt_parser/interface"
)

// JukeBoxNBT ..
type JukeBoxNBT struct {
	CustomName string
	HaveDisc   bool
	Disc       nbt_parser_interface.Item
}

// 唱片机
type JukeBox struct {
	DefaultBlock
	NBT JukeBoxNBT
}

func (j JukeBox) NeedSpecialHandle() bool {
	if len(j.NBT.CustomName) > 0 {
		return true
	}
	if j.NBT.HaveDisc {
		return true
	}
	return false
}

func (JukeBox) NeedCheckCompletely() bool {
	return true
}

func (j JukeBox) formatNBT(prefix string) string {
	result := ""

	if len(j.NBT.CustomName) > 0 {
		result += prefix + fmt.Sprintf("自定义名称: %s\n", j.NBT.CustomName)
	}
	if j.NBT.HaveDisc {
		result += prefix + "唱片数据: \n"
		result += j.NBT.Disc.Format(prefix + "\t")
	}

	return result
}

func (j *JukeBox) Format(prefix string) string {
	result := j.DefaultBlock.Format(prefix)
	if j.NeedSpecialHandle() {
		result += prefix + "附加数据: \n"
		result += j.formatNBT(prefix + "\t")
	}
	return result
}

func (j *JukeBox) Parse(nbtMap map[string]any) error {
	j.NBT.CustomName, _ = nbtMap["CustomName"].(string)
	discMap, ok := nbtMap["RecordItem"].(map[string]any)
	if ok {
		disc, canGetByCommand, err := nbt_parser_interface.ParseItemNormal(j.NameChecker, discMap)
		if err != nil {
			return fmt.Errorf("Parse: %v", err)
		}
		if canGetByCommand {
			j.NBT.HaveDisc = true
			j.NBT.Disc = disc
		}
	}
	return nil
}

func (j JukeBox) NBTStableBytes() []byte {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	w.String(&j.NBT.CustomName)
	w.Bool(&j.NBT.HaveDisc)
	if j.NBT.HaveDisc {
		bookStableBytes := j.NBT.Disc.TypeStableBytes()
		w.ByteSlice(&bookStableBytes)
	}

	return buf.Bytes()
}

func (j *JukeBox) FullStableBytes() []byte {
	return append(j.DefaultBlock.FullStableBytes(), j.NBTStableBytes()...)
}
