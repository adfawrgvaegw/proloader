package command

import (
	"encoding/binary"
	"io"

	"github.com/Yeah114/bdump/types"
)

type PlaceRuntimeBlockWithChestData struct {
	BlockRuntimeID uint16
	ChestSlots     []types.ChestSlot
}

func (_ *PlaceRuntimeBlockWithChestData) ID() uint16 {
	return IDPlaceRuntimeBlockWithChestData
}

func (_ *PlaceRuntimeBlockWithChestData) Name() string {
	return NamePlaceRuntimeBlockWithChestData
}

func (cmd *PlaceRuntimeBlockWithChestData) Marshal(writer io.Writer) error {
	uint16Buf := make([]byte, 2)
	binary.BigEndian.PutUint16(uint16Buf, cmd.BlockRuntimeID)
	_, err := writer.Write(append(uint16Buf, uint8(len(cmd.ChestSlots))))
	// They are different parts, but wrote together for convenient
	if err != nil {
		return err
	}
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

func (cmd *PlaceRuntimeBlockWithChestData) Unmarshal(reader io.Reader) error {
	uint16Buf := make([]byte, 2)
	_, err := io.ReadAtLeast(reader, uint16Buf, 2)
	if err != nil {
		return err
	}
	cmd.BlockRuntimeID = binary.BigEndian.Uint16(uint16Buf)
	uint8Buf := make([]byte, 1)
	_, err = io.ReadAtLeast(reader, uint8Buf, 1)
	if err != nil {
		return err
	}
	cmd.ChestSlots = make([]types.ChestSlot, int(uint8Buf[0]))
	for i := 0; i < int(uint8Buf[0]); i++ {
		itemName, err := readString(reader)
		if err != nil {
			return err
		}
		cmd.ChestSlots[i].Name = itemName
		countDamageSlotBuf := make([]byte, 4)
		_, err = io.ReadAtLeast(reader, countDamageSlotBuf, 4)
		if err != nil {
			return err
		}
		cmd.ChestSlots[i].Count = countDamageSlotBuf[0]
		cmd.ChestSlots[i].Damage = binary.BigEndian.Uint16(countDamageSlotBuf[1:3])
		cmd.ChestSlots[i].Slot = countDamageSlotBuf[3]
	}
	return nil
}
