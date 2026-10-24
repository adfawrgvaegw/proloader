package structure

import (
	"os"
	"testing"

	"github.com/Yeah114/WaterStructure/define"
)

func TestFuHongV4_FromFile_ParsesContainerCommandAndSignNBT(t *testing.T) {
	jsonText := `{"FuHongBuild":[{"startX":0,"startZ":0,"block":[` +
		`[1,0,[0],[0],[0],[[["stone",0,1,0]]]],` + // chest with 1 item
		`[2,0,[1],[0],[0],[["/say hi",3,1,"Tester"]]],` + // command block
		`[3,0,[2],[0],[0],["hello"]]` + // sign text
		`]}],"BlocksList":["minecraft:air","minecraft:chest","minecraft:command_block","minecraft:wall_sign"]}`

	tmp, err := os.CreateTemp("", "fuhong_v4_*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.WriteString(jsonText); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}

	var f FuHongV4
	if err := f.FromFile(tmp); err != nil {
		t.Fatalf("FromFile: %v", err)
	}

	nbts, err := f.GetChunksNBT([]define.ChunkPos{{0, 0}})
	if err != nil {
		t.Fatalf("GetChunksNBT: %v", err)
	}
	chunkNBT := nbts[define.ChunkPos{0, 0}]
	if len(chunkNBT) != 3 {
		t.Fatalf("expected 3 block entities, got %d", len(chunkNBT))
	}

	// chest at (0,0,0)
	chestNBT := chunkNBT[define.BlockPos{0, chunkLocalYFromWorld(0), 0}]
	if chestNBT == nil || chestNBT["id"] != "Chest" {
		t.Fatalf("expected chest nbt id Chest, got %v", chestNBT)
	}
	items, ok := chestNBT["Items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected chest Items length 1, got %T %v", chestNBT["Items"], chestNBT["Items"])
	}
	if items[0]["Name"] != "minecraft:stone" {
		t.Fatalf("expected item Name minecraft:stone, got %v", items[0]["Name"])
	}
	if _, ok := items[0]["Block"].(map[string]any); !ok {
		t.Fatalf("expected item Block compound, got %T", items[0]["Block"])
	}

	// command block at (1,0,0)
	cmdNBT := chunkNBT[define.BlockPos{1, chunkLocalYFromWorld(0), 0}]
	if cmdNBT == nil || cmdNBT["id"] != "CommandBlock" {
		t.Fatalf("expected command nbt id CommandBlock, got %v", cmdNBT)
	}
	if cmdNBT["Command"] != "/say hi" {
		t.Fatalf("expected Command /say hi, got %v", cmdNBT["Command"])
	}
	if cmdNBT["CustomName"] != "Tester" {
		t.Fatalf("expected CustomName Tester, got %v", cmdNBT["CustomName"])
	}
	if cmdNBT["auto"] != byte(1) {
		t.Fatalf("expected auto byte(1), got %T %v", cmdNBT["auto"], cmdNBT["auto"])
	}

	// sign at (2,0,0)
	signNBT := chunkNBT[define.BlockPos{2, chunkLocalYFromWorld(0), 0}]
	if signNBT == nil {
		t.Fatalf("expected sign nbt, got nil")
	}
	front, ok := signNBT["FrontText"].(map[string]any)
	if !ok || front["Text"] != "hello" {
		t.Fatalf("expected FrontText.Text hello, got %v", signNBT["FrontText"])
	}
}

