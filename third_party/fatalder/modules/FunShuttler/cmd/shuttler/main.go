package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"

	//	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"image/color"
	//	"math"
	//	"encoding/json"

	"github.com/go-gl/mathgl/mgl32"

	//	"github.com/Yeah114/FunShuttler/bind"
	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/game_control/special_command"
	"github.com/Yeah114/FunShuttler/i18n"
	"github.com/Yeah114/FunShuttler/translate"
	"github.com/Yeah114/FunShuttler/utils"
	"github.com/Yeah114/FunShuttler/utils/config"

	//	"github.com/Yeah114/FunShuttler/core/minecraft"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	standard_minecraft "github.com/Yeah114/FunShuttler/core/standard_minecraft"
	standard_protocol "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol"
	standard_packet "github.com/Yeah114/FunShuttler/core/standard_minecraft/protocol/packet"
)

var (
	// 数据包池初始化
	pool               = packet.ListAllPackets()
	standardClientPool = standard_packet.NewClientPool()
	standardServerPool = standard_packet.NewServerPool()
	ConvertStruct      = translate.ConvertStruct

	// 需要过滤的网易数据包ID列表
	filteredNeteasePacketIDs = map[uint32]bool{
		//		307: true, // 示例ID
		//		302: true,
		161: true, //IDCorrectPlayerMovePrediction
		//		packet.IDBlockActorData: true,
	}
)

