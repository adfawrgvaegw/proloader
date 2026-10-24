package chat_connection

import (
	"encoding/binary"
	"fmt"
)

// 协议常量，与官方客户端保持一致。
const (
	// handshakeRequestCmd 登录握手指令编号。
	handshakeRequestCmd uint16 = 0x0101 // 257
	// heartbeatIntervalSeconds 心跳间隔（秒）。
	heartbeatIntervalSeconds = 10
)

var (
	emptyFrame = []byte{0x00, 0x00}
)

// Message 表示从聊天服务器收到的一条消息。
type Message struct {
	Sequence uint16
	Command  uint16
	Payload  []byte
}

// frameLengthPrefix 生成长度前缀（小端 2 字节）。
func frameLengthPrefix(length int) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(length))
	return buf
}

func wrapFrame(payload []byte) ([]byte, error) {
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("chat_connection: payload 过长: %d", len(payload))
	}
	prefix := frameLengthPrefix(len(payload))
	framed := make([]byte, 2+len(payload))
	copy(framed[:2], prefix)
	copy(framed[2:], payload)
	return framed, nil
}
