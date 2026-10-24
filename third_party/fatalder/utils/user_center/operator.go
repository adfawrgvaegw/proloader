package user_center

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tanauth "github.com/Happy2018new/nemc-tan-lobby-solver/bunker"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
)

type UserCenterOperator struct {
	baseRoot   string
	apiKey     string
	client     *http.Client
	serverPub  *rsa.PublicKey
	clientPriv *rsa.PrivateKey
}

// NewUserCenterOperater creates an operator bound to backend base root (e.g. http://127.0.0.1:8000) and apiKey used in X-API-Key.
func NewUserCenterOperater(baseURL, apiKey string) (*UserCenterOperator, error) {
	spub, err := parsePub(ServerPublicPEM)
	if err != nil {
		return nil, fmt.Errorf("parse server public key: %w", err)
	}
	cpriv, err := parsePriv(ClientPrivatePEM)
	if err != nil {
		return nil, fmt.Errorf("parse client private key: %w", err)
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &UserCenterOperator{
		baseRoot:   baseURL,
		apiKey:     apiKey,
		client:     &http.Client{Timeout: 30 * time.Second},
		serverPub:  spub,
		clientPriv: cpriv,
	}, nil
}

func parsePub(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("invalid public pem")
	}
	pubIfc, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := pubIfc.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return pub, nil
}

func parsePriv(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("invalid private pem")
	}
	// try PKCS8 then PKCS1
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, fmt.Errorf("pkcs8 is not rsa private key")
	}
	if rk, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rk, nil
	}
	return nil, fmt.Errorf("unsupported private key format")
}

func (op *UserCenterOperator) APIKey() string {
	return op.apiKey
}

// doJSONEncrypted sends payload as JSON encrypted envelope and expects encrypted JSON envelope in response.
func (op *UserCenterOperator) doJSONEncrypted(ctx context.Context, method, path string, payload any) ([]byte, int, error) {
	var body []byte
	// 即使无业务负载，也发送空对象 {} 的加密封包，满足后端“只能加密请求”的约束
	var b []byte
	var err error
	if payload != nil {
		b, err = json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("encode payload: %w", err)
		}
	} else {
		b = []byte("{}")
	}
	env, err := makeEnvelope(b, op.serverPub)
	if err != nil {
		return nil, 0, fmt.Errorf("encrypt payload: %w", err)
	}
	body = env

	// Always hit proxy under /api for encrypted channel
	fullPath := "/" + strings.TrimLeft(path, "/")
	u := op.baseRoot + fullPath
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", op.apiKey)
	resp, err := op.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}
	pt, err := openEnvelope(respBody, op.clientPriv)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decrypt response: %w", err)
	}
	return pt, resp.StatusCode, nil
}

// GetClient
func (op *UserCenterOperator) GetClient() *http.Client {
	return op.client
}

// ---- High level methods mapped from bot/core/bunker/auth ----

type AuthRequest struct {
	FBToken         string `json:"login_token,omitempty"`
	UserName        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	ServerCode      string `json:"server_code"`
	ServerPassword  string `json:"server_passcode"`
	ClientPublicKey string `json:"client_public_key"`
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

type SAuthResponse struct {
	SuccessStates bool `json:"success"`
	Data          struct {
		LoginToken string `json:"login_token"`
		RandomMode bool   `json:"random_mode"`
	} `json:"data"`
	Message string `json:"message"`
}

func (op *UserCenterOperator) PhoenixNew(ctx context.Context) (string, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodGet, "/api/new", nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func (op *UserCenterOperator) PhoenixSAuth(ctx context.Context) (string, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodGet, "/api/phoenix/sauth", nil)
	if err != nil {
		return "", err
	}
	var out SAuthResponse
	if err := json.Unmarshal(pt, &out); err != nil {
		return "", err
	}
	return out.Data.LoginToken, nil
}

func (op *UserCenterOperator) PhoenixLogin(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	var out AuthResponse
	// ClientPublicKey base64 is expected by old API, but our proxy expects encrypted body; keep original behavior if needed upstream
	// Here we still pass as field, using PEM as-is or base64-encoded elsewhere by caller if required.
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodPost, "/api/phoenix/login", req)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(pt, &out); err != nil {
		return out, err
	}
	return out, nil
}

