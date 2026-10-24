package chat_connection

import (
	"context"
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	g79client "github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/utils"
)

// ChatConnection 表示一个与聊天服务器的活跃连接。
type ChatConnection struct {
	service *ChatConnectionService
	entry   g79client.G79ChatServerEntry
	conn    net.Conn

	encrypt *utils.ChaChaEngine
	decrypt *utils.ChaChaEngine

	seqMu sync.Mutex
	seq   uint16

	sendMu sync.Mutex

	messages chan Message
	closed   chan struct{}

	closeOnce sync.Once
	wg        sync.WaitGroup

	errMu   sync.Mutex
	lastErr error
	online  atomic.Bool
}

func newChatConnection(service *ChatConnectionService, conn net.Conn, entry g79client.G79ChatServerEntry) *ChatConnection {
	return &ChatConnection{
		service:  service,
		entry:    entry,
		conn:     conn,
		messages: make(chan Message, 32),
		closed:   make(chan struct{}),
	}
}

func (c *ChatConnection) handshake(ctx context.Context) error {
	tokenBytes, err := decodeToken(c.service.client.UserToken)
	if err != nil {
		return fmt.Errorf("chat_connection.handshake: 解析 token 失败: %w", err)
	}

	uid64, err := c.service.client.GetUserIDInt()
	if err != nil {
		return fmt.Errorf("chat_connection.handshake: 解析用户 ID 失败: %w", err)
	}
	if uid64 < 0 || uid64 > math.MaxUint32 {
		return fmt.Errorf("chat_connection.handshake: 用户 ID 超出 32 位整数范围: %d", uid64)
	}
	uid := uint32(uid64)

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("chat_connection.handshake: 生成随机数失败: %w", err)
	}

	block, err := aes.NewCipher(tokenBytes)
	if err != nil {
		return fmt.Errorf("chat_connection.handshake: 创建 AES 实例失败: %w", err)
	}

	aesRand := make([]byte, 16)
	block.Encrypt(aesRand, randBytes)

	head := make([]byte, 2)
	binary.LittleEndian.PutUint16(head, 1)

	body := make([]byte, 2+len(randBytes)+len(aesRand)+4)
	binary.LittleEndian.PutUint16(body[:2], handshakeRequestCmd)
	copy(body[2:18], randBytes)
	copy(body[18:34], aesRand)
	binary.LittleEndian.PutUint32(body[34:], uid)

	payload := append(head, body...)
	framed, err := wrapFrame(payload)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return err
	}
	defer c.conn.SetDeadline(time.Time{})

	if _, err := c.conn.Write(framed); err != nil {
		return fmt.Errorf("chat_connection.handshake: 发送握手请求失败: %w", err)
	}

	for {
		frame, err := readFrame(c.conn)
		if err != nil {
			return fmt.Errorf("chat_connection.handshake: 读取握手响应失败: %w", err)
		}
		if len(frame) == 0 {
			continue
		}
		if len(frame) < 5 {
			return fmt.Errorf("chat_connection.handshake: 握手响应过短: %d", len(frame))
		}
		ret := frame[4]
		if ret != 0 {
			return fmt.Errorf("chat_connection.handshake: 服务器返回错误码 %d", ret)
		}
		break
	}

	encryptKey := append(append([]byte{}, tokenBytes...), randBytes...)
	decryptKey := append(append([]byte{}, randBytes...), tokenBytes...)

	encryptEngine, err := utils.NewNeteaseChaCha(encryptKey)
	if err != nil {
		return fmt.Errorf("chat_connection.handshake: 创建加密引擎失败: %w", err)
	}
	decryptEngine, err := utils.NewNeteaseChaCha(decryptKey)
	if err != nil {
		return fmt.Errorf("chat_connection.handshake: 创建解密引擎失败: %w", err)
	}

	c.encrypt = encryptEngine
	c.decrypt = decryptEngine

	c.seqMu.Lock()
	c.seq = 2
	c.seqMu.Unlock()

	c.online.Store(true)
	return nil
}

func (c *ChatConnection) startLoops() {
	c.wg.Add(2)
	go c.readLoop()
	go c.heartbeatLoop()
}

func (c *ChatConnection) readLoop() {
	defer c.wg.Done()
	defer close(c.messages)

	for {
		frame, err := readFrame(c.conn)
		if err != nil {
			c.setError(err)
			return
		}
		if len(frame) == 0 {
			continue
		}
		if len(frame) < 2 {
			c.setError(fmt.Errorf("chat_connection: 收到异常帧长度 %d", len(frame)))
			return
		}

		seq := binary.LittleEndian.Uint16(frame[:2])
		encrypted := frame[2:]
		plain, err := c.decrypt.Process(encrypted)
		if err != nil {
			c.setError(fmt.Errorf("chat_connection: 解密数据失败: %w", err))
			return
		}
		if len(plain) < 2 {
			continue
		}
		cmd := binary.LittleEndian.Uint16(plain[:2])
		payload := append([]byte(nil), plain[2:]...)

		select {
		case c.messages <- Message{Sequence: seq, Command: cmd, Payload: payload}:
		case <-c.closed:
			return
		}
	}
}

