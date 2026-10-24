package command

import (
	"fmt"
	"io"
)

type Command interface {
	ID() uint16 // Extra ID spaces (uint16) may be allocated in the future.
	Name() string
	Marshal(writer io.Writer) error
	Unmarshal(reader io.Reader) error
}

// Some deprecated commands may not be placed in this directory
// as I think we do not have to make them work

func readString(reader io.Reader) (string, error) {
	fullBuf := []byte{}
	buf := make([]byte, 1)
	for {
		_, err := io.ReadAtLeast(reader, buf, 1)
		if err != nil {
			return "", err
		}
		if buf[0] == 0 {
			return string(fullBuf), nil
		}
		fullBuf = append(fullBuf, buf...)
	}
}

func ReadCommand(reader io.Reader) (Command, error) {
	buf := make([]byte, 1)
	_, err := io.ReadAtLeast(reader, buf, 1)
	if err != nil {
		return nil, err
	}
	commandFunc, foundCommand := BDumpCommandPool[uint16(buf[0])]
	if !foundCommand {
		return nil, fmt.Errorf("Command::ReadCommand: Unknown Command ID: %d", int(buf[0]))
	}
	cmd := commandFunc()
	err = cmd.Unmarshal(reader)
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func WriteCommand(command Command, writer io.Writer) error {
	_, err := writer.Write([]byte{uint8(command.ID())})
	if err != nil {
		return err
	}
	return command.Marshal(writer)
}
