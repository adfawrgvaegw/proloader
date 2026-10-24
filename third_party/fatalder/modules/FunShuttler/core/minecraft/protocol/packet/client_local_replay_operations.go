/*
PhoenixBuilder specific NEMC packet.
Author: Happy2018new
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ClientLocalReplayOperations struct {
	Unknown1 uint32
	Unknown2 string
}

// ID ...
func (*ClientLocalReplayOperations) ID() uint32 {
	return IDClientLocalReplayOperations
}

func (pk *ClientLocalReplayOperations) Marshal(io protocol.IO) {
	io.Varuint32(&pk.Unknown1)
	io.String(&pk.Unknown2)
}
