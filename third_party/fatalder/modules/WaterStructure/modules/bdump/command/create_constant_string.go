package command

import (
	"io"
)

type CreateConstantString struct {
	ConstantString string
}

func (_ *CreateConstantString) ID() uint16 {
	return IDCreateConstantString
}

func (_ *CreateConstantString) Name() string {
	return NameCreateConstantString
}

func (cmd *CreateConstantString) Marshal(writer io.Writer) error {
	strC := append([]byte(cmd.ConstantString), 0)
	_, err := writer.Write(strC)
	return err
}

func (cmd *CreateConstantString) Unmarshal(reader io.Reader) error {
	singleBuf := make([]byte, 1)
	cmd.ConstantString = ""
	for {
		_, err := io.ReadAtLeast(reader, singleBuf, 1)
		if err != nil {
			return err
		}
		if singleBuf[0] == 0 {
			break
		}
		cmd.ConstantString += string(singleBuf[0])
	}
	return nil
}
