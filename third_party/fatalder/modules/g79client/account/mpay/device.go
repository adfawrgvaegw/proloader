package mpay

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Yeah114/g79client/utils"
	"github.com/google/uuid"
)

const (
	deviceEndpoint          = "/mpay/games/aecfrxodyqaaaajp-g-x19/devices"
	realNameEndpoint        = "/mpay/api/users/realname/update_by_token"
	defaultGameID           = "aecfrxodyqaaaajp-g-x19"
	defaultGV               = "840268037"
	defaultGVN              = "3.3.15.268037"
	defaultCV               = "a5.2.0"
	defaultSV               = "32"
	defaultAppType          = "games"
	defaultAppMode          = "2"
	defaultAppChannel       = "netease"
	defaultMCountAppKey     = "EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq"
	defaultCloudExtraBase64 = "e30="
	defaultOptFields        = "nickname,avatar,realname_status,mobile_bind_status,exit_popup_info,mask_related_mobile,related_login_status"
	defaultRAMBytes         = "8034369536"
	defaultROMBytes         = "128849018880"
)

// Device 描述网易 MPay 服务返回的设备信息。
type Device struct {
	Key                 string `json:"key"`
	ID                  string `json:"id"`
	MCountTransactionID string `json:"mcount_transaction_id"`
	TransID             string `json:"transid"`
	UniqueID            string `json:"unique_id"`
	UDID                string `json:"udid"`
	Mac                 string `json:"mac"`
	Ram                 string `json:"ram"`
	Rom                 string `json:"rom"`
	ExtCI               string `json:"ext_ci"`
	Timestamp           int64  `json:"timestamp"`

	client *Client
}

// Clone 返回设备的浅拷贝，保留原有 client 以便继续调用接口。
func (d *Device) Clone() *Device {
	if d == nil {
		return nil
	}
	cloned := *d
	return &cloned
}

// GenerateDevice 便捷函数，使用默认客户端生成设备。
func GenerateDevice(ctx context.Context) (*Device, error) {
	client := NewClient(nil)
	return client.GenerateDevice(ctx)
}

// GenerateDevice 调用 MPay 接口生成设备凭据。
func (c *Client) GenerateDevice(ctx context.Context) (*Device, error) {
	if c == nil {
		return nil, errors.New("mpay: client 未初始化")
	}

	timestamp := time.Now().Unix()
	macHex, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	ursUDID, err := randomHex(20)
	if err != nil {
		return nil, err
	}
	udid, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	extCI, err := randomHex(32)
	if err != nil {
		return nil, err
	}

	uniqueID := fmt.Sprintf("%s%d20114", uuid.NewString(), timestamp)
	mcountID := fmt.Sprintf("%s_%d114_307044629", udid, timestamp)
	transID := fmt.Sprintf("%s_%d10114_399039478", udid, timestamp)

	form := c.buildDeviceForm(macHex, ursUDID, uniqueID, udid, extCI, mcountID, transID)

	body, status, err := c.postForm(ctx, deviceEndpoint, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("mpay: 生成设备失败 status=%d body=%s", status, string(body))
	}

	var parsed struct {
		Device struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"device"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Device.ID == "" || parsed.Device.Key == "" {
		return nil, fmt.Errorf("mpay: 设备响应缺少必要字段: %s", string(body))
	}

	return &Device{
		Key:                 parsed.Device.Key,
		ID:                  parsed.Device.ID,
		MCountTransactionID: mcountID,
		TransID:             transID,
		UniqueID:            uniqueID,
		UDID:                udid,
		Mac:                 macHex,
		Ram:                 defaultRAMBytes,
		Rom:                 defaultROMBytes,
		ExtCI:               extCI,
		Timestamp:           timestamp,
		client:              c,
	}, nil
}

// LoginEmail 使用邮箱账号登录。

func (d *Device) LoginEmail(ctx context.Context, email, password string) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	if email == "" || password == "" {
		return nil, errors.New("mpay: email 与 password 不能为空")
	}

	hash := md5.Sum([]byte(password))
	return d.loginEmailWithHash(ctx, email, hex.EncodeToString(hash[:]), password)
}

// LoginEmailMD5 使用邮箱账号登录，接受 32 位 MD5 密码。
func (d *Device) LoginEmailMD5(ctx context.Context, email, passwordMD5 string) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	if email == "" || passwordMD5 == "" {
		return nil, errors.New("mpay: email 与 password_md5 不能为空")
	}
	hash := strings.ToLower(strings.TrimSpace(passwordMD5))
	if len(hash) != 32 {
		return nil, fmt.Errorf("mpay: password_md5 长度非法: %s", passwordMD5)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return nil, fmt.Errorf("mpay: password_md5 非法: %w", err)
	}
	return d.loginEmailWithHash(ctx, email, hash, hash)
}

