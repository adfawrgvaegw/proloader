package special_command

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/mitchellh/mapstructure"

	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors"
	"github.com/Yeah114/FunShuttler/utils"
)

// splitCoordinateString 解析坐标分量字符串（可能包含连写的相对坐标）
func splitCoordinateString(s string) []string {
	var components []string
	var current strings.Builder

	for _, r := range s {
		if r == '~' {
			// 遇到 '~' 时结束当前分量（如果有）
			if current.Len() > 0 {
				components = append(components, current.String())
				current.Reset()
			}
			// 开始新的相对坐标分量
			current.WriteRune(r)
		} else {
			// 如果是相对坐标分量（以 ~ 开头），或绝对坐标分量
			if current.Len() > 0 || r == '-' || (r >= '0' && r <= '9') {
				current.WriteRune(r)
			}
		}
	}

	// 添加最后一个分量
	if current.Len() > 0 {
		components = append(components, current.String())
	}

	return components
}

// ParseCoordinates 解析 Minecraft 坐标指令
func ParseCoordinates(base mgl32.Vec3, args []string) (mgl32.Vec3, error) {
	// 收集所有分量（最多3个）
	components := make([]string, 0, 3)
	for _, arg := range args {
		// 拆分可能连写的坐标分量
		parts := splitCoordinateString(arg)
		for _, part := range parts {
			if len(components) >= 3 {
				break // 已收集足够分量
			}
			components = append(components, part)
		}
	}

	// 补足三个分量（缺失部分默认为 "~"）
	for len(components) < 3 {
		components = append(components, "~")
	}

	// 解析三个分量
	result := base
	for i, comp := range components[:3] {
		if strings.HasPrefix(comp, "~") {
			// 相对坐标处理
			offsetStr := strings.TrimPrefix(comp, "~")
			if offsetStr == "" {
				continue // ~ 等价于 ~0
			}

			offset, err := strconv.ParseFloat(offsetStr, 32)
			if err != nil {
				return mgl32.Vec3{}, err
			}
			result[i] += float32(offset)
		} else {
			// 绝对坐标处理
			abs, err := strconv.ParseFloat(comp, 32)
			if err != nil {
				return mgl32.Vec3{}, err
			}
			result[i] = float32(abs)
		}
	}

	return result, nil
}

// NEMCSubChunkDecode 解码NEMC子区块数据
func NEMCSubChunkDecode(data []byte, r define.Range) (subChunk *chunk.SubChunk, index int, nbts []map[string]interface{}, err error) {
	var (
		buf = bytes.NewBuffer(data)
	)
	subChunk, index, err = chunk.DecodeSubChunk(buf, r, chunk.NetworkEncoding)

	nbts = make([]map[string]interface{}, 0)
	if buf.Len() > 0 {
		blockData := make(map[string]interface{})
		for buf.Len() != 0 {
			err := nbt.NewDecoderWithEncoding(buf, nbt.NetworkLittleEndian).Decode(&blockData)
			if err != nil {
				err = fmt.Errorf("NEMCSubChunkDecode: decode chunk NBT error %v", err)
				break
			}
			// decode block NBT
			id, has1 := blockData["id"].(string)
			tagNBT, has2 := blockData["__tag"].(string)
			if has1 && has2 {
				result, err := NEMCTagNBTDecode(id, tagNBT)
				if err != nil {
					utils.Log.Warn(fmt.Sprintf("decode chunk __tag NBT error %v", err))
					continue
				}
				delete(blockData, "__tag")
				blockData = utils.MergeMaps(blockData, result)
			}
			// decode __tag NBT
			nbts = append(nbts, blockData)
		}
	}
	return
}

// NEMCTagNBTDecode 将方块类型为 ID 且 方块实体数据 为 tag 的 方块实体 解析为正常格式
func NEMCTagNBTDecode(ID string, tag string) (result map[string]any, err error) {
	buffer := bytes.NewBuffer([]byte(tag))
	reader := protocol.NewReader(buffer, 0, false)
	block, has := block_actors.NewPool()[ID]
	if !has {
		return nil, fmt.Errorf("NEMCTagNBTDecode: Target block not found in pool; ID = %#v", ID)
	}
	block.Marshal(reader)
	err = mapstructure.Decode(block, &result)
	if err != nil {
		return nil, fmt.Errorf("NEMCTagNBTDecode: %v", err)
	}
	return
}

func NEMCSubChunkEncode(subChunk *chunk.SubChunk, r define.Range, index int, nbts []map[string]interface{}) (data []byte, err error) {
	data = chunk.EncodeSubChunk(subChunk, r, index, chunk.NetworkEncoding)
	if len(data) == 0 {
		return nil, fmt.Errorf("NEMCSubChunkEncode: encode chunk error %v", err)
	}
	buf := bytes.NewBuffer(data)
	/*
		for _, n := range nbts {
			ID, tag, err := NEMCTagNBTEncode(n)
			if err != nil {
				utils.Log.Warn(fmt.Sprintf("encode chunk __tag NBT error %v", err))
				continue
			}
			blockData := make(map[string]interface{})
			blockData["id"] = ID
			blockData["__tag"] = tag
			err = nbt.NewEncoderWithEncoding(buf, nbt.NetworkLittleEndian).Encode(&blockData)
			if err != nil {
				err = fmt.Errorf("NEMCSubChunkEncode: encode chunk NBT error %v", err)
				break
			}
		}
	*/
	return buf.Bytes(), err
}

func NEMCTagNBTEncode(n map[string]any) (ID string, tag string, err error) {
	buffer := bytes.NewBuffer([]byte(""))
	writer := protocol.NewWriter(buffer, 0)
	ID = n["id"].(string)
	block, has := block_actors.NewPool()[ID]
	if !has {
		return "", "", fmt.Errorf("NEMCTagNBTEncode: Target block not found in pool; ID = %#v", ID)
	}
	err = mapstructure.Decode(n, &block)
	if err != nil {
		return "", "", fmt.Errorf("NEMCTagNBTEncode: %v", err)
	}
	block.Marshal(writer)
	return ID, string(buffer.Bytes()), nil
}

func ParseBool(boolStr string) bool {
	lowerBool := strings.ToLower(boolStr)
	switch lowerBool {
	case "true":
		return true
	default:
		return false
	}
}
