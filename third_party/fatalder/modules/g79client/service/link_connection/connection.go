package link_connection

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	g79client "github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/utils"
)

const (
	heartbeatInterval = 10 * time.Second
	chatServerID      = 112
)

var (
	serverPublicKeyOnce sync.Once
	serverPublicKeyVal  *rsa.PublicKey
	serverPublicKeyErr  error

	clientPrivateKeyOnce sync.Once
	clientPrivateKeyVal  *rsa.PrivateKey
	clientPrivateKeyErr  error
)

const (
	serverPublicKeyBase64  = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArht9ioo6tc3Z7On/80iYjNI+HpxnEpSc0tXC9JLykvwkxluZiLPrlvO6sgkkPsQMBXudGRu335dBCVdwfMefY7wswrQG51U+Nw3xfSSRgSptNV8PcmNjh6EYAluRSy7AZLcWc6+qJ6fJFOeABGYxNwMVvbpDC0R+t7BtcmQCk+4uXP2dsRjJSs6ALlfT7iEs8IL7iRfu1IvomTAc6eJarStgxEBTWdV/d2XfoIshbNYQ9ziBk0iWzHoI15UXFWLL+jZwhQYwzB0f+ilckgFT0IFKU4msUUQ7io4CBY2iI1G0BnSQYwpm84WR0HrgKL7uoxtJQ98iPk2GbZgWFv1OTwIDAQAB"
	clientPrivateKeyBase64 = "MIIEpAIBAAKCAQEAx/Ipxphk+HO16oMY8HBOpauKgrdBnuQpH3LhJzsEkgYVK88ObW8w5ts3e0+UWCRJYoCwxPT2CPKeUvJr86gt3ajaOSXEej9iudOUcU6C5bARmkET5EXGvRLH7dnqJt0X2oGUG7H4Okit8+4UydcbjLs3VfXZba73+zqVUWhJPnpm5bnVzrsqu/mDo/EuClH9zp8oWz2RQfyCdVPtCwmY8wg/nrrUZNVV/AbgchuTedUHgs0IQ7eqUEZTVWrUFkkv82OvWUlu2RFxfFw4e9cTzEJJVPGxLmowFpFNKzp206jP3uL23VdnHrI8m4CGOqIlyfXgfCfIu/NxbLnzRnbQpQIDAQABAoIBAEvGXciy1olGKOpAVsJAfb3RfgO9+bOC2obdnbClcDz66ykYJmqY2hqTd7pW1Wx2DA21ochy4Y9Qi2n6D6le0ksQA+vmgUinHv43zikGzRrJGFKyWRyIySG8rWJZ1KB35+NaekvorZ9BDhPE5cH8sKcsCHOeYZFs3vQqJo6cjC2NwBSC+8HzeO+Btcgz88FX46tFvNokeGuJqZEEcfhss5dggEdHiUKTJsRXIdxwXqnYv08uPEzn/ogsnCBc5+ujwwds2qxiua599J8lY5KbB3/kBVqtea2MYUX/JUNEAIQRm7ygJ0DJJKRjPO0vYnkxXWC2waY5cRBpKQiTmJVwvf0CgYEA7xA8816Y42zRk5eU1ngmVX1GEoVkK6wf0obIgSMHlrwKl5u4rb3Vu8qGIPZ/OPw6zIr+sfAmsR2hFy6mAjQrLsGoGmegU1s6Ogr5fqMw55/OWv7tEhfI+myX2N9xw9IpodBsFfvd8yDDMmByiFtFx1oGu41VbbAJZdvsdlBIyV8CgYEA1hx5DspkRth44zIzgV+7EmCRSghCJ0c85MJwohpXX2wKg09gWm9AuuH74DEo0akIZGIRUZqxelMh3dc/mKmQmoMRbfMJojb7fytw3CtZEDNiXXDBtrZW2FFrwixK8XzFCKjYxwqOCt+kYbQcCz7MABWUb+heUxH+QMYtXTRr8HsCgYEAqup6GSktt5NKNvItmDQofABng8BYgJy716FDYogv2cWw8PmFTLonP+6ofJKfHJfAVhKdy4u9re1YCaHxUCwKH5CW5eHmjxHvDCZif/aedUsclpQh3EijCN9wpL4DsRPlben8DK+Y3EU1KSQpXnGa7s7fd2GxjQ1JesiEQ4Zcs5MCgYEAkdfpOgLw1TUk+xU58jkkMztmG/iOHzUuLGCp2jF5LH1ql9Ecv90iSWofaLHzrQSnu8D1LRHjLICuA+9X2YQ/BJCc8bjn6f/rxc7wXHiGfTuTGDTzLqL7evPTI/uJvP6RM/nXV5U/9fYqgYbux1YqHTCV4Lh2b71E5BhZ1DAeCjsCgYA5taKd1CCN1KyflOpiDJPskQSBdF9wO+P+dUdmFZDNccUFk2rNQ63idcH/n2L258Wf9SiI2dDjaaxUEetP3Ei75eHf4ixw22zlZ/kl+/BPWBdmTb+22xzoVMnt/ya/kvEC27/GAbrV4v+c5WFTs+ExWWkYbrpYJ6LnxAHiNf7/Uw=="
	clientPublicKeyMD5Hex  = "3f17d36cec77e47f1d301c0cc41cd73f"
)

