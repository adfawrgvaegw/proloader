/*
PhoenixBuilder specific struct.
Author: Liliya233
*/
package protocol

// Netease
type NeteaseBiomeData struct {
	Flag     uint8
	Unknown1 int32
	Unknown2 string
	Pos1     BlockPos
	Pos2     BlockPos
}

// Marshal encodes/decodes an NeteaseBiomeData.
func (x *NeteaseBiomeData) Marshal(r IO) {
	r.Uint8(&x.Flag)
	r.Varint32(&x.Unknown1)
	r.String(&x.Unknown2)
	r.UBlockPos(&x.Pos1)
	if x.Flag == 1 {
		r.UBlockPos(&x.Pos2)
	}
}
