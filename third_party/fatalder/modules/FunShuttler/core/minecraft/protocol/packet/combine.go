/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type Combine struct {
	Unknown1 uint8
}

// ID ...
func (*Combine) ID() uint32 {
	return IDCombine
}

func (pk *Combine) Marshal(io protocol.IO) {
	io.Uint8(&pk.Unknown1)
}
