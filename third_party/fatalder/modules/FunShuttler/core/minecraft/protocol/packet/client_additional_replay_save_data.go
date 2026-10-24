/*
PhoenixBuilder specific NEMC packet.
Author: Happy2018new
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ClientAdditionalReplaySaveData struct {
	Unknown1 []protocol.ClientAdditionalReplayUnknownData
	Unknown2 bool
}

// ID ...
func (*ClientAdditionalReplaySaveData) ID() uint32 {
	return IDClientAdditionalReplaySaveData
}

func (pk *ClientAdditionalReplaySaveData) Marshal(io protocol.IO) {
	protocol.SliceOfLen(io, 8, &pk.Unknown1)
	io.Bool(&pk.Unknown2)
}
