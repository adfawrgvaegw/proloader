package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/utils"
)

var Trust bool = false
var NoTrustRandIntRange int = 2

type secretLoadingTransport struct {
	secret string
}

func (s secretLoadingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.secret))
	return http.DefaultTransport.RoundTrip(req)
}

type ClientOptions struct {
	AuthServer          string
	RespondUserOverride string
}

func MakeDefaultClientOptions() *ClientOptions {
	return &ClientOptions{}
}

type ClientInfo struct {
	UserID      string
	GrowthLevel int
	RespondTo   string
}

type Client struct {
	ClientInfo
	client http.Client
	*ClientOptions
}

// ClientAPI captures the behaviour required by clients of this package.
type ClientAPI interface {
	Auth(ctx context.Context, serverCode string, serverPassword string, key string, fbtoken string, username string, password string) (AuthResponse, error)
	TransferData(content string) (string, error)
	TransferCheckNum(req FNumRequest) (string, error)
	TanLobbyAuth(roomID string, fbtoken string) (tanauth.TanLobbyLoginResponse, error)
	TanLobbyCreate(fbtoken string) (tanauth.TanLobbyCreateResponse, error)
	TanLobbyDebug(loginResponse string, raknetRand []byte, fbtoken string) (tanauth.TanLobbyDebugResponse, error)
}

var _ ClientAPI = (*Client)(nil)

func parsedError(message string) error {
	error_regex := regexp.MustCompile("^(\\d{3} [a-zA-Z ]+)\n\n(.*?)($|\n)")
	err_matches := error_regex.FindAllStringSubmatch(message, 1)
	if len(err_matches) == 0 {
		return fmt.Errorf("unknown error")
	}
	return fmt.Errorf("%s: %s", err_matches[0][1], err_matches[0][2])
}

func assertAndParse[T any](resp *http.Response) (T, error) {
	var ret T
	if resp.StatusCode == 503 {
		return ret, errors.New("API server is down")
	}
	_body, _ := io.ReadAll(resp.Body)
	body := string(_body)
	if resp.StatusCode != 200 {
		return ret, parsedError(body)
	}
	err := json.Unmarshal([]byte(body), &ret)
	if err != nil {
		return ret, fmt.Errorf("error parsing API response: %v", err)
	}
	return ret, nil
}

func CreateClient(options *ClientOptions) (*Client, error) {
	secret_res, err := http.Get(fmt.Sprintf("%s/api/new", options.AuthServer))
	if err != nil {
		return nil, fmt.Errorf("failed to contact with API: %v", err)
	}
	_secret_body, _ := io.ReadAll(secret_res.Body)
	secret_body := string(_secret_body)
	if secret_res.StatusCode == 503 {
		return nil, errors.New("API server is down")
	} else if secret_res.StatusCode != 200 {
		return nil, parsedError(secret_body)
	}
	authclient := &Client{
		client: http.Client{
			Transport: secretLoadingTransport{
				secret: secret_body,
			},
			Timeout: 180 * time.Second,
		},
		ClientOptions: options,
		ClientInfo: ClientInfo{
			RespondTo: options.RespondUserOverride,
		},
	}
	return authclient, nil
}

type AuthRequest struct {
	FBToken string `json:"login_token,omitempty"`

	UserName string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	ServerCode     string `json:"server_code"`
	ServerPassword string `json:"server_passcode"`

	ClientPublicKey string `json:"client_public_key"`
}

type AuthResponse struct {
	SuccessStates bool   `json:"success"`
	ServerMessage string `json:"server_msg,omitempty"`
	Message

	BotLevel     int             `json:"growth_level"`
	BotSkin      SkinInfo        `json:"skin_info,omitempty"`
	BotComponent map[string]*int `json:"outfit_info,omitempty"`
	FBToken      string          `json:"token"`
	MasterName   string          `json:"respond_to"`

	RentalServerIP string `json:"ip_address"`
	ChainInfo      string `json:"chainInfo"`
}

type Message struct {
	Information string `json:"message,omitempty"`
	Translation int    `json:"translation,omitempty"`
}

type SkinInfo struct {
	ItemID          string `json:"entity_id"`
	SkinDownloadURL string `json:"res_url"`
	SkinIsSlim      bool   `json:"is_slim"`
}

