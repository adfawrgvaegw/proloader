package netease_world

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()

	plain := map[string][]byte{
		"CURRENT":          []byte("MANIFEST-000001\n"),
		"MANIFEST-000001":  []byte("manifest-bytes"),
		"000001.log":       []byte("log-bytes"),
		"000002.ldb":       []byte{0x00, 0x01, 0x02, 0x03, 0xff},
		"OPTIONS-000123":   []byte("options"),
		"random-file.data": []byte("hello"),
	}

	for name, data := range plain {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := Encrypt(dir, &Options{BufferSize: 64}); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	for name := range plain {
		ok, err := fileHasHeader(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("fileHasHeader %s: %v", name, err)
		}
		if !ok {
			t.Fatalf("%s should be encrypted", name)
		}
	}

	if err := Decrypt(dir, &Options{BufferSize: 64}); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	for name, want := range plain {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s mismatch: got %q want %q", name, got, want)
		}
	}
}
