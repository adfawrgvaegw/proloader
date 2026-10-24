package main

import (
	"fmt"

	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/utils/data_manager"
)

func main() {
	fmt.Println(6)
	if err := utils.InitStorage(); err != nil {
		panic(err)
	}
	fmt.Println(6)
	if err := utils.InitConfig(); err != nil {
		panic(err)
	}
	fmt.Println(6)
	dataManager := data_manager.NewDataManager(utils.GetStoragePath())

	fmt.Println(dataManager.ListServerConfigs())
}
