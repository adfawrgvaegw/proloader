package user_center

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type AdItem struct {
	Message   string `json:"message"`
	ExpiresAt int64  `json:"expires_at"`
}

// AdResponse 广告信息响应
type AdResponse struct {
	Success     bool     `json:"success"`
	Enabled     bool     `json:"enabled"`   // 广告是否启用
	Message     string   `json:"message"`   // 兼容字段：多条广告可能被换行拼接
	Ads         []AdItem `json:"ads"`       // 多条广告（每条可独立过期）
	Timestamp   int64    `json:"timestamp"` // 服务器时间戳
	Signature   string   `json:"signature"` // RSA-PSS 签名（Base64URL 编码）
	SignatureV2 string   `json:"signature_v2"`
	Nonce       string   `json:"nonce"` // 随机数
	SigVersion  int      `json:"sig_version"`
}

// 默认广告内容（当服务器不可用时使用）
const DefaultAdMessage = `本导入器为"Fatalder导入器"，官方群号为：945166144。`

// 最大时间偏差（5分钟，防止重放攻击）
const MaxTimeDrift = 5 * 60

// decodeBase64URLPadded 解码 Base64URL 编码的字符串（自动补齐 padding）
func decodeBase64URLPadded(s string) ([]byte, error) {
	// 补齐 padding
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// createAdSignaturePayload 创建与服务端一致的待签名数据
func createAdSignaturePayload(enabled bool, message string, timestamp int64, nonce string) string {
	return fmt.Sprintf("%t|%s|%d|%s", enabled, message, timestamp, nonce)
}

func createAdSignaturePayloadCompat(enabled bool, message string, timestamp int64, nonce string) []string {
	// Go: %t -> "true"/"false"
	payloadLower := createAdSignaturePayload(enabled, message, timestamp, nonce)

	// Python: f"{bool}" -> "True"/"False"
	enabledTitle := "False"
	if enabled {
		enabledTitle = "True"
	}
	payloadTitle := fmt.Sprintf("%s|%s|%d|%s", enabledTitle, message, timestamp, nonce)

	if payloadTitle == payloadLower {
		return []string{payloadLower}
	}
	return []string{payloadLower, payloadTitle}
}

func createAdSignaturePayloadV2(enabled bool, ads []AdItem, timestamp int64, nonce string) string {
	enabledStr := "false"
	if enabled {
		enabledStr = "true"
	}

	parts := []string{"v2", enabledStr, fmt.Sprintf("%d", timestamp), nonce, fmt.Sprintf("%d", len(ads))}
	for _, item := range ads {
		message := strings.TrimSpace(item.Message)
		expiresAt := item.ExpiresAt
		if expiresAt <= 0 {
			expiresAt = 0
		}
		msgB64 := base64.RawURLEncoding.EncodeToString([]byte(message))
		parts = append(parts, msgB64, fmt.Sprintf("%d", expiresAt))
	}
	return strings.Join(parts, "|")
}

// verifyAdSignature 验证 RSA-PSS 签名
func verifyAdSignature(data []byte, signatureB64 string, pubKeyPEM string) error {
	// 解析公钥
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return errors.New("failed to parse PEM block containing the public key")
	}

	pubIfc, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	pubKey, ok := pubIfc.(*rsa.PublicKey)
	if !ok {
		return errors.New("not an RSA public key")
	}

	// 解码签名
	signature, err := decodeBase64URLPadded(signatureB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// 计算哈希
	hashed := sha256.Sum256(data)

	// 验证 PSS 签名
	opts := &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
		Hash:       crypto.SHA256,
	}

	if err := rsa.VerifyPSS(pubKey, crypto.SHA256, hashed[:], signature, opts); err != nil {
		// 兼容部分后端/旧实现使用 PKCS1v15 签名
		if err2 := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], signature); err2 != nil {
			return fmt.Errorf("signature verification failed (rsa-pss=%v, rsa-pkcs1v15=%v)", err, err2)
		}
	}

	return nil
}

