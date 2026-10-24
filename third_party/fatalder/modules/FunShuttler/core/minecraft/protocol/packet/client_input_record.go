/*
PhoenixBuilder specific NEMC packet.
Author: Happy2018new
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ClientInputRecord struct {
	Unknown1 string
	Unknown2 bool
	Unknown3 float32
	Unknown4 float32
	Unknown5 uint32
	Unknown6 uint32
	Unknown7 string
	Unknown8 protocol.NeteaseClientInputRecordData
}

// ID ...
func (*ClientInputRecord) ID() uint32 {
	return IDClientInputRecord
}

func (pk *ClientInputRecord) Marshal(io protocol.IO) {
	io.String(&pk.Unknown1)
	io.Bool(&pk.Unknown2)
	io.Float32(&pk.Unknown3)
	io.Float32(&pk.Unknown4)
	io.Varuint32(&pk.Unknown5)
	io.Varuint32(&pk.Unknown6)
	io.String(&pk.Unknown7)
	protocol.Single(io, &pk.Unknown8)
}
