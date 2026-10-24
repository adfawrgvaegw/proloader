/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ModEffect struct {
	Items []protocol.MobEffectV2UnknownEntry
}

// ID ...
func (*ModEffect) ID() uint32 {
	return IDModEffect
}

func (pk *ModEffect) Marshal(io protocol.IO) {
	protocol.SliceVaruint32Length(io, &pk.Items)
}