// validateAdResponseV1 验证 v1 广告响应（兼容单消息）
func validateAdResponseV1(resp *AdResponse) error {
	if resp == nil {
		return errors.New("nil response")
	}

	if !resp.Success {
		return errors.New("server returned failure")
	}

	// 如果广告未启用，不需要验证消息内容
	if !resp.Enabled {
		return nil
	}

	if strings.TrimSpace(resp.Message) == "" {
		return errors.New("empty ad message")
	}

	// 检查时间戳是否在合理范围内（防止重放攻击）
	now := time.Now().Unix()
	if abs(now-resp.Timestamp) > MaxTimeDrift {
		return fmt.Errorf("timestamp out of range: server=%d, local=%d", resp.Timestamp, now)
	}

	// 验证签名
	var signatureErr error
	for _, payload := range createAdSignaturePayloadCompat(resp.Enabled, resp.Message, resp.Timestamp, resp.Nonce) {
		if err := verifyAdSignature([]byte(payload), resp.Signature, ServerPublicPEM); err == nil {
			signatureErr = nil
			break
		} else {
			signatureErr = err
		}
	}
	if signatureErr != nil {
		return fmt.Errorf("invalid signature: %w", signatureErr)
	}

	return nil
}

// validateAdResponseV2 验证 v2 广告响应（覆盖 ads 列表与过期时间）
func validateAdResponseV2(resp *AdResponse) error {
	if resp == nil {
		return errors.New("nil response")
	}

	if !resp.Success {
		return errors.New("server returned failure")
	}

	// 如果广告未启用，不需要验证内容
	if !resp.Enabled {
		return nil
	}

	if len(resp.Ads) == 0 {
		return errors.New("empty ads")
	}
	for _, item := range resp.Ads {
		if strings.TrimSpace(item.Message) == "" {
			return errors.New("empty ad item message")
		}
	}

	// 检查时间戳是否在合理范围内（防止重放攻击）
	now := time.Now().Unix()
	if abs(now-resp.Timestamp) > MaxTimeDrift {
		return fmt.Errorf("timestamp out of range: server=%d, local=%d", resp.Timestamp, now)
	}

	if strings.TrimSpace(resp.SignatureV2) == "" {
		return errors.New("empty signature_v2")
	}

	payload := createAdSignaturePayloadV2(resp.Enabled, resp.Ads, resp.Timestamp, resp.Nonce)
	if err := verifyAdSignature([]byte(payload), resp.SignatureV2, ServerPublicPEM); err != nil {
		return fmt.Errorf("invalid signature_v2: %w", err)
	}

	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// GetAdvertisement 从服务器获取广告信息并验证签名
// 返回：(广告内容, 是否显示广告, 错误)
// 如果广告被禁用或用户被豁免，返回空字符串和 false
func (op *UserCenterOperator) GetAdvertisement(ctx context.Context) (string, bool, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodGet, "/service_api/advertisement", nil)
	if err != nil {
		return DefaultAdMessage, true, fmt.Errorf("failed to fetch advertisement: %w", err)
	}

	var resp AdResponse
	if err := json.Unmarshal(pt, &resp); err != nil {
		return DefaultAdMessage, true, fmt.Errorf("failed to parse ad response: %w", err)
	}

	// 验证响应完整性
	if err := validateAdResponseV1(&resp); err != nil {
		return DefaultAdMessage, true, fmt.Errorf("ad validation failed: %w", err)
	}

	// 如果广告未启用或用户被豁免
	if !resp.Enabled || resp.Message == "" {
		return "", false, nil
	}

	return resp.Message, true, nil
}

