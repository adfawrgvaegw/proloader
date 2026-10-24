/*
PhoenixBuilder specific.
Author: Happy2018new
*/
package protocol

const (
	NeteaseRecordUnknownFlag1 = 1 << iota
	NeteaseRecordUnknownFlag2
)

// Netease
type NeteaseRecordUnknownEntry1 struct {
	Unknown1 uint32
	Unknown2 float32
	Unknown3 uint32
}

// Marshal ...
func (x *NeteaseRecordUnknownEntry1) Marshal(r IO) {
	r.Varuint32(&x.Unknown1)
	r.Float32(&x.Unknown2)
	r.Varuint32(&x.Unknown3)
}

// Netease
type NeteaseRecordUnknownEntry2 struct {
	Unknown1 uint32
	Unknown2 uint32
	Unknown3 uint32
}

// Marshal ...
func (x *NeteaseRecordUnknownEntry2) Marshal(r IO) {
	r.Varuint32(&x.Unknown1)
	r.Varuint32(&x.Unknown2)
	r.Varuint32(&x.Unknown3)
}

// Netease
type NeteaseClientInputRecordData struct {
	Unknown1 uint32
	Unknown2 uint32
	Unknown3 float32
	Unknown4 float32 // Comment find in IDA: Y
	Unknown5 uint32
	Unknown6 []NeteaseRecordUnknownEntry1
	Unknown7 []NeteaseRecordUnknownEntry2
}

// Marshal ...
func (x *NeteaseClientInputRecordData) Marshal(r IO) {
	r.Varuint32(&x.Unknown1)
	if x.Unknown1&NeteaseRecordUnknownFlag2 != 0 {
		r.Varuint32(&x.Unknown2)
		r.Float32(&x.Unknown3)
		r.Float32(&x.Unknown4)
		r.Varuint32(&x.Unknown5)
	}
	if x.Unknown2&NeteaseRecordUnknownFlag1 != 0 {
		Slice(r, &x.Unknown6)
	}
	Slice(r, &x.Unknown7)
}

// Netease
type ClientAdditionalReplayUnknownData struct {
	Unknown1 bool
	Unknown2 byte
	Unknown3 string
	Unknown4 string // Guess: Script Data
}

// Marshal ...
func (x *ClientAdditionalReplayUnknownData) Marshal(r IO) {
	r.Bool(&x.Unknown1)
	r.Uint8(&x.Unknown2)
	r.String(&x.Unknown3)
	if x.Unknown3 == "scriptData" {
		r.String(&x.Unknown4)
	}
}
