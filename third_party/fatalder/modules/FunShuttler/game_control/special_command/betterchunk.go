package special_command

import (
	//	"fmt"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"path/filepath"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/Yeah114/FunShuttler/utils/config"
	//	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
)

var opened bool
var bedrockWorld *world.BedrockWorld

func GetBetterChunkCommand() *game_interface.SpecialCommandDefinition {
	return &game_interface.SpecialCommandDefinition{
		Init: func(gameInterface *game_interface.GameInterface, standardConn *standard_minecraft.Conn) {
			if opened {
				if err := bedrockWorld.CloseWorld(); err != nil {
					panic(err)
				}
			}
			bedrockWorld, err := world.Open(filepath.Join("worlds", config.ShuttlerConfig.RentalServerCode))
			if err != nil {
				panic(err)
			}
			/*
				if err = bedrockWorld.UpdateLevelDat(); err != nil {
					panic(err)
				}
			*/
			opened = true
			gameInterface.Resources().PacketInterceptor().InterceptPacket(
				[]uint32{standard_packet.IDSubChunk},
				func(pk *packet.Packet) (allow bool) {
					allow = true
					p := (*pk).(*packet.SubChunk)
					subChunkPosition := p.Position
					dimension := define.Dimension(p.Dimension)
					r := dimension.Range()
					subChunkNBTs := map[define.ChunkPos][]map[string]interface{}{}
					for i, subChunkEntry := range p.SubChunkEntries {
						subChunkPos := define.SubChunkPos{
							subChunkPosition.X() + int32(subChunkEntry.Offset[0]),
							subChunkPosition.Y() + int32(subChunkEntry.Offset[1]),
							subChunkPosition.Z() + int32(subChunkEntry.Offset[2]),
						}
						chunkPos := define.ChunkPos{
							subChunkPos.X(),
							subChunkPos.Z(),
						}

						if subChunkEntry.Result != protocol.SubChunkResultChunkNotFound {
							_ = i
							/*
								subChunk := bedrockWorld.LoadSubChunk(dimension, subChunkPos)
								if subChunk == nil {
									continue
								}
								nbts, _ := bedrockWorld.LoadNBT(dimension, chunkPos)
								rawPayload, err := NEMCSubChunkEncode(subChunk, r, i, nbts)
								fmt.Println(len(rawPayload))
								if err != nil || len(rawPayload) == 0 {
									fmt.Println("%v", err)
									continue
								}
								p.SubChunkEntries[i].Result = protocol.SubChunkResultSuccess
								p.SubChunkEntries[i].RawPayload = rawPayload
							*/
						} else {
							if len(subChunkEntry.RawPayload) == 0 {
								continue
							}
							subChunk, _, nbts, err := NEMCSubChunkDecode(
								subChunkEntry.RawPayload,
								r,
							)
							if err != nil {
								utils.Log.Warn(err.Error())
								continue
							}
							/*
								rawPayload, err := NEMCSubChunkEncode(subChunk, r, i, nbts)
								if err != nil {
									utils.Log.Warn(err.Error())
									continue
								}
								p.SubChunkEntries[i].RawPayload = rawPayload
							*/

							if err = bedrockWorld.SaveSubChunk(
								dimension,
								subChunkPos,
								subChunk,
							); err != nil {
								continue
							}

							if _, exists := subChunkNBTs[chunkPos]; !exists {
								subChunkNBTs[chunkPos] = []map[string]interface{}{}
							}
							subChunkNBTs[chunkPos] = append(subChunkNBTs[chunkPos], nbts...)
						}
					}
					for chunkPos, nbts := range subChunkNBTs {
						if err = bedrockWorld.SaveNBT(
							dimension,
							chunkPos,
							nbts,
						); err != nil {
							continue
						}
					}
					return
				},
			)
		},
	}
}