// forwardNeteaseToStandard 转发网易数据包到标准服务器
func forwardNeteaseToStandard(neteasePacket packet.Packet, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) error {
	/*
		pjb, _ := json.Marshal(neteasePacket)
		fmt.Println("NeteaseServer", neteasePacket.ID(), string(pjb))
	*/
	resources := gameInterface.Resources()
	/*
		if neteasePacket.ID() != packet.IDChangeDimension {
			if !resources.HandlePacket(&neteasePacket) {
				return nil
			}
		}
	*/
	// 检查是否需要过滤该包
	if filteredNeteasePacketIDs[neteasePacket.ID()] {
		return nil
	}

	// 获取对应的标准数据包类型
	standardPacketType := standardServerPool[neteasePacket.ID()]
	if standardPacketType == nil {
		return nil
	}

	standardPacket := standardPacketType()

	// 特殊包处理逻辑
	switch p := neteasePacket.(type) {
	case *packet.InventoryTransaction:
		std := standardPacket.(*standard_packet.InventoryTransaction)
		std.LegacyRequestID = p.LegacyRequestID
		if err := ConvertStruct(&p.LegacySetItemSlots, &std.LegacySetItemSlots); err != nil {
			return err
		}
		if err := ConvertStruct(&p.Actions, &std.Actions); err != nil {
			return err
		}
		switch transactionData := p.TransactionData.(type) {
		case *protocol.NormalTransactionData:
			std.TransactionData = &standard_protocol.NormalTransactionData{}
			if err := ConvertStruct(transactionData, std.TransactionData); err != nil {
				return err
			}
		case *protocol.MismatchTransactionData:
			std.TransactionData = &standard_protocol.MismatchTransactionData{}
			if err := ConvertStruct(transactionData, std.TransactionData); err != nil {
				return err
			}
		case *protocol.UseItemTransactionData:
			std.TransactionData = &standard_protocol.UseItemTransactionData{}
			if err := ConvertStruct(transactionData, std.TransactionData); err != nil {
				return err
			}
		case *protocol.UseItemOnEntityTransactionData:
			std.TransactionData = &standard_protocol.UseItemOnEntityTransactionData{}
			if err := ConvertStruct(transactionData, std.TransactionData); err != nil {
				return err
			}
		case *protocol.ReleaseItemTransactionData:
			std.TransactionData = &standard_protocol.ReleaseItemTransactionData{}
			if err := ConvertStruct(transactionData, std.TransactionData); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown transaction data type: %T", transactionData)
		}
	case *packet.Event:
		// 特殊处理事件数据包
		std := standardPacket.(*standard_packet.Event)
		std.EntityRuntimeID = p.EntityRuntimeID
		std.UsePlayerID = p.UsePlayerID

		// 转换事件内容
		standardEvent, err := N2SEvent(p.Event)
		if err != nil {
			return fmt.Errorf("failed to convert event: %w", err)
		}
		std.Event = standardEvent
	case *packet.CraftingData:
		// 特殊处理事件数据包
		std := standardPacket.(*standard_packet.CraftingData)
		N2SByIO(p, std)
	case *packet.UnlockedRecipes:
		// 特殊处理事件数据包
		std := standardPacket.(*standard_packet.UnlockedRecipes)
		N2SByIO(p, std)
	case *packet.ClientBoundMapItemData:
		std := standardPacket.(*standard_packet.ClientBoundMapItemData)
		std.MapID = p.MapID
		std.UpdateFlags = p.UpdateFlags
		std.Dimension = p.Dimension
		std.LockedMap = p.LockedMap
		if err := ConvertStruct(&p.Origin, &std.Origin); err != nil {
			return err
		}
		std.Scale = p.Scale
		std.MapsIncludedIn = p.MapsIncludedIn
		if err := ConvertStruct(&p.TrackedObjects, &std.TrackedObjects); err != nil {
			return err
		}
		if err := ConvertStruct(&p.Decorations, &std.Decorations); err != nil {
			return err
		}
		if p.Pixels.IsEmpty {
			break
		} else {
			std.Pixels = make([]color.RGBA, 0)
		}
		std.Height = p.Height
		std.Width = p.Width
		std.YOffset = p.YOffset
		std.XOffset = p.XOffset

		switch data := p.Pixels.Data.(type) {
		case *protocol.Uint8Pixels:
			colorMap := make(map[int]color.RGBA)
			for _, value := range data.ColorMap {
				colorMap[int(value.Index)] = value.Colour
			}
			for _, value := range data.Pixels {
				std.Pixels = append(std.Pixels, colorMap[int(value)])
			}
		case *protocol.Uint16Pixels:
			colorMap := make(map[int]color.RGBA)
			for _, value := range data.ColorMap {
				colorMap[int(value.Index)] = value.Colour
			}
			for _, value := range data.Pixels {
				std.Pixels = append(std.Pixels, colorMap[int(value)])
			}
		case *protocol.StandardPixels:
			std.Pixels = data.Pixels
		}
	default:
		if err := ConvertStruct(neteasePacket, standardPacket); err != nil {
			return fmt.Errorf("conversion failed for packet %d: %w", neteasePacket.ID(), err)
		}
	}

	// 实体元数据特殊处理
	switch p := neteasePacket.(type) {
	case *packet.AddPlayer, *packet.AddActor, *packet.AddItemActor, *packet.SetActorData:
		var metadata map[uint32]any
		var _pkt packet.Packet
		switch pkt := p.(type) {
		case *packet.AddPlayer:
			metadata = pkt.EntityMetadata
			_pkt = pkt
		case *packet.AddActor:
			metadata = pkt.EntityMetadata
		case *packet.AddItemActor:
			metadata = pkt.EntityMetadata
			_pkt = pkt
		case *packet.SetActorData:
			metadata = pkt.EntityMetadata
		}

		converted, err := N2SEntityMetadata(metadata)
		if err != nil {
			return err
		}

		switch std := standardPacket.(type) {
		case *standard_packet.AddPlayer:
			std.EntityMetadata = converted
			std.HeldItem.Stack.NBTData = _pkt.(*packet.AddPlayer).HeldItem.Stack.NBTData
		case *standard_packet.AddActor:
			std.EntityMetadata = converted
		case *standard_packet.AddItemActor:
			std.EntityMetadata = converted
			std.Item.Stack.NBTData = _pkt.(*packet.AddItemActor).Item.Stack.NBTData
		case *standard_packet.SetActorData:
			std.EntityMetadata = converted
		}
	case *packet.InventoryContent:
		std := standardPacket.(*standard_packet.InventoryContent)
		inventory, existed := resources.Inventories().GetAllItemStack(resources_control.WindowID(p.WindowID))
		if !existed {
			return nil
		}
		keys := make([]int, 0, len(inventory))
		for k := range inventory {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		contents := make([]protocol.ItemInstance, len(keys))
		for i, k := range keys {
			contents[int(i)] = *inventory[resources_control.SlotID(k)]
		}
		if err := ConvertStruct(&contents, &std.Content); err != nil {
			return err
		}
		for k, v := range contents {
			std.Content[k].Stack.NBTData = v.Stack.NBTData
		}
	case *packet.ItemStackResponse:
		std := standardPacket.(*standard_packet.ItemStackResponse)
		responses := std.Responses
		for _, response := range responses {
			if response.ContainerInfo == nil {
				return nil
			}
			for i := range response.ContainerInfo {
				containerInfo := &response.ContainerInfo[i]
				if containerInfo.ContainerID > 17 {
					containerInfo.ContainerID--
				}
			}
		}
	case *packet.SubChunk:
		std := standardPacket.(*standard_packet.SubChunk)
		err := N2SSubChunk(std)
		if err != nil {
			utils.Log.Error(err.Error())
		}
		std.Dimension = resources.UQHolder().Micro().GetExtendInfo().GetClientDimension()
	case *packet.ClientBoundMapItemData:
		std := standardPacket.(*standard_packet.ClientBoundMapItemData)
		std.Dimension = byte(resources.UQHolder().Micro().GetExtendInfo().GetClientDimension())
	case *packet.AvailableCommands:
		std := standardPacket.(*standard_packet.AvailableCommands)
		customCommands := gameInterface.SpecialCommandManager().GenerateAvailableSpecialCommands()
		std.Commands = append(std.Commands, customCommands...)
	case *packet.UpdatePlayerGameType:
		std := standardPacket.(*standard_packet.UpdatePlayerGameType)
		if std.PlayerUniqueID == resources.UQHolder().Micro().GetBotBasicInfo().GetBotUniqueID() {
			forwardNeteaseToStandard(
				&packet.SetPlayerGameType{
					GameType: std.GameType,
				},
				standardConn,
				gameInterface,
			)
		}
	case *packet.ChangeDimension: // 屌你妈的臭维度。摆烂了。不修了。太他妈难了。
		std := standardPacket.(*standard_packet.ChangeDimension)
		sourceDimension, _ := resources.UQHolder().Micro().GetExtendInfo().GetBotDimension()
		targetDimension := std.Dimension
		sourceIsDm := sourceDimension > 2
		targetIsDm := targetDimension > 2
		clientDimension := resources.UQHolder().Micro().GetExtendInfo().GetClientDimension()
		if !sourceIsDm && !targetIsDm { // 正常 -> 正常
			clientDimension = targetDimension
		} else if !sourceIsDm && targetIsDm { // 正常 -> dm
			if clientDimension == 0 {
				clientDimension = 2
			} else {
				clientDimension = 0
			}
		} else if sourceIsDm && !targetIsDm { // dm -> 正常
			if clientDimension == 0 && targetDimension == 0 {
				clientDimension = 2
			} else if clientDimension == 2 && targetDimension == 2 {
				clientDimension = 0
			}
		} else if sourceIsDm && targetIsDm { // dm -> dm
			if clientDimension == 0 {
				clientDimension = 2
			} else if clientDimension == 0 {
				clientDimension = 0
			}
		}

		/*
			if !sourceIsDm && !targetIsDm { // 正常 -> 正常
				clientDimension = targetDimension
			} else if !sourceIsDm && targetIsDm { // 正常 -> dm
				clientDimension = 2
				if sourceDimension == 2 {
					clientDimension = 1
				}
			} else if sourceIsDm && !targetIsDm { // dm -> 正常
				if targetDimension == 0 {
					clientDimension = 2
				} else {
					clientDimension = targetDimension
				}
			} else if sourceIsDm && targetIsDm { // dm -> dm
				clientDimension = 2
			}
		*/
		resources.UQHolder().Micro().GetExtendInfo().SetClientDimension(clientDimension)
		std.Dimension = clientDimension
		resources.HandlePacket(&neteasePacket)
	case *packet.LevelChunk:
		std := standardPacket.(*standard_packet.LevelChunk)
		std.Dimension = resources.UQHolder().Micro().GetExtendInfo().GetClientDimension()
	case *packet.InventorySlot:
		std := standardPacket.(*standard_packet.InventorySlot)
		std.NewItem.Stack.NBTData = p.NewItem.Stack.NBTData
	case *packet.MobArmourEquipment:
		std := standardPacket.(*standard_packet.MobArmourEquipment)
		std.Helmet.Stack.NBTData = p.Helmet.Stack.NBTData
		std.Chestplate.Stack.NBTData = p.Chestplate.Stack.NBTData
		std.Leggings.Stack.NBTData = p.Leggings.Stack.NBTData
		std.Boots.Stack.NBTData = p.Boots.Stack.NBTData
	case *packet.MobEquipment:
		std := standardPacket.(*standard_packet.MobEquipment)
		std.NewItem.Stack.NBTData = p.NewItem.Stack.NBTData
	case *packet.InventoryTransaction:
		std := standardPacket.(*standard_packet.InventoryTransaction)
		for k, v := range p.Actions {
			std.Actions[k].OldItem.Stack.NBTData = v.OldItem.Stack.NBTData
			std.Actions[k].NewItem.Stack.NBTData = v.NewItem.Stack.NBTData
		}
		switch td := p.TransactionData.(type) {
		case *protocol.UseItemTransactionData:
			uitd := std.TransactionData.(*standard_protocol.UseItemTransactionData)
			for k, v := range td.Actions {
				uitd.Actions[k].OldItem.Stack.NBTData = v.OldItem.Stack.NBTData
				uitd.Actions[k].NewItem.Stack.NBTData = v.NewItem.Stack.NBTData
			}
			uitd.HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		case *protocol.UseItemOnEntityTransactionData:
			std.TransactionData.(*standard_protocol.UseItemOnEntityTransactionData).HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		case *protocol.ReleaseItemTransactionData:
			std.TransactionData.(*standard_protocol.ReleaseItemTransactionData).HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		}
	}
	/*
		pjb, _ = json.Marshal(standardPacket)
		fmt.Println("Server", neteasePacket.ID(), string(pjb))
	*/

	// 写入标准服务器
	if err := standardConn.WritePacket(standardPacket); err != nil {
		return err
	}

	return nil
}

// forwardStandardToNetease 转发标准客户端数据包到网易服务器
func forwardStandardToNetease(standardPacket standard_packet.Packet, standardConn *standard_minecraft.Conn, gameInterface *game_interface.GameInterface) error {
	/*
		pjb, _ := json.Marshal(standardPacket)
		fmt.Println("StandardClient", standardPacket.ID(), string(pjb))
	*/
	resources := gameInterface.Resources()
	// 获取对应的网易数据包类型
	neteasePacketType := pool[standardPacket.ID()]
	if neteasePacketType == nil {
		return nil
	}

	neteaseConn := resources.Client().Conn()
	neteasePacket := neteasePacketType()

	// 特殊包处理逻辑
	switch p := standardPacket.(type) {
	case *standard_packet.InventoryTransaction:
		ne := neteasePacket.(*packet.InventoryTransaction)
		ne.LegacyRequestID = p.LegacyRequestID
		if err := ConvertStruct(&p.LegacySetItemSlots, &ne.LegacySetItemSlots); err != nil {
			return err
		}
		if err := ConvertStruct(&p.Actions, &ne.Actions); err != nil {
			return err
		}
		switch transactionData := p.TransactionData.(type) {
		case *standard_protocol.NormalTransactionData:
			ne.TransactionData = &protocol.NormalTransactionData{}
			if err := ConvertStruct(transactionData, ne.TransactionData); err != nil {
				return err
			}
		case *standard_protocol.MismatchTransactionData:
			ne.TransactionData = &protocol.MismatchTransactionData{}
			if err := ConvertStruct(transactionData, ne.TransactionData); err != nil {
				return err
			}
		case *standard_protocol.UseItemTransactionData:
			ne.TransactionData = &protocol.UseItemTransactionData{}
			if err := ConvertStruct(transactionData, ne.TransactionData); err != nil {
				return err
			}
		case *standard_protocol.UseItemOnEntityTransactionData:
			ne.TransactionData = &protocol.UseItemOnEntityTransactionData{}
			if err := ConvertStruct(transactionData, ne.TransactionData); err != nil {
				return err
			}
		case *standard_protocol.ReleaseItemTransactionData:
			ne.TransactionData = &protocol.ReleaseItemTransactionData{}
			if err := ConvertStruct(transactionData, ne.TransactionData); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown transaction data type: %T", transactionData)
		}
	case *standard_packet.ItemStackRequest:
		ne := neteasePacket.(*packet.ItemStackRequest)
		neteaseItemStackRequests := make([]protocol.ItemStackRequest, 0)
		for _, request := range p.Requests {
			neteaseItemStackRequest, err := S2NItemStackRequest(request)
			if err != nil {
				return fmt.Errorf("conversion failed for packet %d: %w", standardPacket.ID(), err)
			}
			neteaseItemStackRequests = append(neteaseItemStackRequests, neteaseItemStackRequest)
		}
		ne.Requests = neteaseItemStackRequests
	case *standard_packet.PlayerAuthInput:
		ne := neteasePacket.(*packet.PlayerAuthInput)
		ne.Pitch = p.Pitch
		ne.Yaw = p.Yaw
		ne.Position = p.Position
		ne.MoveVector = p.MoveVector
		ne.HeadYaw = p.HeadYaw
		ne.InputData = p.InputData
		ne.InputMode = p.InputMode
		ne.PlayMode = p.PlayMode
		ne.InteractionModel = p.InteractionModel
		ne.GazeDirection = p.GazeDirection
		ne.Tick = p.Tick
		ne.Delta = p.Delta
		ne.ItemInteractionData = protocol.UseItemTransactionData{}
		if err := ConvertStruct(&p.ItemInteractionData, &ne.ItemInteractionData); err != nil {
			return err
		}
		neteaseItemStackRequest, err := S2NItemStackRequest(p.ItemStackRequest)
		if err != nil {
			return fmt.Errorf("conversion failed for packet %d: %w", standardPacket.ID(), err)
		}
		ne.ItemStackRequest = neteaseItemStackRequest // 他妈的。我真服了。
		ne.BlockActions = []protocol.PlayerBlockAction{}
		if err := ConvertStruct(&p.BlockActions, &ne.BlockActions); err != nil {
			return err
		}
		ne.VehicleRotation = p.VehicleRotation
		ne.ClientPredictedVehicle = p.ClientPredictedVehicle
		ne.AnalogueMoveVector = p.AnalogueMoveVector
	default:
		if err := ConvertStruct(standardPacket, neteasePacket); err != nil {
			return fmt.Errorf("conversion failed for packet %d: %w", standardPacket.ID(), err)
		}
	}

	switch ne := neteasePacket.(type) {
	case *packet.CommandRequest:
		if gameInterface.SpecialCommandManager().HandleSpecialCommand(ne.CommandLine, standardConn, gameInterface) {
			return nil
		}
	case *packet.PlayerAuthInput:
		ne.Tick = 0
		ne.InputMode = packet.InputModeTouch
		ne.PlayMode = packet.PlayModeScreen
		ne.InteractionModel = packet.InteractionModelCrosshair
		ne.Delta = mgl32.Vec3{}
		ne.MoveVector = mgl32.Vec2{}
	case *packet.SubChunkRequest:
		dimension, _ := resources.UQHolder().Micro().GetExtendInfo().GetBotDimension()
		ne.Dimension = dimension
	case *packet.AddItemActor:
		p := standardPacket.(*standard_packet.AddItemActor)
		ne.Item.Stack.NBTData = p.Item.Stack.NBTData
	case *packet.AddPlayer:
		p := standardPacket.(*standard_packet.AddPlayer)
		ne.HeldItem.Stack.NBTData = p.HeldItem.Stack.NBTData
	case *packet.InventoryContent:
		p := standardPacket.(*standard_packet.InventoryContent)
		for k, v := range p.Content {
			ne.Content[k].Stack.NBTData = v.Stack.NBTData
		}
	case *packet.InventorySlot:
		p := standardPacket.(*standard_packet.InventorySlot)
		ne.NewItem.Stack.NBTData = p.NewItem.Stack.NBTData
	case *packet.MobArmourEquipment:
		p := standardPacket.(*standard_packet.MobArmourEquipment)
		ne.Helmet.Stack.NBTData = p.Helmet.Stack.NBTData
		ne.Chestplate.Stack.NBTData = p.Chestplate.Stack.NBTData
		ne.Leggings.Stack.NBTData = p.Leggings.Stack.NBTData
		ne.Boots.Stack.NBTData = p.Boots.Stack.NBTData
	case *packet.MobEquipment:
		p := standardPacket.(*standard_packet.MobEquipment)
		ne.NewItem.Stack.NBTData = p.NewItem.Stack.NBTData
	case *packet.InventoryTransaction:
		p := standardPacket.(*standard_packet.InventoryTransaction)
		for k, v := range p.Actions {
			ne.Actions[k].OldItem.Stack.NBTData = v.OldItem.Stack.NBTData
			ne.Actions[k].NewItem.Stack.NBTData = v.NewItem.Stack.NBTData
		}
		switch td := p.TransactionData.(type) {
		case *standard_protocol.UseItemTransactionData:
			uitd := ne.TransactionData.(*protocol.UseItemTransactionData)
			for k, v := range td.Actions {
				uitd.Actions[k].OldItem.Stack.NBTData = v.OldItem.Stack.NBTData
				uitd.Actions[k].NewItem.Stack.NBTData = v.NewItem.Stack.NBTData
			}
			uitd.HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		case *standard_protocol.UseItemOnEntityTransactionData:
			ne.TransactionData.(*protocol.UseItemOnEntityTransactionData).HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		case *standard_protocol.ReleaseItemTransactionData:
			ne.TransactionData.(*protocol.ReleaseItemTransactionData).HeldItem.Stack.NBTData = td.HeldItem.Stack.NBTData
		}
		/*
			case *packet.PlayerAction:
				p := standardPacket.(*standard_packet.PlayerAction)
				clientDimension := resources.UQHolder().Micro().GetExtendInfo().GetClientDimension()
				dimension, _ := resources.UQHolder().Micro().GetExtendInfo().GetBotDimension()
				position, _ := resources.UQHolder().Micro().GetExtendInfo().GetBotPosition()
				if p.ActionType == standard_protocol.PlayerActionDimensionChangeDone && clientDimension != dimension && clientDimension != 0 {
					if dimension > 2 {
						clientDimension = 0
					} else {
						clientDimension = dimension
					}
					resources.UQHolder().Micro().GetExtendInfo().SetClientDimension(clientDimension)
					if err := standardConn.WritePacket(&standard_packet.ChangeDimension{
						Dimension: clientDimension,
						Position:  position,
					}); err != nil {
						return err
					}
					return nil
				}
		*/
	}
	if !resources.HandlePacket(&neteasePacket) {
		return nil
	}
	/*
		pjb, _ = json.Marshal(neteasePacket)
		fmt.Println("Client", neteasePacket.ID(), string(pjb))
	*/
	// 写入网易服务器
	if err := neteaseConn.WritePacket(neteasePacket); err != nil {
		return err
	}

	return nil
}

func checkForUpdate() (bool, string, error) {
	url := "https://github.yeqing.net/https://raw.githubusercontent.com/Yeah114/FunShuttlerRelease/main/version"

	// 创建带超时的HTTP客户端
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("HTTP错误: %s", resp.Status)
	}

	// 读取版本内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", err
	}

	// 去除空白字符并标准化格式
	remoteVersion := strings.TrimSpace(string(body))
	cleanRemote := strings.ToLower(strings.TrimPrefix(remoteVersion, "v"))

	// 处理当前版本格式
	cleanCurrent := strings.ToLower(strings.TrimPrefix(currentVersion, "v"))

	// 比较版本
	if cleanRemote != cleanCurrent {
		return true, remoteVersion, nil
	}

	return false, remoteVersion, nil
}

