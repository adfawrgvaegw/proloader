package command

import (
	"io"
)

type AddInt8ZValue struct {
	Value int8
}

func (_ *AddInt8ZValue) ID() uint16 {
	return IDAddInt8ZValue
}

func (_ *AddInt8ZValue) Name() string {
	return NameAddInt8ZValue
}

func (cmd *AddInt8ZValue) Marshal(writer io.Writer) error {
	buf := []byte{uint8(cmd.Value)}
	_, err := writer.Write(buf)
	return err
}

func (cmd *AddInt8ZValue) Unmarshal(reader io.Reader) error {
	buf := make([]byte, 1)
	_, err := io.ReadAtLeast(reader, buf, 1)
	if err != nil {
		return err
	}
	cmd.Value = int8(buf[0])
	return nil
}
