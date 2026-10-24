package block_actors

import (
	"bytes"
	"strings"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
)

// 饰纹陶罐
type DecoratedPot struct {
	general.RandomizableBlockActor `mapstructure:",squash"` // Should remove when NetEase support this block

	Sherds    []any         `mapstructure:"sherds,omitempty"` // TAG_List[TAG_Compound] (9[10]) = []
	Animation byte          `mapstructure:"animation"`        // Not used; TAG_Byte(1) = 0
	Item      protocol.Item `mapstructure:"item"`             // Not used; TAG_Compound(10) = {}
}

// ID ...
func (*DecoratedPot) ID() string {
	return IDDecoratedPot
}

func (d *DecoratedPot) unmarshal(io protocol.IO) {
	var sherds []byte
	io.ByteSlice(&sherds)

	for value := range strings.SplitSeq(string(sherds), "@") {
		d.Sherds = append(d.Sherds, value)
	}
	d.Sherds = d.Sherds[:len(d.Sherds)-1]
}

func (d *DecoratedPot) marshal(io protocol.IO) {
	var sherds []byte

	buf := bytes.NewBuffer(nil)
	for _, value := range d.Sherds {
		buf.WriteString(value.(string) + "@")
	}
	sherds = buf.Bytes()

	io.ByteSlice(&sherds)
}

/*
Waiting NetEase to support this block

func (d *DecoratedPot) Marshal(io protocol.IO) {
	if _, ok := io.(*protocol.Reader); ok {
		d.unmarshal(io)
		return
	}
	d.marshal(io)
}
*/
