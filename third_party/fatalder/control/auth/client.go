package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/Yeah114/Fatalder/utils/auth"
	uc "github.com/Yeah114/Fatalder/utils/user_center"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/Yeah114/g79client"
)

const defaultAuthServer = "http://127.0.0.1:8080"

type secretLoadingTransport struct {
	secret string
}

func (s secretLoadingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.secret))
	return http.DefaultTransport.RoundTrip(req)
}

type ExtraClientInfo struct {
	EngineVersion string
	PatchVersion  string
	IsPC          bool
}

// Client implements bunkerauth.ClientAPI with optional user-center integration.
type Client struct {
	bunkerauth.ClientInfo
	ExtraClientInfo
	httpClient http.Client
	options    *bunkerauth.ClientOptions
	authServer string
}

var _ bunkerauth.ClientAPI = (*Client)(nil)

// CreateClient builds a client capable of talking to the bunker auth server.
// When the global user-center operator is available, requests are tunneled
// through the encrypted proxy to fetch the upstream secret automatically.
func CreateClient(options *bunkerauth.ClientOptions) (*Client, error) {
	opts := &bunkerauth.ClientOptions{}
	if options != nil {
		*opts = *options
	}

	authServer := strings.TrimSpace(opts.AuthServer)
	if authServer == "" {
		authServer = defaultAuthServer
	}
	opts.AuthServer = authServer

	if uc.GlobalOperator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		secretBody, err := uc.GlobalOperator.PhoenixNew(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to contact with API: %v", err)
		}

		opClient := uc.GlobalOperator.GetClient()
		opClient.Transport = secretLoadingTransport{secret: secretBody}

		return &Client{
			httpClient: http.Client{
				Transport: opClient.Transport,
				Timeout:   180 * time.Second,
			},
			options:    opts,
			authServer: authServer,
			ClientInfo: bunkerauth.ClientInfo{
				RespondTo: opts.RespondUserOverride,
			},
		}, nil
	}

	resp, err := http.Get(fmt.Sprintf("%s/api/new", authServer))
	if err != nil {
		return nil, fmt.Errorf("failed to contact with API: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	secret := string(body)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, errors.New("API server is down")
	} else if resp.StatusCode != http.StatusOK {
		return nil, parsedError(secret)
	}

	return &Client{
		httpClient: http.Client{
			Transport: secretLoadingTransport{secret: secret},
			Timeout:   180 * time.Second,
		},
		options:    opts,
		authServer: authServer,
		ClientInfo: bunkerauth.ClientInfo{
			RespondTo: opts.RespondUserOverride,
		},
	}, nil
}

func parsedError(message string) error {
	errorRegex := regexp.MustCompile("^(\\d{3} [a-zA-Z ]+)\n\n(.*?)($|\n)")
	matches := errorRegex.FindAllStringSubmatch(message, 1)
	if len(matches) == 0 {
		return fmt.Errorf("unknown error")
	}
	return fmt.Errorf("%s: %s", matches[0][1], matches[0][2])
}

func assertAndParse[T any](resp *http.Response) (T, error) {
	var out T
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return out, errors.New("API server is down")
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return out, parsedError(string(body))
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("error parsing API response: %v", err)
	}
	return out, nil
}

