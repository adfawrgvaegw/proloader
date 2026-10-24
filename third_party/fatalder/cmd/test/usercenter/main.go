package main

import (
	"context"
	"fmt"

	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/utils/user_center"
)

func main() {
	if err := utils.InitStorage(); err != nil {
		panic(err)
	}
	if err := utils.InitConfig(); err != nil {
		panic(err)
	}
	if err := user_center.InitGlobalFromConfig(); err != nil {
		panic(err)
	}

	operator := user_center.GlobalOperator
	fmt.Println(operator.PhoenixSAuth(context.Background()))
}
