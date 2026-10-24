// MPay 本地桥接：复用 g79client，只接受 stdin，不读取命令行密码。
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Yeah114/g79client/account/mpay"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type request struct {
	Recovery   *mpay.Device `json:"recovery"`
	Diagnostic bool         `json:"diagnostic"`
	Mode       string       `json:"mode"`
	Email      string       `json:"email"`
	Password   string       `json:"password"`
}
type response struct {
	Recovery   *mpay.Device `json:"recovery,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	VerifyURL  string       `json:"verify_url,omitempty"`
	Stage      string       `json:"stage,omitempty"`
	HTTPStatus int          `json:"http_status,omitempty"`
	OK         bool         `json:"ok"`
	Cookie     string       `json:"cookie,omitempty"`
	Error      string       `json:"error,omitempty"`
	Code       *int         `json:"code,omitempty"`
}

// 本地 fork 的 LoginEmail 将 query.un 写死为示例邮箱，按当前输入修正副本。
// 不改缓存源码，其余设备和加密参数仍复用参考实现。
type emailTransport struct {
	email string
	base  http.RoundTripper
}

func (t emailTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyReq := req.Clone(req.Context())
	copyURL := *req.URL
	copyReq.URL = &copyURL
	if copyURL.Host == "service.mkey.163.com" && copyURL.Query().Has("un") {
		query := copyURL.Query()
		query.Set("un", base64.StdEncoding.EncodeToString([]byte(t.email)))
		copyURL.RawQuery = query.Encode()
	}
	return t.base.RoundTrip(copyReq)
}

// Only retain status and numeric error code; never emit remote bodies.
type diagnosticTransport struct {
	base   http.RoundTripper
	stage  string
	status int
	code   *int
}

func (t *diagnosticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.stage = "device"
	if strings.HasSuffix(req.URL.Path, "/users/by_guest") {
		t.stage = "guest"
	}
	t.status = 0
	t.code = nil
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1048577))
	resp.Body.Close()
	if err != nil || len(body) > 1048576 {
		return nil, errors.New("invalid_response")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var payload struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(body, &payload) == nil {
		t.code = payload.Code
	}
	return resp, nil
}

// Rehydrate through the normal constructor, replaying only the saved device response.
// No device registration request is sent during resume.
type recoveryTransport struct {
	base   http.RoundTripper
	device *mpay.Device
}

func (t recoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, "/devices") && t.device != nil {
		body, _ := json.Marshal(map[string]any{"device": map[string]string{"id": t.device.ID, "key": t.device.Key}})
		return &http.Response{StatusCode: 201, Body: io.NopCloser(bytes.NewReader(body))}, nil
	}
	return t.base.RoundTrip(r)
}
func execute(req request, base http.RoundTripper) response {
	if req.Mode == "" {
		req.Mode = "email"
	}
	if (req.Mode != "email" && req.Mode != "guest") || (req.Mode == "email" && (req.Email == "" || req.Password == "")) || (req.Mode == "guest" && (req.Email != "" || req.Password != "")) {
		return response{Error: "invalid_input"}
	}
	if req.Recovery != nil {
		if req.Mode != "guest" || req.Recovery.ID == "" || req.Recovery.Key == "" {
			return response{Error: "invalid_recovery"}
		}
		base = recoveryTransport{base: base, device: req.Recovery}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	transport := &diagnosticTransport{base: emailTransport{email: req.Email, base: base}}
	client := mpay.NewClient(&http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }})
	device, err := client.GenerateDevice(ctx)
	if err == nil && req.Recovery != nil {
		saved, _ := json.Marshal(req.Recovery)
		err = json.Unmarshal(saved, device) // preserves constructor's unexported client pointer
	}
	var cookie string
	if err == nil {
		var user *mpay.User
		if req.Mode == "guest" {
			user, err = device.Guest(ctx)
		} else {
			user, err = device.LoginEmail(ctx, req.Email, req.Password)
		}
		if err == nil {
			user.Password = ""
			cookie, err = user.CookieString()
		}
	}
	if err != nil {
		result := response{Error: "mpay_login_failed", Stage: transport.stage, HTTPStatus: transport.status, Code: transport.code}
		var verify *mpay.NeedVerifyError
		if errors.As(err, &verify) {
			result.Error = "mpay_verification_required"
			if req.Mode == "guest" {
				result.Error = "mpay_rejected"
			}
			result.Code = &verify.Code
			if req.Diagnostic && req.Mode == "guest" {
				result.Reason = verify.Reason
				result.VerifyURL = verify.VerifyURL
			}
		}
		if req.Diagnostic && req.Mode == "guest" {
			result.Recovery = device
		}
		return result
	}
	return response{OK: true, Cookie: cookie, Stage: transport.stage, HTTPStatus: transport.status}
}
func main() {
	var req request
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&req); err != nil {
		json.NewEncoder(os.Stdout).Encode(response{Error: "invalid_input"})
		return
	}
	json.NewEncoder(os.Stdout).Encode(execute(req, http.DefaultTransport))
}