// LinkMessage 表示从 Link 服务器收到的一条消息。
type LinkMessage struct {
	ServerID int32
	Flag     byte
	Method   string
	Payload  json.RawMessage
}

// CommonPushData 表示 Link 服务器下发的公共频道推送。
type CommonPushData struct {
	Method   string          `json:"-"`
	ServerID int32           `json:"server_id"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Entity   json.RawMessage `json:"entity"`
	Data     json.RawMessage `json:"data"`
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Details  string          `json:"details"`
	Raw      json.RawMessage `json:"-"`
}

type loginResult struct {
	err error
}

// LinkConnection 表示一个活跃的 Link 连接。
type LinkConnection struct {
	service *LinkConnectionService
	entry   g79client.G79LinkServerEntry
	conn    net.Conn

	cipherR1 []byte

	encrypt *utils.ChaChaEngine
	decrypt *utils.ChaChaEngine

	recvBuf      []byte
	handshakeBuf []byte

	handshakeDone bool

	messages   chan LinkMessage
	commonPush chan CommonPushData
	loginCh    chan loginResult
	closed     chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup

	sendMu sync.Mutex

	errMu   sync.Mutex
	lastErr error

	online atomic.Bool
}

func newLinkConnection(service *LinkConnectionService, conn net.Conn, entry g79client.G79LinkServerEntry) (*LinkConnection, error) {
	c := &LinkConnection{
		service:    service,
		entry:      entry,
		conn:       conn,
		messages:   make(chan LinkMessage, 32),
		commonPush: make(chan CommonPushData, 32),
		loginCh:    make(chan loginResult, 1),
		closed:     make(chan struct{}),
	}

	if err := c.sendHandshake(); err != nil {
		return nil, err
	}

	c.startLoops()
	return c, nil
}

func (c *LinkConnection) sendHandshake() error {
	c.cipherR1 = make([]byte, 16)
	if _, err := crand.Read(c.cipherR1); err != nil {
		return fmt.Errorf("link_connection.handshake: generate cipherR1: %w", err)
	}

	enc, err := rsaEncrypt(c.cipherR1)
	if err != nil {
		return fmt.Errorf("link_connection.handshake: encrypt cipherR1: %w", err)
	}
	if _, err := c.conn.Write(enc); err != nil {
		return fmt.Errorf("link_connection.handshake: send cipherR1: %w", err)
	}

	md5Bytes, err := hex.DecodeString(clientPublicKeyMD5Hex)
	if err != nil {
		return fmt.Errorf("link_connection.handshake: decode client md5: %w", err)
	}
	if _, err := c.conn.Write(md5Bytes); err != nil {
		return fmt.Errorf("link_connection.handshake: send client md5: %w", err)
	}

	return nil
}

func (c *LinkConnection) startLoops() {
	c.wg.Add(2)
	go c.readLoop()
	go c.heartbeatLoop()
}

func (c *LinkConnection) readLoop() {
	defer c.wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				c.setError(err)
			} else {
				c.setError(fmt.Errorf("link_connection.read: %w", err))
			}
			return
		}
		if n == 0 {
			continue
		}
		if err := c.handleIncoming(buf[:n]); err != nil {
			c.setError(err)
			return
		}
	}
}

func (c *LinkConnection) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.setError(err)
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *LinkConnection) sendHeartbeat() error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.handshakeDone {
		if _, err := c.conn.Write([]byte{0}); err != nil {
			return fmt.Errorf("link_connection.heartbeat: %w", err)
		}
	}
	return nil
}

func (c *LinkConnection) handleIncoming(data []byte) error {
	if !c.handshakeDone {
		c.handshakeBuf = append(c.handshakeBuf, data...)
		priv, err := clientPrivateKey()
		if err != nil {
			return err
		}
		size := priv.Size()
		if len(c.handshakeBuf) < size {
			return nil
		}
		cipher := c.handshakeBuf[:size]
		rest := append([]byte(nil), c.handshakeBuf[size:]...)
		c.handshakeBuf = c.handshakeBuf[:0]
		if err := c.completeHandshake(cipher); err != nil {
			return err
		}
		if len(rest) > 0 {
			return c.handleIncoming(rest)
		}
		return nil
	}

	c.recvBuf = append(c.recvBuf, data...)
	return c.processFrames()
}

func (c *LinkConnection) completeHandshake(payload []byte) error {
	plain, err := rsaDecrypt(payload)
	if err != nil {
		return fmt.Errorf("link_connection.handshake: decrypt cipherR2: %w", err)
	}
	plain = bytes.TrimRight(plain, "\x00")
	if len(plain) != 16 {
		return fmt.Errorf("link_connection.handshake: unexpected cipherR2 length %d", len(plain))
	}

	encryptKey := append(append([]byte{}, c.cipherR1...), plain...)
	decryptKey := append(append([]byte{}, plain...), c.cipherR1...)

	enc, err := utils.NewNeteaseChaCha(encryptKey)
	if err != nil {
		return fmt.Errorf("link_connection.handshake: create encrypter: %w", err)
	}
	dec, err := utils.NewNeteaseChaCha(decryptKey)
	if err != nil {
		return fmt.Errorf("link_connection.handshake: create decrypter: %w", err)
	}

	c.encrypt = enc
	c.decrypt = dec
	c.handshakeDone = true

	if err := c.sendLoginRequest(); err != nil {
		return err
	}
	return nil
}

func (c *LinkConnection) sendLoginRequest() error {
	uid, err := c.service.client.GetUserIDInt()
	if err != nil {
		return fmt.Errorf("link_connection.login: parse uid: %w", err)
	}
	token := utils.GetEncryptedToken(c.service.client.UserToken)
	if len(token) != 16 {
		return fmt.Errorf("link_connection.login: unexpected token length %d", len(token))
	}

	randBlock := make([]byte, 16)
	if _, err := crand.Read(randBlock); err != nil {
		return fmt.Errorf("link_connection.login: generate random block: %w", err)
	}
	randEnc, err := utils.AesECBEncryptBlock(randBlock, token)
	if err != nil {
		return fmt.Errorf("link_connection.login: encrypt random block: %w", err)
	}

	payload := map[string]interface{}{
		"uid":    uid,
		"s1":     hex.EncodeToString(randBlock),
		"s2":     hex.EncodeToString(randEnc),
		"is_zip": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.sendMessage(0, "LoginV2", body)
}

func (c *LinkConnection) processFrames() error {
	for {
		length, header, ok := readVarInt(c.recvBuf)
		if !ok {
			return nil
		}
		total := header + int(length)
		if len(c.recvBuf) < total {
			return nil
		}
		body := append([]byte(nil), c.recvBuf[header:total]...)
		c.recvBuf = c.recvBuf[total:]

		msg, err := c.decodeMessage(body)
		if err != nil {
			return err
		}
		if err := c.dispatchMessage(msg); err != nil {
			return err
		}
	}
}

func (c *LinkConnection) decodeMessage(body []byte) (*LinkMessage, error) {
	if c.decrypt == nil {
		return nil, errors.New("link_connection.decodeMessage: decrypt engine not ready")
	}
	plain, err := c.decrypt.Process(body)
	if err != nil {
		return nil, fmt.Errorf("link_connection.decodeMessage: decrypt body: %w", err)
	}

	serverID, off, ok := readVarInt(plain)
	if !ok {
		return nil, errors.New("link_connection.decodeMessage: invalid server id")
	}
	if len(plain) < off+2 {
		return nil, errors.New("link_connection.decodeMessage: payload too short")
	}
	flag := plain[off]
	methodLen := int(plain[off+1])
	start := off + 2
	end := start + methodLen
	if methodLen < 0 || end > len(plain) {
		return nil, errors.New("link_connection.decodeMessage: invalid method length")
	}
	method := string(plain[start:end])
	payload := append(json.RawMessage(nil), plain[end:]...)

	return &LinkMessage{
		ServerID: int32(serverID),
		Flag:     flag,
		Method:   method,
		Payload:  payload,
	}, nil
}

func (c *LinkConnection) dispatchMessage(msg *LinkMessage) error {
	if msg.Method == "LoginV2" {
		var resp struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			select {
			case c.loginCh <- loginResult{err: fmt.Errorf("link_connection.login: parse response: %w", err)}:
			default:
			}
		} else if resp.Code != 0 {
			select {
			case c.loginCh <- loginResult{err: fmt.Errorf("link_connection.login: server returned code %d", resp.Code)}:
			default:
			}
		} else {
			c.online.Store(true)
			select {
			case c.loginCh <- loginResult{err: nil}:
			default:
			}
		}
		return nil
	}

	if msg.Method == "CommonPush" || msg.Method == "RollingMsg" {
		var data CommonPushData
		if err := json.Unmarshal(msg.Payload, &data); err == nil {
			data.Method = msg.Method
			data.ServerID = msg.ServerID
			data.Raw = append(json.RawMessage(nil), msg.Payload...)
			select {
			case c.commonPush <- data:
			default:
			}
		}
	}

	select {
	case c.messages <- *msg:
	case <-c.closed:
		return io.EOF
	}
	return nil
}

// SendMessage 发送一条 Link 消息。
func (c *LinkConnection) SendMessage(serverID int32, method string, payload []byte) error {
	if !c.handshakeDone {
		return fmt.Errorf("link_connection.SendMessage: 连接尚未准备就绪")
	}
	return c.sendMessage(serverID, method, payload)
}

// SendChatRequest 发送聊天相关请求。
func (c *LinkConnection) SendChatRequest(serverID int32, method string, data map[string]interface{}) error {
	var payload []byte
	if data != nil && len(data) > 0 {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		payload = b
	}
	return c.SendMessage(serverID, method, payload)
}

// SendGlobalMessage 在世界频道发送文本消息。
func (c *LinkConnection) SendGlobalMessage(content string) error {
	if !c.online.Load() {
		return fmt.Errorf("link_connection.SendGlobalMessage: 连接未登录")
	}
	msg := map[string]interface{}{
		"type":               "text",
		"quick":              false,
		"data":               content,
		"message_visibility": 0,
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req := map[string]interface{}{
		"msg":              string(msgJSON),
		"refer_message_id": "",
	}
	return c.SendChatRequest(chatServerID, "SendMessage", req)
}

// SendGameStart 发送 GameStart，请求订阅公共频道等推送。
func (c *LinkConnection) SendGameStart(data map[string]interface{}) error {
	if !c.online.Load() {
		return fmt.Errorf("link_connection.SendGameStart: 连接未登录")
	}
	if data == nil {
		data = map[string]interface{}{"strict_mode": true}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.SendMessage(0, "GameStart", payload)
}

// SendGameStop 发送 GameStop，通知服务器玩家已退出游戏。
func (c *LinkConnection) SendGameStop(data map[string]interface{}) error {
	if !c.online.Load() {
		return fmt.Errorf("link_connection.SendGameStop: 连接未登录")
	}
	if data == nil {
		data = map[string]interface{}{"strict_mode": true}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.SendMessage(0, "GameStop", payload)
}

// SendChatOnline 通知服务器进入在线状态。
func (c *LinkConnection) SendChatOnline() error {
	return c.SendChatRequest(chatServerID, "Online", nil)
}

// RequestChatDumpState 请求服务器状态映射。
func (c *LinkConnection) RequestChatDumpState() error {
	return c.SendChatRequest(0, "DumpState", nil)
}

// RequestChatConnectState 主动上报聊天室连接状态。
func (c *LinkConnection) RequestChatConnectState() error {
	return c.SendChatRequest(0, "ConnectState", map[string]interface{}{"server_id": chatServerID})
}

// FetchGlobalMessages 请求公共频道历史消息。
func (c *LinkConnection) FetchGlobalMessages(length int) error {
	if length <= 0 {
		length = 20
	}
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"length": length,
		},
		"length": length,
	}
	return c.SendChatRequest(chatServerID, "GetGlobalMessages", data)
}

// IsOnline 返回当前连接是否已登录。
func (c *LinkConnection) IsOnline() bool {
	return c.online.Load()
}

func (c *LinkConnection) sendMessage(serverID int32, method string, payload []byte) error {
	if c.encrypt == nil {
		return fmt.Errorf("link_connection.sendMessage: encrypt engine 未准备")
	}
	if len(method) > 255 {
		return fmt.Errorf("link_connection.sendMessage: method 长度超出 255")
	}

	bodyBuf := bytes.NewBuffer(nil)
	writeVarInt(bodyBuf, serverID)
	bodyBuf.WriteByte(0)
	bodyBuf.WriteByte(byte(len(method)))
	bodyBuf.WriteString(method)
	bodyBuf.Write(payload)

	encrypted, err := c.encrypt.Process(bodyBuf.Bytes())
	if err != nil {
		return fmt.Errorf("link_connection.sendMessage: 加密失败: %w", err)
	}

	frame := bytes.NewBuffer(nil)
	writeVarInt(frame, int32(len(encrypted)))
	frame.Write(encrypted)

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if _, err := c.conn.Write(frame.Bytes()); err != nil {
		return fmt.Errorf("link_connection.sendMessage: 写入失败: %w", err)
	}
	return nil
}

func (c *LinkConnection) waitForLogin(ctx context.Context) error {
	select {
	case res := <-c.loginCh:
		return res.err
	case <-c.closed:
		if err := c.Err(); err != nil {
			return err
		}
		return io.EOF
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Messages 返回服务器推送消息通道。
func (c *LinkConnection) Messages() <-chan LinkMessage {
	return c.messages
}

// CommonPushMessages 返回解析后的公共频道推送消息。
func (c *LinkConnection) CommonPushMessages() <-chan CommonPushData {
	return c.commonPush
}

func (c *LinkConnection) Conn() net.Conn {
	return c.conn
}

// Close 关闭连接并释放资源。
func (c *LinkConnection) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		close(c.closed)
		closeErr = c.conn.Close()
		c.wg.Wait()
		close(c.messages)
		close(c.commonPush)
		c.online.Store(false)
	})
	return closeErr
}

// Err 返回连接的最终错误。
func (c *LinkConnection) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr
}

func (c *LinkConnection) setError(err error) {
	if err == nil {
		return
	}
	c.errMu.Lock()
	if c.lastErr == nil {
		c.lastErr = err
	}
	c.errMu.Unlock()
	c.online.Store(false)
	c.Close()
}

func rsaEncrypt(data []byte) ([]byte, error) {
	pub, err := serverPublicKey()
	if err != nil {
		return nil, err
	}
	chunkSize := pub.Size() - 11
	var out bytes.Buffer
	for len(data) > 0 {
		n := chunkSize
		if len(data) < n {
			n = len(data)
		}
		chunk := data[:n]
		data = data[n:]
		enc, err := rsa.EncryptPKCS1v15(crand.Reader, pub, chunk)
		if err != nil {
			return nil, err
		}
		out.Write(enc)
	}
	return out.Bytes(), nil
}

func rsaDecrypt(data []byte) ([]byte, error) {
	priv, err := clientPrivateKey()
	if err != nil {
		return nil, err
	}
	chunkSize := priv.Size()
	var out bytes.Buffer
	for len(data) > 0 {
		if len(data) < chunkSize {
			return nil, fmt.Errorf("link_connection.rsaDecrypt: chunk too small: %d", len(data))
		}
		chunk := data[:chunkSize]
		data = data[chunkSize:]
		dec, err := rsa.DecryptPKCS1v15(crand.Reader, priv, chunk)
		if err != nil {
			return nil, err
		}
		out.Write(dec)
	}
	return out.Bytes(), nil
}

func serverPublicKey() (*rsa.PublicKey, error) {
	serverPublicKeyOnce.Do(func() {
		der, err := base64.StdEncoding.DecodeString(serverPublicKeyBase64)
		if err != nil {
			serverPublicKeyErr = err
			return
		}
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			serverPublicKeyErr = err
			return
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			serverPublicKeyErr = errors.New("link_connection: server public key not RSA")
			return
		}
		serverPublicKeyVal = rsaPub
	})
	return serverPublicKeyVal, serverPublicKeyErr
}

func clientPrivateKey() (*rsa.PrivateKey, error) {
	clientPrivateKeyOnce.Do(func() {
		der, err := base64.StdEncoding.DecodeString(clientPrivateKeyBase64)
		if err != nil {
			clientPrivateKeyErr = err
			return
		}
		priv, err := x509.ParsePKCS1PrivateKey(der)
		if err != nil {
			clientPrivateKeyErr = err
			return
		}
		clientPrivateKeyVal = priv
	})
	return clientPrivateKeyVal, clientPrivateKeyErr
}

func writeVarInt(buf *bytes.Buffer, value int32) {
	v := uint32(value)
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			buf.WriteByte(b | 0x80)
		} else {
			buf.WriteByte(b)
			break
		}
	}
}

func readVarInt(data []byte) (value int32, size int, ok bool) {
	var x int32
	var s uint
	for size < len(data) {
		b := data[size]
		size++
		if b < 0x80 {
			x |= int32(b) << s
			return x, size, true
		}
		x |= int32(b&0x7F) << s
		s += 7
		if s >= 35 {
			return 0, size, false
		}
	}
	return 0, size, false
}
