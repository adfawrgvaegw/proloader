package defines

import "github.com/google/uuid"

type InterruptConfig struct {
	Name   string        `json:"name"`
	UUID   uuid.UUID     `json:"uuid"`
	Server ControlConfig `json:"server"`
	Data   []string      `json:"data"`
}
