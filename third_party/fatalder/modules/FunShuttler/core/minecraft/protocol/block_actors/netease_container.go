package block_actors

import (
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// Netease container ..
type NeteaseContainer struct {
	general.BlockActor `mapstructure:",squash"`
	Size               int32 `mapstructure:"Size"`  // TAG_Int(3)
	Items              []any `mapstructure:"Items"` // TAG_List[TAG_Compound] (9[10])
}

// ID ...
func (*NeteaseContainer) ID() string {
	return IDNeteaseContainer
}

func (n *NeteaseContainer) Marshal(io protocol.IO) {
	protocol.Single(io, &n.BlockActor)
	io.Varint32(&n.Size)
	protocol.NBTSlice(io, &n.Items, func(t *[]protocol.ItemWithSlot) { io.ItemList(t) })
}
