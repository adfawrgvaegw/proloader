package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/go-gl/mathgl/mgl32"
)

type MoveTask struct {
	control  control.Control
	id       int
	TaskType TaskType `json:"type"`
	Pos      defines.BlockPos `json:"pos"`
}

func NewMoveTask() *MoveTask {
	return &MoveTask{
		TaskType: TaskTypeMove,
	}
}

func (m *MoveTask) oplessMoveToPosition(ctx context.Context, position mgl32.Vec3, facing mgl32.Vec3) (err error) {
	for j := 0; j < 10; j++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err = m.control.Resources().WritePacket(&packet.PlayerAuthInput{
			Pitch: facing[0],
			Yaw: facing[1],
			HeadYaw: facing[2],
			InputData: packet.InputFlagPersistSneak | packet.InputFlagPaddlingRight,
			InputMode: packet.InputModeTouch,
			PlayMode: packet.PlayModeScreen,
			InteractionModel: packet.InteractionModelCrosshair,
			Position: position,
		})
		if err != nil {
			return
		}
	}
	return
}

func (m *MoveTask) SetControl(c defines.Control) {
	ctrl, ok := c.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	m.control = *ctrl
	m.id = m.control.GetTaskID()
}

func (m *MoveTask) ID() int {
	return m.id
}

func (m *MoveTask) Type() string {
	return string(m.TaskType)
}

func (m *MoveTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), m)
}

func (m *MoveTask) ToJSON() (string, error) {
	bs, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (m *MoveTask) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if utils.GetExtraOptions().UseOplessTp {
		return m.oplessMoveToPosition(ctx, mgl32.Vec3{float32(m.Pos.X()), float32(m.Pos.Y()), float32(m.Pos.Z())}, mgl32.Vec3{0, 0, 0})
	} else {
		return m.control.GameInterface().Commands().SendSettingsCommand(
			fmt.Sprintf(`tp @s %d %d %d`, m.Pos.X(), m.Pos.Y(), m.Pos.Z()),
			true,
		)
	}
}

func (m *MoveTask) Pause() error {
	return nil
}

func (m *MoveTask) Resume() error {
	return nil
}

func (m *MoveTask) Stop() error {
	return nil
}

func (m *MoveTask) IsRunning() bool {
	return false
}

func (m *MoveTask) IsPaused() bool {
	return false
}

func (m *MoveTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

func (m *MoveTask) RestoreInterruptJSON(string) error {
	return nil
}
