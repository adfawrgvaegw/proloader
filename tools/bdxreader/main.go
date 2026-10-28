// Offline adapter. Reuses the unmodified Fatalder bdump command decoder and
// pinned block mapper. Decode errors are fatal instead of being skipped.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Yeah114/bdump/command"
	"github.com/Yeah114/blocks"
	"github.com/andybalholm/brotli"
)

func run(input io.Reader, output io.Writer, poolPath string) error {
	header := make([]byte, 3)
	if _, err := io.ReadFull(input, header); err != nil {
		return err
	}
	if string(header) != "BD@" {
		return fmt.Errorf("invalid BD@ header")
	}
	// Bound decompression, including malformed strings and debug payloads.
	raw, err := io.ReadAll(io.LimitReader(brotli.NewReader(input), 256*1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 256*1024*1024 {
		return fmt.Errorf("BDX decompressed data exceeds 256 MiB")
	}
	reader := bufio.NewReader(bytes.NewReader(raw))
	if _, err = io.ReadFull(reader, header); err != nil {
		return err
	}
	if string(header) != "BDX" {
		return fmt.Errorf("invalid BDX metadata")
	}
	// BDX\0, author\0 (also matches the bundled upstream exporter).
	if b, e := reader.ReadByte(); e != nil || b != 0 {
		return fmt.Errorf("missing BDX metadata separator")
	}
	author, err := reader.ReadString(0)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(output)
	if err = enc.Encode(map[string]any{"author": strings.TrimSuffix(author, "\x00")}); err != nil {
		return err
	}
	constants := map[uint16]string{}
	next := uint32(0)
	poolID := -1
	var pool [][2]json.RawMessage
	cache := map[string]map[string]any{}
	pos := [3]int64{}
	movement := map[uint16][2]int64{6: {2, 0}, 8: {2, 1}, 12: {2, 0}, 14: {0, 1}, 15: {0, -1}, 16: {1, 1}, 17: {1, -1}, 18: {2, 1}, 19: {2, -1}, 20: {0, 0}, 21: {0, 0}, 22: {1, 0}, 23: {1, 0}, 24: {2, 0}, 25: {2, 0}, 28: {0, 0}, 29: {1, 0}, 30: {2, 0}}
	for count := 0; count < 16_000_000; count++ {
		// Validate debug length before the upstream decoder allocates its buffer.
		if peek, _ := reader.Peek(5); len(peek) > 0 && peek[0] == 39 {
			if len(peek) < 5 {
				return io.ErrUnexpectedEOF
			}
			n := uint64(peek[1])<<24 | uint64(peek[2])<<16 | uint64(peek[3])<<8 | uint64(peek[4])
			if n > uint64(len(raw)) {
				return fmt.Errorf("invalid debug payload length")
			}
		}
		cmd, e := command.ReadCommand(reader)
		if e != nil {
			return fmt.Errorf("BDX command %d: %w", count, e)
		}
		data, _ := json.Marshal(cmd)
		fields := map[string]any{}
		if e = json.Unmarshal(data, &fields); e != nil {
			return e
		}
		id := cmd.ID()
		if move, ok := movement[id]; ok {
			delta := move[1]
			if delta == 0 {
				delta = int64(fields["Value"].(float64))
			}
			pos[move[0]] += delta
			for _, v := range pos {
				if v < -2147483648 || v > 2147483647 {
					return fmt.Errorf("BDX coordinate overflow")
				}
			}
			continue
		}
		if id == 9 || id == 39 {
			continue
		}
		if id == 1 {
			if next > 65535 {
				return fmt.Errorf("too many constant strings")
			}
			constants[uint16(next)] = fields["ConstantString"].(string)
			next++
		}
		if id == 31 {
			poolID = int(fields["PoolID"].(float64))
			if poolID != 117 {
				return fmt.Errorf("unsupported runtime pool %d", poolID)
			}
			content, e := os.ReadFile(poolPath)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(content, &pool); e != nil {
				return e
			}
		}
		placement := id == 5 || id == 7 || id == 13 || id == 27 || (id >= 32 && id <= 38) || id == 40 || id == 41
		if placement {
			name := ""
			state := ""
			legacy := false
			value := uint16(0)
			if ref, ok := fields["BlockConstantStringID"]; ok {
				var found bool
				name, found = constants[uint16(ref.(float64))]
				if !found {
					return fmt.Errorf("undefined block constant")
				}
			}
			if ref, ok := fields["BlockStatesConstantStringID"]; ok {
				var found bool
				state, found = constants[uint16(ref.(float64))]
				if !found {
					return fmt.Errorf("undefined state constant")
				}
			}
			if v, ok := fields["BlockStatesString"]; ok {
				state = v.(string)
			}
			if v, ok := fields["BlockData"]; ok {
				legacy = true
				value = uint16(v.(float64))
			}
			if id == 36 {
				mode := int(fields["CommandBlockData"].(map[string]any)["Mode"].(float64))
				if mode < 0 || mode > 2 {
					return fmt.Errorf("invalid command block mode")
				}
				name = []string{"command_block", "repeating_command_block", "chain_command_block"}[mode]
			}
			if ref, ok := fields["BlockRuntimeID"]; ok {
				index := int(ref.(float64))
				if poolID != 117 || index >= len(pool) {
					return fmt.Errorf("runtime ID without valid pool/index")
				}
				if e = json.Unmarshal(pool[index][0], &name); e != nil {
					return e
				}
				if e = json.Unmarshal(pool[index][1], &value); e != nil {
					return e
				}
				legacy = true
			}
			key := fmt.Sprintf("%s|%s|%t|%d", name, state, legacy, value)
			mapped, ok := cache[key]
			if !ok {
				var rid uint32
				var found bool
				if legacy {
					rid, found = blocks.LegacyBlockToRuntimeID(name, value)
				} else {
					rid, found = blocks.BlockNameAndStateStrToRuntimeID(name, state)
				}
				if !found {
					return fmt.Errorf("unknown BDX block %s data=%d states=%s", name, value, state)
				}
				target, props, found := blocks.RuntimeIDToState(rid)
				if !found {
					return fmt.Errorf("unmapped BDX block %s", name)
				}
				if !strings.Contains(target, ":") {
					target = "minecraft:" + target
				}
				if target == "minecraft:unknown" || (target == "minecraft:air" && strings.TrimPrefix(name, "minecraft:") != "air") {
					return fmt.Errorf("invalid block mapping %s", name)
				}
				for k, v := range props {
					switch n := v.(type) {
					case uint8:
						props[k] = n != 0
					case int8:
						props[k] = n != 0
					}
				}
				mapped = map[string]any{"name": target, "states": props}
				cache[key] = mapped
			}
			fields["block"] = mapped
		}
		if id == 1 || id == 31 {
			continue
		}
		fields["position"] = pos
		fields["id"] = id
		if err = enc.Encode(fields); err != nil {
			return err
		}
		if id == 88 {
			// Consume signature/trailer bytes without making any authenticity claim.
			return nil
		}
	}
	return fmt.Errorf("BDX exceeds sixteen million commands")
}

func main() {
	if err := run(os.Stdin, os.Stdout, os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
