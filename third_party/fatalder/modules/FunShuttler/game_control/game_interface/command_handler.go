package game_interface

import (
	"fmt"
	"github.com/google/shlex"
	"strings"
	"sync"

	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
)

type SpecialCommandHandler func(args []string, standardConn *standard_minecraft.Conn, gameInterface *GameInterface)
type SpecialCommandInit func(gameInterface *GameInterface, standardConn *standard_minecraft.Conn)

type SpecialCommandDefinition struct {
	Name        string
	Description string
	Handler     SpecialCommandHandler
	Init        SpecialCommandInit
	Overloads   []standard_protocol.CommandOverload
	NoParse     bool
}

type SpecialCommandManager struct {
	commands map[string]*SpecialCommandDefinition
	mutex    sync.RWMutex
}

func NewSpecialCommandManager() *SpecialCommandManager {
	return &SpecialCommandManager{
		commands: make(map[string]*SpecialCommandDefinition),
	}
}

func (cm *SpecialCommandManager) RegisterSpecialCommand(def *SpecialCommandDefinition) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.commands[def.Name] = def
}

func (cm *SpecialCommandManager) UnregisterSpecialCommand(name string) bool {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	if _, exists := cm.commands[name]; exists {
		delete(cm.commands, name)
		return true
	}
	return false
}

func (cm *SpecialCommandManager) HandleSpecialCommand(commandLine string, standardConn *standard_minecraft.Conn, gameInterface *GameInterface) bool {
	commandLine = strings.TrimPrefix(commandLine, "/")
	commandFields, err := shlex.Split(commandLine)
	if err != nil {
		standardConn.WritePacket(&standard_packet.ToastRequest{
			Title:   "§eFunShuttler",
			Message: fmt.Sprintf("§c命令解析发生错误: %s", err.Error()),
		})
		return false
	}

	if len(commandFields) == 0 {
		return false
	}

	command := commandFields[0]
	commandArgs := commandFields[1:]

	cm.mutex.RLock()
	commandDef, exists := cm.commands[command]
	cm.mutex.RUnlock()

	if !exists {
		return false
	}

	if commandDef.NoParse {
		commandArgs = []string{commandLine}
	}

	go commandDef.Handler(commandArgs, standardConn, gameInterface)
	return true
}

func (cm *SpecialCommandManager) GenerateAvailableSpecialCommands() []standard_protocol.Command {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	commands := make([]standard_protocol.Command, 0, len(cm.commands))

	for _, cmdDef := range cm.commands {
		cmd := standard_protocol.Command{
			Name:          cmdDef.Name,
			Description:   cmdDef.Description,
			AliasesOffset: 0xFFFFFFFF,
			Overloads:     cmdDef.Overloads,
		}
		commands = append(commands, cmd)
	}

	return commands
}