func checkAuth(authServer, token, serverCode, serverPassword string) (bool, error) {
	reqData := map[string]string{
		"auth_server":     authServer,
		"token":           token,
		"server_code":     serverCode,
		"server_password": serverPassword,
	}
	payload, err := json.Marshal(reqData)
	if err != nil {
		return false, fmt.Errorf("encode payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://funcore.yeah114.top/check", bytes.NewBuffer(payload))
	if err != nil {
		return false, fmt.Errorf("construct request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("non-200 status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read response: %v", err)
	}
	var result struct {
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("decode response: %v", err)
	}
	return result.Result, nil
}

func (c *Client) authServerURL() string {
	if strings.TrimSpace(c.authServer) == "" {
		return defaultAuthServer
	}
	return c.authServer
}

func (c *Client) Auth(
	ctx context.Context,
	serverCode, serverPassword, key, fbtoken, username, password string,
) (bunkerauth.AuthResponse, error) {
	var authResponse bunkerauth.AuthResponse

	if uc.GlobalOperator != nil {
		cookie, err := uc.GlobalOperator.PhoenixSAuth(ctx)
		if err != nil {
			return authResponse, err
		}
		client, err := g79client.NewClient()
		if err != nil {
			return authResponse, err
		}
		err = client.G79AuthenticateWithCookie(cookie)
		if err != nil {
			return authResponse, err
		}
		res, err := auth.Login(ctx, client, auth.LoginParams{
			ServerCode:      serverCode,
			ServerPassword:  serverPassword,
			ClientPublicKey: key,
		})
		success := err == nil
		info := "ok"
		if !success {
			info = err.Error()
		}
		botSkin, err := auth.GetSkinInfo(client)
		if err != nil {
			return authResponse, err
		}
		authResponse = bunkerauth.AuthResponse{
			SuccessStates: success,
			ServerMessage: "",
			Message: bunkerauth.Message{
				Information: info,
			},
			BotLevel: res.BotLevel,
			BotSkin: bunkerauth.SkinInfo{
				ItemID:          botSkin.ItemID,
				SkinDownloadURL: botSkin.SkinDownloadURL,
				SkinIsSlim:      botSkin.SkinIsSlim,
			},
			BotComponent:   res.BotComponent,
			FBToken:        "",
			MasterName:     res.MasterName,
			RentalServerIP: res.IP,
			ChainInfo:      res.ChainInfo,
		}
		if !authResponse.SuccessStates {
			return bunkerauth.AuthResponse{}, errors.New(authResponse.Message.Information)
		}
		if msg := strings.TrimSpace(authResponse.ServerMessage); msg != "" {
			utils.Log.Info(i18n.T(i18n.Auth_MessageFromAuthServer))
			utils.Log.Info(msg)
		}
		c.GrowthLevel = authResponse.BotLevel
		if name := strings.TrimSpace(authResponse.MasterName); name != "" && c.RespondTo == "" {
			c.RespondTo = name
		}
		c.EngineVersion = res.EngineVersion
		c.PatchVersion = res.PatchVersion
		c.IsPC = res.IsPC
		c.UserID = res.UID
		bunkerauth.Trust = true
		return authResponse, nil
	}

	result, err := checkAuth(c.authServerURL(), fbtoken, serverCode, serverPassword)
	if err != nil {
		return authResponse, fmt.Errorf("FunCore Update: %v", err)
	}
	if result {
		bunkerauth.Trust = true
	}
	utils.Log.Info("FunCore Update: OK")

	request := bunkerauth.AuthRequest{
		FBToken:         `{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"pc\",\"sdkuid\":\"aibgraaffciluq2g\",\"sessionid\":\"1-eyJzIjogInlnZWVwdTZoOGRodjA1bXlvcTliYTRmOWZ6OTJrZTdvIiwgIm9kaSI6ICJhbWF3cmFhYWF3cjV0N2tlLWQiLCAic2kiOiAiZDAwZjI2ZTU3MzE0M2VmYjg4NzYwOTBjY2Y1ZmQ2MzgyMThjZTA0NCIsICJ1IjogImFpYmdyYWFmZmNpbHVxMmciLCAidCI6IDIsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCJ9\",\"sdk_version\":\"3.9.0\",\"udid\":\"kro84mkhpdncv2dxx019cc3mfgil5w7m\",\"deviceid\":\"amawraaaawr5t7ke-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"client_login_sn\":\"CCDD2BD7C8B71E569DCBAE3F5FB27DF1\",\"gas_token\":\"\",\"source_platform\":\"pc\",\"ip\":\"127.0.0.1\"}"}`, //fbtoken,
		UserName:        username,
		Password:        password,
		ServerCode:      serverCode,
		ServerPassword:  serverPassword,
		ClientPublicKey: key,
	}
	payload, _ := json.Marshal(request)
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/phoenix/login", c.authServerURL()),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return authResponse, fmt.Errorf("auth: %v", err)
	}
	authResponse, err = assertAndParse[bunkerauth.AuthResponse](resp)
	if err != nil {
		return authResponse, err
	}
	if !authResponse.SuccessStates {
		errMsg := authResponse.Message.Information
		return bunkerauth.AuthResponse{}, errors.New(errMsg)
	}
	if msg := strings.TrimSpace(authResponse.ServerMessage); msg != "" {
		utils.Log.Info(i18n.T(i18n.Auth_MessageFromAuthServer))
		utils.Log.Info(msg)
	}
	c.GrowthLevel = authResponse.BotLevel
	if name := strings.TrimSpace(authResponse.MasterName); name != "" && c.RespondTo == "" {
		c.RespondTo = name
	}
	return authResponse, nil
}

func (c *Client) TransferData(content string) (string, error) {
	if uc.GlobalOperator != nil {
		data, err := auth.TransferStartType(c.UserID, content)
		if err != nil {
			return "", err
		}
		return data, nil
	}

	const maxRetries = 10
	const baseDelay = time.Second
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if lastErr != nil {
			utils.Log.Error(fmt.Sprintf("TransferData: %v", lastErr))
		}
		if attempt > 0 {
			delay := baseDelay * time.Duration(attempt)
			utils.Log.Info(fmt.Sprintf("Wait %d second to retry", attempt))
			time.Sleep(delay)
			utils.Log.Info("Retry TransferData...")
		}

		url := fmt.Sprintf("%s/api/phoenix/transfer_start_type?content=%s", c.authServerURL(), content)
		resp, err := c.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %v", err)
			continue
		}
		result, err := assertAndParse[map[string]any](resp)
		if err != nil {
			lastErr = fmt.Errorf("response parsing failed: %v", err)
			continue
		}
		succ, ok := result["success"].(bool)
		if !ok {
			lastErr = fmt.Errorf("invalid 'success' field type")
			continue
		}
		if !succ {
			errMsg, _ := result["message"].(string)
			lastErr = fmt.Errorf("server error: %s", errMsg)
			continue
		}
		data, ok := result["data"].(string)
		if !ok {
			lastErr = fmt.Errorf("invalid 'data' field type")
			continue
		}
		return data, nil
	}
	return "", fmt.Errorf("after %d attempts, last error: %v", maxRetries, lastErr)
}

