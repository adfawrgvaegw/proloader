package command

import (
	"encoding/binary"
	"io"

	"github.com/Yeah114/bdump/nbt"
)

type PlaceBlockWithNBTData struct {
	BlockConstantStringID       uint16
	BlockStatesConstantStringID uint16
	BlockNBTBytes               []byte
	BlockNBT                    map[string]interface{}
}

func (_ *PlaceBlockWithNBTData) ID() uint16 {
	return IDPlaceBlockWithNBTData
}

func (_ *PlaceBlockWithNBTData) Name() string {
	return NamePlaceBlockWithNBTData
}

func (cmd *PlaceBlockWithNBTData) Marshal(writer io.Writer) error {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, cmd.BlockConstantStringID)
	_, err := writer.Write(buf)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint16(buf, cmd.BlockStatesConstantStringID)
	_, err = writer.Write(buf)
	if err != nil {
		return err
	}
	_, err = writer.Write(append(buf, cmd.BlockNBTBytes...)) // cmd.BlockNBTBytes 以 nbt.LittleEndian 编码
	return err
}

func (cmd *PlaceBlockWithNBTData) Unmarshal(reader io.Reader) error {
	buf := make([]byte, 2)
	_, err := io.ReadAtLeast(reader, buf, 2)
	if err != nil {
		return err
	}
	cmd.BlockConstantStringID = binary.BigEndian.Uint16(buf)
	buf = make([]byte, 2)
	_, err = io.ReadAtLeast(reader, buf, 2)
	if err != nil {
		return err
	}
	cmd.BlockStatesConstantStringID = binary.BigEndian.Uint16(buf)
	_, err = io.ReadAtLeast(reader, buf, 2)
	if err != nil {
		return err
	}
	err = nbt.NewDecoderWithEncoding(reader, nbt.LittleEndian).Decode(&cmd.BlockNBT)
	return err
}
