package mpay

import (
	"encoding/json"
	"errors"
	"strings"
)

type Sauth struct {
	AimInfo          string `json:"aim_info"`
	AppChannel       string `json:"app_channel"`
	ClientLoginSN    string `json:"client_login_sn"`
	DeviceID         string `json:"deviceid"`
	GameID           string `json:"gameid"`
	GasToken         string `json:"gas_token"`
	GetAccessToken   string `json:"get_access_token"`
	IP               string `json:"ip"`
	IsUnisdkGuest    int    `json:"is_unisdk_guest"`
	LoginChannel     string `json:"login_channel"`
	Platform         string `json:"platform"`
	SDKVersion       string `json:"sdk_version"`
	SDKUID           string `json:"sdkuid"`
	SessionID        string `json:"sessionid"`
	SourceAppChannel string `json:"source_app_channel"`
	SourcePlatform   string `json:"source_platform"`
	UDID             string `json:"udid"`
}

func newSauth(deviceID, sdkuid, sessionID, udid string, isGuest bool) Sauth {
	guestFlag := 0
	if isGuest {
		guestFlag = 1
	}
	return Sauth{
		AimInfo:          "{\"aim\":\"127.0.0.1\",\"country\":\"CN\",\"tz\":\"+0800\",\"tzid\":\"\"}",
		AppChannel:       "netease",
		ClientLoginSN:    randomHexUpperString(16),
		DeviceID:         deviceID,
		GameID:           "x19",
		GasToken:         "",
		GetAccessToken:   "1",
		IP:               "127.0.0.1",
		IsUnisdkGuest:    guestFlag,
		LoginChannel:     "netease",
		Platform:         "pc",
		SDKVersion:       "3.9.0",
		SDKUID:           sdkuid,
		SessionID:        sessionID,
		SourceAppChannel: "netease",
		SourcePlatform:   "pc",
		UDID:             udid,
	}
}

type User struct {
	Sauth    Sauth  `json:"sauth"`
	Mac      string `json:"mac"`
	IsGuest  bool   `json:"is_guest"`
	Ram      string `json:"ram"`
	Rom      string `json:"rom"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}

func newUser(deviceID, sdkuid, sessionID, udid, mac string, isGuest bool, ram, rom, email, password string) *User {
	return &User{
		Sauth:    newSauth(deviceID, sdkuid, sessionID, udid, isGuest),
		Mac:      mac,
		IsGuest:  isGuest,
		Ram:      ram,
		Rom:      rom,
		Email:    email,
		Password: password,
	}
}

func (u *User) CookieString() (string, error) {
	if u == nil {
		return "", errors.New("mpay: user 不能为空")
	}
	sauthJSON, err := json.Marshal(u.Sauth)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"sauth_json": string(sauthJSON),
	}
	cookie, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(cookie), nil
}

func randomHexUpperString(byteLen int) string {
	value, err := randomHex(byteLen)
	if err != nil {
		return strings.ToUpper(strings.Repeat("0", byteLen*2))
	}
	return strings.ToUpper(value)
}
