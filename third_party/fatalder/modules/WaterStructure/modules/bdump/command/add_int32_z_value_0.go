package command

import (
	"encoding/binary"
	"io"
)

type AddInt32ZValue0 struct {
	Value int32
}

func (_ *AddInt32ZValue0) ID() uint16 {
	return IDAddInt32ZValue0
}

func (_ *AddInt32ZValue0) Name() string {
	return NameAddInt32ZValue0
}

func (cmd *AddInt32ZValue0) Marshal(writer io.Writer) error {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(cmd.Value))
	_, err := writer.Write(buf)
	return err
}

func (cmd *AddInt32ZValue0) Unmarshal(reader io.Reader) error {
	buf := make([]byte, 4)
	_, err := io.ReadAtLeast(reader, buf, 4)
	if err != nil {
		return err
	}
	cmd.Value = int32(binary.BigEndian.Uint32(buf))
	return nil
}
