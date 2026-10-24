package main

import (
	//"fmt"
	"os"

	"github.com/mholt/archiver/v3"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/TriM-Organization/bedrock-world-operator/define"
)

func main() {
	// 解压 test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087].mcworld 到 test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087]
	os.RemoveAll("test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087]")
	z := archiver.Zip{}
	err := z.Unarchive("test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087].mcworld", "test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087]")
	if err != nil {
		panic(err)
	}
	bedrockWorld, err := world.Open("test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087]", nil)
	if err != nil {
		panic(err)
	}
	/*
	// 遍历读取所有 [2000,104,20000]~[2089,154,20087] 区块
	for x := 2000 / 16; x <= 2089 / 16; x++ {
		for z := 20000 / 16; z <= 20087 / 16; z++ {
			bedrockWorld.LoadChunk(define.Dimension(0), define.ChunkPos{int32(x), int32(z)})
			fmt.Println(x, z)
		}
	}
	*/
	// 130 1255
	bedrockWorld.LoadChunk(define.Dimension(0), define.ChunkPos{int32(130), int32(1255)})
	bedrockWorld.CloseWorld()
	os.RemoveAll("test/测试文件/色盲派对new@[2000,104,20000]~[2089,154,20087]")
}