// TransferStartType mimics TransferData(content) but returns raw JSON map.
func (op *UserCenterOperator) TransferStartType(ctx context.Context, content string) (map[string]any, error) {
	// Build relative path to avoid duplicating baseURL prefix
	u := url.URL{Path: "/api/phoenix/transfer_start_type"}
	q := u.Query()
	q.Set("content", content)
	u.RawQuery = q.Encode()

	// GET with encrypted empty body
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(pt, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (op *UserCenterOperator) TransferCheckNum(ctx context.Context, req bunkerauth.FNumRequest) (map[string]any, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodPost, "/api/phoenix/transfer_check_num", req)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(pt, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (op *UserCenterOperator) TanLobbyAuth(ctx context.Context, req tanauth.TanLobbyLoginRequest) (tanauth.TanLobbyLoginResponse, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodPost, "/api/phoenix/tan_lobby_login", req)
	if err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	var out tanauth.TanLobbyLoginResponse
	if err := json.Unmarshal(pt, &out); err != nil {
		return tanauth.TanLobbyLoginResponse{}, err
	}
	return out, nil
}

func (op *UserCenterOperator) TanLobbyCreate(ctx context.Context, req tanauth.TanLobbyCreateRequest) (tanauth.TanLobbyCreateResponse, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodPost, "/api/phoenix/tan_lobby_create", req)
	if err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}
	var out tanauth.TanLobbyCreateResponse
	if err := json.Unmarshal(pt, &out); err != nil {
		return tanauth.TanLobbyCreateResponse{}, err
	}
	return out, nil
}

// ---------------- Service API: Import Cost Charge ----------------

// CostChargePayload mirrors backend /service_api/cost payload
type CostChargePayload struct {
	Block        int    `json:"block"`
	CommandBlock int    `json:"commandblock"`
	NBTBlock     int    `json:"nbtblock"`
	Server       string `json:"server"`
	FileHash     string `json:"file_hash"`
}

// CostChargeResponse mirrors backend encrypted response
type CostChargeResponse struct {
	Success  bool   `json:"success"`
	Deducted int    `json:"deducted"`
	Balance  *int   `json:"balance,omitempty"`
	Message  string `json:"message"`
}

// ChargeImportCost 调用后端 /service_api/cost 扣费接口（加密通道 + X-API-Key）
// 返回解密后的响应体以及 HTTP 状态码。
func (op *UserCenterOperator) ChargeImportCost(ctx context.Context, payload CostChargePayload) (CostChargeResponse, int, error) {
	var out CostChargeResponse
	pt, status, err := op.doJSONEncrypted(ctx, http.MethodPost, "/service_api/cost", payload)
	if err != nil {
		return out, status, err
	}
	if err := json.Unmarshal(pt, &out); err != nil {
		return out, status, err
	}
	return out, status, nil
}

// ---------------- Service API: Complete ----------------

type CompletePayload struct {
	Server   string `json:"server"`
	FileHash string `json:"file_hash"`
}

type CompleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CompleteImport 调用后端 /service_api/complete 删除去重键
func (op *UserCenterOperator) CompleteImport(ctx context.Context, server, fileHash string) (CompleteResponse, int, error) {
	var out CompleteResponse
	payload := CompletePayload{Server: server, FileHash: fileHash}
	pt, status, err := op.doJSONEncrypted(ctx, http.MethodPost, "/service_api/complete", payload)
	if err != nil {
		return out, status, err
	}
	if err := json.Unmarshal(pt, &out); err != nil {
		return out, status, err
	}
	return out, status, nil
}

// ---------------- Service API: Billing Mode ----------------

type BillingModeResponse struct {
	BillingMode          string `json:"billing_mode"`
	DeductTimesPerImport int    `json:"deduct_times_per_import"`
}

func (op *UserCenterOperator) GetBillingMode(ctx context.Context) (BillingModeResponse, int, error) {
	var out BillingModeResponse
	pt, status, err := op.doJSONEncrypted(ctx, http.MethodGet, "/service_api/billing_mode", map[string]string{"_": "_"})
	if err != nil {
		return out, status, err
	}
	if err := json.Unmarshal(pt, &out); err != nil {
		return out, status, err
	}
	return out, status, nil
}
