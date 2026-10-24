package main

import (
	"fmt"
	"os"

	"github.com/Yeah114/WaterStructure/structure"
)

func main() {
	file, err := os.Open("1_冰河禁区.bdx")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	s, err := structure.StructureFromFile(file)
	if err != nil {
		panic(err)
	}
	defer s.Close()
	fmt.Println(s.GetSize())
}
