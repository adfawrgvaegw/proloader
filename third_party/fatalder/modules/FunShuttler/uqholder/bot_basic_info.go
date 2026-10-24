package uqholder

import (
	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/uqholder/defines"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	if false {
		func(defines.BotBasicInfoHolder) {}(&BotBasicInfoHolder{})
	}
}

type BotBasicInfoHolder struct {
	BotName      string
	BotRuntimeID uint64
	BotUniqueID  int64
	BotIdentity  string
}

func (b *BotBasicInfoHolder) Marshal() ([]byte, error) {
	return msgpack.Marshal(b)
}

func (b *BotBasicInfoHolder) Unmarshal(data []byte) error {
	return msgpack.Unmarshal(data, b)
}

func (b *BotBasicInfoHolder) UpdateFromPacket(packet packet.Packet) {}

func (b *BotBasicInfoHolder) GetBotName() string      { return b.BotName }
func (b *BotBasicInfoHolder) GetBotRuntimeID() uint64 { return b.BotRuntimeID }
func (b *BotBasicInfoHolder) GetBotUniqueID() int64   { return b.BotUniqueID }
func (b *BotBasicInfoHolder) GetBotIdentity() string  { return b.BotIdentity }
func (b *BotBasicInfoHolder) GetBotUUIDStr() string   { return b.BotIdentity }

func NewBotInfoHolder(conn *minecraft.Conn) defines.BotBasicInfoHolder {
	h := &BotBasicInfoHolder{}
	gd := conn.GameData()
	h.BotRuntimeID = gd.EntityRuntimeID
	h.BotUniqueID = gd.EntityUniqueID
	h.BotName = conn.IdentityData().DisplayName
	h.BotIdentity = conn.IdentityData().Identity
	if DEBUG {
		println("BotRuntimeID:", h.BotRuntimeID)
		println("BotUniqueID:", h.BotUniqueID)
		println("BotName:", h.BotName)
		println("BotIdentity:", h.BotIdentity)
	}
	return h
}
