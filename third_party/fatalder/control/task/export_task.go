package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
	futils "github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/FunShuttler/core/minecraft/nbt"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/block_actors"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/utils"
	structure_define "github.com/Yeah114/WaterStructure/define"
	"github.com/mholt/archiver/v3"
	"github.com/mitchellh/mapstructure"
)

func MergeMaps(mapping ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, value := range mapping {
		maps.Copy(result, value)
	}
	return result
}

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
				// pterm.Printfln("decode chunk NBT error %v", err)
				utils.Log.Warn(fmt.Sprintf("NEMCSubChunkDecode: decode chunk NBT error %v", err))
				break
			}
			// decode block NBT
			id, has1 := blockData["id"].(string)
			tagNBT, has2 := blockData["__tag"].(string)
			if has1 && has2 {
				result, err := NEMCTagNBTDecode(id, tagNBT)
				if err != nil {
					utils.Log.Warn(fmt.Sprintf("NEMCSubChunkDecode: decode chunk __tag NBT error %v", err))
					continue
					// panic(fmt.Sprintf("NEMCSubChunkDecode: decode chunk __tag NBT error %v", err))
				}
				delete(blockData, "__tag")
				blockData = MergeMaps(blockData, result)
			}
			// decode __tag NBT
			nbts = append(nbts, blockData)
			// submit sub result
		}
	}
	return
}

// 将方块类型为 ID 且 方块实体数据 为 tag 的 方块实体 解析为正常格式
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

// isSensitiveFile 检查文件名是否为敏感文件，不应被打包进导出的 mcworld
// 这可以防止 API Key 等敏感信息被意外包含在导出文件中
func isSensitiveFile(filename string) bool {
	// 敏感文件列表（小写比较）
	sensitiveFiles := []string{
		"config.json",          // Fatalder 配置文件，包含 API Key
		"build_interrupt.json", // 构建中断恢复文件
		".env",                 // 环境变量文件
		"credentials.json",     // 凭据文件
		"secrets.json",         // 密钥文件
	}
	lowerName := strings.ToLower(filename)
	for _, sensitive := range sensitiveFiles {
		if lowerName == sensitive {
			return true
		}
	}
	// 过滤以 . 开头的隐藏文件（Unix 风格）
	if strings.HasPrefix(filename, ".") && filename != "." && filename != ".." {
		return true
	}
	return false
}

type ExportTask struct {
	control       control.Control
	id            int
	TaskType      TaskType         `json:"type"`
	FilePath      string           `json:"file_path"`
	FileType      string           `json:"file_type"`
	DimensionName string           `json:"dimension_name"`
	StartPos      defines.BlockPos `json:"start_pos"`
	EndPos        defines.BlockPos `json:"end_pos"`

	startCallback                        func(size structure_define.Size)
	everyChunkExportCallback             func(chunkPos define.ChunkPos)
	everySubChunkSaveFailedCallback      func(subChunkPos define.SubChunkPos, err error)
	everySubChunkDecodeNBTFailedCallback func(subChunkPos define.SubChunkPos, err error)
	everyChunkNBTSaveFailedCallback      func(chunkPos define.ChunkPos, err error)
	finishedCallback                     func(path string)

	stateMu sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
}

func NewExportTask() *ExportTask {
	return &ExportTask{
		TaskType: TaskTypeExport,
		FileType: "mcworld",
	}
}

func (e *ExportTask) SetControl(c defines.Control) {
	ctrl, ok := c.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	e.control = *ctrl
	e.id = e.control.GetTaskID()
}

func (e *ExportTask) ID() int {
	return e.id
}

func (e *ExportTask) Type() string {
	return string(e.TaskType)
}

func (e *ExportTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), e)
}

