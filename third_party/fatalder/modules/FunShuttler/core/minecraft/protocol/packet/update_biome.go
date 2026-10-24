/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type UpdateBiome struct {
	Pos       protocol.BlockPos
	BiomeData []protocol.NeteaseBiomeData
}

// ID ...
func (*UpdateBiome) ID() uint32 {
	return IDUpdateBiome
}

func (pk *UpdateBiome) Marshal(io protocol.IO) {
	io.UBlockPos(&pk.Pos)
	protocol.Slice(io, &pk.BiomeData)
}
