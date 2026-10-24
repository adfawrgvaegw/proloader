package command

import (
	"encoding/binary"
	"io"
)

type AssignDebugData struct {
	Data []byte
}

func (_ *AssignDebugData) ID() uint16 {
	return IDAssignDebugData
}

func (_ *AssignDebugData) Name() string {
	return NameAssignDebugData
}

func (cmd *AssignDebugData) Marshal(writer io.Writer) error {
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(cmd.Data)))
	_, err := writer.Write(append(lenBuf, cmd.Data...))
	return err
}

func (cmd *AssignDebugData) Unmarshal(reader io.Reader) error {
	lenBuf := make([]byte, 4)
	_, err := io.ReadAtLeast(reader, lenBuf, 4)
	if err != nil {
		return err
	}
	cmd.Data = make([]byte, int(binary.BigEndian.Uint32(lenBuf)))
	_, err = io.ReadAtLeast(reader, cmd.Data, len(cmd.Data))
	return err
}
