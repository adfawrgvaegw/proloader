package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// 用 Gophertunnel 官方库连接 BDS 并发 setblock 命令，验证协议格式。
// 同时打印手动 marshal 的 CommandRequest 字节，供 Python 实现对比。
func main() {
	host := "127.0.0.1:19132"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}

	// ── 第一部分：手动 marshal CommandRequest，打印字节 ──
	log.Printf("=== 手动构建 CommandRequest，打印字节 ===")
	cmdPkt := &packet.CommandRequest{
		CommandLine: "setblock 0 10 0 gold_block",
		CommandOrigin: protocol.CommandOrigin{
			Origin:   0,
			RequestID: "proloader_go_1",
		},
		Internal: false,
		Version:  "", // ⚠️ v1.58.1: Version 是 string（不再是 int32！）
	}
	// 用 Gophertunnel 的 protocol.Writer 手动 marshal
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)
	// 手动写 packet ID varuint(77)
	buf.WriteByte(0x4d) // varuint(77) = 0x4d
	cmdPkt.Marshal(w)
	pktBytes := buf.Bytes()
	log.Printf("Gophertunnel CommandRequest [%d B]: %s", len(pktBytes), hex.EncodeToString(pktBytes))

	// ── 第二部分：连接 BDS 发命令 ──
	log.Printf("\n=== 连接 %s ===", host)
	dialer := minecraft.Dialer{
		PacketFunc: func(hdr packet.Header, payload []byte, src, dst net.Addr) {
			// 打印所有发送和接收的包
			full := append([]byte{}, encodeVaruint(hdr.PacketID)...)
			full = append(full, payload...)
			log.Printf(">>> 包 [id=%d payload=%dB full=%dB]: %s",
				hdr.PacketID, len(payload), len(full), hex.EncodeToString(full))
		},
	}
	conn, err := dialer.Dial("raknet", host)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer conn.Close()
	log.Printf("✓ 已连接，等待 DoSpawn...")

	// ⚠️ 关键：必须调用 DoSpawn() 等待玩家完全生成！
	// DoSpawn 内部等待 StartGame → RequestChunkRadius → PlayStatusPlayerSpawn → SetLocalPlayerAsInitialised
	// 不调用的话玩家没生成，发命令会收到 PacketViolationWarning
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := conn.DoSpawnContext(ctx); err != nil {
		log.Fatalf("DoSpawn 失败: %v", err)
	}
	log.Printf("✓✓✓ DoSpawn 完成，玩家已生成！runtime_id=%d", conn.GameData().EntityRuntimeID)

	// 等一下让服务器完全处理 spawn
	time.Sleep(2 * time.Second)

	// Flush 确保所有缓冲数据已发送
	_ = conn.Flush()

	// 发 setblock
	log.Printf("发命令: setblock 0 10 0 gold_block")
	err = conn.WritePacket(&packet.CommandRequest{
			CommandLine: "setblock 0 10 0 gold_block",
			CommandOrigin: protocol.CommandOrigin{
				Origin:   0,
				RequestID: "proloader_go_1",
			},
			Internal: false,
			Version:  "", // ⚠️ v1.58.1: string
		})
	if err != nil {
		log.Fatalf("发命令失败: %v", err)
	}
	log.Printf("✓ 命令已发，等回包...")

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		pk, err := conn.ReadPacket()
		if err != nil {
			log.Printf("读包结束: %v", err)
			break
		}
		switch p := pk.(type) {
		case *packet.CommandOutput:
			log.Printf("✓✓✓ CommandOutput: success=%d", p.SuccessCount)
			fmt.Println("=== 成功！Gophertunnel 官方库能发命令 ===")
			return
		case *packet.PacketViolationWarning:
			log.Printf("✗ PacketViolationWarning: type=%d sev=%d pid=%d context=\"%s\"", p.Type, p.Severity, p.PacketID, p.ViolationContext)
		default:
			// 只打印非 chunk 包，避免刷屏
		}
	}
	fmt.Println("=== 未收到 CommandOutput ===")
}

func encodeVaruint(v uint32) []byte {
	var buf [5]byte
	n := 0
	for {
		buf[n] = byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return buf[:n+1]
		}
		buf[n] |= 0x80
		n++
	}
}
