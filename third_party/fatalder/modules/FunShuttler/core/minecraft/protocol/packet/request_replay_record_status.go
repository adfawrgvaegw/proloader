/*
PhoenixBuilder specific NEMC packet.
Author: Happy2018new
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type RequestReplayRecordStatus struct {
	Unknown1 uint8
	Unknown2 uint8
	Unknown3 float32
	Unknown4 string
}

// ID ...
func (*RequestReplayRecordStatus) ID() uint32 {
	return IDRequestReplayRecordStatus
}

func (pk *RequestReplayRecordStatus) Marshal(io protocol.IO) {
	io.Uint8(&pk.Unknown1)
	io.Uint8(&pk.Unknown2)
	io.Float32(&pk.Unknown3)
	io.String(&pk.Unknown4)
}