const currentVersion = "v1.0.0"

// getCurrentDeviceInfo 基于gopsutil获取系统和硬件信息
func getCurrentDeviceInfo() string {
	var infoParts []string

	// 1. 系统信息（操作系统、版本）
	osInfo, err := host.Info()
	if err == nil {
		osStr := fmt.Sprintf("%s %s", osInfo.OS, osInfo.PlatformVersion)
		infoParts = append(infoParts, osStr)
	} else {
		infoParts = append(infoParts, fmt.Sprintf("OS: Unknown (%v)", err))
	}

	// 2. 系统架构
	infoParts = append(infoParts, fmt.Sprintf("Arch: %s", runtime.GOARCH))

	// 3. 处理器信息（型号）
	cpuInfo, err := cpu.Info()
	if err == nil && len(cpuInfo) > 0 {
		// 取第一个CPU核心的型号（通常所有核心型号一致）
		cpuModel := strings.TrimSpace(cpuInfo[0].ModelName)
		infoParts = append(infoParts, fmt.Sprintf("CPU: %s", cpuModel))
	} else {
		infoParts = append(infoParts, "CPU: Unknown")
	}

	// 4. 内存信息（总内存）
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		// 转换为GB（保留1位小数）
		totalMemGB := float64(memInfo.Total) / 1024 / 1024 / 1024
		infoParts = append(infoParts, fmt.Sprintf("Mem: %.1f GB", totalMemGB))
	} else {
		infoParts = append(infoParts, "Mem: Unknown")
	}

	// 拼接所有信息，用 | 分隔
	return strings.Join(infoParts, " | ")
}