func (c *ChatConnection) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(heartbeatIntervalSeconds * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.sendRaw(emptyFrame); err != nil {
				c.setError(fmt.Errorf("chat_connection: 心跳发送失败: %w", err))
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *ChatConnection) sendMessage(cmd uint16, payload []byte) (uint16, error) {
	if c.encrypt == nil {
		return 0, fmt.Errorf("chat_connection.sendMessage: 连接未就绪")
	}

	c.seqMu.Lock()
	seq := c.seq
	if seq == 0 {
		seq = 1
	}
	c.seq++
	c.seqMu.Unlock()

	head := make([]byte, 2)
	binary.LittleEndian.PutUint16(head, seq)

	body := make([]byte, 2+len(payload))
	binary.LittleEndian.PutUint16(body[:2], cmd)
	copy(body[2:], payload)

	encrypted, err := c.encrypt.Process(body)
	if err != nil {
		return 0, fmt.Errorf("chat_connection.sendMessage: 加密失败: %w", err)
	}

	packet := append(head, encrypted...)
	if len(packet) > math.MaxUint16 {
		return 0, fmt.Errorf("chat_connection.sendMessage: 数据过长 %d", len(packet))
	}

	frame := make([]byte, 2+len(packet))
	binary.LittleEndian.PutUint16(frame[:2], uint16(len(packet)))
	copy(frame[2:], packet)

	if err := c.sendRaw(frame); err != nil {
		return 0, err
	}
	return seq, nil
}

// ChatTo 发送私聊消息，返回本次使用的序列号。
func (c *ChatConnection) ChatTo(uid any, words string) (uint16, error) {
	if c == nil {
		return 0, fmt.Errorf("chat_connection.ChatTo: 连接未初始化")
	}
	payload, err := json.Marshal([]any{uid, words})
	if err != nil {
		return 0, fmt.Errorf("chat_connection.ChatTo: 序列化消息失败: %w", err)
	}
	return c.SendCommand(CommandChatPrivate, payload)
}

// GroupChatTo 发送群聊消息，返回本次使用的序列号。
func (c *ChatConnection) GroupChatTo(groupID any, words string) (uint16, error) {
	if c == nil {
		return 0, fmt.Errorf("chat_connection.GroupChatTo: 连接未初始化")
	}
	payload, err := json.Marshal([]any{groupID, words})
	if err != nil {
		return 0, fmt.Errorf("chat_connection.GroupChatTo: 序列化消息失败: %w", err)
	}
	return c.SendCommand(CommandChatGroup, payload)
}

func (c *ChatConnection) sendRaw(data []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if len(data) == 0 {
		return nil
	}
	_, err := c.conn.Write(data)
	return err
}

// SendCommand 发送一条聊天命令，并返回使用的序列号。
func (c *ChatConnection) SendCommand(cmd uint16, payload []byte) (uint16, error) {
	plain := make([]byte, 2+len(payload))
	binary.LittleEndian.PutUint16(plain[:2], cmd)
	copy(plain[2:], payload)

	c.seqMu.Lock()
	seq := c.seq
	if seq == 0 {
		seq = 1
	}
	c.seq++
	c.seqMu.Unlock()

	encrypted, err := c.encrypt.Process(plain)
	if err != nil {
		return 0, fmt.Errorf("chat_connection.SendCommand: 加密失败: %w", err)
	}

	msg := make([]byte, 2+len(encrypted))
	binary.LittleEndian.PutUint16(msg[:2], seq)
	copy(msg[2:], encrypted)

	framed, err := wrapFrame(msg)
	if err != nil {
		return 0, err
	}

	if err := c.sendRaw(framed); err != nil {
		return 0, err
	}
	return seq, nil
}

// IsOnline 返回连接是否已登录。
func (c *ChatConnection) IsOnline() bool {
	return c.online.Load()
}

// SerialNumber 返回当前序列号。
func (c *ChatConnection) SerialNumber() uint16 {
	c.seqMu.Lock()
	defer c.seqMu.Unlock()
	return c.seq
}

// Messages 返回一个只读通道，用于读取服务器推送的消息。
func (c *ChatConnection) Messages() <-chan Message {
	return c.messages
}

// Close 关闭连接并释放资源。
func (c *ChatConnection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		err = c.conn.Close()
		c.wg.Wait()
		c.online.Store(false)
	})
	return err
}

// Err 返回读写循环中出现的最新错误。
func (c *ChatConnection) Err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr
}

func (c *ChatConnection) setError(err error) {
	if err == io.EOF {
		// 正常断线也需要通知外部
	}
	c.errMu.Lock()
	if c.lastErr == nil {
		c.lastErr = err
	}
	c.errMu.Unlock()
	c.online.Store(false)
	c.Close()
}

func readFrame(r io.Reader) ([]byte, error) {
	lengthBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint16(lengthBuf)
	if size == 0 {
		return nil, nil
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
