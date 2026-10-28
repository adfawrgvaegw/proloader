// Packet-only adapter for the upstream sign editor. Never opens a game connection.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	oldprotocol "github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	oldpacket "github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/blocks"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type request struct {
	Action         string         `json:"action"`
	Protocol       int            `json:"protocol"`
	Position       [3]int32       `json:"position"`
	PlayerPosition mgl32.Vec3     `json:"player_position"`
	Name           string         `json:"name"`
	States         map[string]any `json:"states"`
	Front          string         `json:"front"`
	Back           string         `json:"back"`
	FrontPersist   uint8          `json:"front_persist"`
	BackPersist    uint8          `json:"back_persist"`
	Tick           uint64         `json:"tick"`
	ItemName       string         `json:"item_name"`
	Damage         uint16         `json:"damage"`
	BlockNBT       struct {
		Name   string         `json:"name"`
		States map[string]any `json:"states"`
	} `json:"block_nbt"`
}
type encoded struct {
	ID  uint32 `json:"id"`
	Hex string `json:"hex"`
}

func encode(pk packet.Packet) encoded {
	buf := bytes.NewBuffer(nil)
	pk.Marshal(protocol.NewWriter(buf, 0))
	return encoded{pk.ID(), hex.EncodeToString(buf.Bytes())}
}
func encodeNetease(pk oldpacket.Packet) encoded {
	buf := bytes.NewBuffer(nil)
	pk.Marshal(oldprotocol.NewWriter(buf, 0))
	return encoded{pk.ID(), hex.EncodeToString(buf.Bytes())}
}
func signPackets(r request) (out []encoded, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("sign codec: %v", v)
		}
	}()
	if r.Protocol != 860 && !(r.Protocol == 686 && r.Action == "teleport_ack") {
		return nil, fmt.Errorf("sign editor is configured for Netease wire profile 860; live acceptance pending")
	}
	pos := protocol.BlockPos(r.Position)
	switch r.Action {
	case "validate_item_block":
		rid, ok := blocks.LegacyBlockToRuntimeID(r.ItemName, r.Damage)
		if !ok {
			return nil, fmt.Errorf("unknown legacy item block")
		}
		name, states, ok := blocks.RuntimeIDToState(rid)
		if !ok {
			return nil, fmt.Errorf("unknown item state")
		}
		expected, ok := block.StateToRuntimeID(name, states)
		if !ok {
			return nil, fmt.Errorf("unmapped item state")
		}
		for k, v := range r.BlockNBT.States {
			if n, ok := v.(float64); ok {
				r.BlockNBT.States[k] = int32(n)
			}
		}
		actual, ok := block.StateToRuntimeID(r.BlockNBT.Name, r.BlockNBT.States)
		if !ok || expected != actual {
			return nil, fmt.Errorf("item block states cannot be recreated by name/data")
		}
		return []encoded{}, nil
	case "sign_text":
		if r.FrontPersist > 1 || r.BackPersist > 1 {
			return nil, fmt.Errorf("invalid sign formatting flag")
		}
		nbt := map[string]any{"FrontText": map[string]any{"Text": r.Front, "PersistFormatting": r.FrontPersist},
			"BackText": map[string]any{"Text": r.Back, "PersistFormatting": r.BackPersist}}
		modern := encode(&packet.BlockActorData{Position: pos, NBTData: nbt})
		// Actor payload is unchanged between the pinned upstream and profile860.
		buf := bytes.NewBuffer(nil)
		(&oldpacket.BlockActorData{Position: oldprotocol.BlockPos(r.Position), NBTData: nbt}).Marshal(oldprotocol.NewWriter(buf, 0))
		decoded := &packet.BlockActorData{}
		decoded.Marshal(protocol.NewReader(bytes.NewBuffer(buf.Bytes()), 0, true))
		if decoded.Position != pos || !reflect.DeepEqual(decoded.NBTData, nbt) {
			return nil, fmt.Errorf("block actor codec profiles disagree")
		}
		return []encoded{modern}, nil
	case "sign_click":
		for k, v := range r.States {
			if n, ok := v.(float64); ok {
				r.States[k] = int32(n)
			}
		}
		rid, ok := block.StateToRuntimeID(r.Name, r.States)
		if !ok {
			return nil, fmt.Errorf("unknown sign runtime state")
		}
		return []encoded{
			encodeNetease(&oldpacket.PlayerHotBar{SelectedHotBarSlot: 0, WindowID: 0, SelectHotBarSlot: true}),
			encodeNetease(&oldpacket.InventoryTransaction{TransactionData: &oldprotocol.UseItemTransactionData{
				ActionType:    oldprotocol.UseItemActionClickBlock,
				BlockPosition: oldprotocol.BlockPos(r.Position), BlockFace: 0, HotBarSlot: 0, Position: r.PlayerPosition, BlockRuntimeID: rid,
			}}),
		}, nil
	case "auth", "teleport_ack":
		if r.Protocol == 860 {
			// NEMC860 schema: NTvector commit1fee42c2, PlayerAuthInput.h.
			// Vanilla omits bypassMoveVector and the 12-byte camera/ground tail.
			buf := bytes.NewBuffer(nil)
			w := protocol.NewWriter(buf, 0)
			zero := float32(0)
			no := false
			yes := true
			mouse := uint32(1)
			none := uint32(0)
			vec2 := mgl32.Vec2{}
			vec3 := mgl32.Vec3{}
			flags := uint64(1) << 42
			if r.Action == "teleport_ack" {
				flags = uint64(1) << 37
			}
			w.Float32(&zero)
			w.Float32(&zero)
			w.Vec3(&r.PlayerPosition)
			w.Vec2(&vec2)
			w.Float32(&zero)
			w.Varuint64(&flags)
			w.Varuint32(&mouse)
			w.Varuint32(&none)
			w.Varuint32(&none)
			w.Float32(&zero)
			w.Float32(&zero)
			w.Varuint64(&r.Tick)
			w.Vec3(&vec3)
			w.Bool(&no)
			w.Vec2(&vec2)
			w.Vec3(&vec3)
			w.Vec2(&vec2)
			w.Bool(&no)
			w.Float32(&zero)
			w.Float32(&zero)
			w.Bool(&no)
			w.Bool(&yes)
			w.Bool(&no)
			return []encoded{{ID: 144, Hex: hex.EncodeToString(buf.Bytes())}}, nil
		}
		flags := uint64(oldpacket.InputFlagStartFlying)
		if r.Action == "teleport_ack" {
			flags = oldpacket.InputFlagHandledTeleport
		}
		return []encoded{encodeNetease(&oldpacket.PlayerAuthInput{Position: r.PlayerPosition, InputData: flags, Tick: r.Tick})}, nil
	default:
		return nil, fmt.Errorf("unknown codec action")
	}
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--stream" {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		encoder := json.NewEncoder(os.Stdout)
		for scanner.Scan() {
			var r request
			err := json.Unmarshal(scanner.Bytes(), &r)
			var out []encoded
			if err == nil {
				out, err = signPackets(r)
			}
			if err != nil {
				if encoder.Encode(map[string]any{"error": "invalid codec input"}) != nil {
					os.Exit(1)
				}
			} else if encoder.Encode(map[string]any{"result": map[string]any{"packets": out}}) != nil {
				os.Exit(1)
			}
		}
		if scanner.Err() != nil {
			os.Exit(1)
		}
		return
	}
	var r request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := signPackets(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
		os.Exit(1)
	}
}
