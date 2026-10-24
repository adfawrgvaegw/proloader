package defines

import (
	"encoding/json"
)

type ControlConfig struct {
	AuthServerAddress    string `json:"auth_server_address,omitempty"`
	AuthServerToken      string `json:"auth_server_token,omitempty"`
	AuthUsername         string `json:"auth_username,omitempty"`
	AuthPassword         string `json:"auth_password,omitempty"`
	RentalServerCode     string `json:"rental_server_code"`
	RentalServerPasscode string `json:"rental_server_password"`
}

func NewControlConfig() *ControlConfig {
	return &ControlConfig{}
}

func (c *ControlConfig) FromJSON(str string) error {
	return json.Unmarshal([]byte(str), c)
}

func (c *ControlConfig) ToJSON() (string, error) {
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