func checkAuth(authUrl, token, serverCode, serverPassword string) (bool, error) {
	// 1. 构造请求数据
	requestData := map[string]string{
		"auth_server":     authUrl,
		"token":           token,
		"server_code":     serverCode,
		"server_password": serverPassword,
	}
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return false, fmt.Errorf("JSON编码失败: %v", err)
	}

	// 2. 创建HTTP请求
	req, err := http.NewRequest(
		"POST",
		"https://funcore.yeah114.top/check",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return false, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 3. 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 4. 处理响应
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("非200状态码: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %v", err)
	}

	// 5. 解析JSON
	var result struct {
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("JSON解析失败: %v", err)
	}

	return result.Result, nil
}

func (client *Client) Auth(
	ctx context.Context,
	serverCode string, serverPassword string,
	key string,
	fbtoken string, username string, password string,
) (AuthResponse, error) {
	var authResponse AuthResponse

	/*
		result, err := checkAuth(client.AuthServer, fbtoken, serverCode, serverPassword)
		if err != nil {
			return authResponse, fmt.Errorf("FunCore Update: %v", err)
		}

		if result {
			Trust = true
		}
	*/
	Trust = true

	utils.Log.Info("FunCore Update: OK")
	request := AuthRequest{
		FBToken:         fbtoken,
		UserName:        username,
		Password:        password,
		ServerCode:      serverCode,
		ServerPassword:  serverPassword,
		ClientPublicKey: key,
	}
	authRequest, _ := json.Marshal(request)
	httpResponse, err := client.client.Post(
		fmt.Sprintf("%s/api/phoenix/login", client.AuthServer),
		"application/json",
		bytes.NewBuffer(authRequest),
	)
	if err != nil {
		return authResponse, fmt.Errorf("auth: %v", err)
	}
	authResponse, err = assertAndParse[AuthResponse](httpResponse)
	if err != nil {
		return authResponse, err
	}
	if !authResponse.SuccessStates {
		failedReason := authResponse.Message
		err := failedReason.Information
		// if t := failedReason.Translation; t != -1 && t != 0 {
		// 	err = I18n.T(uint16(t))
		// }
		return AuthResponse{}, fmt.Errorf(err)
	}
	if len(authResponse.ServerMessage) > 0 {
		utils.Log.Info(i18n.T(i18n.Auth_MessageFromAuthServer))
		utils.Log.Info(authResponse.ServerMessage)
	}
	client.GrowthLevel = authResponse.BotLevel
	if len(authResponse.MasterName) > 0 && client.RespondTo == "" {
		client.RespondTo = authResponse.MasterName
	}
	return authResponse, nil
}

func (client *Client) TransferData(content string) (string, error) {
	const maxRetries = 10
	const delaySecond = 1
	const baseDelay = delaySecond * time.Second // 基础重试间隔，可根据需要调整
	var lastErr error = nil                     // 记录最后一次错误信息

	for attempt := 0; attempt < maxRetries; attempt++ {
		if lastErr != nil {
			utils.Log.Error(fmt.Sprintf("TransferData: %s", lastErr))
		}
		// 重试时等待（首次立即尝试）
		if attempt > 0 {
			delay := baseDelay * time.Duration(attempt) // 指数退避：1s, 2s, 3s...
			utils.Log.Info(fmt.Sprintf("Wait %d second to retry", delaySecond*attempt))
			time.Sleep(delay)
			utils.Log.Info("Retry TransferData...")
		}

		// 1. 发起 HTTP GET 请求
		url := fmt.Sprintf("%s/api/phoenix/transfer_start_type?content=%s", client.AuthServer, content)
		r, err := client.client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %v", err)
			continue // 请求失败，进行重试
		}

		// 2. 解析响应
		resp, err := assertAndParse[map[string]any](r)
		if err != nil {
			lastErr = fmt.Errorf("response parsing failed: %v", err)
			continue // 解析失败，进行重试
		}

		// 3. 检查 success 字段
		succ, ok := resp["success"].(bool)
		if !ok {
			lastErr = fmt.Errorf("invalid 'success' field type")
			continue // 字段类型错误，进行重试
		}
		if !succ {
			errMsg, _ := resp["message"].(string)
			lastErr = fmt.Errorf("server error: %s", errMsg)
			continue // 业务逻辑失败，进行重试
		}

		// 4. 提取 data 字段
		data, ok := resp["data"].(string)
		if !ok {
			lastErr = fmt.Errorf("invalid 'data' field type")
			continue // 字段类型错误，进行重试
		}

		return data, nil // 所有步骤成功，返回结果
	}

	return "", fmt.Errorf("after %d attempts, last error: %v", maxRetries, lastErr)
}

