package main

import (
	"fmt"
	"bytes"
	"github.com/mitchellh/mapstructure"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"

	"github.com/Yeah114/FunShuttler/utils"
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
)

// N2SEntityMetadata 转换实体元数据格式
func N2SEntityMetadata(src map[uint32]any) (map[uint32]any, error) {
	dst := make(map[uint32]any, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case protocol.BlockPos:
			dst[k] = standard_protocol.BlockPos{val.X(), val.Y(), val.Z()}
		case byte, int16, int32, float32, string, int64, mgl32.Vec3, map[string]any:
			dst[k] = v
		default:
			return nil, fmt.Errorf("unknown entity metadata type: %T", val)
		}
	}
	return dst, nil
}

// N2SEvent 将网易事件转换为标准事件
func N2SEvent(neteaseEvent protocol.Event) (standard_protocol.Event, error) {
	switch e := neteaseEvent.(type) {
	case *protocol.AchievementAwardedEvent:
		std := &standard_protocol.AchievementAwardedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.EntityInteractEvent:
		std := &standard_protocol.EntityInteractEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PortalBuiltEvent:
		std := &standard_protocol.PortalBuiltEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PortalUsedEvent:
		std := &standard_protocol.PortalUsedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.MobKilledEvent:
		std := &standard_protocol.MobKilledEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.CauldronUsedEvent:
		std := &standard_protocol.CauldronUsedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PlayerDiedEvent:
		std := &standard_protocol.PlayerDiedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.BossKilledEvent:
		std := &standard_protocol.BossKilledEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.AgentCommandEvent:
		std := &standard_protocol.AgentCommandEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PatternRemovedEvent:
		std := &standard_protocol.PatternRemovedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.SlashCommandExecutedEvent:
		std := &standard_protocol.SlashCommandExecutedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.FishBucketedEvent:
		std := &standard_protocol.FishBucketedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.MobBornEvent:
		std := &standard_protocol.MobBornEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PetDiedEvent:
		std := &standard_protocol.PetDiedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.CauldronInteractEvent:
		std := &standard_protocol.CauldronInteractEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.ComposterInteractEvent:
		std := &standard_protocol.ComposterInteractEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.BellUsedEvent:
		std := &standard_protocol.BellUsedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.EntityDefinitionTriggerEvent:
		std := &standard_protocol.EntityDefinitionTriggerEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.RaidUpdateEvent:
		std := &standard_protocol.RaidUpdateEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.MovementAnomalyEvent:
		std := &standard_protocol.MovementAnomalyEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.MovementCorrectedEvent:
		std := &standard_protocol.MovementCorrectedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.ExtractHoneyEvent:
		std := &standard_protocol.ExtractHoneyEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.TargetBlockHitEvent:
		std := &standard_protocol.TargetBlockHitEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.PiglinBarterEvent:
		std := &standard_protocol.PiglinBarterEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.WaxedOrUnwaxedCopperEvent:
		std := &standard_protocol.WaxedOrUnwaxedCopperEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.CodeBuilderRuntimeActionEvent:
		std := &standard_protocol.CodeBuilderRuntimeActionEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.CodeBuilderScoreboardEvent:
		std := &standard_protocol.CodeBuilderScoreboardEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.StriderRiddenInLavaInOverworldEvent:
		std := &standard_protocol.StriderRiddenInLavaInOverworldEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.SneakCloseToSculkSensorEvent:
		std := &standard_protocol.SneakCloseToSculkSensorEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.CarefulRestorationEvent:
		std := &standard_protocol.CarefulRestorationEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	case *protocol.ItemUsedEvent:
		std := &standard_protocol.ItemUsedEvent{}
		if err := ConvertStruct(e, std); err != nil {
			return nil, err
		}
		return std, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %T", e)
	}
}

func N2SByIO(src packet.Packet, des standard_packet.Packet) {
	buf := &bytes.Buffer{}
	writer := protocol.NewWriter(buf, 0)
	src.Marshal(writer)
	bs := buf.Bytes()
	rbuf := bytes.NewReader(bs)
	reader := standard_protocol.NewReader(rbuf, 0, false)
	des.Marshal(reader)
}


// FixNEMCSubChunkNBT 修正NEMC子区块数据中的NBT部分
// 输入: data - 原始子区块字节数据
// 输出: 修正后的完整字节数据
func FixNEMCSubChunkNBT(data []byte, r define.Range) ([]byte, error) {
	buf := bytes.NewBuffer(data)
	
	// 1. 解码子区块部分（推进缓冲区读取位置）
	_, _, err := chunk.DecodeSubChunk(buf, r, chunk.NetworkEncoding)
	if err != nil {
		return nil, fmt.Errorf("failed to decode subchunk: %w", err)
	}
	
	// 2. 获取子区块部分原始字节（已读取部分）
	subChunkData := data[:len(data)-buf.Len()]
	
	// 3. 处理并修正NBT部分
	var fixedNBT bytes.Buffer
	encoder := nbt.NewEncoderWithEncoding(&fixedNBT, nbt.NetworkLittleEndian)
	
	for buf.Len() > 0 {
		// 解码原始NBT
		var blockData map[string]interface{}
		decoder := nbt.NewDecoderWithEncoding(buf, nbt.NetworkLittleEndian)
		if err := decoder.Decode(&blockData); err != nil {
			return nil, fmt.Errorf("NBT decode error: %w", err)
		}
		
		// 修正NBT中的__tag字段
		if id, hasID := blockData["id"].(string); hasID {
			if tagNBT, hasTag := blockData["__tag"].(string); hasTag {
				// 解析__tag字段
				parsedTag, err := parseTagNBT(id, tagNBT)
				if err != nil {
					utils.Log.Warn(fmt.Sprintf("parse __tag error: %v", err))
				} else {
					// 合并解析结果
					for k, v := range parsedTag {
						blockData[k] = v
					}
					delete(blockData, "__tag")
				}
			}
		}
		
		// 重新编码修正后的NBT
		if err := encoder.Encode(blockData); err != nil {
			return nil, fmt.Errorf("NBT encode error: %w", err)
		}
	}
	
	// 4. 组合修正后的数据
	result := append(subChunkData, fixedNBT.Bytes()...)
	return result, nil
}

// parseTagNBT 解析__tag字段的NBT数据
func parseTagNBT(ID, tag string) (map[string]interface{}, error) {
	buffer := bytes.NewBuffer([]byte(tag))
	reader := protocol.NewReader(buffer, 0, false)
	
	// 获取方块实体处理器
	pool := block_actors.NewPool()
	block, has := pool[ID]
	if !has {
		return nil, fmt.Errorf("block not found in pool: %s", ID)
	}
	
	// 解析NBT数据到方块实体结构
	block.Marshal(reader)
	
	// 转换为map
	var result map[string]interface{}
	if err := mapstructure.Decode(block, &result); err != nil {
		return nil, fmt.Errorf("mapstructure decode error: %w", err)
	}
	
	return result, nil
}

func N2SSubChunk(subChunk *standard_packet.SubChunk) error {
	dimension := define.Dimension(subChunk.Dimension)
	r := dimension.Range()
	for i := range subChunk.SubChunkEntries {
	    subChunkEntry := &subChunk.SubChunkEntries[i]
	    if subChunkEntry.RawPayload == nil || len(subChunkEntry.RawPayload) == 0 {
	        continue
	    }
	    rawPayload, err := FixNEMCSubChunkNBT(subChunkEntry.RawPayload, r)
	    if err != nil {
	        return err
	    }
    	subChunkEntry.RawPayload = rawPayload
	}
	return nil
}
