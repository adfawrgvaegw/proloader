package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/mitchellh/mapstructure"
)

func fixture(t *testing.T, tagged bool) string {
	t.Helper()
	raw := chunk.EncodeSubChunk(chunk.NewSubChunk(block.AirRuntimeID), define.Dimension(0).Range(), -4, chunk.NetworkEncoding)
	buf := bytes.NewBuffer(raw)
	for i := 0; i < 2; i++ {
		m := map[string]any{"id": "CommandBlock", "x": int32(-17 + i), "y": int32(-60), "z": int32(-33), "Command": "say hello", "CustomName": "test", "auto": byte(0), "TrackOutput": byte(1), "TickDelay": int32(7), "ExecuteOnFirstTick": byte(0)}
		if tagged {
			actor := block_actors.NewPool()["CommandBlock"]
			if err := mapstructure.Decode(m, actor); err != nil {
				t.Fatal(err)
			}
			data := bytes.NewBuffer(nil)
			actor.Marshal(protocol.NewWriter(data, 0))
			m = map[string]any{"id": "CommandBlock", "x": int32(-17 + i), "y": int32(-60), "z": int32(-33), "__tag": string(data.Bytes())}
		}
		if err := nbt.NewEncoderWithEncoding(buf, nbt.NetworkLittleEndian).Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	p := packet.SubChunk{Dimension: 0, Position: protocol.SubChunkPos{-2, 0, -3}, SubChunkEntries: []protocol.SubChunkEntry{{Offset: protocol.SubChunkOffset{0, -4, 0}, Result: 1, RawPayload: buf.Bytes()}}}
	out := bytes.NewBuffer(nil)
	p.Marshal(protocol.NewWriter(out, 0))
	return base64.StdEncoding.EncodeToString(out.Bytes())
}

func TestUpstreamNBTDecode(t *testing.T) {
	if (&packet.CommandBlockUpdate{}).ID() != 78 || (&packet.SubChunk{}).ID() != 174 || (&packet.SubChunkRequest{}).ID() != 175 {
		t.Fatal("packet ID changed")
	}
	for _, tagged := range []bool{false, true} {
		f := fixture(t, tagged)
		r, err := run(request{Action: "decode_subchunk", Data: f})
		if err != nil {
			t.Fatal(err)
		}
		n := r.Entries[0].NBT
		if len(n) != 2 || n[0]["x"] == n[1]["x"] || n[0]["Command"] != "say hello" || n[0]["TickDelay"] != int32(7) {
			t.Fatalf("incorrect decode: %#v", n)
		}
		if tagged {
			if _, ok := n[0]["LPCommandMode"]; !ok {
				t.Fatal("missing expanded fields")
			}
		}
		// Optional golden export for cross-language tests. No game connection.
		if folder := os.Getenv("PROLOADER_NBT_FIXTURE_DIR"); folder != "" {
			name := "plain"
			if tagged {
				name = "tagged"
			}
			b, _ := json.Marshal(map[string]any{"data": f, "decoded": r})
			if err := os.WriteFile(filepath.Join(folder, name+".json"), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestMalformedDecodeFails(t *testing.T) {
	for _, data := range []string{"!", base64.StdEncoding.EncodeToString([]byte{0, 0, 0})} {
		if _, err := run(request{Action: "decode_subchunk", Data: data}); err == nil {
			t.Fatal("accepted malformed response")
		}
	}
}

func TestModernRenderHeightmap(t *testing.T) {
	old, err := base64.StdEncoding.DecodeString(fixture(t, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []byte{0, 1, 2, 3, 4} {
		modern := append(append([]byte{}, old...), kind)
		if kind == 1 {
			modern = append(modern, make([]byte, 256)...)
		}
		out, err := run(request{Action: "decode_subchunk", Data: base64.StdEncoding.EncodeToString(modern)})
		if err != nil || len(out.Entries) != 1 || len(out.Entries[0].NBT) != 2 {
			t.Fatalf("render kind %d: %v", kind, err)
		}
	}
	malformed := append(append([]byte{}, old...), byte(9))
	if _, err = run(request{Action: "decode_subchunk", Data: base64.StdEncoding.EncodeToString(malformed)}); err == nil {
		t.Fatal("accepted unknown render map")
	}
}

func TestUnrelatedBrokenActorDoesNotDiscardValidTarget(t *testing.T) {
	raw := chunk.EncodeSubChunk(chunk.NewSubChunk(block.AirRuntimeID), define.Dimension(0).Range(), -4, chunk.NetworkEncoding)
	buf := bytes.NewBuffer(raw)
	for _, m := range []map[string]any{
		{"id": "Skull", "x": int32(0), "y": int32(-60), "z": int32(0), "__tag": "bad"},
		{"id": "CommandBlock", "x": int32(1), "y": int32(-60), "z": int32(0), "Command": "say preserved"},
	} {
		if err := nbt.NewEncoderWithEncoding(buf, nbt.NetworkLittleEndian).Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	result, err := decodeNBT(buf.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0]["_decode_error"] == nil || result[1]["Command"] != "say preserved" {
		t.Fatal(result)
	}
}

func TestContainerExtensionBeforePopulatedInventoryIsBounded(t *testing.T) {
	for _, id := range []string{"Chest", "Barrel"} {
		for _, tail := range [][]byte{nil, {0, 0}, {0}, {0, 1}, {0, 0, 0}} {
			actor := block_actors.NewPool()[id]
			if err := mapstructure.Decode(map[string]any{"Items": []any{map[string]any{"Slot": byte(5), "Name": "minecraft:stone", "Count": byte(3), "Damage": int16(0)}}}, actor); err != nil {
				t.Fatal(err)
			}
			original := bytes.NewBuffer(nil)
			actor.Marshal(protocol.NewWriter(original, 0))
			data := original.Bytes()
			tag := append(append(append([]byte{}, data[:8]...), tail...), data[8:]...)
			raw := chunk.EncodeSubChunk(chunk.NewSubChunk(block.AirRuntimeID), define.Dimension(0).Range(), -4, chunk.NetworkEncoding)
			buf := bytes.NewBuffer(raw)
			if err := nbt.NewEncoderWithEncoding(buf, nbt.NetworkLittleEndian).Encode(map[string]any{
				"id": id, "x": int32(1), "y": int32(-60), "z": int32(0), "__tag": string(tag),
			}); err != nil {
				t.Fatal(err)
			}
			result, err := decodeNBT(buf.Bytes(), 0)
			if err != nil {
				t.Fatal(err)
			}
			accepted := len(tail) == 0 || bytes.Equal(tail, []byte{0, 0})
			if (result[0]["_decode_error"] == nil) != accepted {
				t.Fatalf("%s tail %x: %v", id, tail, result)
			}
			if accepted {
				items := result[0]["Items"].([]any)
				if len(items) != 1 {
					t.Fatal("lost inventory", result)
				}
				if len(tail) == 2 && result[0]["_nemc_container_zero_extension"] != 2 {
					t.Fatal("missing profile marker")
				}
			}
		}
	}
}
