package main

import "testing"

func TestEmptyPrefixIsAValidDeliveredLiteral(t *testing.T) {
	value, end, err := readLiteral([]byte{'s', 0, 0, 0, 0, 's', 1, 0, 0, 0, 'x'}, 0)
	if err != nil || value != "" || end != 5 {
		t.Fatalf("empty prefix: %q %d %v", value, end, err)
	}
	value, end, err = readLiteral([]byte{'s', 0, 0, 0, 0, 's', 1, 0, 0, 0, 'x'}, end)
	if err != nil || value != "x" || end != 11 {
		t.Fatalf("suffix: %q %d %v", value, end, err)
	}
}

func TestLiteralRejectsTruncationAndNonPrintableBytes(t *testing.T) {
	for _, b := range [][]byte{{'s', 2, 0, 0, 0, 'x'}, {'s', 1, 0, 0, 0, 0}, {'x', 0, 0, 0, 0}, {'s', 255, 255, 255, 255}} {
		if _, _, err := readLiteral(b, 0); err == nil {
			t.Fatal("accepted invalid literal")
		}
	}
}