func (c *Client) TransferCheckNum(req bunkerauth.FNumRequest) (string, error) {
	if uc.GlobalOperator != nil {
		ctx := context.TODO()
		req.EngineVersion = c.EngineVersion
		req.PatchVersion = c.PatchVersion
		req.IsPC = &c.IsPC
		resp, err := uc.GlobalOperator.TransferCheckNum(ctx, req)
		if err != nil {
			return "", err
		}
		succ, _ := resp["success"].(bool)
		if !succ {
			errMsg, _ := resp["message"].(string)
			return "", fmt.Errorf("failed to transfer check num: %s", errMsg)
		}
		if val, ok := resp["value"].(string); ok {
			return val, nil
		}
		return "", fmt.Errorf("invalid 'value' field type")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to encode json")
	}
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/phoenix/transfer_check_num", c.authServerURL()),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return "", err
	}
	result, err := assertAndParse[map[string]any](resp)
	if err != nil {
		return "", err
	}
	succ, _ := result["success"].(bool)
	if !succ {
		errMsg, _ := result["message"].(string)
		return "", fmt.Errorf("failed to transfer check num: %s", errMsg)
	}
	val, _ := result["value"].(string)
	return val, nil
}

func (c *Client) TanLobbyAuth(roomID, fbtoken string) (result tanauth.TanLobbyLoginResponse, err error) {
	roomID = strings.TrimPrefix(roomID, "TanLobby:")
	if uc.GlobalOperator != nil {
		ctx := context.TODO()
		cookie, err := uc.GlobalOperator.PhoenixSAuth(ctx)
		if err != nil {
			return result, err
		}
		client, err := g79client.NewClient()
		if err != nil {
			return result, err
		}
		err = client.G79AuthenticateWithCookie(cookie)
		if err != nil {
			return result, err
		}
		res, err := auth.TanLobbyLogin(ctx, client, auth.TanLobbyLoginParams{
			RoomID: roomID,
		})
		errorInfo := ""
		success := err == nil
		if !success {
			errorInfo = err.Error()
		}
		botSkin, err := auth.GetSkinInfo(client)
		if err != nil {
			return result, err
		}
		result = tanauth.TanLobbyLoginResponse{
			Success:        success,
			ErrorInfo:      errorInfo,
			UserUniqueID:   res.UserUniqueID,
			UserPlayerName: res.UserPlayerName,
			BotLevel:       res.BotLevel,
			BotSkin: tanauth.PhoenixSkinInfo{
				ItemID:          botSkin.ItemID,
				SkinDownloadURL: botSkin.SkinDownloadURL,
				SkinIsSlim:      botSkin.SkinIsSlim,
			},
			BotComponent:           res.BotComponent,
			RoomOwnerID:            res.RoomOwnerID,
			RoomModDisplayName:     res.RoomModDisplayName,
			RoomModDownloadURL:     res.RoomModDownloadURL,
			RoomModEncryptKey:      res.RoomModEncryptKey,
			RaknetServerAddress:    res.RaknetServerAddress,
			RaknetRand:             res.RaknetRand,
			RaknetAESRand:          res.RaknetAESRand,
			EncryptKeyBytes:        res.EncryptKeyBytes,
			DecryptKeyBytes:        res.DecryptKeyBytes,
			SignalingServerAddress: res.SignalingServerAddress,
			SignalingSeed:          res.SignalingSeed,
			SignalingTicket:        res.SignalingTicket,
		}
		return result, nil
	}
	req := tanauth.TanLobbyLoginRequest{FBToken: fbtoken, RoomID: roomID}
	payload, _ := json.Marshal(req)
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_login", c.authServerURL()),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return result, fmt.Errorf("TanLobbyAuth: %v", err)
	}
	result, err = assertAndParse[tanauth.TanLobbyLoginResponse](resp)
	if err != nil {
		return result, fmt.Errorf("TanLobbyAuth: %v", err)
	}
	return result, nil
}