// GetAdvertisements 从服务器获取多条广告信息并验证签名
// 返回：(广告列表, 是否显示广告, 错误)
// 如果广告被禁用或用户被豁免，返回空列表和 false
func (op *UserCenterOperator) GetAdvertisements(ctx context.Context) ([]string, bool, error) {
	pt, _, err := op.doJSONEncrypted(ctx, http.MethodGet, "/service_api/advertisement", nil)
	if err != nil {
		return []string{DefaultAdMessage}, true, fmt.Errorf("failed to fetch advertisement: %w", err)
	}

	var resp AdResponse
	if err := json.Unmarshal(pt, &resp); err != nil {
		return []string{DefaultAdMessage}, true, fmt.Errorf("failed to parse ad response: %w", err)
	}

	// 如果广告未启用或用户被豁免
	if !resp.Success {
		return []string{DefaultAdMessage}, true, fmt.Errorf("server returned failure")
	}
	if !resp.Enabled {
		return []string{}, false, nil
	}

	// 优先使用 v2：覆盖 ads 列表与过期时间
	if resp.SigVersion == 2 && strings.TrimSpace(resp.SignatureV2) != "" && len(resp.Ads) > 0 {
		if err := validateAdResponseV2(&resp); err != nil {
			return []string{DefaultAdMessage}, true, fmt.Errorf("ad validation v2 failed: %w", err)
		}
		msgs := make([]string, 0, len(resp.Ads))
		for _, item := range resp.Ads {
			msg := strings.TrimSpace(item.Message)
			if msg == "" {
				continue
			}
			msgs = append(msgs, msg)
		}
		if len(msgs) == 0 {
			return []string{}, false, nil
		}
		return msgs, true, nil
	}

	// 回退 v1：只校验 message 字段
	if err := validateAdResponseV1(&resp); err != nil {
		return []string{DefaultAdMessage}, true, fmt.Errorf("ad validation failed: %w", err)
	}
	if strings.TrimSpace(resp.Message) == "" {
		return []string{}, false, nil
	}
	return []string{resp.Message}, true, nil
}

// GetAdvertisementSafe 安全获取广告信息
// 失败时返回默认广告（除非明确禁用）
func (op *UserCenterOperator) GetAdvertisementSafe(ctx context.Context) (string, bool) {
	msg, show, err := op.GetAdvertisement(ctx)
	if err != nil {
		// 获取失败时显示默认广告
		fmt.Printf("warning: %v, using default ad message\n", err)
		return DefaultAdMessage, true
	}
	return msg, show
}

// GetAdvertisementsSafe 安全获取多条广告信息
// 失败时返回默认广告（除非明确禁用）
func (op *UserCenterOperator) GetAdvertisementsSafe(ctx context.Context) ([]string, bool) {
	msgs, show, err := op.GetAdvertisements(ctx)
	if err != nil {
		fmt.Printf("warning: %v, using default ad message\n", err)
		return []string{DefaultAdMessage}, true
	}
	return msgs, show
}

// GetAdvertisement 全局封装：获取广告信息
func GetAdvertisement(ctx context.Context) (string, bool, error) {
	if GlobalOperator == nil {
		return DefaultAdMessage, true, fmt.Errorf("user center not initialized")
	}
	return GlobalOperator.GetAdvertisement(ctx)
}

// GetAdvertisementSafe 全局封装：安全获取广告信息
func GetAdvertisementSafe(ctx context.Context) (string, bool) {
	if GlobalOperator == nil {
		return DefaultAdMessage, true
	}
	return GlobalOperator.GetAdvertisementSafe(ctx)
}

// GetAdvertisements 全局封装：获取多条广告信息
func GetAdvertisements(ctx context.Context) ([]string, bool, error) {
	if GlobalOperator == nil {
		return []string{DefaultAdMessage}, true, fmt.Errorf("user center not initialized")
	}
	return GlobalOperator.GetAdvertisements(ctx)
}

// GetAdvertisementsSafe 全局封装：安全获取多条广告信息
func GetAdvertisementsSafe(ctx context.Context) ([]string, bool) {
	if GlobalOperator == nil {
		return []string{DefaultAdMessage}, true
	}
	return GlobalOperator.GetAdvertisementsSafe(ctx)
}