func (e *ExportTask) ToJSON() (string, error) {
	bs, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (e *ExportTask) SetStartCallback(callback func(size structure_define.Size)) {
	e.startCallback = callback
}

func (e *ExportTask) SetEveryChunkExportCallback(callback func(chunkPos define.ChunkPos)) {
	e.everyChunkExportCallback = callback
}

func (e *ExportTask) SetEverySubChunkSaveFailedCallback(callback func(subChunkPos define.SubChunkPos, err error)) {
	e.everySubChunkSaveFailedCallback = callback
}

func (e *ExportTask) SetEverySubChunkDecodeNBTFailedCallback(callback func(subChunkPos define.SubChunkPos, err error)) {
	e.everySubChunkDecodeNBTFailedCallback = callback
}

func (e *ExportTask) SetEveryChunkNBTSaveFailedCallback(callback func(chunkPos define.ChunkPos, err error)) {
	e.everyChunkNBTSaveFailedCallback = callback
}

func (e *ExportTask) SetFinishedCallback(callback func(path string)) {
	e.finishedCallback = callback
}

func (e *ExportTask) taskContext() context.Context {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.ctx != nil {
		return e.ctx
	}
	return context.Background()
}

func (e *ExportTask) setContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	e.stateMu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.running = true
	e.stateMu.Unlock()
}

func (e *ExportTask) clearContext() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.ctx = nil
	e.running = false
}