func (c *Client) TanLobbyCreate(fbtoken string) (result tanauth.TanLobbyCreateResponse, err error) {
	if uc.GlobalOperator != nil {
		ctx := context.TODO()
		cookie, err := uc.GlobalOperator.PhoenixSAuth(ctx)
		if err != nil {
			return result, err
		}
		client, err := g79client.NewClient()
		if err != nil {
			return result, err
		}
		err = client.G79AuthenticateWithCookie(cookie)
		if err != nil {
			return result, err
		}
		res, err := auth.TanLobbyCreate(ctx, client)
		errorInfo := ""
		success := err == nil
		if !success {
			errorInfo = err.Error()
		}
		result = tanauth.TanLobbyCreateResponse{
			Success:                success,
			ErrorInfo:              errorInfo,
			UserUniqueID:           res.UserUniqueID,
			UserPlayerName:         res.UserPlayerName,
			RaknetServerAddress:    res.RaknetServerAddress,
			RaknetRand:             res.RaknetRand,
			RaknetAESRand:          res.RaknetAESRand,
			EncryptKeyBytes:        res.EncryptKeyBytes,
			DecryptKeyBytes:        res.DecryptKeyBytes,
			SignalingServerAddress: res.SignalingServerAddress,
			SignalingSeed:          res.SignalingSeed,
			SignalingTicket:        res.SignalingTicket,
		}
		return result, nil
	}
	req := tanauth.TanLobbyCreateRequest{FBToken: fbtoken}
	payload, _ := json.Marshal(req)
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_create", c.authServerURL()),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: %v", err)
	}
	result, err = assertAndParse[tanauth.TanLobbyCreateResponse](resp)
	if err != nil {
		return result, fmt.Errorf("TanLobbyCreate: %v", err)
	}
	return result, nil
}

func (c *Client) TanLobbyDebug(loginResponse string, raknetRand []byte, fbtoken string) (result tanauth.TanLobbyDebugResponse, err error) {
	req := tanauth.TanLobbyDebugRequest{
		FBToken:       fbtoken,
		LoginResponse: loginResponse,
		RaknetRand:    raknetRand,
	}
	payload, _ := json.Marshal(req)
	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_debug", c.authServerURL()),
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}
	result, err = assertAndParse[tanauth.TanLobbyDebugResponse](resp)
	if err != nil {
		return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}
	return result, nil
}

// AuthServer exposes the upstream auth server address.
func (c *Client) AuthServer() string {
	return c.authServerURL()
}
