package special_command

import (
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
)

type SpecialCommandPool struct {
	commands []*game_interface.SpecialCommandDefinition
}

func NewSpecialCommandPool() *SpecialCommandPool {
	return &SpecialCommandPool{
		commands: make([]*game_interface.SpecialCommandDefinition, 0),
	}
}

func (pool *SpecialCommandPool) AddCommand(cmd *game_interface.SpecialCommandDefinition) {
	pool.commands = append(pool.commands, cmd)
}

func (pool *SpecialCommandPool) GetAllCommands() []*game_interface.SpecialCommandDefinition {
	return pool.commands
}

func (pool *SpecialCommandPool) RegisterAllCommands(gameInterface *game_interface.GameInterface) {
	for _, cmd := range pool.GetAllCommands() {
		if cmd.Name != "" {
			gameInterface.SpecialCommandManager().RegisterSpecialCommand(cmd)
		}
	}
}

func (pool *SpecialCommandPool) InitAllCommands(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
	for _, cmd := range pool.commands {
		if cmd.Init != nil {
			cmd.Init(gameInterface, standardConn)
		}
	}
}

func GetGlobalCommandPool() *SpecialCommandPool {
	pool := NewSpecialCommandPool()

	pool.AddCommand(GetKashShinfuCommand())
	pool.AddCommand(GetGetNBTCommand())
	pool.AddCommand(GetGetHoldNBTCommand())
	pool.AddCommand(GetNightVisionCommand())
	pool.AddCommand(GetBetterChunkCommand())
	pool.AddCommand(GetSouloutCommand())

	// Internal Version
	pool.AddCommand(GetMapCommand())
	pool.AddCommand(GetBigMapCommand())
	pool.AddCommand(GetSendWebSocketCommand())
	pool.AddCommand(GetSendSettingsCommand())
	pool.AddCommand(GetFlyCommand())
	pool.AddCommand(GetWalkSpeedCommand())
	pool.AddCommand(GetFlySpeedCommand())
	pool.AddCommand(GetNoClipCommand())
	pool.AddCommand(GetMTPCommand())
	pool.AddCommand(GetKillAuraCommand())
	pool.AddCommand(GetGetPosCommand())

	return pool
}

func RegisterAllSpecialCommands(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
	pool := GetGlobalCommandPool()
	pool.InitAllCommands(gameInterface, standardConn)
	pool.RegisterAllCommands(gameInterface)
}
