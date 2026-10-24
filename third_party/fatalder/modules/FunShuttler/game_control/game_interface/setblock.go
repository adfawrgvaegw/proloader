package game_interface

import (
	"fmt"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
)

// 用作铁砧的承重方块
const BaseAnvil string = "minecraft:polished_andesite"

// SetBlock 是基于 Commands 实现的，
// 通过发送 MC 命令实现方块放置的若干实现
type SetBlock struct {
	api *Commands
}

// NewSetBlock 基于 api 创建并返回一个新的 SetBlock
func NewSetBlock(api *Commands) *SetBlock {
	return &SetBlock{api: api}
}

// SetBlock 在 pos 处以 setblock 命令放
// 置名为 name 且方块状态为 states 的方块。
// 此实现是阻塞的，它将等待租赁服回应后再返回值
func (s *SetBlock) SetBlock(pos protocol.BlockPos, name string, states string) error {
	api := s.api
	request := fmt.Sprintf("setblock %d %d %d %s %s", pos[0], pos[1], pos[2], name, states)
	_, isTimeout, err := api.SendWSCommandWithTimeout(request, DefaultTimeoutCommandRequest)

	if isTimeout {
		err = api.SendSettingsCommand(request, true)
		if err != nil {
			return fmt.Errorf("SetBlock: %v", err)
		}
		err = api.AwaitChangesGeneral()
		if err != nil {
			return fmt.Errorf("SetBlock: %v", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("SetBlock: %v", err)
	}

	return nil
}

// SetAnvil 在 pos 处放置铁砧，并返回所放置铁砧的方块状态 states。
// placeBaseBlock 指示是否需要在 pos 下一格处放置铁砧的承重方块
func (s *SetBlock) SetAnvil(pos protocol.BlockPos, placeBaseBlock bool) (states map[string]any, err error) {
	if placeBaseBlock {
		err = s.SetBlock(protocol.BlockPos{pos[0], pos[1] - 1, pos[2]}, BaseAnvil, "[]")
		if err != nil {
			return nil, fmt.Errorf("SetAnvil: %v", err)
		}
	}

	err = s.SetBlock(pos, "minecraft:anvil", `["damage"="undamaged","minecraft:cardinal_direction"="east"]`)
	if err != nil {
		return nil, fmt.Errorf("SetAnvil: %v", err)
	}

	return map[string]any{
		"damage":                       "undamaged",
		"minecraft:cardinal_direction": "east",
	}, nil
}

// SetBlockAsync 在 pos 处以 setblock 命令
// 放置名为 name 且方块状态为 states 的方块。
//
// 此实现不会等待租赁服响应，这意味着调用
// SetBlockAsync 后将立即返回
func (s *SetBlock) SetBlockAsync(pos protocol.BlockPos, name string, states string) error {
	err := s.api.SendSettingsCommand(
		fmt.Sprintf("setblock %d %d %d %s %s", pos[0], pos[1], pos[2], name, states),
		true,
	)
	if err != nil {
		return fmt.Errorf("SetBlockAsync: %v", err)
	}
	return nil
}
