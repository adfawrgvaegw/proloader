package block_actors

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// 床
type Bed struct {
	general.BlockActor `mapstructure:",squash"`
	Color              byte `mapstructure:"color"` // TAG_Byte(1) = 0
}

// ID ...
func (*Bed) ID() string {
	return IDBed
}

func (b *Bed) Marshal(io protocol.IO) {
	protocol.Single(io, &b.BlockActor)
	protocol.NBTInt(&b.Color, io.Varuint32)
}
