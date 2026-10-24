package protocol

// Netease
type NeteaseStartGameData struct {
	// Netease
	Unknown1 uint64
	// Netease
	Unknown2 float32
}

// Marshal...
func (x *NeteaseStartGameData) Marshal(r IO) {
	r.Uint64(&x.Unknown1)
	r.Float32(&x.Unknown2)
}
