package structure

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/vmihailenco/msgpack/v5"
)

func TestBDSParseAndGetChunks(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.bds")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	enc := msgpack.NewEncoder(f)
	// top-level: [ blocks ]
	if err := enc.Encode([]any{
		[]any{
			[]any{"minecraft:stone", 0, 0, 0, 0, false},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	var r BDS
	if err := r.FromFile(f2); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	chunks, err := r.GetChunks([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	want := legacyBlockToBedrockRuntimeID("minecraft:stone", 0)
	got := chunks[define.ChunkPos{0, 0}].Block(0, -64, 0, 0)
	if got != want {
		t.Fatalf("runtimeID mismatch: got=%d want=%d", got, want)
	}
}

func TestCovStructureParseAndGetChunks(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.covstructure")
	jsonData := `{
  "size":[2,1,1],
  "structure":{
    "palette":[
      {"val":0,"name":"minecraft:stone","data":0},
      {"val":1,"name":"minecraft:air"}
    ],
    "block_indices":[0,1]
  }
}`
	if err := os.WriteFile(tmp, []byte(jsonData), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var r CovStructure
	if err := r.FromFile(f); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	chunks, err := r.GetChunks([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	want := legacyBlockToBedrockRuntimeID("minecraft:stone", 0)
	got := chunks[define.ChunkPos{0, 0}].Block(0, -64, 0, 0)
	if got != want {
		t.Fatalf("runtimeID mismatch: got=%d want=%d", got, want)
	}
}

func TestBCFParseAndGetChunks(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.bcf")
	if err := writeMinimalBCF(tmp); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var r BCF
	if err := r.FromFile(f); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	chunks, err := r.GetChunks([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	want := runtimeIDForBlock("minecraft:stone", nil)
	got := chunks[define.ChunkPos{0, 0}].Block(0, -64, 0, 0)
	if got != want {
		t.Fatalf("runtimeID mismatch: got=%d want=%d", got, want)
	}
}

func writeMinimalBCF(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// We'll write a v3-like file:
	// header -> 1 subchunk -> offsets table -> palette -> type map -> state maps
	write := func(data any) error {
		return binary.Write(f, binary.LittleEndian, data)
	}

	// header
	if _, err := f.Write([]byte("BCF")); err != nil {
		return err
	}
	if err := write(uint8(3)); err != nil {
		return err
	}
	if err := write(uint16(1)); err != nil { // width
		return err
	}
	if err := write(uint16(1)); err != nil { // length
		return err
	}
	if err := write(uint16(1)); err != nil { // height
		return err
	}
	if err := write(uint8(144)); err != nil { // subChunkBaseSize
		return err
	}
	if err := write(uint64(1)); err != nil { // subChunkCount
		return err
	}

	offsetTablePos := int64(0)
	palettePos := int64(0)
	typeMapPos := int64(0)
	stateNamePos := int64(0)
	stateValuePos := int64(0)
	headerOffsetsPos := int64(0)

	headerOffsetsPos, _ = f.Seek(0, io.SeekCurrent)
	for i := 0; i < 5; i++ {
		if err := write(uint64(0)); err != nil {
			return err
		}
	}

	// subchunk offset
	subchunkOffset, _ := f.Seek(0, io.SeekCurrent)
	if err := write(uint64(0)); err != nil { // subchunkSize (unused)
		return err
	}
	if err := write(int16(0)); err != nil { // originX
		return err
	}
	if err := write(int16(0)); err != nil { // originY
		return err
	}
	if err := write(int16(0)); err != nil { // originZ
		return err
	}
	if err := write(uint32(1)); err != nil { // regionCount
		return err
	}
	if err := write(uint32(0)); err != nil { // paletteId
		return err
	}
	coords := []int16{0, 0, 0, 0, 0, 0}
	for _, v := range coords {
		if err := write(v); err != nil {
			return err
		}
	}

	offsetTablePos, _ = f.Seek(0, io.SeekCurrent)
	if err := write(uint64(1)); err != nil { // offsetCount
		return err
	}
	if err := write(uint64(subchunkOffset)); err != nil {
		return err
	}

	palettePos, _ = f.Seek(0, io.SeekCurrent)
	if err := write(uint32(1)); err != nil { // paletteCount
		return err
	}
	if err := write(uint32(0)); err != nil { // paletteId
		return err
	}
	if err := write(uint16(0)); err != nil { // typeId
		return err
	}
	if err := write(uint16(0)); err != nil { // stateCount
		return err
	}

	typeMapPos, _ = f.Seek(0, io.SeekCurrent)
	if err := write(uint32(1)); err != nil { // typeCount
		return err
	}
	if err := write(uint16(0)); err != nil { // typeId
		return err
	}
	nameBytes := []byte("minecraft:stone")
	if err := write(uint16(len(nameBytes))); err != nil {
		return err
	}
	if _, err := f.Write(nameBytes); err != nil {
		return err
	}

	stateNamePos, _ = f.Seek(0, io.SeekCurrent)
	if err := write(uint32(0)); err != nil { // stateNameCount
		return err
	}

	stateValuePos, _ = f.Seek(0, io.SeekCurrent)
	if err := write(uint32(0)); err != nil { // stateValueCount
		return err
	}

	// patch header offsets
	if _, err := f.Seek(headerOffsetsPos, io.SeekStart); err != nil {
		return err
	}
	if err := write(uint64(offsetTablePos)); err != nil {
		return err
	}
	if err := write(uint64(palettePos)); err != nil {
		return err
	}
	if err := write(uint64(typeMapPos)); err != nil {
		return err
	}
	if err := write(uint64(stateNamePos)); err != nil {
		return err
	}
	if err := write(uint64(stateValuePos)); err != nil {
		return err
	}

	return nil
}

func TestTIBIParseAndGetChunks(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.tibi")
	if err := writeMinimalTIBI(tmp); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var r TIBI
	if err := r.FromFile(f); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	chunks, err := r.GetChunks([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	want := legacyBlockToBedrockRuntimeID("minecraft:stone", 0)
	got := chunks[define.ChunkPos{0, 0}].Block(0, -64, 0, 0)
	if got != want {
		t.Fatalf("runtimeID mismatch: got=%d want=%d", got, want)
	}
}

func writeMinimalTIBI(path string) error {
	// payload encoding helpers
	writeVarint := func(v uint64) []byte {
		out := make([]byte, 0, 10)
		for {
			b := byte(v & 0x7f)
			v >>= 7
			if v != 0 {
				out = append(out, b|0x80)
			} else {
				out = append(out, b)
				break
			}
		}
		return out
	}

	payload := make([]byte, 0)
	payload = append(payload, writeVarint(1)...) // blockCount
	payload = append(payload, writeVarint(0)...) // line placeholder
	blockStr := []byte("minecraft:stone")
	payload = append(payload, writeVarint(uint64(len(blockStr)))...)
	payload = append(payload, blockStr...)

	payload = append(payload, writeVarint(1)...) // prorCount
	payload = append(payload, writeVarint(0)...) // line placeholder
	prorStr := []byte("0")
	payload = append(payload, writeVarint(uint64(len(prorStr)))...)
	payload = append(payload, prorStr...)

	payload = append(payload, writeVarint(1)...) // cmdCount
	payload = append(payload, writeVarint(0)...) // qtype setblock
	payload = append(payload, writeVarint(0)...) // blockIndex
	payload = append(payload, writeVarint(0)...) // x
	payload = append(payload, writeVarint(0)...) // y
	payload = append(payload, writeVarint(0)...) // z
	payload = append(payload, writeVarint(0)...) // prorIndex

	header15 := make([]byte, 15)
	if _, err := rand.Read(header15); err != nil {
		return err
	}
	full := append(append([]byte{}, header15...), payload...)
	key := tibiMD5Key(header15, len(payload))
	xorInPlace(full, 15, key)

	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return err
	}
	if _, err := w.Write(full); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func TestLegacyStoneNotAir(t *testing.T) {
	if got := legacyBlockToBedrockRuntimeID("minecraft:stone", 0); got == block.AirRuntimeID {
		t.Fatal("legacy stone mapped to air")
	}
}

func TestNexusNPParseAndGetChunks(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.np")

	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	enc := msgpack.NewEncoder(f)
	if err := enc.Encode([]any{
		[]any{
			[]any{"minecraft:stone", 0, 0, 0, 0},
		},
		[]any{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	var r NexusNP
	if err := r.FromFile(f2); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	chunks, err := r.GetChunks([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}

	want := legacyBlockToBedrockRuntimeID("minecraft:stone", 0)
	got := chunks[define.ChunkPos{0, 0}].Block(0, -64, 0, 0)
	if got != want {
		t.Fatalf("runtimeID mismatch: got=%d want=%d", got, want)
	}
}
