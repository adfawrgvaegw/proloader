/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease Packet
type ModBlockActorChanged struct {
	Position protocol.BlockPos
}

// ID ...
func (*ModBlockActorChanged) ID() uint32 {
	return IDModBlockActorChanged
}

func (pk *ModBlockActorChanged) Marshal(io protocol.IO) {
	io.BlockPos(&pk.Position)
}
