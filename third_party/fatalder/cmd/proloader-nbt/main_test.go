package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	waterdefine "github.com/Yeah114/WaterStructure/define"
	waterstructure "github.com/Yeah114/WaterStructure/structure"
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

func TestMCStructureExport(t *testing.T) {
	path := os.Getenv("PROLOADER_MCSTRUCTURE_EXPORT")
	if path == "" {
		t.Skip("Python cross-language export fixture not provided")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var m waterstructure.MCStructure
	if err = m.FromFile(file); err != nil {
		t.Fatal(err)
	}
	chunks, err := m.GetChunks([]waterdefine.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 2; x++ {
		for y := 0; y < 3; y++ {
			for z := 0; z < 4; z++ {
				index := int32((x*12+y*4+z)%3 + 1) // Proloader palette includes air at zero.
				if chunks[waterdefine.ChunkPos{0, 0}].Block(uint8(x), int16(y-64), uint8(z), 0) != m.GetPalette()[index] {
					t.Fatalf("coordinate mismatch: %d,%d,%d", x, y, z)
				}
			}
		}
	}
	n, ok := m.BlockNBT[23]
	if !ok || n["Command"] != "say test" || n["LastExecution"] != int64(1<<40) {
		t.Fatalf("entity not preserved: %#v", n)
	}
	if _, ok = n["auto"].(byte); !ok {
		t.Fatal("auto tag width changed")
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
