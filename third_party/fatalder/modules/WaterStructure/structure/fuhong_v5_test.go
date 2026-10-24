package structure

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"
)

func TestFuHongV5EncodeDecodeRoundTrip(t *testing.T) {
	plain := `{"FuHongBuild":[{"startX":0,"startZ":0,"block":[]}],"BlocksList":["minecraft:air"]}`

	encoded, err := encodeFuHongV5(plain)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := io.WriteString(zw, encoded); err != nil {
		t.Fatalf("zlib write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close failed: %v", err)
	}

	decoded, err := decodeFuHongV5(buf.Bytes())
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded != plain {
		t.Fatalf("roundtrip mismatch got %q want %q", decoded, plain)
	}
}

