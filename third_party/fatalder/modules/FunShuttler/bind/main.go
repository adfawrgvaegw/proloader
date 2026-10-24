package bind

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

// 客户端结构体
type Client struct {
	BaseURL      string
	ServerPubKey *rsa.PublicKey  // 服务端公钥（加密AES密钥用）
	ClientPrivKey *rsa.PrivateKey // 客户端私钥（解密响应AES密钥用）
}

// 初始化客户端
func NewCryptoClient(baseURL string, serverPubPEM, clientPrivPEM []byte) (*Client, error) {
	// 加载服务端公钥
	pubBlock, _ := pem.Decode(serverPubPEM)
	pubKey, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return nil, err
	}
	serverPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("服务端公钥格式错误")
	}

	privBlock, _ := pem.Decode(clientPrivPEM)
	privKey, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return nil, err
	}
	clientPrivKey, ok := privKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("客户端私钥格式错误")
	}

	return &Client{
		BaseURL:      baseURL,
		ServerPubKey: serverPubKey,
		ClientPrivKey: clientPrivKey,
	}, nil
}

// 生成签名
func (c *Client) generateSignature(qq int64, timestamp float64) string {
	// 简化签名：时间戳+QQ+固定盐值
	salt := "simple_salt_123"  // 实际应使用更安全的盐值
	signStr := fmt.Sprintf("%v:%d:%s", timestamp, qq, salt)
	hash := sha256.Sum256([]byte(signStr))
	return hex.EncodeToString(hash[:])
}

// AES-CBC加密
func (c *Client) aesEncrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	data = pkcs7Padding(data, block.BlockSize())
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	ciphertext := make([]byte, len(iv)+len(data))
	copy(ciphertext[:aes.BlockSize], iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext[aes.BlockSize:], data)
	return ciphertext, nil
}

// AES-CBC解密
func (c *Client) aesDecrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("数据过短")
	}
	iv := data[:aes.BlockSize]
	data = data[aes.BlockSize:]
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)
	return pkcs7Unpadding(data)
}

// PKCS7填充
func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(data, padtext...)
}

// PKCS7去填充
func pkcs7Unpadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("数据为空")
	}
	unpadding := int(data[length-1])
	return data[:length-unpadding], nil
}

// 发送请求
func (c *Client) SendRequest(path string, qq int64, extra map[string]interface{}) (map[string]interface{}, error) {
	// 1. 准备数据
	timestamp := float64(time.Now().UnixNano()) / 1e9
	signature := c.generateSignature(qq, timestamp)
	payload := map[string]interface{}{
		"qq":        qq,
		"timestamp": timestamp,
		"signature": signature,
	}
	for k, v := range extra {
		payload[k] = v
	}

	// 2. 加密流程
	aesKey := make([]byte, 32)  // 32字节AES-256密钥
	if _, err := io.ReadFull(rand.Reader, aesKey); err != nil {
		return nil, err
	}
	payloadJSON, _ := json.Marshal(payload)
	encryptedPayload, err := c.aesEncrypt(aesKey, payloadJSON)
	if err != nil {
		return nil, err
	}

	// 用服务端公钥加密AES密钥
	encryptedAESKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		c.ServerPubKey,
		aesKey,
		nil,
	)
	if err != nil {
		return nil, err
	}

	// 拼接并转16进制
	encryptedData := hex.EncodeToString(append(encryptedAESKey, encryptedPayload...))

	// 3. 发送请求
	reqBody, _ := json.Marshal(map[string]string{"encrypted": encryptedData})
	resp, err := http.Post(c.BaseURL+path, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 4. 解密响应
	respBody, _ := io.ReadAll(resp.Body)
	var respData map[string]string
	json.Unmarshal(respBody, &respData)
	
	encryptedResp, _ := hex.DecodeString(respData["encrypted"])
	encryptedAESKeyResp := encryptedResp[:256]
	encryptedPayloadResp := encryptedResp[256:]

	// 用客户端私钥解密AES密钥
	aesKeyResp, err := rsa.DecryptOAEP(
		sha256.New(),
		rand.Reader,
		c.ClientPrivKey,
		encryptedAESKeyResp,
		nil,
	)
	if err != nil {
		return nil, err
	}

	// 用AES密钥解密响应数据
	decryptedPayload, err := c.aesDecrypt(aesKeyResp, encryptedPayloadResp)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	json.Unmarshal(decryptedPayload, &result)
	return result, nil
}

// 接口封装
func (c *Client) CheckBind(qq int64) (map[string]interface{}, error) {
	return c.SendRequest("/check_bind", qq, nil)
}

func (c *Client) BindDevice(qq int64, deviceInfo string) (map[string]interface{}, error) {
	return c.SendRequest("/bind_device", qq, map[string]interface{}{
		"device_info": deviceInfo,
	})
}

func (c *Client) GetDevice(qq int64) (map[string]interface{}, error) {
	return c.SendRequest("/get_device", qq, nil)
}
