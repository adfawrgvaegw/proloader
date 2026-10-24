package main

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/Yeah114/g79client/account/mpay"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReplaceExampleEmailWithoutMutatingRequest(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://service.mkey.163.com/mpay/games/game/devices/device/users?un=EXAMPLE", nil)
	base := transportFunc(func(got *http.Request) (*http.Response, error) {
		expected := base64.StdEncoding.EncodeToString([]byte("actual@163.com"))
		if got.URL.Query().Get("un") != expected {
			t.Fatal("username query does not match actual login")
		}
		return &http.Response{StatusCode: 200}, nil
	})
	_, err := (emailTransport{email: "actual@163.com", base: base}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Query().Get("un") != "EXAMPLE" {
		t.Fatal("mutated caller request")
	}
}

func TestGuestRequestAndCookie(t *testing.T) {
	calls := 0
	base := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		r.ParseForm()
		body := `{"device":{"id":"test-device","key":"00000000000000000000000000000000"}}`
		if calls == 2 {
			if !strings.HasSuffix(r.URL.Path, "/test-device/users/by_guest") {
				t.Fatal("wrong endpoint")
			}
			block, _ := aes.NewCipher(make([]byte, 16))
			encrypted, err := hex.DecodeString(r.Form.Get("params"))
			if err != nil || len(encrypted) != 16 {
				t.Fatal("bad ciphertext")
			}
			plain := make([]byte, 16)
			block.Decrypt(plain, encrypted)
			if string(plain[:2]) != "{}" || plain[2] != 14 {
				t.Fatal("wrong guest payload/padding")
			}
			body = `{"user":{"id":"test-user","token":"SECRET"}}`
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result := execute(request{Mode: "guest"}, base)
	if !result.OK || calls != 2 || !strings.Contains(result.Cookie, "is_unisdk_guest") {
		t.Fatal("guest failed")
	}
}
func TestGuestFailureRedactedAndNoRetry(t *testing.T) {
	calls := 0
	base := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"code":123,"reason":"SECRET"}`))}, nil
	})
	result := execute(request{Mode: "guest"}, base)
	data, _ := json.Marshal(result)
	if calls != 1 || result.OK || result.Code == nil || *result.Code != 123 || strings.Contains(string(data), "SECRET") {
		t.Fatal("bad failure")
	}
}
func TestInvalidModeNeverRequests(t *testing.T) {
	result := execute(request{Mode: "batch"}, transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("network called"); return nil, nil }))
	if result.Error != "invalid_input" {
		t.Fatal("invalid accepted")
	}
}

func TestResumeUsesSameDeviceWithoutRegistration(t *testing.T) {
	calls := 0
	base := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasSuffix(r.URL.Path, "/saved-device/users/by_guest") {
			t.Fatal("unexpected registration or identity")
		}
		r.ParseForm()
		if r.Form.Get("transid") != "saved-transid" {
			t.Fatal("device state lost")
		}
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"code":1351,"reason":"PRIVATE","verify_url":"https://example.test/SECRET"}`))}, nil
	})
	result := execute(request{Mode: "guest", Diagnostic: true, Recovery: &mpay.Device{ID: "saved-device", Key: "00000000000000000000000000000000", TransID: "saved-transid"}}, base)
	if calls != 1 || result.Reason != "PRIVATE" || result.Recovery.ID != "saved-device" || result.Code == nil || *result.Code != 1351 {
		t.Fatal("resume failed")
	}
}
