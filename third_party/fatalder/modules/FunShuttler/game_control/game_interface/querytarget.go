package game_interface

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// TargetQueryingPos 是单个 querytarget 结果中的位置信息
type TargetQueryingPos struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// 用于描述单个 querytarget 结果的结构体
type TargetQueryingInfo struct {
	Dimension      uint8             `json:"dimension"`
	PlayerUniqueID int64             `json:"id"`
	Position       TargetQueryingPos `json:"position"`
	PlayerUUID     uuid.UUID         `json:"uniqueId"`
	YRot           float32           `json:"yRot"`
}

// Querytarget 是基于 Commands 包装的 Querytarget 实现
type Querytarget struct {
	api *Commands
}

// NewQuerytarget 根据 api 返回并创建一个新的 Querytarget
func NewQuerytarget(api *Commands) *Querytarget {
	return &Querytarget{api: api}
}

// DoQuerytarget 查询目标选择器为 target 的坐标信息。
// 如果返回的切片为空，则说明目标不存在
func (q *Querytarget) DoQuerytarget(target string) ([]TargetQueryingInfo, error) {
	api := q.api

	resp, err := api.SendWSCommandWithResp(fmt.Sprintf("querytarget %s", target))
	if err != nil {
		return nil, fmt.Errorf("DoQuerytarget: %v", err)
	}

	result := make([]TargetQueryingInfo, 0)
	if resp.SuccessCount <= 0 || len(resp.OutputMessages[0].Parameters) <= 0 {
		return nil, nil
	}

	err = json.Unmarshal([]byte(resp.OutputMessages[0].Parameters[0]), &result)
	if err != nil {
		return nil, fmt.Errorf("DoQuerytarget: %v", err)
	}

	return result, nil
}
