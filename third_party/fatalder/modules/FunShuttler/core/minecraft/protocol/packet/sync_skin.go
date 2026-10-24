/*
PhoenixBuilder specific NEMC packet.
Author: Liliya233
*/
package packet

import "github.com/Yeah114/FunShuttler/core/minecraft/protocol"

// Netease packet
type SyncSkin struct {
	// Skin info
	PlayerData []protocol.NeteasePlayerData
	// Player uids
	PlayerUIDs []string
	// Skin item ids
	SkinItemIDs []string
	// Skin
	Skin protocol.Skin

	Unknown []string
}

// ID ...
func (*SyncSkin) ID() uint32 {
	return IDSyncSkin
}

func (pk *SyncSkin) Marshal(io protocol.IO) {
	protocol.Slice(io, &pk.PlayerData)
	protocol.FuncSliceOfLen(io, uint32(len(pk.PlayerData)), &pk.PlayerUIDs, io.String)
	protocol.FuncSliceOfLen(io, uint32(len(pk.PlayerData)), &pk.Unknown, io.String)
	protocol.FuncSliceOfLen(io, uint32(len(pk.PlayerData)), &pk.SkinItemIDs, io.String)
	protocol.Single(io, &pk.Skin)
}
