package main

import (
	"fmt"

	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		panic(err)
	}
	resp, err := client.PurchaseItem("4673747905260015239")
	if err != nil {
		panic(err)
	}
	fmt.Println(resp)
	resp2, err := client.GetDownloadInfo("4673747905260015239")
	if err != nil {
		panic(err)
	}
	fmt.Println(resp2)
}