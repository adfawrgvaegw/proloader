package block_actors

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// 附魔台
type EnchantTable struct {
	general.BlockActor `mapstructure:",squash"`
	Rotation           float32 `mapstructure:"rott"` // TAG_Float(6) = 0
}

// ID ...
func (*EnchantTable) ID() string {
	return IDEnchantTable
}

func (e *EnchantTable) Marshal(io protocol.IO) {
	var name string = e.CustomName

	protocol.Single(io, &e.BlockActor)
	io.String(&name)
	io.Float32(&e.Rotation)

	if len(name) > 0 {
		e.CustomName = name
	}
}