type FNumRequest struct {
	Data          string `json:"data"`
	EngineVersion string `json:"engine_version,omitempty"`
	PatchVersion  string `json:"patch_version,omitempty"`
	IsPC          *bool  `json:"is_pc,omitempty"`
}

func (client *Client) TransferCheckNum(req FNumRequest) (string, error) {
	rspreq := &req
	msg, err := json.Marshal(rspreq)
	if err != nil {
		return "", fmt.Errorf("failed to encode json")
	}
	r, err := client.client.Post(fmt.Sprintf("%s/api/phoenix/transfer_check_num", client.AuthServer), "application/json", bytes.NewBuffer(msg))
	if err != nil {
		return "", err
	}
	resp, err := assertAndParse[map[string]any](r)
	if err != nil {
		return "", err
	}
	succ, _ := resp["success"].(bool)
	if !succ {
		err_m, _ := resp["message"].(string)
		return "", fmt.Errorf("failed to transfer check num: %s", err_m)
	}
	val, _ := resp["value"].(string)
	return val, nil
}

// about tan lobby login

func (client *Client) TanLobbyAuth(roomID string, fbtoken string) (tanauth.TanLobbyLoginResponse, error) {
	roomID = strings.TrimPrefix(roomID, "TanLobby:")
	// Pack request
	request := tanauth.TanLobbyLoginRequest{
		FBToken: fbtoken,
		RoomID:  roomID,
	}
	requestJsonBytes, _ := json.Marshal(request)

	// Post request
	resp, err := client.client.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_login", client.AuthServer),
		"application/json",
		bytes.NewBuffer(requestJsonBytes),
	)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, fmt.Errorf("TanLobbyAuth: %v", err)
	}

	// Parse response
	tanLobbyLoginResp, err := assertAndParse[tanauth.TanLobbyLoginResponse](resp)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, fmt.Errorf("TanLobbyAuth: %v", err)
	}

	// Return
	return tanLobbyLoginResp, nil
}

func (client *Client) TanLobbyCreate(fbtoken string) (tanauth.TanLobbyCreateResponse, error) {
	request := tanauth.TanLobbyCreateRequest{
		FBToken: fbtoken,
	}
	requestJsonBytes, _ := json.Marshal(request)
	resp, err := client.client.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_create", client.AuthServer),
		"application/json",
		bytes.NewBuffer(requestJsonBytes),
	)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyCreate: %v", err)
	}
	tanLobbyCreateResp, err := assertAndParse[tanauth.TanLobbyCreateResponse](resp)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, fmt.Errorf("TanLobbyCreate: %v", err)
	}
	return tanLobbyCreateResp, nil
}

func (client *Client) TanLobbyDebug(loginResponse string, raknetRand []byte, fbtoken string) (tanauth.TanLobbyDebugResponse, error) {
	request := tanauth.TanLobbyDebugRequest{
		FBToken:       fbtoken,
		LoginResponse: loginResponse,
		RaknetRand:    raknetRand,
	}
	requestJsonBytes, _ := json.Marshal(request)
	resp, err := client.client.Post(
		fmt.Sprintf("%s/api/phoenix/tan_lobby_debug", client.AuthServer),
		"application/json",
		bytes.NewBuffer(requestJsonBytes),
	)
	if err != nil {
		return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}
	tanLobbyDebugResp, err := assertAndParse[tanauth.TanLobbyDebugResponse](resp)
	if err != nil {
		return tanauth.TanLobbyDebugResponse{}, fmt.Errorf("TanLobbyDebug: %v", err)
	}
	return tanLobbyDebugResp, nil
}
