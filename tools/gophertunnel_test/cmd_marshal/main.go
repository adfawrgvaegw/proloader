// CommandRequest marshal-only 对比工具（v1.58.1，无网络）。
//
// 用固定字段 marshal CommandRequest 并打印 hex，供 Python 实现逐字节对比。
// 字段全部固定（UUID/RequestID/Version），这样 Go 和 Python 必须产生完全相同的字节。
package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func main() {
	// 固定 UUID（与 Python 对比脚本用同一个值）
	fixedUUID := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")

	cmdPkt := &packet.CommandRequest{
		CommandLine: "setblock 0 10 0 gold_block",
		CommandOrigin: protocol.CommandOrigin{
			Origin:        0, // Player → commandOriginToString → "player"
			UUID:          fixedUUID,
			RequestID:     "proloader_1",
			PlayerUniqueID: 0,
		},
		Internal: false,
		Version:  "", // v1.58.1: Version 是 string（不再是 int32！）
	}

	// 手动 marshal：先写 packet ID varuint(77)=0x4d，再写 body
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(0x4d) // varuint(77)
	w := protocol.NewWriter(buf, 0)
	cmdPkt.Marshal(w)

	pktBytes := buf.Bytes()
	fmt.Printf("GO [%d B]: %s\n", len(pktBytes), hex.EncodeToString(pktBytes))
	log.SetFlags(0)
	log.Printf("GO_BYTES=%s", hex.EncodeToString(pktBytes))
}
