package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/nbt_assigner"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	"github.com/pterm/pterm"
)

var (
	c         *client.Client
	resources *resources_control.Resources
	api       *game_interface.GameInterface
)

var console *nbt_console.Console

func main() {
	tA := time.Now()

	SystemTestingLogin()
	defer func() {
		c.Conn().Close()
		time.Sleep(time.Second)
	}()

	// SystemTestingItemCache()
	// SystemTestingBaseContainerCache()

	{
		bs, _ := os.ReadFile("x.mcstructure")
		// fmt.Println(bs)

		var m map[string]any
		buf := bytes.NewBuffer(bs)
		nbt.NewDecoderWithEncoding(buf, nbt.LittleEndian).Decode(&m)

		m = m["structure"].(map[string]any)["palette"].(map[string]any)["default"].(map[string]any)
		m = m["block_position_data"].(map[string]any)["0"].(map[string]any)["block_entity_data"].(map[string]any)

		// kk := []nbt_parser_item.Shield{}
		// for _, value := range block.(*nbt_parser_block.Container).NBT.Items {
		// 	kk = append(kk, *value.Item.(*nbt_parser_item.Shield))
		// }

		cache := nbt_cache.NewNBTCacheSystem(console)
		a := nbt_assigner.NewNBTAssigner(console, cache)
		fmt.Println(
			a.PlaceNBTBlock(
				"minecraft:chest",
				map[string]any{
					// "crafting":      byte(0),
					// "orientation":   "west_up",
					// "triggered_bit": byte(0),
					"minecraft:cardinal_direction": "north",
					// "powered_bit":                  byte(0),
					// "brewing_stand_slot_b_bit":     byte(1),
					// "brewing_stand_slot_c_bit":     byte(1),
				},
				m,
			),
		)

		// fmt.Println(b.Make())
		// fmt.Println(b.Make())
		// fmt.Println(b.Make())
		// fmt.Println(b.Make())

		// for {
		// 	mm, err := b.Make()
		// 	fmt.Println(mm, err)
		// 	if len(mm) == 0 {
		// 		break
		// 	}

		// 	api.ContainerOpenAndClose().OpenInventory()
		// 	tt := api.ItemStackOperation().OpenTransaction()
		// 	for _, value := range mm {
		// 		tt.DropInventoryItem(value, 1)
		// 	}
		// 	fmt.Println(tt.Commit())
		// 	api.ContainerOpenAndClose().CloseContainer()
		// }
	}

	// cache := nbt_cache.NewNBTCacheSystem(console)
	// a := nbt_assigner.NewNBTAssigner(console, cache)

	// for idx := 0; idx <= 0; idx++ {
	// 	var req PlaceNBTBlockRequest
	// 	var blockNBT map[string]any

	// 	bs, err := os.ReadFile(fmt.Sprintf("logs/%d.log", idx))
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	if err = json.Unmarshal(bs, &req); err != nil {
	// 		panic(err)
	// 	}

	// 	blockNBTBytes, err := base64.StdEncoding.DecodeString(req.BlockNBTBase64String)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	err = nbt.UnmarshalEncoding(blockNBTBytes, &blockNBT, nbt.LittleEndian)
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	// kk := []nbt_parser_item.Shield{}
	// 	// for _, value := range block.(*nbt_parser_block.Container).NBT.Items {
	// 	// 	kk = append(kk, *value.Item.(*nbt_parser_item.Shield))
	// 	// }

	// 	fmt.Printf("%d ", idx)
	// 	k1, k2, k3, k4 := a.PlaceNBTBlock(
	// 		req.BlockName,
	// 		utils.ParseBlockStatesString(req.BlockStatesString),
	// 		blockNBT,
	// 	)
	// 	fmt.Println(k1, k2, k3, k4)
	// 	if k4 != nil {
	// 		panic(idx)
	// 	}
	// }

	pterm.Success.Printfln("System Testing: ALL PASS (Time used = %v)", time.Since(tA))
}

type PlaceNBTBlockRequest struct {
	BlockName            string `json:"block_name"`
	BlockStatesString    string `json:"block_states_string"`
	BlockNBTBase64String string `json:"block_nbt_base64_string"`
}
