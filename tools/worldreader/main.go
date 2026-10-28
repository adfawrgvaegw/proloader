// Offline codecs only. The Python session remains the sole network owner.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors"
	general "github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors/general_actors"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/mitchellh/mapstructure"
	modernprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	modernpacket "github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type state struct {
	Name   string         `json:"name"`
	States map[string]any `json:"states"`
}

type request struct {
	Palette   []state                   `json:"palette"`
	Protocol  int                       `json:"protocol"`
	Action    string                    `json:"action"`
	Data      string                    `json:"data"`
	Dimension int32                     `json:"dimension"`
	Position  [3]int32                  `json:"position"`
	Update    packet.CommandBlockUpdate `json:"update"`
}

type response struct {
	Hashes    []uint32             `json:"hashes,omitempty"`
	Dimension int32                `json:"dimension"`
	Position  protocol.SubChunkPos `json:"position"`
	Entries   []entry              `json:"entries"`
	Hex       string               `json:"hex,omitempty"`
}
type entry struct {
	HeaderY *int                      `json:"header_y,omitempty"`
	Offset  protocol.SubChunkOffset   `json:"offset"`
	Result  byte                      `json:"result"`
	NBT     []map[string]any          `json:"nbt"`
	Blocks  []uint32                  `json:"blocks,omitempty"`
	Palette map[uint32]map[string]any `json:"palette,omitempty"`
}

func expandActor(actor interface{ Marshal(protocol.IO) }, buf *bytes.Buffer) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("%v", v)
		}
	}()
	actor.Marshal(protocol.NewReader(buf, 0, false))
	return nil
}

// The rental 860 shared container prefix adds two zero fields immediately
// before ItemList, not after it. Empty inventories cannot reveal this offset.
// Parse the pinned prefix, accept only those exact zeros, then let the original
// actor codec validate the entire inventory and remaining actor bytes.
func normaliseContainerTag(raw []byte) (normal []byte, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("container prefix: %v", v)
		}
	}()
	buf := bytes.NewBuffer(raw)
	r := protocol.NewReader(buf, 0, false)
	base := general.RandomizableBlockActor{}
	base.Marshal(r)
	var lead, pair byte
	r.Uint8(&lead)
	r.Uint8(&pair)
	if pair > 1 {
		return nil, fmt.Errorf("invalid pair flag")
	}
	if pair == 1 {
		var x, z int32
		r.Varint32(&x)
		r.Varint32(&z)
	} else {
		var unpair byte
		r.Uint8(&unpair)
	}
	var hasSize bool
	r.Bool(&hasSize)
	if hasSize {
		var size int16
		r.Varint16(&size)
	}
	prefix := len(raw) - buf.Len()
	if buf.Len() < 2 || !bytes.Equal(buf.Next(2), []byte{0, 0}) {
		return nil, fmt.Errorf("unknown container extension")
	}
	normal = append(append([]byte{}, raw[:prefix]...), buf.Bytes()...)
	return normal, nil
}

// Use upstream codecs, with a fresh map per entity and strict error propagation.
// The old GetNBT convenience wrapper reuses a map and can suppress tag errors.
func decodeNBT(raw []byte, dimension int32) ([]map[string]any, error) {
	b := bytes.NewBuffer(raw)
	if _, _, err := chunk.DecodeSubChunk(b, define.Dimension(dimension).Range(), chunk.NetworkEncoding); err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for b.Len() > 0 {
		m := map[string]any{}
		if err := nbt.NewDecoderWithEncoding(b, nbt.NetworkLittleEndian).Decode(&m); err != nil {
			return nil, err
		}
		if tag, ok := m["__tag"].(string); ok {
			id, ok := m["id"].(string)
			if !ok {
				return nil, fmt.Errorf("missing block actor id")
			}
			actor, ok := block_actors.NewPool()[id]
			if !ok {
				m["_decode_error"] = "unsupported tagged block actor"
				delete(m, "__tag")
				result = append(result, m)
				continue
			}
			buf := bytes.NewBufferString(tag)
			actorErr := expandActor(actor, buf)
			if (actorErr != nil || buf.Len() != 0) && (id == "Chest" || id == "Barrel") {
				if normal, err := normaliseContainerTag([]byte(tag)); err == nil {
					candidate := block_actors.NewPool()[id]
					candidateBuf := bytes.NewBuffer(normal)
					if err = expandActor(candidate, candidateBuf); err == nil && candidateBuf.Len() == 0 {
						actor = candidate
						buf = candidateBuf
						actorErr = nil
						m["_nemc_container_zero_extension"] = 2
					}
				}
			}
			if actorErr != nil {
				m["_decode_error"] = fmt.Sprintf("actor %s codec: %v", id, actorErr)
				delete(m, "__tag")
				result = append(result, m)
				continue
			}
			if buf.Len() != 0 {
				m["_decode_error"] = "trailing tagged block actor data"
				delete(m, "__tag")
				result = append(result, m)
				continue
			}
			expanded := map[string]any{}
			if err := mapstructure.Decode(actor, &expanded); err != nil {
				return nil, err
			}
			delete(m, "__tag")
			for k, v := range expanded {
				m[k] = v
			}
		}
		result = append(result, m)
	}
	return result, nil
}

