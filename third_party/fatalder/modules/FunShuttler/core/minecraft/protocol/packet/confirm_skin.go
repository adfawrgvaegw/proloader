/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type ConfirmSkin struct {
	// Skin info
	PlayerData []protocol.NeteasePlayerData
	// Launcher uids
	Uids []string
	// Netease
	Unknown3 []string
}

// ID ...
func (*ConfirmSkin) ID() uint32 {
	return IDConfirmSkin
}

func (pk *ConfirmSkin) Marshal(io protocol.IO) {
	protocol.SliceVaruint32Length(io, &pk.PlayerData)
	protocol.FuncSliceOfLen(io, uint32(len(pk.PlayerData)), &pk.Uids, io.String)
	protocol.FuncSliceOfLen(io, uint32(len(pk.PlayerData)), &pk.Unknown3, io.String)
}
