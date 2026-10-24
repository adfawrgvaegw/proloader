package command

import (
	"encoding/binary"
	"io"

	"github.com/Yeah114/bdump/types"
)

type PlaceBlockWithChestData struct {
	BlockConstantStringID uint16
	BlockData             uint16
	ChestSlots            []types.ChestSlot
}

func (_ *PlaceBlockWithChestData) ID() uint16 {
	return IDPlaceBlockWithChestData
}

func (_ *PlaceBlockWithChestData) Name() string {
	return NamePlaceBlockWithChestData
}

func (cmd *PlaceBlockWithChestData) Marshal(writer io.Writer) error {
	uint16Buf := make([]byte, 2)
	binary.BigEndian.PutUint16(uint16Buf, cmd.BlockConstantStringID)
	_, err := writer.Write(uint16Buf)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint16(uint16Buf, cmd.BlockData)
	_, err = writer.Write(uint16Buf)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte{uint8(len(cmd.ChestSlots))})
	for _, slot := range cmd.ChestSlots {
		binary.BigEndian.PutUint16(uint16Buf, slot.Damage)
		tmpBuf := append([]byte(slot.Name), []byte{0, slot.Count}...)
		tmpBuf = append(tmpBuf, append(uint16Buf, slot.Slot)...)
		_, err = writer.Write(tmpBuf)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cmd *PlaceBlockWithChestData) Unmarshal(reader io.Reader) error {
	buf := make([]byte, 4)
	_, err := io.ReadAtLeast(reader, buf, 4)
	if err != nil {
		return err
	}
	cmd.BlockConstantStringID = binary.BigEndian.Uint16(buf[0:2])
	cmd.BlockData = binary.BigEndian.Uint16(buf[2:])
	chestSlotsLenBuf := make([]byte, 1)
	_, err = io.ReadAtLeast(reader, chestSlotsLenBuf, 1)
	if err != nil {
		return err
	}
	chestSlotsLen := int(chestSlotsLenBuf[0])
	cmd.ChestSlots = make([]types.ChestSlot, chestSlotsLen)
	for i := 0; i < chestSlotsLen; i++ {
		itemName, err := readString(reader)
		if err != nil {
			return err
		}
		cmd.ChestSlots[i].Name = itemName
		cdsBuf := make([]byte, 4)
		_, err = io.ReadAtLeast(reader, cdsBuf, 4)
		if err != nil {
			return err
		}
		cmd.ChestSlots[i].Count = cdsBuf[0]
		cmd.ChestSlots[i].Damage = binary.BigEndian.Uint16(cdsBuf[1:3])
		cmd.ChestSlots[i].Slot = cdsBuf[3]
	}
	return nil
}
