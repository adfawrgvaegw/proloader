package packet

import (
	"bytes"
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// MarshalPayloadBytes serialises the payload of pk to a byte slice using the latest protocol writer.
func MarshalPayloadBytes(pk Packet) ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := protocol.NewWriter(buf, 0)
	pk.Marshal(writer)
	return buf.Bytes(), nil
}

// UnmarshalPayloadBytes deserialises data into pk using the latest protocol reader.
func UnmarshalPayloadBytes(data []byte, pk Packet) error {
	buf := bytes.NewBuffer(data)
	reader := protocol.NewReader(buf, 0, false)
	pk.Marshal(reader)

	if buf.Len() > 0 {
		return fmt.Errorf("unmarshal packet payload %T: %d unread bytes left: 0x%x", pk, buf.Len(), buf.Bytes())
	}

	return nil
}
