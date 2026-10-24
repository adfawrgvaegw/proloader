package utils

import "github.com/Yeah114/Fatalder/cmd/args"

func RegistTemporaryAPIKeyFlag() {
	args.Flags().StringP(
		"API-Key",
		"a",
		"",
		"Set temporary API-Key",
	)
}

func GetTemporaryAPIKey() string {
	key, _ := args.Flags().GetString("API-Key")
	return key
}