var ReqPublicKey = []byte(`-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwVkvDSb8N3mdTC3w0fSt
JFAP+dQI94kW5XFLLqMdUHo5tanNz4AnZ4NHRwA2EATel1bt8Bp0H0aD44LotpxO
jmo5M46KllvZ9erJfmYEuAKJJBc9zMlgdOvcstPfeKArKXY7mL1uT8feWe0xVz32
LJnjHFQtCqnAJnTaz5tkPoRjwmR5IVJ92TKIqHxG9QDAYweFgt2aEEB06CnZNLcE
8+VEOqYosRt0KKnxhQYiob6QFPbkcLzpY2qnBX2fC7aVkHf1euSkvjg+fVGppReB
w2orcwYYw4AJm/HKOKrOi8EnpZHwWr9clcXGm1Y2TP42hiR+UPqjk99w8s2U8+q7
9QIDAQAB
-----END PUBLIC KEY-----`)

var ResPrivateKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCinZiWCFR+UYNx
g2uP3nXUBjGkhps7EbUlLC2jv2BqVLYx+sE8EXwfsTzcDzw6oiB0Rve1OBPRm8nA
W/PuvuEn0MVATSwIvSyldqQA0b6sA7u08mb1dj0uoz46d+799BC1we1qsruFLfZk
Uzn7fLmJbcoK1CUTqqGEqDSkKrcwtxatSUeeMk03BHPLdjjZtpuhSYgrHgi12M5o
n/j9KIg8eouHtVvptUCQMVNorxd5SSiJpVEfK029VQ9eaCGqVwYCdJ+pbVsIaiug
G3Ht1uxMzhkKAerJvj1UXjHFPy34qTEtgD3tGedsUWvJCn8PqHeTu6m3sTbHbMkM
aL6KNtDvAgMBAAECggEAMOhCEHmP4+/jHvw+GOZ4vJxiSM+9Fx5jwzrOvS1dM42n
wE9g146QrL1zBuJEJIRHS4Fela+4cjWvD334VukssXteH9E1+ajfqSIOADsU5Ao5
ZZnz8kYt731kRVR12gYSEoAxRgJcWJkyvgiz6xmfeaA+AFS+k/QhINMwBeHc97bW
TbRcl31uRgS1o/dOxKa/8mSMEfw4BosPvbF06k/ME1WSLU7TpoyF1CNgbkNyVAwb
45zw2vxAzIaZWbqR1muFP9bTVuywtQ5Tyn/qr9D0oXic+goi2Lg+6aI139put2f9
k/hD6otiND3gil33+2ZXrJa2rv2BeKtZETP85TbG9QKBgQDMxioM9dzS7nE/bgZe
HLJHXYughDYQQS1ONIFMz04j+qhZgggnWIFWcdN2xSYZprKWIJopXHi9s6JKeRl9
dTW02zeN6ZoGHpM3rKI4+54TKv/5zL+vrEdsSE359GETh1xY1BoTYC14Ugb/FzKm
Mdkt75tKLLX8pVbeJDM5NoTWxQKBgQDLS5UAH2AIkv8ZrcmyJsCC0yLL0Xof1qTO
66nnDNxeBWshBBXjoKmqU3IpRc8N1+hWtUW5oTaJWhhqMj4erZODBcR626ORDRoE
ujGZAkemEG44JE0BhYx8QAHcFK/bzbUFpuGMFhr+JPjogZK4b+aqRCjZ0NdjHvoy
uTsSaA7kIwKBgGkelTmgASM7CmHAbn5jhtWB3QqitEo12zcZVwMCLJ/lrgR87wib
s5GXzzFaVYu3elf9FoaDFOx1SaunbQFtaQuVuXgdYlcf3P0onwJXkLdQTKWEiSZ6
bxPULktRO4BV7X8gKt6lcdIhcbwDwZl1X4LoTLSQLUAr9GMG/FCzfw/tAoGATkXG
k6qA3ixCvg9UlhwniPdj8VexDqsk/2prG0OvYR/y9F5PUzobuTGxxcRUCT90/FWF
CAjTJ6Uhu1mm40bvgcXptWf1nTaPKvzJdeET1xOCbGlFoO1bfsdiL+tjHl0gcpGj
S72DHRD9KQAijpK/HBVsKRkopMhbisYYwWiBO0MCgYBXXMob7lMkyin1ZFdxhdoW
hfGzYdcHf9n1+jBO+UlAieAR44Q/dXHuCEVcLmltM4pzwWZYYvWsoWRQzWwLRk3I
63J2AIei5v3EsQ7CioIc7ED7NaqTZ0pHN8yR8VgJHLVqatjWxNmpKfvCkEnFj784
KEAo31attRWDFfaurxDd7Q==
-----END PRIVATE KEY-----`)

const ServerURL = "https://funshuttler.yeah114.top"

func main() {
	i18n.Init(i18n.LanguageSimplifiedChinese)
	utils.Log.Info(fmt.Sprintf("FunShuttler %s", currentVersion))
	utils.Log.Info("Authors: Yeah、KashShinfu")
	utils.Log.Info("正在检查更新")

	// 检查更新
	/*
		updateAvailable, remoteVersion, err := checkForUpdate()
		if err != nil {
			utils.Log.Error(fmt.Sprintf("检查更新失败: %v\n", err))
			return
		}
	*/
	updateAvailable := false
	remoteVersion := ""

	if updateAvailable {
		utils.Log.Info(fmt.Sprintf("发现新版本: %s (当前版本: %s)", remoteVersion, currentVersion))
		utils.Log.Info("请前往 https://github.com/Yeah114/FunShuttlerRelease 下载最新版本")
		return
	}

	utils.Log.Info("当前已是最新版本")
	utils.Log.Info("更新检查完成")

	/*
		// 初始化绑定客户端
		bindClient, err := bind.NewCryptoClient(
			ServerURL,
			ReqPublicKey,
			ResPrivateKey,
		)
		if err != nil {
			utils.Log.Error(fmt.Sprintf("初始化绑定客户端失败: %v", err))
			return
		}

		currentDeviceInfo := getCurrentDeviceInfo()

		// 检查QQ绑定状态
		utils.Log.Info("正在检查QQ绑定状态")
		qq, err := strconv.ParseInt(config.ShuttlerConfig.QQ, 10, 64)
		if err != nil {
			utils.Log.Error(fmt.Sprintf("QQ号解析失败: %v", err))
			return
		}
		checkRes, err := bindClient.CheckBind(qq)
		if err != nil {
			utils.Log.Error(fmt.Sprintf("检查绑定状态失败: %v", err))
			return
		}

		// 处理绑定逻辑
		bound := checkRes["bound"].(bool)
		if !bound {
			utils.Log.Error("QQ未绑定，请前往群聊1050701691发送FunShuttlerBind绑定")
			return
		}
		utils.Log.Info("QQ已绑定，正在验证设备信息")
		deviceRes, err := bindClient.GetDevice(qq)
		if err != nil {
			utils.Log.Error(fmt.Sprintf("获取绑定设备信息失败: %v", err))
			return
		}
		if deviceRes["device_info"] == nil {
			utils.Log.Info("未发现绑定记录，正在绑定当前设备")
			bindRes, err := bindClient.BindDevice(qq, currentDeviceInfo)
			if err != nil || !bindRes["success"].(bool) {
				utils.Log.Error(fmt.Sprintf("设备绑定失败: %v", err))
				return
			}
			utils.Log.Info("设备绑定成功")
		} else {
			// 已绑定，检查设备信息是否一致
			utils.Log.Info("已发现绑定记录，正在验证设备信息")
			deviceRes, err := bindClient.GetDevice(qq)
			if err != nil {
				utils.Log.Error(fmt.Sprintf("获取绑定设备信息失败: %v", err))
				return
			}

			boundDeviceInfo := deviceRes["device_info"].(string)
			if boundDeviceInfo != currentDeviceInfo {
				utils.Log.Error("设备信息不匹配")
				return
			}
			utils.Log.Info("设备信息验证通过")
		}
	*/

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
	resourcesControl := resources_control.NewResourcesControl(neteaseClient)
	gameInterface := game_interface.NewGameInterface(resourcesControl)

	neteaseConn := neteaseClient.Conn()
	utils.Log.Info("登录完成")
	if !auth.Trust {
		utils.Log.Warn("您所使用的验证服务被初步判定是不受信任的")
		utils.Log.Warn("一切后果应由您自行承担")
	}

	utils.Log.Info("正在创建基岩版服务器")
	// 创建标准服务器监听器
	listenerCfg := standard_minecraft.ListenConfig{
		MaximumPlayers:         1,
		AuthenticationDisabled: true,
		AllowInvalidPackets:    true,
		AllowUnknownPackets:    true,
		StatusProvider:         standard_minecraft.NewStatusProvider(fmt.Sprintf("§bFunShuttler §e%s§r", config.ShuttlerConfig.RentalServerCode), currentVersion),
	}
	listener, err := listenerCfg.Listen("raknet", config.ShuttlerConfig.ListenAddress)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	// 接受标准客户端连接
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	utils.Log.Info("基岩版服务器创建完成")
	utils.Log.Info(fmt.Sprintf("请使用基岩版Minecraft连接至 %s", config.ShuttlerConfig.ListenAddress))

	closed := make(chan struct{})
	go func() {
		for {
			standardNetConn, err := listener.Accept()
			utils.Log.Info("正在与客户端连接")
			if err != nil {
				utils.Log.Error(fmt.Sprintf("连接至客户端时发生错误: ", err))
				panic(err)
			}
			standardConn := standardNetConn.(*standard_minecraft.Conn)
			utils.Log.Info(fmt.Sprintf("已连接至客户端: %v", standardConn.RemoteAddr()))
			defer standardConn.Close()

			utils.Log.Info("正在转换登录数据")
			neteaseGameData := neteaseConn.GameData()
			standardGameData := standard_minecraft.GameData{}
			if err := ConvertStruct(&neteaseGameData, &standardGameData); err != nil {
				utils.Log.Error(fmt.Sprintf("转换登录数据时发生错误: %v", err))
				panic(err)
			}
			if standardGameData.Dimension > 2 {
				standardGameData.Dimension = 0
			}
			resourcesControl.UQHolder().Micro().GetExtendInfo().SetClientDimension(standardGameData.Dimension)
			utils.Log.Info("登录数据转换完成")

			utils.Log.Info("正在启动客户端游戏")
			// 注册所有特殊命令

			if err := standardConn.StartGameContext(ctx, standardGameData); err != nil {
				_ = listener.Disconnect(standardConn, "Connection timeout.")
				utils.Log.Error("启动客户端游戏超时")
				return
			}
			utils.Log.Info("客户端游戏启动完成")
			special_command.RegisterAllSpecialCommands(gameInterface, standardConn)

			utils.Log.Info("正在向客户端发送登录时数据包")
			for {
				neteasePacket := <-neteaseClient.CachedPacket()
				if neteasePacket == nil {
					break
				}
				if err := forwardNeteaseToStandard(neteasePacket, standardConn, gameInterface); err != nil {
					utils.Log.Warn(fmt.Sprintf("转发至基岩版数据包 %d 时出错: %v", neteasePacket.ID(), err))
				}
			}
			utils.Log.Info("向客户端发送登录时数据包完成")

			utils.Log.Info("正在启动数据包转发")
			// 处理网易->标准方向的流量
			go func() {
				//				close(done)
				//				<-started
				standardConn.WritePacket(&standard_packet.ToastRequest{
					Title:   "§6◆ FunShuttler §r",
					Message: fmt.Sprintf("§b✓ 成功连接至租赁服: §e%s", config.ShuttlerConfig.RentalServerCode),
				})
				standardConn.WritePacket(&standard_packet.SetTitle{
					ActionType: standard_packet.TitleActionSetActionBar,
					Text:       "§a请合理使用 FunShuttler",
				})
				for {
					neteasePacket, err := neteaseConn.ReadPacket()
					if err != nil {
						utils.Log.Info(fmt.Sprintf("租赁服连接已关闭: %v", err))
						close(closed)
						return
					}
					if err := forwardNeteaseToStandard(neteasePacket, standardConn, gameInterface); err != nil {
						utils.Log.Warn(fmt.Sprintf("转发至基岩版数据包 %d 时出错: %v", neteasePacket.ID(), err))
					}
				}
			}()

			// 处理标准->网易方向的流量
			go func() {
				//		MoveToPosition(*neteaseConn, mgl32.Vec3{0, 0, 0}, mgl32.Vec3{0, 0, 0})
				for {
					standardPacket, err := standardConn.ReadPacket()
					if err != nil {
						utils.Log.Info(fmt.Sprintf("基岩版连接已关闭: %v", err))
						close(closed)
						return
					}
					if err := forwardStandardToNetease(standardPacket, standardConn, gameInterface); err != nil {
						utils.Log.Warn(fmt.Sprintf("转发至租赁服数据包 %d 时出错: %v", standardPacket.ID(), err))
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
