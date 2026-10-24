package main

import (
	"fmt"
	"os"

	"github.com/Yeah114/WaterStructure/utils/json"
)

func main() {
	file, _ := os.Open("/root/WaterStructure/structure/bdx_runtimeIds_117.json")
	reader := sjson.NewJSONReader(file)
	_ = reader.BeginArray()
	fmt.Println(reader.BeginArray())
	fmt.Println(reader.ReadString())
	fmt.Println(reader.ReadInt64())
	reader.NextObjectKey()
	reader.SkipValue()
	reader.BeginArray()
	fmt.Println(reader.ReadString())
	fmt.Println(reader.ReadInt64())
}