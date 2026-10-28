// Offline static decoding only. No received Python is imported or executed.
package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"proloader/mcpcheck/internal/pymarshal"
)

const templateHash = "574eef18e0deb853d397c1344b805f513565623c9393c07511450b288bf8912a"

type request struct {
	Marshal  string `json:"marshal"`
	Salt     string `json:"salt"`
	Engine   string `json:"engine"`
	Patch    string `json:"patch"`
	UniqueID int64  `json:"unique_id"`
}

func readLiteral(data []byte, offset int) (string, int, error) {
	if offset < 0 || offset+5 > len(data) || data[offset] != 's' {
		return "", 0, fmt.Errorf("unsupported_template")
	}
	n := int(binary.LittleEndian.Uint32(data[offset+1 : offset+5]))
	if n < 0 || n > 64 || offset+5+n > len(data) {
		return "", 0, fmt.Errorf("unsupported_template")
	}
	value := data[offset+5 : offset+5+n]
	for _, b := range value {
		if b < 32 || b > 126 {
			return "", 0, fmt.Errorf("unsupported_template")
		}
	}
	return string(value), offset + 5 + n, nil
}

func literal(value string) []byte {
	out := []byte{'s', 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(out[1:], uint32(len(value)))
	return append(out, []byte(value)...)
}

func solve(r request) (map[string]any, error) {
	if len(r.Marshal) > 256*1024 || len(r.Salt) > 4096 || r.Engine != "3.9.15.297907" || r.Patch != "3.9.43.301387" {
		return nil, fmt.Errorf("unsupported_profile")
	}
	raw, err := base64.StdEncoding.DecodeString(r.Marshal)
	if err != nil {
		return nil, fmt.Errorf("invalid_module")
	}
	// PycRepairer only decodes the custom marshal and remaps opcodes.
	repaired, err := pymarshal.PycRepairer(raw, true)
	if err != nil {
		return nil, fmt.Errorf("invalid_module")
	}
	prefix, end, err := readLiteral(repaired, 246)
	if err != nil {
		return nil, err
	}
	suffix, tail, err := readLiteral(repaired, end)
	if err != nil {
		return nil, err
	}
	normalized := append([]byte{}, repaired[:246]...)
	normalized = append(normalized, literal("<prefix>")...)
	normalized = append(normalized, literal("<suffix>")...)
	normalized = append(normalized, repaired[tail:]...)
	digest := sha256.Sum256(normalized)
	if hex.EncodeToString(digest[:]) != templateHash {
		return nil, fmt.Errorf("unsupported_template")
	}
	calc := func(value string) string {
		v := md5.Sum([]byte(prefix + value + suffix))
		return hex.EncodeToString(v[:])
	}
	val := calc(r.Salt + "0")
	var tmp strings.Builder
	for _, c := range val {
		tmp.WriteString(strconv.Itoa((int(c)*2 + 5) ^ 255))
	}
	tmp.WriteString(r.Engine + "android" + r.Patch + "android" + "2" + "12" + strconv.FormatInt(r.UniqueID, 10))
	num := calc(tmp.String())
	sign := calc(val[16:] + "False[]3" + num + val[:16])
	return map[string]any{"response": []any{val, sign, false, []any{}, "", "", 3, num}, "template_sha256": templateHash}, nil
}

func handle(line []byte) (reply map[string]any) {
	defer func() {
		if recover() != nil {
			reply = map[string]any{"error": "invalid_module"}
		}
	}()
	var r request
	if err := json.NewDecoder(bytes.NewReader(line)).Decode(&r); err != nil {
		return map[string]any{"error": "invalid_request"}
	}
	result, err := solve(r)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"result": result}
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 512*1024)
	writer := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		b, _ := json.Marshal(handle(scanner.Bytes()))
		writer.Write(b)
		writer.WriteByte('\n')
		writer.Flush()
	}
}
