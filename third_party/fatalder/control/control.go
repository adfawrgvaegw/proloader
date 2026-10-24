package control

import (
	"context"
	"fmt"

	"github.com/Happy2018new/nemc-tan-lobby-solver/core/nethernet"
	"github.com/Happy2018new/nemc-tan-lobby-solver/protocol/service"
	controlauth "github.com/Yeah114/Fatalder/control/auth"
	"github.com/Yeah114/Fatalder/defines"
	futils "github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/FunShuttler/client"
	bunkerauth "github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/core/py_rpc"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/utils"
)

type Control struct { // implements defines.Control
	config        defines.ControlConfig
	client        *client.Client
	gameInterface *game_interface.GameInterface
	resources     *resources_control.Resources
	isConnected   bool
	taskManager   *TaskManager
	taskID        int

	tanLobbyRoomListenConfig *service.ListenConfig
	tanLobbyRoomListener     *nethernet.Listener
	tanLobbyRoomID           uint32
}

func NewControl(config defines.ControlConfig) *Control {
	return &Control{
		config:      config,
		taskManager: NewTaskManager(),
	}
}

func (c *Control) Client() *client.Client {
	return c.client
}

func (c *Control) GetConfig() defines.ControlConfig {
	return c.config
}

func (c *Control) SetConfig(config defines.ControlConfig) {
	c.config = config
}

func (c *Control) EnterRentalServer() error {
	if c.isConnected {
		return ErrAlreadyConnected
	}

	authClient, err := controlauth.CreateClient(&bunkerauth.ClientOptions{
		AuthServer: c.config.AuthServerAddress,
	})
	if err != nil {
		return err
	}
	accessWrapper := controlauth.NewAccessWrapper(
		authClient,
		c.config.RentalServerCode,
		c.config.RentalServerPasscode,
		c.config.AuthServerToken,
		c.config.AuthUsername,
		c.config.AuthPassword,
	)

	enableTanLobbyRoom := futils.GetConfig().GetExtraConfig().TanLobbyRoomOperateMode
	cfg := client.Config{
		CacheAllPacket:       enableTanLobbyRoom,
		AuthServerAddress:    c.config.AuthServerAddress,
		AuthServerToken:      c.config.AuthServerToken,
		RentalServerCode:     c.config.RentalServerCode,
		RentalServerPasscode: c.config.RentalServerPasscode,
		AccessWrapper:        accessWrapper,
	}
	if cfg.AuthServerAddress == "" {
		cfg.AuthServerAddress = authClient.AuthServer()
	}

	cl, err := client.LoginRentalServer(cfg)
	if err != nil {
		return err
	}
	c.client = cl
	c.resources = resources_control.NewResourcesControl(c.client)
	c.gameInterface = game_interface.NewGameInterface(c.resources)
	c.isConnected = true
	if enableTanLobbyRoom {
		ok := make(chan struct{})
		cb := func() { close(ok) }
		go c.openTanLobbyRoomOperateMode(cb)
		<-ok
	}
	return nil
}

func (c *Control) openTanLobbyRoomOperateMode(cb func()) {
	var err error
	roomConfig := service.DefaultRoomConfig(
		"",
		"",
		10,
		service.PlayerPermissionMember,
	)
	c.tanLobbyRoomListenConfig, c.tanLobbyRoomListener, c.tanLobbyRoomID, err = service.Listen(
		roomConfig,
		c.client.Authenticator().TanLobbyAuthenticator(),
	)
	if err != nil {
		panic(err)
	}
	defer c.tanLobbyRoomListenConfig.CloseRoom()
	utils.Log.Info(fmt.Sprintf("Tan Lobby Room ID: %v", c.tanLobbyRoomID))
	conn := c.client.Conn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan struct{})
	go func() {
		for {
			neteaseNetConn, err := c.tanLobbyRoomListener.Accept()
			utils.Log.Info("正在与客户端连接")
			if err != nil {
				utils.Log.Error(fmt.Sprintf("连接至客户端时发生错误: %v", err))
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
				_ = c.tanLobbyRoomListener.Close()
				utils.Log.Error("启动客户端游戏超时")
				return
			}
			utils.Log.Info("客户端游戏启动完成")

			utils.Log.Info("正在启动数据包转发")
			// 处理服务器->客户端方向的流量
			ids := map[uint32]bool{
				307: true,
			}
			go func() {
				neteaseConn.WritePacket(&packet.ToastRequest{
					Title:   "§6◆ Fatalder-TanLobbyRoomMode §r",
					Message: fmt.Sprintf("§b✓ 成功连接至服务器: §e%s", c.client.Authenticator().RentalServerCode()),
				})
				neteaseConn.WritePacket(&packet.SetTitle{
					ActionType: packet.TitleActionSetActionBar,
					Text:       "§a请合理使用 Fatalder-TanLobbyRoomMode",
				})
				for p := range c.gameInterface.Resources().AllPacket() {
					if ids[p.ID()] {
						continue
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
			if cb != nil {
				cb()
			}
		}
	}()

	<-closed
	utils.Log.Info("Fatalder-TanLobbyRoomMode 已关闭")
}

func (c *Control) LeaveRentalServer() error {
	if !c.isConnected {
		return ErrNotConnected
	}
	c.client.Conn().Close()
	c.client = nil
	c.resources = nil
	c.gameInterface = nil
	c.isConnected = false
	return nil
}

func (c *Control) GameInterface() *game_interface.GameInterface {
	return c.gameInterface
}

func (c *Control) Resources() *resources_control.Resources {
	return c.resources
}

func (c *Control) TaskManager() defines.TaskManager {
	return c.taskManager
}

func (c *Control) DeleteTask(t defines.Task) {
	c.taskManager.RemoveTask(t.ID())
}

func (c *Control) RunAllTask(ctx context.Context, autoDelete bool) error {
	for _, t := range c.taskManager.GetTasks() {
		err := t.Start(ctx)
		if err != nil {
			return err
		}
		if autoDelete {
			c.DeleteTask(t)
		}
	}
	return nil
}

func (c *Control) GetTaskID() int {
	return c.taskID
}

func (c *Control) AddTask(t defines.Task) {
	t.SetControl(c)
	c.taskManager.AddTask(t)
	c.taskID++
}
