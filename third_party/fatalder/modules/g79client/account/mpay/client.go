package mpay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://service.mkey.163.com"
	defaultUserAgent = "com.netease.x19/840268037 NeteaseMobileGame/a5.2.0 (23117RK66C;32)"
	requestTimeout   = 5 * time.Second
)

// Client 封装与网易 MPay 服务交互的 HTTP 客户端。
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewClient 使用默认超时和 User-Agent 创建 MPay 客户端。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    defaultBaseURL,
		userAgent:  defaultUserAgent,
	}
}

// SetUserAgent 允许自定义请求使用的 User-Agent。
func (c *Client) SetUserAgent(userAgent string) {
	if userAgent != "" {
		c.userAgent = userAgent
	}
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) ([]byte, int, error) {
	if c == nil {
		return nil, 0, errors.New("mpay: client 未初始化")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) postEncoded(ctx context.Context, path string, payload string) ([]byte, int, error) {
	if c == nil {
		return nil, 0, errors.New("mpay: client 未初始化")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) buildDeviceForm(mac, ursUDID, uniqueID, udid, extCI, mcountID, transID string) url.Values {
	form := url.Values{}
	form.Set("mac", mac)
	form.Set("urs_udid", ursUDID)
	form.Set("init_urs_device", "0")
	form.Set("unique_id", uniqueID)
	form.Set("brand", "Redmi")
	form.Set("device_name", "MuMu")
	form.Set("device_type", "tablet")
	form.Set("device_model", "23117RK66C")
	form.Set("resolution", "900*1600")
	form.Set("system_name", "Android")
	form.Set("system_version", "12")
	form.Set("udid", udid)
	form.Set("app_channel", defaultAppChannel)
	form.Set("ext_ci", extCI)
	form.Set("ci_code", "3")
	form.Set("game_id", defaultGameID)
	form.Set("gv", defaultGV)
	form.Set("gvn", defaultGVN)
	form.Set("cv", defaultCV)
	form.Set("sv", defaultSV)
	form.Set("app_type", defaultAppType)
	form.Set("app_mode", defaultAppMode)
	form.Set("mcount_app_key", defaultMCountAppKey)
	form.Set("mcount_transaction_id", mcountID)
	form.Set("transid", transID)
	form.Set("_cloud_extra_base64", defaultCloudExtraBase64)
	return form
}

func (c *Client) ensureResponse(status int, expect int, body []byte, context string) error {
	if status != expect {
		return fmt.Errorf("mpay: %s失败 status=%d body=%s", context, status, string(body))
	}
	return nil
}