func (d *Device) loginEmailWithHash(ctx context.Context, email, passwordHash, storedPassword string) (*User, error) {
	if email == "" || passwordHash == "" {
		return nil, errors.New("mpay: email 与 password 不能为空")
	}
	loginURL := fmt.Sprintf("%s/%s/users?un=ZmdsbmIxMjNAMTYzLmNvbQ%%3D%%3D", deviceEndpoint, d.ID)

	payload := map[string]string{
		"username":  email,
		"password":  strings.ToLower(passwordHash),
		"unique_id": d.UniqueID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	aesKey, err := hex.DecodeString(d.Key)
	if err != nil {
		return nil, err
	}
	encrypted, err := utils.AesECBEncrypt(payloadBytes, aesKey)
	if err != nil {
		return nil, err
	}

	form := d.baseForm()
	form.Set("opt_fields", defaultOptFields)
	form.Set("params", hex.EncodeToString(encrypted))

	body, status, err := d.client.postForm(ctx, loginURL, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("mpay: 邮箱登录失败 status=%d body=%s", status, string(body))
	}
	if err := checkMPayError(body); err != nil {
		return nil, err
	}
	if storedPassword == "" {
		storedPassword = strings.ToLower(passwordHash)
	}
	return d.decodeUser(body, false, email, storedPassword)
}

// Guest 生成游客账号。
func (d *Device) Guest(ctx context.Context) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	guestURL := fmt.Sprintf("%s/%s/users/by_guest", deviceEndpoint, d.ID)

	aesKey, err := hex.DecodeString(d.Key)
	if err != nil {
		return nil, err
	}
	encrypted, err := utils.AesECBEncrypt([]byte("{}"), aesKey)
	if err != nil {
		return nil, err
	}

	form := d.baseForm()
	form.Set("opt_fields", defaultOptFields)
	form.Set("params", hex.EncodeToString(encrypted))

	body, status, err := d.client.postForm(ctx, guestURL, form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("mpay: 生成游客账号失败 status=%d body=%s", status, string(body))
	}
	if err := checkMPayError(body); err != nil {
		return nil, err
	}
	return d.decodeUser(body, true, "", "")
}

// AuthRealName 调用 MPay 接口完成实名认证。
func (d *Device) AuthRealName(ctx context.Context, user *User, realName, idNum string) error {
	if err := d.ensureClient(); err != nil {
		return err
	}
	if user == nil {
		return errors.New("mpay: user 不能为空")
	}
	if realName == "" || idNum == "" {
		return errors.New("mpay: realName 与 idNum 不能为空")
	}

	form := d.baseForm()
	form.Set("device_id", d.ID)
	form.Set("user_id", user.Sauth.SDKUID)
	form.Set("token", user.Sauth.SessionID)
	form.Set("realname", realName)
	form.Set("id_region", "86")
	form.Set("id_num", idNum)

	body, status, err := d.client.postForm(ctx, realNameEndpoint, form)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		if err := checkMPayError(body); err != nil {
			return err
		}
		return fmt.Errorf("mpay: 实名认证失败 status=%d body=%s", status, string(body))
	}
	return checkMPayError(body)
}

func (d *Device) baseForm() url.Values {
	values := url.Values{}
	values.Set("game_id", defaultGameID)
	values.Set("gv", defaultGV)
	values.Set("gvn", defaultGVN)
	values.Set("cv", defaultCV)
	values.Set("sv", defaultSV)
	values.Set("app_type", defaultAppType)
	values.Set("app_mode", defaultAppMode)
	values.Set("app_channel", defaultAppChannel)
	values.Set("mcount_app_key", defaultMCountAppKey)
	values.Set("mcount_transaction_id", d.MCountTransactionID)
	values.Set("transid", d.TransID)
	values.Set("_cloud_extra_base64", defaultCloudExtraBase64)
	values.Set("sc", "1")
	return values
}

func (d *Device) decodeUser(body []byte, isGuest bool, email, password string) (*User, error) {
	var payload struct {
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.User == nil {
		return nil, fmt.Errorf("mpay: 响应缺少 user 字段: %s", string(body))
	}

	tokenVal, ok := payload.User["token"]
	if !ok {
		return nil, fmt.Errorf("mpay: user 缺少 token 字段: %s", string(body))
	}
	token, err := toString(tokenVal)
	if err != nil || token == "" {
		return nil, fmt.Errorf("mpay: 无法解析 token 字段: %w", err)
	}

	idVal, ok := payload.User["id"]
	if !ok {
		return nil, fmt.Errorf("mpay: user 缺少 id 字段: %s", string(body))
	}
	sdkuid, err := toString(idVal)
	if err != nil || sdkuid == "" {
		return nil, fmt.Errorf("mpay: 无法解析 id 字段: %w", err)
	}

	if udidVal, ok := payload.User["udid"]; ok {
		if parsed, err := toString(udidVal); err == nil && parsed != "" {
			d.UDID = parsed
		}
	}

	return newUser(d.ID, sdkuid, token, d.UDID, d.Mac, isGuest, d.Ram, d.Rom, email, password), nil
}

func (d *Device) ensureClient() error {
	if d == nil {
		return errors.New("mpay: 设备未初始化")
	}
	if d.client == nil {
		d.client = NewClient(nil)
	}
	return nil
}

func checkMPayError(body []byte) error {
	var resp struct {
		Code      *int   `json:"code"`
		Reason    string `json:"reason"`
		VerifyURL string `json:"verify_url"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Code != nil {
		return &NeedVerifyError{Code: *resp.Code, Reason: resp.Reason, VerifyURL: resp.VerifyURL}
	}
	return nil
}

func toString(v any) (string, error) {
	switch value := v.(type) {
	case string:
		return value, nil
	case fmt.Stringer:
		return value.String(), nil
	case json.Number:
		return value.String(), nil
	case float64:
		return fmt.Sprintf("%.0f", value), nil
	case float32:
		return fmt.Sprintf("%.0f", value), nil
	case int:
		return fmt.Sprintf("%d", value), nil
	case int64:
		return fmt.Sprintf("%d", value), nil
	case uint64:
		return fmt.Sprintf("%d", value), nil
	case int32:
		return fmt.Sprintf("%d", value), nil
	case uint32:
		return fmt.Sprintf("%d", value), nil
	default:
		return "", fmt.Errorf("mpay: 不支持的类型 %T", v)
	}
}
