/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ChangeBiome struct {
	Unknown1 string
	Unknown2 float32
	Unknown3 float32
	Unknown4 float32
	Unknown5 float32
	Unknown6 bool
}

// ID ...
func (*ChangeBiome) ID() uint32 {
	return IDChangeBiome
}

func (pk *ChangeBiome) Marshal(io protocol.IO) {
	io.String(&pk.Unknown1)
	io.Float32(&pk.Unknown2)
	io.Float32(&pk.Unknown3)
	io.Float32(&pk.Unknown4)
	io.Float32(&pk.Unknown5)
	io.Bool(&pk.Unknown6)
}
