package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/account/mpay"
)

func main() {
	baseCtx := context.Background()
	requestTimeout := 15 * time.Second

	reader := bufio.NewReader(os.Stdin)

	deviceCtx, cancel := context.WithTimeout(baseCtx, requestTimeout)
	device, err := mpay.GenerateDevice(deviceCtx)
	cancel()
	if err != nil {
		log.Fatalf("生成设备失败: %v", err)
	}

	var guest *mpay.User
	for {
		guestCtx, guestCancel := context.WithTimeout(baseCtx, requestTimeout)
		guest, err = device.Guest(guestCtx)
		guestCancel()
		if err == nil {
			break
		}
		var verifyErr *mpay.NeedVerifyError
		if !errors.As(err, &verifyErr) {
			log.Fatalf("生成游客账号失败: %v", err)
		}
		fmt.Printf("生成游客账号需要验证: %s (code=%d)\n", verifyErr.Reason, verifyErr.Code)
		if verifyErr.VerifyURL != "" {
			fmt.Printf("请打开链接完成验证: %s\n", verifyErr.VerifyURL)
		}
		fmt.Println("验证完成后按回车继续...")
		_, _ = reader.ReadString('\n')
	}

	prettySauth, err := json.MarshalIndent(guest.Sauth, "", "  ")
	if err != nil {
		log.Fatalf("格式化 Sauth 失败: %v", err)
	}
	fmt.Println("生成的游客 Sauth:")
	fmt.Println(string(prettySauth))

	cookiePayload, err := guest.CookieString()
	if err != nil {
		log.Fatalf("生成 Cookie 数据失败: %v", err)
	}

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	for {
		if err := client.G79AuthenticateWithCookie(cookiePayload); err != nil {
			var verifyErr *mpay.NeedVerifyError
			if !errors.As(err, &verifyErr) {
				log.Fatalf("游客登录失败: %v", err)
			}
			fmt.Printf("游客登录需要验证: %s (code=%d)\n", verifyErr.Reason, verifyErr.Code)
			if verifyErr.VerifyURL != "" {
				fmt.Printf("请打开链接完成验证: %s\n", verifyErr.VerifyURL)
			}
			fmt.Println("验证完成后按回车继续...")
			_, _ = reader.ReadString('\n')
			continue
		}
		break
	}

	fmt.Printf("登录成功，UserID=%s，最新昵称=%s\n", client.UserID, client.UserDetail.Name)
}
