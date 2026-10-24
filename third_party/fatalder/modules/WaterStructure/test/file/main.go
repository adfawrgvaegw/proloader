package main

import (
	"fmt"
	"os"
	"time"
	//	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/WaterStructure/structure"
)

func main() {
	file, err := os.Open("(和平)冰场.building")
	if err != nil {
		panic(fmt.Sprintf("打开文件失败: %v", err))
	}
	defer file.Close()

	/*
		structureType, err := WaterStructure.GetStructureType(file)
		if err != nil {
			panic(fmt.Sprintf("判断文件类型失败: %v", err))
		}
	*/

	start := time.Now()
//	reader, _ := structure.StructureFromFile(file)
	reader := structure.MianYangV3{}
	err = reader.FromFile(file)
	if err != nil {
		panic(fmt.Sprintf("打开文件失败: %v", err))
	}
	duration := time.Since(start)
	fmt.Printf("解析文件耗时: %v\n", duration)
	size := reader.GetSize()
	fmt.Println(size)
	volume := size.GetVolume()
	fmt.Println(volume)
	nonAirBlocks, err := reader.CountNonAirBlocks()
	if err != nil {
		panic(fmt.Sprintf("获取非空气方块数量失败: %v", err))
	}
	fmt.Println(nonAirBlocks)
}
