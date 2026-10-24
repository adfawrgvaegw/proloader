package main

import (
	"fmt"
	"os"

	"github.com/Yeah114/WaterStructure/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/structure"
	"github.com/schollz/progressbar/v3"
)

func main() {
	bdx := structure.AxiomBP{}
	file, err := os.OpenFile("test.bp", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	bedrockWorld, _ := world.Open("gkm", nil)
	var bar *progressbar.ProgressBar
	err = bdx.FromMCWorld(
		bedrockWorld,
		file,
		define.BlockPos{
			-2,
			15,
			-1,
		},
		define.BlockPos{
			84,
			55,
			83,
		},
		func(total int) {
			fmt.Printf("需要处理的子区块总数: %d\n", total)
			if total <= 0 {
				bar = nil
				return
			}
			bar = progressbar.NewOptions(
				total,
				progressbar.OptionSetWriter(os.Stdout),
				progressbar.OptionSetDescription("写入进度"),
				progressbar.OptionSetWidth(40),
				progressbar.OptionSetTheme(progressbar.Theme{
					Saucer:        "#",
					SaucerHead:    ">",
					SaucerPadding: "-",
					BarStart:      "[",
					BarEnd:        "]",
				}),
				progressbar.OptionShowCount(),
				progressbar.OptionSetPredictTime(true),
				progressbar.OptionShowElapsedTimeOnFinish(),
				progressbar.OptionSetRenderBlankState(true),
				progressbar.OptionOnCompletion(func() {
					fmt.Println()
				}),
			)
		},
		func() {
			if bar != nil {
				_ = bar.Add(1)
			}
		},
	)
	if err != nil {
		panic(err)
	}
	file.Close()
}
