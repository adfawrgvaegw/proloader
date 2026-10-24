// Independent RFC1951 interop oracle, using only Go's standard library.
// This is not the game server or the klauspost implementation in Fatalder.
package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
)

type sample struct {
	Plain      []byte `json:"plain"`
	Compressed []byte `json:"compressed"`
}
type result struct {
	DecodedHash string `json:"decoded_hash"`
	Compressed  []byte `json:"compressed"`
}

func main() {
	var samples []sample
	if err := json.NewDecoder(os.Stdin).Decode(&samples); err != nil {
		panic("invalid oracle input")
	}
	results := make([]result, 0, len(samples))
	for _, s := range samples {
		input := append(append([]byte{}, s.Compressed...), 1, 0, 0, 255, 255)
		r := flate.NewReader(bytes.NewReader(input))
		plain, err := io.ReadAll(r)
		if err != nil {
			panic("inflate failed")
		}
		if err := r.Close(); err != nil {
			panic("inflate close failed")
		}
		hash := sha256.Sum256(plain)
		var output bytes.Buffer
		w, err := flate.NewWriter(&output, 6)
		if err != nil {
			panic("deflate init failed")
		}
		if _, err := w.Write(s.Plain); err != nil {
			panic("deflate write failed")
		}
		if err := w.Flush(); err != nil {
			panic("deflate flush failed")
		}
		validLen := output.Len()
		if err := w.Close(); err != nil {
			panic("deflate close failed")
		}
		results = append(results, result{hex.EncodeToString(hash[:]), output.Bytes()[:validLen]})
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		panic("oracle output failed")
	}
}
