package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/core/py_rpc"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/Yeah114/FunShuttler/utils/config"
)

var PrintPacket = false

func main() {
	i18n.Init(i18n.LanguageSimplifiedChinese)
	utils.Log.Info("FunShuttler")
	utils.Log.Info("Authors: Yeah、KashShinfu")
	utils.Log.Info("正在登录")
	// 初始化网易客户端配置
	clientCfg := client.Config{
		AuthServerAddress:    config.ShuttlerConfig.AuthServerAddress,
		AuthServerToken:      config.ShuttlerConfig.AuthServerToken,
		RentalServerCode:     config.ShuttlerConfig.RentalServerCode,
		RentalServerPasscode: config.ShuttlerConfig.RentalServerPasscode,
	}

	authClient, err := auth.CreateClient(&auth.ClientOptions{AuthServer: clientCfg.AuthServerAddress})
	if err != nil {
		panic(err)
	}
	clientCfg.AccessWrapper = auth.NewAccessWrapper(
		authClient,
		clientCfg.RentalServerCode,
		clientCfg.RentalServerPasscode,
		clientCfg.AuthServerToken,
		"", "",
	)

	// 登录网易服务器
	neteaseClient, err := client.LoginRentalServer(clientCfg)
	if err != nil {
		panic(err)
	}

	conn := neteaseClient.Conn()
	auth.Trust = true
	utils.Log.Info("登录完成")
	utils.Log.Info("正在创建网易本地联机")
	// 创建网易服务器监听器
	roomConfig := service.DefaultRoomConfig(
		"来和我一起玩吧！",
		config.ShuttlerConfig.RentalServerPasscode,
		10,
		service.PlayerPermissionMember,
	)
	listenConfig, listener, roomID, err := service.Listen(
		roomConfig,
		neteaseClient.Authenticator().TanLobbyAuthenticator(),
	)
	if err != nil {
		panic(err)
	}
	defer listenConfig.CloseRoom()
	utils.Log.Info(fmt.Sprintf("请进入本地联机房间: %d", roomID))

	// 接受网易客户端连接
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan struct{})
	go func() {
		for {
			neteaseNetConn, err := listener.Accept()
			utils.Log.Info("正在与客户端连接")
			if err != nil {
				utils.Log.Error(fmt.Sprintf("连接至客户端时发生错误: ", err))
				panic(err)
			}
			neteaseConn, dec, receive := minecraft.ToConn(neteaseNetConn)
			incoming := make(chan *minecraft.Conn, 1)
			go func() {
				for {
					packets, err := dec.Decode()
					if err != nil {
						panic(err)
					}
					for _, data := range packets {
						loggedInBefore := neteaseConn.LoggedIn()
						if err := receive(data); err != nil {
							panic(err)
						}
						if !loggedInBefore && neteaseConn.LoggedIn() {
							incoming <- neteaseConn
						}
					}
				}
			}()
			<-incoming
			utils.Log.Info(fmt.Sprintf("已连接至客户端: %v", neteaseConn.RemoteAddr()))

			utils.Log.Info("正在启动客户端游戏")
			if err := neteaseConn.StartGameContext(ctx, conn.GameData()); err != nil {
				_ = listener.Close()
				utils.Log.Error("启动客户端游戏超时")
				return
			}
			utils.Log.Info("客户端游戏启动完成")

			utils.Log.Info("正在向客户端发送登录时数据包")
			ids := map[uint32]bool{
				307: true,
				67: true,
			}
			for {
				p := <-neteaseClient.CachedPacket()
				if p == nil {
					break
				}
				if ids[p.ID()] {
					continue
				}
				if PrintPacket {
					pjb, _ := json.Marshal(p)
					fmt.Println(fmt.Sprintf("Cache Server: %d %s", p.ID(), string(pjb)))
				}
				if err := neteaseConn.WritePacket(p); err != nil {
					utils.Log.Warn(fmt.Sprintf("转发至客户端数据包 %d 时出错: %v", p.ID(), err))
				}
			}
			utils.Log.Info("向客户端发送登录时数据包完成")

			utils.Log.Info("正在启动数据包转发")
			// 处理服务器->客户端方向的流量
			go func() {
				neteaseConn.WritePacket(&packet.ToastRequest{
					Title:   "§6◆ FunShuttler §r",
					Message: fmt.Sprintf("§b✓ 成功连接至租赁服: §e%s", config.ShuttlerConfig.RentalServerCode),
				})
				neteaseConn.WritePacket(&packet.SetTitle{
					ActionType: packet.TitleActionSetActionBar,
					Text:       "§a请合理使用 FunShuttler",
				})
				for {
					p, err := conn.ReadPacket()
					if ids[p.ID()] {
						continue
					}
					if err != nil {
						utils.Log.Info(fmt.Sprintf("租赁服连接已关闭: %v", err))
						close(closed)
						return
					}
					if PrintPacket {
						pjb, _ := json.Marshal(p)
						fmt.Println(fmt.Sprintf("Server: %d %s", p.ID(), string(pjb)))
					}
					if err := neteaseConn.WritePacket(p); err != nil {
						utils.Log.Warn(fmt.Sprintf("转发至客户端数据包 %d 时出错: %v", p.ID(), err))
					}
				}
			}()

			// 处理客户端->服务器方向的流量
			go func() {
				for {
					p, err := neteaseConn.ReadPacket()
					if err != nil {
						utils.Log.Info(fmt.Sprintf("本地联机连接已关闭: %v", err))
						close(closed)
						return
					}
					if PrintPacket {
						pjb, _ := json.Marshal(p)
						fmt.Println(fmt.Sprintf("Client: %d %s", p.ID(), string(pjb)))
					}
					if p.ID() == packet.IDPyRpc {
						pk := p.(*packet.PyRpc)
						if pk.Value == nil {
							continue
						}
						content, err := py_rpc.Unmarshal(pk.Value)
						if err != nil {
							continue
						}
						switch content.(type) {
						case *py_rpc.SetMCPCheckNum, *py_rpc.GetMCPCheckNum, *py_rpc.StartType:
							continue
						}
					}
					if err := conn.WritePacket(p); err != nil {
						utils.Log.Warn(fmt.Sprintf("转发至租赁服数据包 %d 时出错: %v", p.ID(), err))
					}
				}
			}()
			utils.Log.Info("数据包转发启动完成")
			utils.Log.Info("正在等待连接关闭")
		}
	}()

	<-closed
	utils.Log.Info("FunShuttler 已关闭")
}
