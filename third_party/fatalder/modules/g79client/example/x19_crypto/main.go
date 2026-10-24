package main

import (
	"fmt"
	"log"
	"os"

	g79client "github.com/Yeah114/g79client"
)

func main() {
	files := []string{
		"request_content.data",
		"response_content.data",
	}

	for _, name := range files {
		path := name
		plaintext, err := decryptFile(path)
		if err != nil {
			log.Fatalf("decrypt %s failed: %v", name, err)
		}
		fmt.Printf("==== %s ====\n%s\n\n", name, plaintext)
	}
}

func decryptFile(path string) (string, error) {
	cipherData, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	plainWithPadding, err := g79client.X19HttpDecrypt(cipherData)
	if err != nil {
		return "", err
	}

	return string(plainWithPadding), nil
}