// Protocol 860 includes RenderHeightMap after each no-cache entry. The vendored
// older Fatalder struct omits it; field order follows Gophertunnel SubChunkEntry.
func decodeEnvelope(raw []byte, modern bool) (p packet.SubChunk, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("invalid subchunk envelope")
		}
	}()
	b := bytes.NewBuffer(raw)
	r := protocol.NewReader(b, 0, false)
	r.Bool(&p.CacheEnabled)
	if p.CacheEnabled {
		return p, fmt.Errorf("unsupported cache")
	}
	r.Varint32(&p.Dimension)
	r.SubChunkPos(&p.Position)
	var count uint32
	r.Uint32(&count)
	if count > 1024 {
		return p, fmt.Errorf("subchunk entry count exceeds limit")
	}
	for i := uint32(0); i < count; i++ {
		var entry protocol.SubChunkEntry
		protocol.SubChunkEntryNoCache(r, &entry)
		if modern {
			var kind byte
			r.Uint8(&kind)
			if kind > 4 {
				return p, fmt.Errorf("invalid render height map")
			}
			if kind == 1 {
				var value int8
				for j := 0; j < 256; j++ {
					r.Int8(&value)
				}
			}
		}
		p.SubChunkEntries = append(p.SubChunkEntries, entry)
	}
	if b.Len() != 0 {
		return p, fmt.Errorf("trailing subchunk data")
	}
	return p, nil
}

func run(r request) (out response, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("invalid NBT codec input: %v", recovered)
		}
	}()
	if r.Dimension < 0 || r.Dimension > 2 {
		return out, fmt.Errorf("invalid dimension")
	}
	switch r.Action {
	case "hash_states":
		for _, s := range r.Palette {
			for k, v := range s.States {
				switch x := v.(type) {
				case bool:
					if x {
						s.States[k] = byte(1)
					} else {
						s.States[k] = byte(0)
					}
				case float64:
					s.States[k] = int32(x)
				}
			}
			out.Hashes = append(out.Hashes, block.ComputeBlockHash(s.Name, s.States))
		}

	case "encode_update":
		b := bytes.NewBuffer(nil)
		if r.Protocol == 686 {
			r.Update.Marshal(protocol.NewWriter(b, 0))
		} else if r.Protocol == 0 || r.Protocol == 860 {
			u := r.Update
			pk := modernpacket.CommandBlockUpdate{Block: u.Block, Position: modernprotocol.BlockPos(u.Position), Mode: u.Mode, NeedsRedstone: u.NeedsRedstone, Conditional: u.Conditional, Command: u.Command, LastOutput: u.LastOutput, Name: u.Name, ShouldTrackOutput: u.ShouldTrackOutput, TickDelay: u.TickDelay, ExecuteOnFirstTick: u.ExecuteOnFirstTick}
			pk.Marshal(modernprotocol.NewWriter(b, 0))
		} else {
			return out, fmt.Errorf("unsupported wire profile")
		}
		out.Hex = hex.EncodeToString(b.Bytes())
	case "encode_request":
		p := packet.SubChunkRequest{Dimension: r.Dimension, Position: protocol.SubChunkPos{r.Position[0] >> 4, 0, r.Position[2] >> 4}, Offsets: []protocol.SubChunkOffset{{0, int8(r.Position[1] >> 4), 0}}}
		b := bytes.NewBuffer(nil)
		p.Marshal(protocol.NewWriter(b, 0))
		out.Hex = hex.EncodeToString(b.Bytes())
	case "decode_subchunk":
		raw, e := base64.StdEncoding.DecodeString(r.Data)
		if e != nil {
			return out, e
		}
		p, e := decodeEnvelope(raw, true)
		if e != nil {
			p, e = decodeEnvelope(raw, false)
		}
		if e != nil {
			return out, e
		}
		out.Dimension = p.Dimension
		out.Position = p.Position
		out.Entries = []entry{}
		if p.Dimension < 0 || p.Dimension > 2 {
			return out, fmt.Errorf("invalid response dimension")
		}
		for _, e := range p.SubChunkEntries {
			item := entry{Offset: e.Offset, Result: e.Result, NBT: []map[string]any{}}
			if e.Result == protocol.SubChunkResultSuccess {
				buf := bytes.NewBuffer(e.RawPayload)
				sub, index, decodeErr := chunk.DecodeSubChunk(buf, define.Dimension(p.Dimension).Range(), chunk.NetworkEncoding)
				if decodeErr != nil {
					return out, decodeErr
				}
				if len(e.RawPayload) > 0 && e.RawPayload[0] == 9 {
					y := index + int(define.Dimension(p.Dimension).Range()[0]>>4)
					item.HeaderY = &y
				}
				item.Blocks = sub.Blocks(0)
				item.Palette = map[uint32]map[string]any{}
				for _, rid := range item.Blocks {
					if _, exists := item.Palette[rid]; !exists {
						name, states, ok := block.RuntimeIDToState(rid)
						if ok {
							item.Palette[rid] = map[string]any{"name": name, "states": states}
						}
					}
				}
				item.NBT, err = decodeNBT(e.RawPayload, p.Dimension)
				if err != nil {
					return out, err
				}
			}
			out.Entries = append(out.Entries, item)
		}
	default:
		return out, fmt.Errorf("unsupported codec action")
	}
	return out, nil
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--stream" {
		if err := serve(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}
	var r request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 32<<20)).Decode(&r); err != nil {
		fmt.Fprintln(os.Stderr, "invalid codec JSON")
		os.Exit(1)
	}
	out, err := run(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "NBT codec failed:", err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
		os.Exit(1)
	}
}

func serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 32<<20)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var r request
		err := json.Unmarshal(scanner.Bytes(), &r)
		var out response
		if err == nil {
			out, err = run(r)
		}
		if err != nil {
			if e := encoder.Encode(map[string]any{"error": "invalid codec input"}); e != nil {
				return e
			}
		} else if err = encoder.Encode(map[string]any{"result": out}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