func (e *ExportTask) waitForContext() error {
	ctx := e.taskContext()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// 定义区域结构体，存储起始和结束坐标
type Area struct {
	Start defines.BlockPos
	End   defines.BlockPos
}

// splitArea 功能：将指定3D区域按blockRadius分割为多个子区域
// 参数：startPos、endPos 分别为原始区域的起始和结束坐标
// 返回值：分割后的子区域切片（每个元素包含起始和结束坐标）
func splitArea(startPos, endPos defines.BlockPos, chunkRadius int32) []Area {
	blockRadius := (chunkRadius - 1) * 15 / 10 * 16
	// 1. 计算原始区域各轴的最小值和最大值（确保顺序正确）
	minX, maxX := startPos.X(), endPos.X()
	if minX > maxX {
		minX, maxX = maxX, minX
	}

	minY, maxY := startPos.Y(), endPos.Y()
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	minZ, maxZ := startPos.Z(), endPos.Z()
	if minZ > maxZ {
		minZ, maxZ = maxZ, minZ
	}

	// 2. 计算X、Z轴方向的长度（Y轴不分割，沿用原始范围）
	length := maxX - minX + 1
	width := maxZ - minZ + 1

	// 3. 计算完整区块的数量和剩余部分（整数除法和取余）
	xSteps := length / blockRadius
	zSteps := width / blockRadius
	xRemainder := length % blockRadius
	zRemainder := width % blockRadius

	// 4. 存储分割后的子区域
	var areas []Area

	// 步骤1：添加所有完整的子区域（X、Z轴均为完整blockRadius）
	for xIdx := range xSteps {
		for zIdx := range zSteps {
			startX := minX + xIdx*blockRadius
			startZ := minZ + zIdx*blockRadius
			endX := startX + blockRadius - 1
			endZ := startZ + blockRadius - 1

			areas = append(areas, Area{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{endX, maxY, endZ},
			})
		}
	}

	// 步骤2：添加X轴剩余部分（右侧）的子区域
	if xRemainder > 0 {
		for zIdx := range zSteps {
			startX := minX + xSteps*blockRadius
			startZ := minZ + zIdx*blockRadius
			endX := startX + xRemainder - 1
			endZ := startZ + blockRadius - 1

			areas = append(areas, Area{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{endX, maxY, endZ},
			})
		}
	}

	// 步骤3：添加Z轴剩余部分（底部）的子区域
	if zRemainder > 0 {
		for xIdx := range xSteps {
			startX := minX + xIdx*blockRadius
			startZ := minZ + zSteps*blockRadius
			endX := startX + blockRadius - 1
			endZ := startZ + zRemainder - 1

			areas = append(areas, Area{
				Start: defines.BlockPos{startX, minY, startZ},
				End:   defines.BlockPos{endX, maxY, endZ},
			})
		}
	}

	// 步骤4：添加X、Z轴均有剩余的子区域（右下角）
	if xRemainder > 0 && zRemainder > 0 {
		startX := minX + xSteps*blockRadius
		startZ := minZ + zSteps*blockRadius
		endX := startX + xRemainder - 1
		endZ := startZ + zRemainder - 1

		areas = append(areas, Area{
			Start: defines.BlockPos{startX, minY, startZ},
			End:   defines.BlockPos{endX, maxY, endZ},
		})
	}

	return areas
}

func copySubChunkPosList(src []define.SubChunkPos) []define.SubChunkPos {
	dst := make([]define.SubChunkPos, len(src))
	copy(dst, src)
	return dst
}

func (e *ExportTask) moveBot(pos defines.BlockPos) error {
	if err := e.waitForContext(); err != nil {
		return err
	}
	m := NewMoveTask()
	m.Pos = pos
	m.SetControl(&e.control)
	if err := m.Start(e.taskContext()); err != nil {
		return err
	}
	return nil
}

func (e *ExportTask) Start(ctx context.Context) error {
	e.setContext(ctx)
	defer e.clearContext()

	if err := e.waitForContext(); err != nil {
		return err
	}

	extraOptions := futils.GetExtraOptions()
	if !extraOptions.UnlimitedExport {
		// 禁止在联机大厅导出
		if strings.HasPrefix(e.control.GetConfig().RentalServerCode, "LobbyGame:") {
			fmt.Println("ExportTask is not allowed in the online lobby")
			return errors.New("export task is not allowed in the online lobby")
		}
		// 禁止在本地联机导出
		if strings.HasPrefix(e.control.GetConfig().RentalServerCode, "TanLobby:") {
			fmt.Println("ExportTask is not allowed in the tan lobby")
			return errors.New("export task is not allowed in the tan lobby")
		}
		// 禁止在我的山头导出
		if strings.HasPrefix(e.control.GetConfig().RentalServerCode, "DomainGame:") {
			fmt.Println("ExportTask is not allowed in the domain")
			return errors.New("export task is not allowed in the domain")
		}
	}
	gameInterface := e.control.GameInterface()
	for !extraOptions.SkipExportOpCheck {
		if err := e.waitForContext(); err != nil {
			return err
		}
		resp, err := gameInterface.Commands().SendWSCommandWithResp("listd")
		if err != nil {
			return err
		}
		// 判断是否成功来检查是否拥有OP
		if resp.SuccessCount != 0 {
			break
		}
		fmt.Println("waiting for OP...")
		select {
		case <-time.After(1 * time.Second):
		case <-e.taskContext().Done():
			return e.taskContext().Err()
		}
	}
	botName := e.control.Resources().BotInfo().BotName
	if err := gameInterface.Commands().SendSettingsCommand(fmt.Sprintf(`gamemode 1 "%s"`, e.control.Resources().UQHolder().Micro().GetBotName()), false); err != nil {
		return err
	}
	if e.DimensionName == "" {
		e.DimensionName = "overworld"
	}
	dimensionID, found := gameInterface.Resources().ConstantPacket().DimensionIDByName(e.DimensionName)
	if !found {
		return fmt.Errorf("invalid dimension: %s", e.DimensionName)
	}
	resp, err := gameInterface.Commands().SendWSCommandWithResp(fmt.Sprintf(
		`execute in %s run tp "%s" %d %d %d`,
		e.DimensionName,
		botName,
		e.StartPos.X(),
		e.StartPos.Y(),
		e.StartPos.Z(),
	))
	if err != nil {
		return err
	}
	if resp.SuccessCount == 0 {
		return fmt.Errorf("tp failed with response: %v", resp.OutputMessages)
	}
	// 去除扩展名为世界路径
	worldPath := strings.TrimSuffix(e.FilePath, filepath.Ext(e.FilePath))
	err = os.RemoveAll(worldPath)
	if err != nil {
		return err
	}
	bedrockWorld, err := world.Open(worldPath, nil)
	if err != nil {
		return err
	}
	subChunkRequest := e.control.GameInterface().SubChunkRequest()
	chunkRadius, err := subChunkRequest.SetChunkRadius(32)
	if err != nil {
		return err
	}
	fetchSubChunks := func(list []protocol.SubChunkPos, centerPos defines.BlockPos) (*packet.SubChunk, error) {
		for {
			if err := e.waitForContext(); err != nil {
				return nil, err
			}
			if err := e.moveBot(centerPos); err != nil {
				return nil, err
			}

			request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(
				define.Dimension(dimensionID),
				list,
			)
			resp, isTimeout, err := subChunkRequest.SendSubChunkRequestWithRespTimeout(
				request,
				time.Second,
			)
			if err != nil {
				return nil, err
			}
			if !isTimeout {
				return resp, nil
			}
			fmt.Println("SubChunkRequest was timeout, retry...")
		}
	}
	worldChunkY := e.StartPos.Y() >> 4
	areas := splitArea(e.StartPos, e.EndPos, chunkRadius)
	if e.startCallback != nil {
		e.startCallback(structure_define.Size{
			Width:  int(e.EndPos.X()-e.StartPos.X()) - 1,
			Height: int(e.EndPos.Y()-e.StartPos.Y()) - 1,
			Length: int(e.EndPos.Z()-e.StartPos.Z()) - 1,
		})
	}
	for _, area := range areas {
		if err := e.waitForContext(); err != nil {
			return err
		}
		centerX := (area.Start.X() + area.End.X()) / 2
		centerZ := (area.Start.Z() + area.End.Z()) / 2
		centerPos := defines.BlockPos{centerX, 320, centerZ}
		if err := e.moveBot(centerPos); err != nil {
			return err
		}
		width := area.End.X() - area.Start.X() + 1
		length := area.End.Z() - area.Start.Z() + 1
		if width <= 0 || length <= 0 {
			continue
		}
		height := area.End.Y() - area.Start.Y() + 1
		if height <= 0 {
			continue
		}
		yCount := (height + 15) / 16
		chunkX := area.Start.X() >> 4
		chunkZ := area.Start.Z() >> 4
		chunkXCount := (width + 15) / 16
		chunkZCount := (length + 15) / 16
		subChunkPosList := make([]protocol.SubChunkPos, 0)
		for x := range chunkXCount {
			for z := range chunkZCount {
				chunkPos := defines.ChunkPos{
					chunkX + x,
					chunkZ + z,
				}
				for y := range yCount {
					subChunkPosList = append(subChunkPosList, protocol.SubChunkPos{
						chunkPos.X(),
						worldChunkY + y,
						chunkPos.Z(),
					})
				}
			}
		}
		if len(subChunkPosList) == 0 {
			continue
		}
		resp, err := fetchSubChunks(subChunkPosList, centerPos)
		if err != nil {
			return err
		}
		NBTs := map[define.ChunkPos][]map[string]any{}
		dimension := define.Dimension(define.DimensionIDOverworld)
		processResponse := func(resp *packet.SubChunk) []define.SubChunkPos {
			subChunkStartPos := resp.Position
			nextUnfetched := make([]define.SubChunkPos, 0)
			for _, entry := range resp.SubChunkEntries {
				subChunkPos := define.SubChunkPos{
					subChunkStartPos.X() + int32(entry.Offset[0]),
					subChunkStartPos.Y() + int32(entry.Offset[1]),
					subChunkStartPos.Z() + int32(entry.Offset[2]),
				}
				chunkPos := define.ChunkPos{
					subChunkPos.X(),
					subChunkPos.Z(),
				}
				switch entry.Result {
				case protocol.SubChunkResultSuccess:
					subChunk, _, nbts, err := NEMCSubChunkDecode(entry.RawPayload, dimension.Range())
					if err != nil {
						if e.everySubChunkDecodeNBTFailedCallback != nil {
							e.everySubChunkDecodeNBTFailedCallback(subChunkPos, err)
						}
						continue
					}
					if err := bedrockWorld.SaveSubChunk(dimension, subChunkPos, subChunk); err != nil {
						if e.everySubChunkSaveFailedCallback != nil {
							e.everySubChunkSaveFailedCallback(subChunkPos, err)
						}
						continue
					}
					if _, ok := NBTs[chunkPos]; !ok {
						NBTs[chunkPos] = make([]map[string]any, 0)
						if e.everyChunkExportCallback != nil {
							e.everyChunkExportCallback(chunkPos)
						}
					}
					NBTs[chunkPos] = append(NBTs[chunkPos], nbts...)
				case protocol.SubChunkResultChunkNotFound:
					nextUnfetched = append(nextUnfetched, subChunkPos)
				}
			}
			return nextUnfetched
		}
		unfetchedSubChunksPos := processResponse(resp)
		var previousUnfetched []define.SubChunkPos
		for len(unfetchedSubChunksPos) > 0 {
			if err := e.waitForContext(); err != nil {
				return err
			}
			if len(previousUnfetched) > 0 && len(previousUnfetched) == len(unfetchedSubChunksPos) {
				uniqueChunks := make(map[define.ChunkPos]struct{})
				for _, subChunkPos := range unfetchedSubChunksPos {
					uniqueChunks[define.ChunkPos{subChunkPos.X(), subChunkPos.Z()}] = struct{}{}
				}
			}
			previousUnfetched = copySubChunkPosList(unfetchedSubChunksPos)
			retryList := make([]protocol.SubChunkPos, len(unfetchedSubChunksPos))
			for i, subChunkPos := range unfetchedSubChunksPos {
				retryList[i] = protocol.SubChunkPos{
					subChunkPos.X(),
					subChunkPos.Y(),
					subChunkPos.Z(),
				}
			}
			resp, err := fetchSubChunks(retryList, centerPos)
			if err != nil {
				return err
			}
			unfetchedSubChunksPos = processResponse(resp)
		}
		for chunkPos, nbt := range NBTs {
			// 可以算是完成了这个区块的所有导出
			if len(nbt) == 0 {
				continue
			}
			if err := bedrockWorld.SaveNBT(dimension, chunkPos, nbt); err != nil {
				if e.everyChunkNBTSaveFailedCallback != nil {
					e.everyChunkNBTSaveFailedCallback(chunkPos, err)
				}
				continue
			}
		}
	}
	if err := bedrockWorld.CloseWorld(); err != nil {
		return err
	}
	worldName := fmt.Sprintf("%s@[%d,%d,%d]~[%d,%d,%d]", filepath.Base(worldPath), e.StartPos.X(), e.StartPos.Y(), e.StartPos.Z(), e.EndPos.X(), e.EndPos.Y(), e.EndPos.Z())
	bedrockWorld.LevelDat().LevelName = worldName
	if err := bedrockWorld.UpdateLevelDat(); err != nil {
		return err
	}
	zipPath := filepath.Join(filepath.Dir(worldPath), fmt.Sprintf("%s.zip", worldName))
	_ = os.RemoveAll(zipPath)
	mcworldPath := filepath.Join(filepath.Dir(worldPath), fmt.Sprintf("%s.mcworld", worldName))
	files, err := os.ReadDir(worldPath)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		// 跳过敏感配置文件，防止 API Key 等信息泄露
		if isSensitiveFile(file.Name()) {
			continue
		}
		paths = append(paths, filepath.Join(worldPath, file.Name()))
	}
	z := archiver.Zip{}
	err = z.Archive(paths, zipPath)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(worldPath); err != nil {
		return err
	}
	if err := os.Rename(zipPath, mcworldPath); err != nil {
		return err
	}
	buildingPath := filepath.Join(futils.GetStoragePath(), futils.BuildingFilesDir, filepath.Base(mcworldPath))
	_, _ = copyFile(mcworldPath, buildingPath)
	if e.finishedCallback != nil {
		e.finishedCallback(mcworldPath)
	}
	return nil
}

func (e *ExportTask) Pause() error {
	return errors.New("not implemented")
}

func (e *ExportTask) Resume() error {
	return errors.New("not implemented")
}

func (e *ExportTask) Stop() error {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.running = false
	return nil
}

func (e *ExportTask) IsRunning() bool {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	return e.running
}

func (e *ExportTask) IsPaused() bool {
	return false
}

func (e *ExportTask) MakeInterruptJSON() (string, error) {
	return "", nil
}

func (e *ExportTask) RestoreInterruptJSON(string) error {
	return nil
}

func copyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
