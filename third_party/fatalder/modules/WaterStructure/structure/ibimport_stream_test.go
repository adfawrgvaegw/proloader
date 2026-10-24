package structure

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestIBImportBoolishUnmarshal(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`0`, false},
		{`1`, true},
		{`false`, false},
		{`true`, true},
		{`"0"`, false},
		{`"1"`, true},
		{`"false"`, false},
		{`"true"`, true},
	}

	for _, tt := range tests {
		var v ibImportBoolish
		if err := json.Unmarshal([]byte(tt.in), &v); err != nil {
			t.Fatalf("unmarshal %s failed: %v", tt.in, err)
		}
		if v.Bool() != tt.want {
			t.Fatalf("unmarshal %s got %v want %v", tt.in, v.Bool(), tt.want)
		}
	}
}

func TestIBImportCommandMessageDecodeFallback(t *testing.T) {
	cmd := ibImportCommand{
		CommandMessage: "say hello world",
	}
	nbt := buildIBImportCommandNBT(cmd)
	got, _ := nbt["Command"].(string)
	if got != cmd.CommandMessage {
		t.Fatalf("plain CommandMessage got %q want %q", got, cmd.CommandMessage)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte("say hi"))
	cmd2 := ibImportCommand{
		CommandMessage: encoded,
	}
	nbt2 := buildIBImportCommandNBT(cmd2)
	got2, _ := nbt2["Command"].(string)
	if got2 != "say hi" {
		t.Fatalf("base64 CommandMessage got %q want %q", got2, "say hi")
	}
}
