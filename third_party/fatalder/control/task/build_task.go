package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwo_define "github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
	"github.com/Yeah114/WaterStructure/structure"
	"github.com/mholt/archiver/v3"
	"golang.org/x/time/rate"

	"github.com/Yeah114/Fatalder/constants"
	"github.com/Yeah114/Fatalder/control"
	"github.com/Yeah114/Fatalder/defines"
	"github.com/Yeah114/Fatalder/utils"
	"github.com/Yeah114/Fatalder/utils/chunk_fill"
	"github.com/Yeah114/Fatalder/utils/command_upgrader"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
	uc "github.com/Yeah114/Fatalder/utils/user_center"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/nbt_assigner"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_cache"
	"github.com/Yeah114/FunShuttler/nbt_assigner/nbt_console"
	nbt_block "github.com/Yeah114/FunShuttler/nbt_parser/block"

	controlauth "github.com/Yeah114/Fatalder/control/auth"
)

// 计算文件 sha256 哈希
func computeFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

const (
	VerifyChunkLevelNone int = iota + 1
	VerifyChunkLevelAir
	VerifyChunkLevelSimilar
	VerifyChunkLevelComplete
)

// BuildTaskInterruptData 中断数据，包含进度信息和完整任务配置
type BuildTaskInterruptData struct {
	// 进度信息
	CurrentChunkIndex int `json:"current_chunk_index"`
	TotalChunks       int `json:"total_chunks"`

	// 任务配置信息（用于恢复任务）
	TaskConfig BuildTask `json:"task_config"`
}

// BlockCountInfo 方块数量统计信息
type BlockCountInfo struct {
	BlockCount        int `json:"block_count"`
	CommandBlockCount int `json:"command_block_count"`
	OtherNBTCount     int `json:"other_nbt_count"`
}

type sizeOverrideStructure struct {
	structure.Structure
	size define.Size
}

func (s sizeOverrideStructure) GetSize() define.Size {
	return s.size
}

type BuildTask struct {
	control              control.Control
	id                   int
	TaskType             TaskType         `json:"type"`
	FilePath             string           `json:"file_path"`
	StartPos             defines.BlockPos `json:"start_pos"`
	originalStartPos     defines.BlockPos `json:"-"`
	buildStartPos        defines.BlockPos `json:"-"`
	DimensionName        string           `json:"dimension_name"`
	Speed                int              `json:"speed"`
	ChunkGroupSide       int              `json:"chunk_group_side"`
	AutoCleanBlock       bool             `json:"auto_clean_block"`
	AutoCleanItem        bool             `json:"auto_clean_item"`
	AutoPlaceDenyBlock   bool             `json:"auth_place_deny_block"`
	AutoPlaceBorderBlock bool             `json:"auto_place_border_block"`
	AutoDisableCommand   bool             `json:"auto_disable_command"`
	EnableDeltaMode      bool             `json:"enable_delta_mode"`
	IgnoreCommandBlock   bool             `json:"ignore_command_block"`
	IgnoreOtherNBTBlock  bool             `json:"ignore_other_nbt_block"`
	AutoUpgradeCommand   bool             `json:"auto_upgrade_command"`
	WaitChunkLoad        bool             `json:"wait_chunk_load"`
	ConvertToMCWorld     bool             `json:"convert_to_mcworld"`
	ConsoleWorldPos      defines.BlockPos `json:"console_world_pos"`
	ShowNBTProgress      bool             `json:"show_nbt_progress"`
	ShowProgressInGame   bool             `json:"show_progress_in_game"`
	VerifyAfterChunk     int              `json:"verify_after_chunk"`
	VerifyChunkLevel     int              `json:"verify_chunk_level"`
	AutoEnterFixMode     bool             `json:"auto_enter_fix_mode"`
	EnterFixModeDirectly bool             `json:"enter_fix_mode_directly"`
	Progress             int              `json:"progress"` // 起始进度（区块索引）
	reader               structure.Structure
	chunkManager         *ChunkManager
	skipCounting         bool
	limiter              *rate.Limiter

	// 状态管理
	running bool
	paused  bool
	stopped bool
	stateMu sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	pauseCh chan struct{}
	// 修补模式监听器唯一ID
	fixModeListenerUniqueID string

	// 开始解析结构文件时的回调函数
	startCallback func()
	// 获取到结构体类型时的回调函数
	gotTypeCallback func(structure.Structure)
	// 开始读取结构文件时的回调函数
	parsingCallback func()
	// 读取结构文件完成时的回调函数
	parsedCallback func(structure.Structure)
	// 开始进行构建时的回调函数
	startBuildCallback func()
	// 每个区块构建完成时的回调函数
	everyChunkBuiltCallback func(defines.ChunkPos, *chunk.Chunk)
	// 每个区块自动修补时的回调函数
	everyChunkAutoFixCallback func(defines.ChunkPos)
	// 区块修补次数超过限制跳过时的回调函数
	chunkFixExceededLimitCallback func(defines.ChunkPos, int)
	// 构建每个NBT方块开始构建时的回调函数
	everyNBTBlockBuildingCallback func(defines.BlockPos, uint32, map[string]any)
	// 扣费成功时的回调函数
	chargeCostSuccessCallback func(deducted int, balance string, blockInfo BlockCountInfo)
	// 扣费重复（已处理过）时的回调函数
	chargeCostDuplicateCallback func(blockInfo BlockCountInfo)
	// 扣费失败时的回调函数
	chargeCostFailedCallback func(message string, blockInfo BlockCountInfo)
	// 用户中心未初始化时的回调函数
	userCenterNotInitCallback func()
	// 进入修补模式时的回调函数
	enterFixModeCallback func()
	// 退出修补模式时的回调函数
	exitFixModeCallback func()
	// 每修补一个区块开始时的回调函数
	everyChunkFixStartCallback func(chunkPos defines.ChunkPos, playerName string)
	// 每修补一个区块结束时的回调函数
	everyChunkFixEndCallback func(chunkPos defines.ChunkPos, playerName string)
	// MCWorld 转换回调
	convertStartCallback        func(srcPath, destPath string)
	convertExistsCallback       func(path string)
	convertSuccessCallback      func(path string)
	convertProgressInitCallback func(total int)
	convertProgressCallback     func()
}

func NewBuildTask() *BuildTask {
	return &BuildTask{
		TaskType:         TaskTypeBuild,
		Speed:            5000,
		ChunkGroupSide:   1,
		AutoCleanBlock:   true,
		AutoCleanItem:    true,
		VerifyAfterChunk: 1,
		VerifyChunkLevel: 1,
		WaitChunkLoad:    true,
		ConvertToMCWorld: false,
	}
}

func (b *BuildTask) DisableCounting() {
	b.skipCounting = true
}

func (b *BuildTask) SetControl(c defines.Control) {
	ctrl, ok := c.(*control.Control)
	if !ok {
		panic(ErrInvalidControl)
	}
	b.control = *ctrl
	b.id = b.control.GetTaskID()
}

func (b *BuildTask) ID() int {
	return b.id
}

func (b *BuildTask) Type() string {
	return string(b.TaskType)
}

func (b *BuildTask) FromJSON(s string) error {
	return json.Unmarshal([]byte(s), b)
}

func (b *BuildTask) ToJSON() (string, error) {
	bs, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (b *BuildTask) taskContext() context.Context {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.ctx != nil {
		return b.ctx
	}
	return context.Background()
}

func (b *BuildTask) setRuntimeContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.stateMu.Lock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.pauseCh = nil
	b.running = true
	b.paused = false
	b.stopped = false
	b.stateMu.Unlock()
}

func (b *BuildTask) cleanupRuntime() {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	if b.pauseCh != nil {
		close(b.pauseCh)
		b.pauseCh = nil
	}
	b.ctx = nil
	b.running = false
	b.paused = false
	b.stopped = false
}

func (b *BuildTask) waitForControl() error {
	for {
		b.stateMu.Lock()
		ctx := b.ctx
		pauseCh := b.pauseCh
		b.stateMu.Unlock()

		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		if pauseCh == nil {
			return nil
		}

		var done <-chan struct{}
		if ctx != nil {
			done = ctx.Done()
		}
		select {
		case <-pauseCh:
		case <-done:
			if ctx != nil {
				return ctx.Err()
			}
		}
	}
}

func (b *BuildTask) waitForControlWithContext(extra context.Context) error {
	for {
		b.stateMu.Lock()
		ctx := b.ctx
		pauseCh := b.pauseCh
		b.stateMu.Unlock()

		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if extra != nil {
			select {
			case <-extra.Done():
				return extra.Err()
			default:
			}
		}

		if pauseCh == nil {
			return nil
		}

		var done <-chan struct{}
		if ctx != nil {
			done = ctx.Done()
		}
		var extraDone <-chan struct{}
		if extra != nil {
			extraDone = extra.Done()
		}

		select {
		case <-pauseCh:
		case <-done:
			if ctx != nil {
				return ctx.Err()
			}
		case <-extraDone:
			if extra != nil {
				return extra.Err()
			}
		}
	}
}

func (b *BuildTask) isStopped() bool {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	return b.stopped
}

func (b *BuildTask) SetStartCallback(callback func()) {
	b.startCallback = callback
}

func (b *BuildTask) SetGotTypeCallback(callback func(structure.Structure)) {
	b.gotTypeCallback = callback
}

func (b *BuildTask) SetParsingCallback(callback func()) {
	b.parsingCallback = callback
}

func (b *BuildTask) SetParsedCallback(callback func(structure.Structure)) {
	b.parsedCallback = callback
}

func (b *BuildTask) SetStartBuildCallback(callback func()) {
	b.startBuildCallback = callback
}

func (b *BuildTask) SetEveryChunkBuiltCallback(callback func(defines.ChunkPos, *chunk.Chunk)) {
	b.everyChunkBuiltCallback = callback
}

func (b *BuildTask) SetEveryChunkAutoFixCallback(callback func(defines.ChunkPos)) {
	b.everyChunkAutoFixCallback = callback
}

func (b *BuildTask) SetChunkFixExceededLimitCallback(callback func(defines.ChunkPos, int)) {
	b.chunkFixExceededLimitCallback = callback
}

func (b *BuildTask) SetEveryNBTBlockBuildingCallback(callback func(defines.BlockPos, uint32, map[string]any)) {
	b.everyNBTBlockBuildingCallback = callback
}

func (b *BuildTask) SetChargeCostSuccessCallback(callback func(deducted int, balance string, blockInfo BlockCountInfo)) {
	b.chargeCostSuccessCallback = callback
}

func (b *BuildTask) SetChargeCostDuplicateCallback(callback func(blockInfo BlockCountInfo)) {
	b.chargeCostDuplicateCallback = callback
}

func (b *BuildTask) SetChargeCostFailedCallback(callback func(message string, blockInfo BlockCountInfo)) {
	b.chargeCostFailedCallback = callback
}

func (b *BuildTask) SetUserCenterNotInitCallback(callback func()) {
	b.userCenterNotInitCallback = callback
}

func (b *BuildTask) SetEnterFixModeCallback(callback func()) {
	b.enterFixModeCallback = callback
}

func (b *BuildTask) SetExitFixModeCallback(callback func()) {
	b.exitFixModeCallback = callback
}

func (b *BuildTask) SetEveryChunkFixStartCallback(callback func(chunkPos defines.ChunkPos, playerName string)) {
	b.everyChunkFixStartCallback = callback
}

func (b *BuildTask) SetEveryChunkFixEndCallback(callback func(chunkPos defines.ChunkPos, playerName string)) {
	b.everyChunkFixEndCallback = callback
}

func (b *BuildTask) SetConvertStartCallback(callback func(srcPath, destPath string)) {
	b.convertStartCallback = callback
}

func (b *BuildTask) SetConvertExistsCallback(callback func(path string)) {
	b.convertExistsCallback = callback
}

func (b *BuildTask) SetConvertSuccessCallback(callback func(path string)) {
	b.convertSuccessCallback = callback
}

func (b *BuildTask) SetConvertProgressInitCallback(callback func(total int)) {
	b.convertProgressInitCallback = callback
}

func (b *BuildTask) SetConvertProgressCallback(callback func()) {
	b.convertProgressCallback = callback
}

func (b *BuildTask) getBuildStartPos() defines.BlockPos {
	if b.buildStartPos == (defines.BlockPos{}) {
		return b.StartPos
	}
	return b.buildStartPos
}

func (b *BuildTask) getOriginalStartPos() defines.BlockPos {
	if b.originalStartPos == (defines.BlockPos{}) {
		return b.StartPos
	}
	return b.originalStartPos
}

// 计算区块所在的组索引
func getGroupIndex(pos int32, groupSide int) int {
	return int(math.Floor(float64(pos) / float64(groupSide)))
}

// 根据组索引和组内偏移计算区块位置
func getChunkPosInGroup(groupIdx, offset, groupSide int) int32 {
	return int32(groupIdx*groupSide + offset)
}

// 根据区块位置计算所属组的坐标
func getGroupPosByChunkPos(chunkPos define.ChunkPos, groupSide int) define.ChunkPos {
	groupCX := getGroupIndex(chunkPos.X(), groupSide)
	groupCZ := getGroupIndex(chunkPos.Z(), groupSide)
	return define.ChunkPos{int32(groupCX), int32(groupCZ)}
}

func snakeGroupPosByIndex(size define.Size, groupSide int, n int) define.ChunkPos {
	xChunkNum := size.GetChunkXCount()
	if groupSide <= 0 {
		groupSide = 1
	}

	xGroupNum := int(math.Ceil(float64(xChunkNum) / float64(groupSide)))
	groupCZ := n / xGroupNum
	groupRowOffset := mod(n, xGroupNum)

	var groupCX int
	if groupCZ%2 == 0 {
		groupCX = groupRowOffset
	} else {
		groupCX = xGroupNum - 1 - groupRowOffset
	}

	return define.ChunkPos{int32(groupCX), int32(groupCZ)}
}

// 蛇形区块组位置生成器（适配组模式，每次生成一组）
func snakeChunkPosGenerator(cm *ChunkManager, n int) define.ChunkPos {
	size := cm.Reader().GetSize()
	groupSide := cm.chunkGroupSide
	return snakeGroupPosByIndex(size, groupSide, n)
}

// ChunkManager结构体（删除onceParseChunkNum字段）
type ChunkManager struct {
	reader            structure.Structure
	chunkPosGenerator func(*ChunkManager, int) define.ChunkPos
	progress          int // 当前进度（按组计数）
	max               int // 总组数
	chunkGroupSide    int // 区块组边长
}

// NewChunkManager（删除onceParseChunkNum参数）
func NewChunkManager(
	reader structure.Structure,
	chunkPosGenerator func(*ChunkManager, int) define.ChunkPos,
	progress, chunkGroupSide int,
) *ChunkManager {
	size := reader.GetSize()
	if chunkGroupSide <= 0 {
		chunkGroupSide = 1
	}
	chunkXGroups := (size.GetChunkXCount() + chunkGroupSide - 1) / chunkGroupSide
	chunkZGroups := (size.GetChunkZCount() + chunkGroupSide - 1) / chunkGroupSide
	totalGroups := chunkXGroups * chunkZGroups

	return &ChunkManager{
		reader:            reader,
		chunkPosGenerator: chunkPosGenerator,
		progress:          progress,
		max:               totalGroups,
		chunkGroupSide:    chunkGroupSide,
	}
}

// 保留原有Reader方法
func (c *ChunkManager) Reader() structure.Structure {
	return c.reader
}

// 按组解析区块（每次仅解析一组，返回错误）
func (c *ChunkManager) parseChunkGroup() (map[define.ChunkPos]*chunk.Chunk, error) {
	// 检查是否已解析完所有组
	if c.progress >= c.max {
		return nil, nil
	}

	// 生成当前组的坐标（每次一组）
	groupPos := c.chunkPosGenerator(c, c.progress)

	// 收集当前组内的所有区块位置
	allChunkPositions := make([]define.ChunkPos, 0)
	groupSide := c.chunkGroupSide
	c.progress++
	size := c.Reader().GetSize()
	maxCX := int32(size.GetChunkXCount())
	maxCZ := int32(size.GetChunkZCount())

	groupCX, groupCZ := int(groupPos.X()), int(groupPos.Z())
	// 遍历组内偏移，过滤超出边界的区块
	for zOffset := 0; zOffset < groupSide; zOffset++ {
		for xOffset := 0; xOffset < groupSide; xOffset++ {
			cx := getChunkPosInGroup(groupCX, xOffset, groupSide)
			cz := getChunkPosInGroup(groupCZ, zOffset, groupSide)
			if cx >= 0 && cx < maxCX && cz >= 0 && cz < maxCZ {
				allChunkPositions = append(allChunkPositions, define.ChunkPos{cx, cz})
			}
		}
	}

	if len(allChunkPositions) == 0 {
		return nil, nil
	}

	// 批量读取当前组的区块
	return c.reader.GetChunks(allChunkPositions)
}

// GetChunks（每次获取一组区块，返回错误，不再panic）
func (c *ChunkManager) GetChunks() (define.ChunkPos, map[define.ChunkPos]*chunk.Chunk, error) {
	// 每次仅解析一组并返回
	chunks, err := c.parseChunkGroup()
	var pos define.ChunkPos
	for p := range chunks {
		pos = p
		break
	}
	return getGroupPosByChunkPos(pos, c.chunkGroupSide), chunks, err
}

// 新增：获取当前进度和总组数（方便外部判断解析状态）
func (c *ChunkManager) Progress() (int, int) {
	return c.progress, c.max
}

func (b *BuildTask) moveBot(pos defines.BlockPos) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	m := NewMoveTask()
	m.Pos = pos
	pos[2] = pos[2] + 64
	m.SetControl(&b.control)
	if err := m.Start(b.taskContext()); err != nil {
		return err
	}
	return nil
}

func (b *BuildTask) buildCommandBlock(n *nbt_block.CommandBlock, worldPosX, worldPosY, worldPosZ int32) error {
	gameInterface := b.control.GameInterface()
	var mode uint32 = packet.CommandBlockImpulse
	switch n.BlockName() {
	case "minecraft:chain_command_block":
		mode = packet.CommandBlockChain
	case "minecraft:repeating_command_block":
		mode = packet.CommandBlockRepeating
	}
	command := n.NBT.Command
	if b.AutoUpgradeCommand {
		cmd, _, err := command_upgrader.UpgradeExecuteCommand(command)
		if err == nil {
			fmt.Printf("UpgradeExecuteCommand: %v\n", err)
			command = cmd
		}
	}
	if err := gameInterface.Resources().WritePacket(&packet.CommandBlockUpdate{
		Block:              true,
		Position:           protocol.BlockPos{worldPosX, worldPosY, worldPosZ},
		Mode:               mode,
		NeedsRedstone:      n.NBT.Auto == 0,
		Conditional:        n.NBT.ConditionalMode == 1,
		Command:            command,
		Name:               n.NBT.CustomName,
		ShouldTrackOutput:  n.NBT.TrackOutput == 1,
		TickDelay:          n.NBT.TickDelay,
		ExecuteOnFirstTick: n.NBT.ExecuteOnFirstTick == 1,
	}); err != nil {
		return fmt.Errorf("buildCommandBlock: failed to write command block update packet: %v", err)
	}
	return nil
}

func (b *BuildTask) prepareStructureFilePath() (string, error) {
	filePath := b.FilePath
	buildingPath := filepath.Join(utils.GetStoragePath(), utils.BuildingFilesDir, filepath.Base(filePath))
	if _, err := os.Stat(buildingPath); err == nil {
		filePath = buildingPath
	}

	if !b.ConvertToMCWorld {
		return filePath, nil
	}

	if strings.EqualFold(filepath.Ext(filePath), ".mcworld") {
		return filePath, nil
	}

	outputPath := filepath.Join(filepath.Dir(filePath), strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))+".mcworld")
	if _, err := os.Stat(outputPath); err == nil {
		if b.convertExistsCallback != nil {
			b.convertExistsCallback(outputPath)
		}
		return outputPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if b.convertStartCallback != nil {
		b.convertStartCallback(filePath, outputPath)
	}

	startCB := func(total int) {
		if b.convertProgressInitCallback != nil {
			b.convertProgressInitCallback(total)
		}
	}
	progressCB := func() {
		if b.convertProgressCallback != nil {
			b.convertProgressCallback()
		}
	}

	if err := convertStructureFileToMCWorld(filePath, outputPath, startCB, progressCB); err != nil {
		return "", fmt.Errorf("%s", fmt.Sprintf(l10n.T(l10n.KeyBuildConvertToMCWorldFailed), err))
	}

	if b.convertSuccessCallback != nil {
		b.convertSuccessCallback(outputPath)
	}
	return outputPath, nil
}

func (b *BuildTask) buildCommandBlockNBTs(
	c *chunk.Chunk,
	nbtList map[define.BlockPos]map[string]any,
	worldChunkX, worldChunkY, worldChunkZ int32,
) error {
	for blockPos, nbt := range nbtList {
		blockRuntimeID := c.Block(uint8(blockPos.X()), int16(blockPos.Y()), uint8(blockPos.Z()), 0)
		blockName, blockStates, found := block.RuntimeIDToState(blockRuntimeID)
		if !found || !strings.Contains(blockName, "command_block") {
			continue
		}
		block, err := nbt_block.ParseBlock(
			nil,
			blockName,
			blockStates,
			nbt,
		)
		if err != nil {
			continue
		}
		n, ok := block.(*nbt_block.CommandBlock)
		if !ok {
			continue
		}
		if err := b.buildCommandBlock(
			n,
			worldChunkX+blockPos.X(),
			worldChunkY+blockPos.Y()+64,
			worldChunkZ+blockPos.Z(),
		); err != nil {
			return err
		}
		delete(nbtList, blockPos)
	}
	return nil
}

func (b *BuildTask) waitChunkLoad(chunkPos define.ChunkPos) (err error) {
	if err := b.waitForControl(); err != nil {
		return err
	}
	x := chunkPos.X() * 16
	startPos := b.getBuildStartPos()
	y := startPos.Y() - mod(startPos.Y(), 16)
	z := chunkPos.Z() * 16
	chunkGroupSide := max(b.ChunkGroupSide, 1)
	chunkGroupWidth := int32(chunkGroupSide * 16)

	if chunkGroupSide != 1 {
		for {
			cmd := fmt.Sprintf(
				"tickingarea add %d %d %d %d %d %d",
				x,
				y,
				z,
				x+chunkGroupWidth-1,
				y,
				z+chunkGroupWidth-1,
			)
			resp, isTimeout, err := b.control.GameInterface().Commands().SendWSCommandWithTimeout(cmd, time.Millisecond*500)
			if isTimeout {
				err = b.afterChunkLoaded(chunkPos)
				if err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to add tickingarea: %w", err)
			}
			if resp.SuccessCount == 0 && len(resp.OutputMessages) > 0 && !resp.OutputMessages[0].Success {
				return fmt.Errorf("tickingarea add failed: %s", resp.OutputMessages[0].Message)
			}
			break
		}

		cmd := fmt.Sprintf(
			"tickingarea preload %d %d %d true",
			x,
			y,
			z,
		)
		if err := b.sendSettingsCommand(cmd, false); err != nil {
			return fmt.Errorf("failed to preload tickingarea: %w", err)
		}
	}

	for {
		err := b.moveBot(defines.BlockPos{x, y, z})
		if err != nil {
			return err
		}
		resp, isTimeout, err := b.control.GameInterface().Commands().SendWSCommandWithTimeout(fmt.Sprintf(
			"setblock %d %d %d air keep",
			x,
			y,
			z,
		), time.Millisecond*500)
		if !isTimeout && err != nil {
			return err
		}
		if !isTimeout && len(resp.OutputMessages) == 1 && resp.OutputMessages[0].Message != "commands.setblock.outOfWorld" {
			break
		}
	}
	return nil
}

func (b *BuildTask) afterChunkLoaded(chunkPos define.ChunkPos) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	x := chunkPos.X() * 16
	startPos := b.getBuildStartPos()
	y := startPos.Y() - mod(startPos.Y(), 16)
	z := chunkPos.Z() * 16
	cmd := fmt.Sprintf(
		"tickingarea remove %d %d %d",
		x,
		y,
		z,
	)
	return b.sendSettingsCommand(cmd, false)
}

// 统计一共有多少方块
func (b *BuildTask) CountBlocks() (int, error) {
	return b.reader.CountNonAirBlocks()
}

// 统计一共有多少命令方块
func (b *BuildTask) CountCommandBlocks() int {
	size := b.reader.GetSize()
	cxCount := size.GetChunkXCount()
	czCount := size.GetChunkZCount()
	count := 0
	for cx := 0; cx < cxCount; cx++ {
		for cz := 0; cz < czCount; cz++ {
			chunksNBT, _ := b.reader.GetChunksNBT([]define.ChunkPos{{int32(cx), int32(cz)}})
			chunkNBT := chunksNBT[define.ChunkPos{int32(cx), int32(cz)}]
			for _, nbt := range chunkNBT {
				_, ok := nbt["Command"]
				if ok {
					count++
				}
			}
		}
	}

	return count
}

// 统计其他 NBT 方块数量
func (b *BuildTask) CountOtherNBTBlocks() int {
	size := b.reader.GetSize()
	cxCount := size.GetChunkXCount()
	czCount := size.GetChunkZCount()
	count := 0
	for cx := 0; cx < cxCount; cx++ {
		for cz := 0; cz < czCount; cz++ {
			chunksNBT, _ := b.reader.GetChunksNBT([]define.ChunkPos{{int32(cx), int32(cz)}})
			chunkNBT := chunksNBT[define.ChunkPos{int32(cx), int32(cz)}]
			for _, nbt := range chunkNBT {
				_, ok := nbt["Command"]
				if !ok {
					count++
				}
			}
		}
	}

	return count
}

var mcColorCodeRegex = regexp.MustCompile(`§.`)

// toPlainName 去除网易版 Minecraft 名字中的颜色代码和特殊格式
// 可用于将 VIP 玩家名转换为普通玩家名
func toPlainName(name string) string {
	// 移除颜色代码
	if strings.Contains(name, "§") {
		name = mcColorCodeRegex.ReplaceAllString(name, "")
	}

	// 处理嵌套格式，如 <<VIP名><玩家名>>
	if strings.Count(name, "<") > 1 {
		var words []string
		var cachedStr strings.Builder

		for _, char := range name {
			switch char {
			case '<':
				cachedStr.Reset()
			case '>':
				words = append(words, cachedStr.String())
			default:
				cachedStr.WriteRune(char)
			}
		}

		if len(words) > 0 {
			return words[len(words)-1]
		}
	}

	// 处理简单格式，如 <玩家名>
	if strings.HasPrefix(name, "<") && strings.HasSuffix(name, ">") {
		return name[1 : len(name)-1]
	}

	// 如果没有特殊格式，直接返回
	return name
}

func (b *BuildTask) EnterFixMode() (err error) {
	if err := b.waitForControl(); err != nil {
		return err
	}
	gameInterface := b.control.GameInterface()
	if b.enterFixModeCallback != nil {
		b.enterFixModeCallback()
	}
	defer func() {
		gameInterface.PacketListener().DestroyListener(b.fixModeListenerUniqueID)
		if b.exitFixModeCallback != nil {
			b.exitFixModeCallback()
		}
	}()
	fixModeMutex := sync.Mutex{}
	exitChan := make(chan struct{})
	b.fixModeListenerUniqueID, err = gameInterface.PacketListener().ListenPacket([]uint32{packet.IDText}, func(p packet.Packet, connCloseErr error) {
		if connCloseErr != nil {
			return
		}
		pk, _ := p.(*packet.Text)
		if pk.TextType != packet.TextTypeChat || pk.SourceName == gameInterface.GetBotInfo().BotName {
			return
		}
		if pk.Message == "exit" {
			close(exitChan)
			return
		} else {
			// 处理玩家名称中的颜色代码和特殊格式
			playerName := toPlainName(pk.SourceName)
			for {
				resp, isTimeout, err := gameInterface.Commands().SendWSCommandWithTimeout(fmt.Sprintf(`querytarget @a[name="%s"]`, playerName), 1*time.Second)
				if isTimeout {
					fmt.Println("querytarget timeout")
					continue
				}
				if err != nil {
					fmt.Printf("querytarget error: %v\n", err)
					return
				}
				if resp.SuccessCount != 1 {
					return
				}
				var targetQueryingInfo []struct {
					Position struct {
						X float32 `json:"x"`
						Y float32 `json:"y"`
						Z float32 `json:"z"`
					} `json:"position"`
				}
				if err := json.Unmarshal([]byte(resp.OutputMessages[0].Parameters[0]), &targetQueryingInfo); err != nil {
					return
				}
				position := targetQueryingInfo[0].Position
				buildStartPos := b.getBuildStartPos()
				startX := buildStartPos.X()
				startY := buildStartPos.Y()
				startZ := buildStartPos.Z()
				startPosX := startX - mod(startX, 16)
				startPosY := startY - mod(startY, 16)
				startPosZ := startZ - mod(startZ, 16)
				targetX := int32(position.X) - startPosX
				targetZ := int32(position.Z) - startPosZ
				if targetX < 0 || targetZ < 0 || targetX >= int32(b.reader.GetSize().Width) || targetZ >= int32(b.reader.GetSize().Length) {
					return
				}
				fixModeMutex.Lock()
				defer fixModeMutex.Unlock()
				if err := b.moveBot(define.BlockPos{int32(position.X), int32(position.Y), int32(position.Z)}); err != nil {
					return
				}
				targetChunkX := targetX / 16
				targetChunkZ := targetZ / 16
				targetChunkPos := define.ChunkPos{targetChunkX, targetChunkZ}
				targetPosX := startPosX + targetChunkX*16
				targetPosY := startPosY
				targetPosZ := startPosZ + targetChunkZ*16
				c, err := b.reader.GetChunks([]define.ChunkPos{targetChunkPos})
				if err != nil {
					return
				}
				if b.everyChunkFixStartCallback != nil {
					b.everyChunkFixStartCallback(targetChunkPos, playerName)
				}
				if err := b.buildChunk(c[targetChunkPos], targetPosX, targetPosY, targetPosZ); err != nil {
					return
				}
				if b.everyChunkFixEndCallback != nil {
					b.everyChunkFixEndCallback(targetChunkPos, playerName)
				}
				break
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to enter fix mode: %w", err)
	}
	<-exitChan
	return nil
}

func FixDoorCommand(cmd string) string {
	// 1. 替换属性
	newCmd1 := strings.ReplaceAll(cmd, `"upper_block_bit"=true`, `"upper_block_bit"=false`)
	ok1 := newCmd1 != cmd
	newCmd2 := strings.ReplaceAll(newCmd1, `"door_hinge_bit"=true`, `"door_hinge_bit"=false`)
	ok2 := newCmd2 != newCmd1

	currentCmd := newCmd2

	// 处理 direction 的逻辑：若 ok2 为 true
	if ok2 {
		// 查找 "direction"= 的起始索引
		dirStr := `"direction"=`
		idxDir := strings.Index(currentCmd, dirStr)
		if idxDir != -1 {
			start := idxDir + len(dirStr)
			// 找到该数字后的第一个非数字字符（如逗号或空格）来确定结束位置
			end := start
			for end < len(currentCmd) && currentCmd[end] >= '0' && currentCmd[end] <= '9' {
				end++
			}
			if end > start {
				numStr := currentCmd[start:end]
				num, err := strconv.Atoi(numStr)
				if err == nil {
					// 计算新值
					newNum := num + 1
					if newNum > 3 {
						newNum = 0
					}
					// 替换字符串
					currentCmd = currentCmd[:start] + strconv.Itoa(newNum) + currentCmd[end:]
				}
			}
		}
	}

	// 若 ok1 为 false，直接返回处理完 direction 后的 currentCmd
	if !ok1 {
		return currentCmd
	}

	// 2. 定位 Y 坐标 (在 currentCmd 基础上处理)
	// 找第 1 个空格（setblock 之后）
	idx1 := strings.Index(currentCmd, " ")
	if idx1 == -1 {
		return currentCmd
	}
	// 找第 2 个空格（X 坐标之后）
	idx2 := strings.Index(currentCmd[idx1+1:], " ")
	if idx2 == -1 {
		return currentCmd
	}
	idx2 += idx1 + 1
	// 找第 3 个空格（Y 坐标之后）
	idx3 := strings.Index(currentCmd[idx2+1:], " ")
	if idx3 == -1 {
		return currentCmd
	}
	idx3 += idx2 + 1

	// 提取 Y 坐标字符串
	yStr := currentCmd[idx2+1 : idx3]
	y, err := strconv.Atoi(yStr)
	if err != nil {
		return currentCmd
	}

	// 3. 构造新字符串
	newY := strconv.Itoa(y - 1)
	return currentCmd[:idx2+1] + newY + currentCmd[idx3:]
}

func (b *BuildTask) sendSettingsCommand(cmd string, dimensional bool) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	waitCtx := b.taskContext()
	if b.Speed != 0 {
		if err := b.limiter.Wait(waitCtx); err != nil {
			return err
		}
	}

	if strings.Contains(cmd, `"upper_block_bit"=false`) {
		return nil
	} else if strings.Contains(cmd, `"upper_block_bit"=true`) {
		cmd = FixDoorCommand(cmd)
	}

	return b.control.GameInterface().Commands().SendSettingsCommand(cmd, dimensional)
}

func formatDurationHMS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	totalSeconds := int64(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func countChunksInFirstNGroups(size define.Size, groupSide int, groupCount int) int64 {
	if groupCount <= 0 {
		return 0
	}
	if groupSide <= 0 {
		groupSide = 1
	}

	maxCX := int32(size.GetChunkXCount())
	maxCZ := int32(size.GetChunkZCount())

	total := int64(0)
	for i := 0; i < groupCount; i++ {
		groupPos := snakeGroupPosByIndex(size, groupSide, i)
		groupCX, groupCZ := int(groupPos.X()), int(groupPos.Z())

		for zOffset := 0; zOffset < groupSide; zOffset++ {
			for xOffset := 0; xOffset < groupSide; xOffset++ {
				cx := getChunkPosInGroup(groupCX, xOffset, groupSide)
				cz := getChunkPosInGroup(groupCZ, zOffset, groupSide)
				if cx >= 0 && cx < maxCX && cz >= 0 && cz < maxCZ {
					total++
				}
			}
		}
	}
	return total
}

func (b *BuildTask) runInGameProgressLoop(
	ctx context.Context,
	startTime time.Time,
	startBase int64,
	processedChunks *int64,
	totalChunks int64,
) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	gameInterface := b.control.GameInterface()
	if gameInterface == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err := b.waitForControlWithContext(ctx); err != nil {
			return
		}

		current := atomic.LoadInt64(processedChunks)
		if current < 0 {
			current = 0
		}
		if totalChunks > 0 && current > totalChunks {
			current = totalChunks
		}

		var percent float64
		if totalChunks > 0 {
			percent = float64(current) / float64(totalChunks) * 100
		}

		doneThisRun := current - startBase
		if doneThisRun < 0 {
			doneThisRun = 0
		}

		elapsed := time.Since(startTime).Seconds()
		speed := 0.0
		if elapsed > 0 {
			speed = float64(doneThisRun) / elapsed
		}

		eta := "--:--:--"
		if totalChunks <= 0 {
			eta = "--:--:--"
		} else if current >= totalChunks {
			eta = "00:00:00"
		} else if speed > 0 {
			remaining := float64(totalChunks-current) / speed
			if remaining >= 0 {
				eta = formatDurationHMS(time.Duration(remaining * float64(time.Second)))
			}
		}

		progressInfo := fmt.Sprintf(
			"%s %s - Building [async]\nProgress: %d/%d (%.2f%%) %.2f chunks/s ETA: %s",
			constants.Name,
			constants.Version,
			current,
			totalChunks,
			percent,
			speed,
			eta,
		)

		payload, err := json.Marshal(map[string]any{
			"rawtext": []map[string]any{
				{"text": progressInfo},
			},
		})
		if err != nil {
			continue
		}

		_ = gameInterface.Commands().SendSettingsCommand(
			fmt.Sprintf("titleraw @a actionbar %s", string(payload)),
			false,
		)
	}
}

func (b *BuildTask) Start(ctx context.Context) error {
	b.setRuntimeContext(ctx)
	defer b.cleanupRuntime()

	code := b.control.Client().Authenticator().RentalServerCode()
	if strings.HasPrefix(code, "PC") || strings.HasPrefix(code, "LobbyGame") || strings.HasPrefix(code, "Tan") {
		b.DisableCounting()
	}
	b.limiter = rate.NewLimiter(rate.Limit(b.Speed), b.Speed)

	if err := b.waitForControl(); err != nil {
		return err
	}

	structurePath, err := b.prepareStructureFilePath()
	if err != nil {
		return err
	}
	//b.FilePath = structurePath

	file, err := os.Open(structurePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if b.startCallback != nil {
		b.startCallback()
	}

	// 获取结构体类型
	structure, err := structure.StructureFromFile(file)
	if err != nil {
		return err
	}
	if b.gotTypeCallback != nil {
		b.gotTypeCallback(structure)
	}

	if b.parsingCallback != nil {
		b.parsingCallback()
	}
	// 获取结构体读取器
	b.reader = structure
	if b.reader == nil {
		return ErrInvalidStructureFile
	}
	defer b.reader.Close()
	b.originalStartPos = b.StartPos
	b.buildStartPos = b.StartPos
	if b.AutoPlaceBorderBlock {
		b.buildStartPos[0] = b.buildStartPos.X() + 1
		b.buildStartPos[2] = b.buildStartPos.Z() + 1
	}
	if b.AutoPlaceDenyBlock {
		b.buildStartPos[1] = b.buildStartPos.Y() + 1
	}
	startPos := b.getBuildStartPos()
	startPosX := startPos.X()
	startPosY := startPos.Y()
	startPosZ := startPos.Z()
	realStartChunkX := startPosX - mod(startPosX, 16)
	realStartChunkY := startPosY - mod(startPosY, 16)
	realStartChunkZ := startPosZ - mod(startPosZ, 16)
	b.reader.SetOffsetPos(define.Offset{startPosX - realStartChunkX, startPosY - realStartChunkY, startPosZ - realStartChunkZ})
	if b.parsedCallback != nil {
		parsedReader := b.reader
		if b.AutoPlaceBorderBlock {
			adjusted := b.reader.GetSize()
			adjusted.Width += 2
			adjusted.Length += 2
			parsedReader = sizeOverrideStructure{Structure: b.reader, size: adjusted}
		}
		b.parsedCallback(parsedReader)
	}

	size := b.reader.GetSize()

	if !b.skipCounting {
		// 扣费：统计数量并调用用户中心接口（带去重），按计费模式决定是否清理去重键
		_ = uc.InitGlobalFromConfig()
		if uc.GlobalOperator != nil {
			ctx := b.taskContext()
			server := b.control.GetConfig().RentalServerCode
			fileHash, err := computeFileHash(structurePath)
			if err != nil {
				return fmt.Errorf("failed to compute file hash: %w", err)
			}

			var blockCount, commandBlockCount, otherNBTCount int
			// 先判定计费模式：times 则跳过统计，直接扣费；并仅在 times 模式下清理去重键
			bm, _, bmErr := uc.GetBillingMode(ctx)
			isTimesMode := bmErr == nil && strings.ToLower(bm.BillingMode) == "times"
			if isTimesMode {
				// 完成后清理（忽略错误）——按次计费每次都应计入一次，去重键可清理
				defer func() { _, _, _ = uc.CompleteImport(ctx, server, fileHash) }()
			}

			// 直接统计方块数量，不查询用户中心缓存
			blockCount, err = b.CountBlocks()
			if err != nil {
				return fmt.Errorf("failed to count blocks: %w", err)
			}
			commandBlockCount = b.CountCommandBlocks()
			otherNBTCount = b.CountOtherNBTBlocks()

			if b.IgnoreCommandBlock {
				commandBlockCount = 0
			}
			if b.IgnoreOtherNBTBlock {
				otherNBTCount = 0
			}

			// 创建方块统计信息
			blockInfo := BlockCountInfo{
				BlockCount:        blockCount,
				CommandBlockCount: commandBlockCount,
				OtherNBTCount:     otherNBTCount,
			}

			resp, _, err := uc.ChargeImportCost(ctx, uc.CostChargePayload{
				Block:        blockCount,
				CommandBlock: commandBlockCount,
				NBTBlock:     otherNBTCount,
				Server:       server,
				FileHash:     fileHash,
			})
			if err != nil {
				if b.chargeCostFailedCallback != nil {
					b.chargeCostFailedCallback("charge import cost failed: "+err.Error(), blockInfo)
				}
				return fmt.Errorf("charge import cost failed: %w", err)
			}
			// 本地化提示余额与扣费信息
			balanceStr := l10n.T(l10n.KeyCostChargeBalanceUnknown)
			if resp.Balance != nil {
				balanceStr = fmt.Sprintf("%d", *resp.Balance)
			}
			if resp.Deducted == 0 && resp.Success {
				if b.chargeCostDuplicateCallback != nil {
					b.chargeCostDuplicateCallback(blockInfo)
				}
			} else if resp.Success {
				if b.chargeCostSuccessCallback != nil {
					b.chargeCostSuccessCallback(resp.Deducted, balanceStr, blockInfo)
				}
			} else {
				// 服务端明确拒绝（例如额度/次数不足等），应立即停止构建
				if b.chargeCostFailedCallback != nil {
					b.chargeCostFailedCallback(resp.Message, blockInfo)
				}
				return fmt.Errorf("charge import cost rejected: %s", resp.Message)
			}
		} else {
			if b.userCenterNotInitCallback != nil {
				b.userCenterNotInitCallback()
			}
			return errors.New("user center not initialized")
		}
	}

	// 检查是否有中断恢复数据或指定的起始进度
	var (
		startChunkIndex  int
		resumeGroupIndex = -1
	)
	if b.chunkManager != nil && b.chunkManager.progress > 0 {
		// 从中断恢复，往前退一个区块避免构建不完整
		resumeGroupIndex = max(b.chunkManager.progress-1, 0)
	} else if b.Progress > 0 {
		// 从指定的进度开始构建
		startChunkIndex = b.Progress
		if startChunkIndex >= size.GetChunkCount() {
			startChunkIndex = size.GetChunkCount() - 1
		}
	} else {
		// 正常启动，从0开始
		startChunkIndex = 0
	}

	chunkGroupSide := max(b.ChunkGroupSide, 1)
	chunkGroupSquare := chunkGroupSide * chunkGroupSide
	chunkXGroups := (size.GetChunkXCount() + chunkGroupSide - 1) / chunkGroupSide
	chunkZGroups := (size.GetChunkZCount() + chunkGroupSide - 1) / chunkGroupSide
	totalGroups := chunkXGroups * chunkZGroups
	startGroupIndex := startChunkIndex / chunkGroupSquare
	if resumeGroupIndex >= 0 {
		startGroupIndex = resumeGroupIndex
	}
	b.chunkManager = NewChunkManager(b.reader, snakeChunkPosGenerator, startGroupIndex, chunkGroupSide)
	unverifyChunks := make(map[defines.ChunkPos]*chunk.Chunk)

	gameInterface := b.control.GameInterface()
	for {
		if err := b.waitForControl(); err != nil {
			return err
		}
		resp, isTimeout, err := gameInterface.Commands().SendWSCommandWithTimeout("execute run list", 1*time.Second)
		if !isTimeout && err != nil {
			return err
		}
		// 判断是否成功来检查是否拥有OP
		if !isTimeout && resp.SuccessCount != 0 {
			fmt.Println("has OP")
			// 获取到OP权限后从服务器拉取广告信息并发送
			// 使用签名验证防止篡改，支持广告禁用和用户豁免
			adCtx, cancel := context.WithTimeout(b.taskContext(), 5*time.Second)
			adMessages, showAd := uc.GetAdvertisementsSafe(adCtx)
			cancel()
			if showAd && len(adMessages) > 0 {
				sendTellrawText := func(text string) error {
					tellrawBytes, err := json.Marshal(map[string]any{
						"rawtext": []map[string]any{
							{
								"text": text,
							},
						},
					})
					if err != nil {
						return fmt.Errorf("marshal tellraw payload: %w", err)
					}
					return b.sendSettingsCommand(fmt.Sprintf(`tellraw @a %s`, string(tellrawBytes)), false)
				}

				welcomeUser := ""
				if b.control.Client() != nil {
					if authClient := b.control.Client().Auth(); authClient != nil {
						if c, ok := authClient.(*controlauth.Client); ok {
							welcomeUser = strings.TrimSpace(c.RespondTo)
						}
					}
				}
				if welcomeUser == "" {
					welcomeUser = strings.TrimSpace(b.control.GetConfig().AuthUsername)
				}
				if welcomeUser == "" && b.control.Resources() != nil {
					welcomeUser = strings.TrimSpace(b.control.Resources().BotInfo().BotName)
				}
				if welcomeUser != "" {
					if err := sendTellrawText(fmt.Sprintf("欢迎:%s", welcomeUser)); err != nil {
						return fmt.Errorf("failed to send welcome message: %w", err)
					}
				}

				for _, adMessage := range adMessages {
					adMessage = strings.TrimSpace(adMessage)
					if adMessage == "" {
						continue
					}
					if err := sendTellrawText(adMessage); err != nil {
						return fmt.Errorf("failed to send ad message: %w", err)
					}
				}
				time.Sleep(1 * time.Second)
			}
			break
		}
		fmt.Println("waiting for OP...")
		select {
		case <-time.After(1 * time.Second):
		case <-b.taskContext().Done():
			return b.taskContext().Err()
		}
	}
	botName := b.control.Resources().BotInfo().BotName
	if err = b.sendSettingsCommand(fmt.Sprintf(`gamemode 1 "%s"`, botName), false); err != nil {
		return err
	}
	if b.DimensionName == "" {
		b.DimensionName = "overworld"
	}
	_, found := gameInterface.Resources().ConstantPacket().DimensionIDByName(b.DimensionName)
	if !found {
		//return fmt.Errorf("invalid dimension: %s", b.DimensionName)
	}

	tpCmd := fmt.Sprintf(
		`execute in %s run tp "%s" %d %d %d`,
		b.DimensionName,
		botName,
		startPosX,
		startPosY,
		startPosZ,
	)
	for {
		resp, isTimeout, err := gameInterface.Commands().SendWSCommandWithTimeout(tpCmd, 1*time.Second)
		if isTimeout {
			fmt.Println("tp timeout")
			continue
		}
		if err != nil {
			return err
		}
		if resp.SuccessCount == 0 {
			return fmt.Errorf("tp failed with response: %v", resp.OutputMessages)
		}
		break
	}

	if err = b.moveBot(b.getBuildStartPos()); err != nil {
		return err
	}

	if b.AutoDisableCommand {
		if err = b.sendSettingsCommand("gamerule commandBlocksEnabled false", false); err != nil {
			return err
		}
	}

	if b.startBuildCallback != nil {
		b.startBuildCallback()
	}

	if b.EnterFixModeDirectly {
		if err = b.EnterFixMode(); err != nil {
			return err
		}
		if b.AutoDisableCommand {
			if err = b.sendSettingsCommand("gamerule commandBlocksEnabled true", false); err != nil {
				return err
			}
		}
		b.stateMu.Lock()
		b.running = false
		b.stateMu.Unlock()
		return nil
	}

	var (
		showProgressStop context.CancelFunc
		processedChunks  int64
	)
	if b.ShowProgressInGame {
		baseChunks := countChunksInFirstNGroups(size, chunkGroupSide, startGroupIndex)
		processedChunks = baseChunks
		progressCtx, cancel := context.WithCancel(b.taskContext())
		showProgressStop = cancel
		go b.runInGameProgressLoop(
			progressCtx,
			time.Now(),
			baseChunks,
			&processedChunks,
			int64(size.GetChunkCount()),
		)
	}
	defer func() {
		if showProgressStop != nil {
			showProgressStop()
		}
	}()

	for i := startGroupIndex; i < totalGroups; i++ {
		if err := b.waitForControl(); err != nil {
			return err
		}
		if b.isStopped() {
			b.stateMu.Lock()
			b.running = false
			b.stateMu.Unlock()
			return nil
		}

		groupPos, chunks, err := b.chunkManager.GetChunks()
		if err != nil {
			return err
		}
		chunksIsEmpty := true
		for _, c := range chunks {
			for _, subChunk := range c.Sub() {
				if !subChunk.Empty() {
					chunksIsEmpty = false
				}
			}
		}

		// 计算当前区块的世界坐标（16 的倍数）
		realChunkX := realStartChunkX + groupPos.X()*int32(chunkGroupSide)*16
		realChunkZ := realStartChunkZ + groupPos.Z()*int32(chunkGroupSide)*16
		realChunkPosX := realChunkX / 16
		realChunkPosZ := realChunkZ / 16
		realChunkPos := define.ChunkPos{
			realChunkPosX,
			realChunkPosZ,
		}

		needVerify := b.VerifyChunkLevel != VerifyChunkLevelNone
		if !chunksIsEmpty {
			// 移动到区块
			if err = b.moveBot(defines.BlockPos{
				realChunkX,
				startPosY,
				realChunkZ,
			}); err != nil {
				return err
			}
			// 根据配置等待区块加载
			if b.WaitChunkLoad {
				if err = b.waitChunkLoad(realChunkPos); err != nil {
					return err
				}
			}
			// 使用通用构建函数构建区块
			if err = b.buildChunks(chunks, realChunkX, realStartChunkY, realChunkZ); err != nil {
				return err
			}
			// 根据配置等待区块加载
			if b.WaitChunkLoad {
				if err = b.afterChunkLoaded(define.ChunkPos{
					realChunkPosX,
					realChunkPosZ,
				}); err != nil {
					return err
				}
			}
		}

		for pos, c := range chunks {
			if b.everyChunkBuiltCallback != nil {
				b.everyChunkBuiltCallback(defines.ChunkPos(pos), c)
			}
			if needVerify {
				unverifyChunks[defines.ChunkPos{
					realStartChunkX/16 + pos.X(),
					realStartChunkZ/16 + pos.Z(),
				}] = c
			}
		}

		if len(unverifyChunks) >= b.VerifyAfterChunk {
			if err := b.waitForControl(); err != nil {
				return err
			}
			subChunkRequest := gameInterface.SubChunkRequest()
			dimensionID, _ := b.control.Resources().UQHolder().Micro().GetExtendInfo().GetBotDimension()
			dimension := bwo_define.Dimension(dimensionID)
			targetSubChunkPosList := make([]protocol.SubChunkPos, 0)
			for realChunkPos, unverifyChunk := range unverifyChunks {
				noAirSubChunkYList := make([]int, 0)
				for subChunkPosY, subChunk := range unverifyChunk.Sub() {
					if subChunk.Empty() {
						continue
					}
					noAirSubChunkYList = append(noAirSubChunkYList, int(unverifyChunk.SubIndex(int16(subChunkPosY*16))))
				}
				if len(noAirSubChunkYList) == 0 {
					continue
				}
				for _, subChunkPosY := range noAirSubChunkYList {
					targetSubChunkPosList = append(targetSubChunkPosList, protocol.SubChunkPos{
						realChunkPos.X(),
						realStartChunkY/16 + int32(subChunkPosY) - 4,
						realChunkPos.Z(),
					})
				}
			}
			if len(targetSubChunkPosList) != 0 {
				request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(dimension, targetSubChunkPosList)
				response, err := subChunkRequest.SendSubChunkRequestWithResp(request)
				if err != nil {
					return err
				}

				// 记录区块验证结果
				chunkStatusMap := make(map[define.ChunkPos]bool)
				// 初始化所有目标区块为"验证通过"
				for _, pos := range targetSubChunkPosList {
					chunkStatusMap[define.ChunkPos{pos.X(), pos.Z()}] = true
				}

				// 使用公共验证方法
				b.verifySubChunkEntries(response, unverifyChunks, chunkStatusMap)

				// 转换为"需要修补"的逻辑（验证失败的区块需要修补）
				chunkNeedFixMap := make(map[define.ChunkPos]bool)
				for chunkPos, verifyPassed := range chunkStatusMap {
					chunkNeedFixMap[chunkPos] = !verifyPassed
				}
				// 实现循环修补逻辑
				if err := b.handleChunkVerificationAndFix(unverifyChunks, chunkNeedFixMap); err != nil {
					return err
				}
				unverifyChunks = make(map[defines.ChunkPos]*chunk.Chunk)
			}
		}

		if b.ShowProgressInGame {
			atomic.AddInt64(&processedChunks, int64(len(chunks)))
		}
	}

	if b.AutoDisableCommand {
		if err = b.sendSettingsCommand("gamerule commandBlocksEnabled true", false); err != nil {
			return err
		}
	}

	if b.AutoEnterFixMode {
		if showProgressStop != nil {
			showProgressStop()
			showProgressStop = nil
		}
		if err = b.EnterFixMode(); err != nil {
			return err
		}
	}

	// 构建完成，设置状态
	b.stateMu.Lock()
	b.running = false
	b.stateMu.Unlock()
	return nil
}

func (b *BuildTask) Pause() error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if !b.running {
		return errors.New("task is not running")
	}
	if b.paused {
		return errors.New("task already paused")
	}
	b.paused = true
	if b.pauseCh == nil {
		b.pauseCh = make(chan struct{})
	}
	return nil
}

func (b *BuildTask) Resume() error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if !b.running {
		return errors.New("task is not running")
	}
	if !b.paused {
		return errors.New("task is not paused")
	}
	if b.pauseCh != nil {
		close(b.pauseCh)
		b.pauseCh = nil
	}
	b.paused = false
	return nil
}

func (b *BuildTask) Stop() error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	b.stopped = true
	b.running = false
	b.paused = false
	if b.pauseCh != nil {
		close(b.pauseCh)
		b.pauseCh = nil
	}
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	return nil
}

func (b *BuildTask) IsRunning() bool {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	return b.running
}

func (b *BuildTask) IsPaused() bool {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	return b.paused
}

// GetChunkCount 获取总区块数量
func (b *BuildTask) GetChunkCount() int {
	if b.reader == nil {
		return 0
	}
	size := b.reader.GetSize()
	return size.GetChunkCount()
}

// GetProgress 获取当前已构建的区块数量
func (b *BuildTask) GetProgress() int {
	if b.chunkManager == nil {
		return b.Progress // 返回配置的起始进度
	}
	return b.chunkManager.progress
}

// GetProgressPercentage 获取构建进度百分比
func (b *BuildTask) GetProgressPercentage() float64 {
	total := b.GetChunkCount()
	if total == 0 {
		return 0.0
	}
	progress := b.GetProgress()
	return float64(progress) / float64(total) * 100.0
}

func (b *BuildTask) MakeInterruptJSON() (string, error) {
	if b.chunkManager == nil {
		return "", errors.New("no chunk manager available")
	}

	interruptData := BuildTaskInterruptData{
		CurrentChunkIndex: b.chunkManager.progress,
		TotalChunks:       b.chunkManager.max,
		TaskConfig:        *b,
	}

	jsonData, err := json.Marshal(interruptData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal interrupt data: %w", err)
	}

	return string(jsonData), nil
}

func (b *BuildTask) RestoreInterruptJSON(jsonStr string) error {
	var interruptData BuildTaskInterruptData
	if err := json.Unmarshal([]byte(jsonStr), &interruptData); err != nil {
		return fmt.Errorf("failed to unmarshal interrupt data: %w", err)
	}

	// 恢复进度信息
	b.chunkManager = &ChunkManager{} // 临时创建
	b.chunkManager.progress = interruptData.CurrentChunkIndex
	b.chunkManager.max = interruptData.TotalChunks

	// 恢复任务配置
	jsonData, _ := json.Marshal(interruptData.TaskConfig)
	if err := json.Unmarshal(jsonData, b); err != nil {
		return fmt.Errorf("failed to restore task config: %w", err)
	}

	return nil
}

// 以下为辅助函数

// 泛型约束：支持所有整数
type Number interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 | uintptr
}

// mod 模拟Python风格取余（结果与除数同号），支持所有数字类型
func mod[T Number](x, y T) T {
	if y == 0 {
		panic("division by zero")
	}
	rem := x % y // 基础取余（整数）或取模（浮点数）
	// 调整符号：当除数与余数符号不同时，加上除数
	if (y > 0 && rem < 0) || (y < 0 && rem > 0) {
		rem += y
	}
	return rem
}

func (b *BuildTask) placeNBTBlock(blockRuntimeId uint32, nbt map[string]any, worldPosX, worldPosY, worldPosZ int32) error {
	blockName, properties, found := block.RuntimeIDToState(blockRuntimeId)
	if !found {
		return fmt.Errorf("failed to get block name and properties from runtime id: %d", blockRuntimeId)
	}
	gameInterface := b.control.GameInterface()
	dimension, _ := gameInterface.Resources().UQHolder().Micro().GetExtendInfo().GetBotDimension()
	console, err := nbt_console.NewConsole(gameInterface, uint8(dimension), protocol.BlockPos(b.ConsoleWorldPos))
	if err != nil {
		return fmt.Errorf("failed to create nbt console: %w", err)
	}

	cache := nbt_cache.NewNBTCacheSystem(console)
	wrapper := nbt_assigner.NewNBTAssigner(console, cache)
	canFast, _, offset, err := wrapper.PlaceNBTBlock(
		blockName,
		properties,
		nbt,
	)
	if err != nil {
		//return fmt.Errorf("failed to place nbt block: %w", err)
		fmt.Printf("failed to place nbt block: %v\n", err)
		return nil
	}

	if canFast {
		if err = b.moveBot(defines.BlockPos{
			worldPosX,
			worldPosY,
			worldPosZ,
		}); err != nil {
			return fmt.Errorf("failed to move bot: %w", err)
		}

		if err = b.sendSettingsCommand(fmt.Sprintf("setblock %d %d %d %s %s", worldPosX, worldPosY, worldPosZ, blockName, utils.PropertiesToStateStr(properties)), false); err != nil {
			return fmt.Errorf("failed to send setblock command: %w", err)
		}
		return nil
	}

	consoleBlockPos := define.BlockPos{
		b.ConsoleWorldPos.X() + offset.X(),
		b.ConsoleWorldPos.Y() + offset.Y(),
		b.ConsoleWorldPos.Z() + offset.Z(),
	}
	resp, isTimeout, err := gameInterface.Commands().SendWSCommandWithTimeout(fmt.Sprintf(
		`structure save "Fatalder_NBT_block" %d %d %d %d %d %d false memory true`,
		consoleBlockPos.X(),
		consoleBlockPos.Y(),
		consoleBlockPos.Z(),
		consoleBlockPos.X(),
		consoleBlockPos.Y(),
		consoleBlockPos.Z(),
	), 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send structure save command: %w", err)
	}
	if isTimeout {
		return fmt.Errorf("structure save command timeout")
	}
	if len(resp.OutputMessages) != 1 {
		return fmt.Errorf("structure save command failed")
	}
	if !resp.OutputMessages[0].Success {
		return fmt.Errorf("structure save command failed: %s", resp.OutputMessages[0].Message)
	}

	if err = b.moveBot(defines.BlockPos{
		worldPosX,
		worldPosY,
		worldPosZ,
	}); err != nil {
		return fmt.Errorf("failed to move bot: %w", err)
	}

	resp, isTimeout, err = gameInterface.Commands().SendWSCommandWithTimeout(fmt.Sprintf(
		`structure load "Fatalder_NBT_block" %d %d %d`,
		worldPosX,
		worldPosY,
		worldPosZ,
	), 60*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send structure load command: %w", err)
	}
	if isTimeout {
		return fmt.Errorf("structure load command timeout")
	}
	if len(resp.OutputMessages) != 1 {
		return fmt.Errorf("structure load command failed")
	}
	if !resp.OutputMessages[0].Success {
		return fmt.Errorf("structure load command failed: %s", resp.OutputMessages[0].Message)
	}

	if err = b.sendSettingsCommand(`structure delete "Fatalder_NBT_block"`, false); err != nil {
		return fmt.Errorf("failed to send structure delete command: %w", err)
	}

	return nil
}

func (b *BuildTask) calcChunkClearArea(startPos define.BlockPos, endPos define.BlockPos) []string {
	// 直接使用起止坐标的X、Z范围（无需再计算区块偏移）
	chunkStartX := startPos.X()
	chunkStartZ := startPos.Z()
	chunkEndX := endPos.X()
	chunkEndZ := endPos.Z()

	// 确保X、Z范围为有效区间（防止start > end的情况）
	if chunkStartX > chunkEndX {
		chunkStartX, chunkEndX = chunkEndX, chunkStartX
	}
	if chunkStartZ > chunkEndZ {
		chunkStartZ, chunkEndZ = chunkEndZ, chunkStartZ
	}

	// Y轴范围处理（同样确保有效区间）
	startY := startPos.Y()
	endY := endPos.Y()
	if startY > endY {
		startY, endY = endY, startY
	}

	if b.VerifyChunkLevel == VerifyChunkLevelNone {
		minX, maxX, minY, maxY, minZ, maxZ, ok := b.structureBounds()
		if !ok {
			return nil
		}

		chunkStartX = max(chunkStartX, minX)
		startY = max(startY, minY)
		chunkStartZ = max(chunkStartZ, minZ)

		chunkEndX = min(chunkEndX, maxX)
		endY = min(endY, maxY)
		chunkEndZ = min(chunkEndZ, maxZ)
	}

	chunkArea := (chunkEndX - chunkStartX + 1) * (chunkEndZ - chunkStartZ + 1)
	const maxBlocks = 32768            // 单次fill指令的最大方块数限制
	maxHeight := maxBlocks / chunkArea // 每次处理的最大高度（32768/256=128）
	if maxHeight < 1 {
		maxHeight = 1
	}

	var airCommands []string

	// 按最大高度分段生成fill指令
	for y := startY; y <= endY; {
		currentEndY := y + int32(maxHeight) - 1
		if currentEndY > endY {
			currentEndY = endY
		}

		airCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:stone",
			chunkStartX, y, chunkStartZ, chunkEndX, currentEndY, chunkEndZ)
		airCommands = append(airCommands, airCmd)

		y = currentEndY + 1
	}

	for y := startY; y <= endY; {
		currentEndY := y + int32(maxHeight) - 1
		if currentEndY > endY {
			currentEndY = endY
		}

		airCmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
			chunkStartX, y, chunkStartZ, chunkEndX, currentEndY, chunkEndZ)
		airCommands = append(airCommands, airCmd)

		y = currentEndY + 1
	}

	return airCommands
}

func (b *BuildTask) placeDenyLayerForChunk(chunkWorldPosX, chunkWorldPosZ int32) error {
	if !b.AutoPlaceDenyBlock || b.reader == nil {
		return nil
	}
	buildStartPos := b.getBuildStartPos()
	realStartChunkX := buildStartPos.X() // - mod(buildStartPos.X(), 16)
	realStartChunkZ := buildStartPos.Z() // - mod(buildStartPos.Z(), 16)
	size := b.reader.GetSize()
	offset := b.reader.GetOffsetPos()
	if size.Width <= 0 || size.Length <= 0 {
		return nil
	}
	maxX := realStartChunkX + int32(size.Width) - offset.X() - 1
	maxZ := realStartChunkZ + int32(size.Length) - offset.Z() - 1

	startX := chunkWorldPosX
	endX := chunkWorldPosX + 15
	startZ := chunkWorldPosZ
	endZ := chunkWorldPosZ + 15

	if endX < realStartChunkX || startX > maxX || endZ < realStartChunkZ || startZ > maxZ {
		return nil
	}
	if startX < realStartChunkX {
		startX = realStartChunkX
	}
	if endX > maxX {
		endX = maxX
	}
	if startZ < realStartChunkZ {
		startZ = realStartChunkZ
	}
	if endZ > maxZ {
		endZ = maxZ
	}
	if startX > endX || startZ > endZ {
		return nil
	}
	denyLayerY := b.getOriginalStartPos().Y()
	cmd := fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:deny",
		startX,
		denyLayerY,
		startZ,
		endX,
		denyLayerY,
		endZ,
	)
	return b.sendSettingsCommand(cmd, false)
}

func (b *BuildTask) structureBounds() (minX, maxX, minY, maxY, minZ, maxZ int32, ok bool) {
	if b.reader == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	size := b.reader.GetSize()
	if size.Width <= 0 || size.Length <= 0 {
		return 0, 0, 0, 0, 0, 0, false
	}
	offset := b.reader.GetOffsetPos()
	buildStartPos := b.getBuildStartPos()

	minX = buildStartPos.X()
	minY = buildStartPos.Y()
	minZ = buildStartPos.Z()
	maxX = minX + int32(size.Width) - offset.X() - 1
	maxY = minY + int32(size.Height) - offset.Y() - 1
	maxZ = minZ + int32(size.Length) - offset.Z() - 1
	return minX, maxX, minY, maxY, minZ, maxZ, true
}

func (b *BuildTask) placeBorderRingForChunk(chunkWorldPosX, chunkWorldPosZ int32) error {
	if !b.AutoPlaceBorderBlock || b.reader == nil {
		return nil
	}
	minX, maxX, _, _, minZ, maxZ, ok := b.structureBounds()
	if !ok {
		return nil
	}
	outerMinX := minX - 1
	outerMaxX := maxX + 1
	outerMinZ := minZ - 1
	outerMaxZ := maxZ + 1

	chunkMinX := chunkWorldPosX
	chunkMaxX := chunkWorldPosX + 15
	chunkMinZ := chunkWorldPosZ
	chunkMaxZ := chunkWorldPosZ + 15

	borderY := b.getOriginalStartPos().Y()

	// 顶边（z = outerMinZ）
	if outerMinZ >= chunkMinZ && outerMinZ <= chunkMaxZ {
		x1 := max(outerMinX, chunkMinX)
		x2 := min(outerMaxX, chunkMaxX)
		if x1 <= x2 {
			if err := b.sendSettingsCommand(fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:border_block",
				x1, borderY, outerMinZ, x2, borderY, outerMinZ,
			), false); err != nil {
				return err
			}
		}
	}
	// 底边（z = outerMaxZ）
	if outerMaxZ >= chunkMinZ && outerMaxZ <= chunkMaxZ {
		x1 := max(outerMinX, chunkMinX)
		x2 := min(outerMaxX, chunkMaxX)
		if x1 <= x2 {
			if err := b.sendSettingsCommand(fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:border_block",
				x1, borderY, outerMaxZ, x2, borderY, outerMaxZ,
			), false); err != nil {
				return err
			}
		}
	}
	// 左边（x = outerMinX）
	if outerMinX >= chunkMinX && outerMinX <= chunkMaxX {
		z1 := max(outerMinZ, chunkMinZ)
		z2 := min(outerMaxZ, chunkMaxZ)
		if z1 <= z2 {
			if err := b.sendSettingsCommand(fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:border_block",
				outerMinX, borderY, z1, outerMinX, borderY, z2,
			), false); err != nil {
				return err
			}
		}
	}
	// 右边（x = outerMaxX）
	if outerMaxX >= chunkMinX && outerMaxX <= chunkMaxX {
		z1 := max(outerMinZ, chunkMinZ)
		z2 := min(outerMaxZ, chunkMaxZ)
		if z1 <= z2 {
			if err := b.sendSettingsCommand(fmt.Sprintf(
				"fill %d %d %d %d %d %d minecraft:border_block",
				outerMaxX, borderY, z1, outerMaxX, borderY, z2,
			), false); err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *BuildTask) placeBorderRingNearChunk(chunkWorldPosX, chunkWorldPosZ int32) error {
	if !b.AutoPlaceBorderBlock || b.reader == nil {
		return nil
	}
	minX, maxX, _, _, minZ, maxZ, ok := b.structureBounds()
	if !ok {
		return nil
	}
	outerMinX := minX - 1
	outerMaxX := maxX + 1
	outerMinZ := minZ - 1
	outerMaxZ := maxZ + 1

	chunkMinX := chunkWorldPosX
	chunkMaxX := chunkWorldPosX + 15
	chunkMinZ := chunkWorldPosZ
	chunkMaxZ := chunkWorldPosZ + 15

	dxList := []int32{0}
	if outerMinX == chunkMinX-1 {
		dxList = append(dxList, -16)
	}
	if outerMaxX == chunkMaxX+1 {
		dxList = append(dxList, 16)
	}
	dzList := []int32{0}
	if outerMinZ == chunkMinZ-1 {
		dzList = append(dzList, -16)
	}
	if outerMaxZ == chunkMaxZ+1 {
		dzList = append(dzList, 16)
	}

	for _, dx := range dxList {
		for _, dz := range dzList {
			if err := b.placeBorderRingForChunk(chunkWorldPosX+dx, chunkWorldPosZ+dz); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *BuildTask) placeBorderRingForChunks(chunks map[define.ChunkPos]*chunk.Chunk, worldChunkX, worldChunkZ int32) error {
	if !b.AutoPlaceBorderBlock || b.reader == nil || len(chunks) == 0 {
		return nil
	}

	minChunkPosX := int32(math.MaxInt32)
	minChunkPosZ := int32(math.MaxInt32)
	for chunkPos := range chunks {
		if chunkPos.X() < minChunkPosX {
			minChunkPosX = chunkPos.X()
		}
		if chunkPos.Z() < minChunkPosZ {
			minChunkPosZ = chunkPos.Z()
		}
	}

	for chunkPos := range chunks {
		chunkWorldPosX := worldChunkX + (chunkPos.X()-minChunkPosX)*16
		chunkWorldPosZ := worldChunkZ + (chunkPos.Z()-minChunkPosZ)*16
		if err := b.placeBorderRingNearChunk(chunkWorldPosX, chunkWorldPosZ); err != nil {
			return err
		}
	}
	return nil
}

func (b *BuildTask) buildChunks(chunks map[define.ChunkPos]*chunk.Chunk, worldChunkX, worldChunkY, worldChunkZ int32) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	buildStartPos := b.getBuildStartPos()

	minChunkPosX := int32(math.MaxInt32)
	maxChunkPosX := int32(math.MinInt32)
	minChunkPosZ := int32(math.MaxInt32)
	maxChunkPosZ := int32(math.MinInt32)
	chunkPositions := make([]define.ChunkPos, 0, len(chunks))

	for chunkPos, _ := range chunks {
		chunkPositions = append(chunkPositions, chunkPos)
		if chunkPos.X() < minChunkPosX {
			minChunkPosX = chunkPos.X()
		}
		if chunkPos.X() > maxChunkPosX {
			maxChunkPosX = chunkPos.X()
		}
		if chunkPos.Z() < minChunkPosZ {
			minChunkPosZ = chunkPos.Z()
		}
		if chunkPos.Z() > maxChunkPosZ {
			maxChunkPosZ = chunkPos.Z()
		}
	}

	if b.AutoCleanBlock {
		widthX := (maxChunkPosX - minChunkPosX + 1) * 16
		widthZ := (maxChunkPosZ - minChunkPosZ + 1) * 16
		endPos := define.BlockPos{worldChunkX + widthX - 1, buildStartPos.Y() - mod(buildStartPos.Y(), 16) + int32(b.reader.GetSize().Height-mod(b.reader.GetSize().Height, 16)) + 16, worldChunkZ + widthZ - 1}
		for _, cmd := range b.calcChunkClearArea(
			define.BlockPos{worldChunkX, worldChunkY, worldChunkZ},
			endPos,
		) {
			if err := b.sendSettingsCommand(cmd, false); err != nil {
				return fmt.Errorf("failed to clean chunk: %w", err)
			}
		}
	}

	if b.AutoPlaceBorderBlock {
		if err := b.placeBorderRingForChunks(chunks, worldChunkX, worldChunkZ); err != nil {
			return err
		}
	}

	if b.AutoPlaceDenyBlock {
		for _, chunkPos := range chunkPositions {
			chunkWorldPosX := worldChunkX + (chunkPos.X()-minChunkPosX)*16
			chunkWorldPosZ := worldChunkZ + (chunkPos.Z()-minChunkPosZ)*16
			if err := b.placeDenyLayerForChunk(chunkWorldPosX, chunkWorldPosZ); err != nil {
				return err
			}
		}
	}

	if b.EnableDeltaMode {
		for command := range chunk_fill.GenerateChunksCommand(
			chunks,
			defines.BlockPos{
				worldChunkX,
				worldChunkY,
				worldChunkZ,
			},
		) {
			if err := b.sendSettingsCommand(command, false); err != nil {
				return fmt.Errorf("failed to send command: %w", err)
			}
		}
	} else {
		for _, chunkPos := range chunkPositions {
			c := chunks[chunkPos]
			if c == nil {
				continue
			}

			chunkWorldPosX := worldChunkX + (chunkPos.X()-minChunkPosX)*16
			chunkWorldPosZ := worldChunkZ + (chunkPos.Z()-minChunkPosZ)*16
			subChunks := c.Sub()
			for subChunkY, subChunk := range subChunks {
				if subChunk == nil || subChunk.Empty() {
					continue
				}

				layerBaseY := worldChunkY + int32(subChunkY)*16
				if err := b.moveBot(defines.BlockPos{
					chunkWorldPosX,
					layerBaseY,
					chunkWorldPosZ,
				}); err != nil {
					return fmt.Errorf("failed to move bot: %w", err)
				}

				layer := subChunk.Layer(0)
				for x := range byte(16) {
					for y := range byte(16) {
						for z := range byte(16) {
							runtimeID := layer.At(x, y, z)
							if runtimeID == block.AirRuntimeID {
								continue
							}

							name, properties, _ := block.RuntimeIDToState(runtimeID)
							stateStr := utils.PropertiesToStateStr(properties)
							cmdStr := fmt.Sprintf(
								"setblock %d %d %d %s %s",
								chunkWorldPosX+int32(x),
								layerBaseY+int32(y),
								chunkWorldPosZ+int32(z),
								name,
								stateStr,
							)
							if err := b.sendSettingsCommand(cmdStr, false); err != nil {
								return fmt.Errorf(
									"failed to build block at [%d,%d,%d]: %w",
									chunkWorldPosX+int32(x),
									layerBaseY+int32(y),
									chunkWorldPosZ+int32(z),
									err,
								)
							}
						}
					}
				}
			}
		}
	}

	if b.IgnoreCommandBlock && b.IgnoreOtherNBTBlock {
		return nil
	}

	chunksNBTs, err := b.reader.GetChunksNBT(chunkPositions)
	if err != nil {
		return fmt.Errorf("failed to get chunks nbt: %w", err)
	}

	for _, chunkPos := range chunkPositions {
		c := chunks[chunkPos]
		if c == nil {
			continue
		}
		nbtList := chunksNBTs[chunkPos]
		if len(nbtList) == 0 {
			continue
		}

		chunkWorldPosX := worldChunkX + (chunkPos.X()-minChunkPosX)*16
		chunkWorldPosZ := worldChunkZ + (chunkPos.Z()-minChunkPosZ)*16

		if !b.IgnoreCommandBlock {
			if err := b.buildCommandBlockNBTs(
				c,
				nbtList,
				chunkWorldPosX,
				worldChunkY,
				chunkWorldPosZ,
			); err != nil {
				return fmt.Errorf("failed to build command block nbt: %w", err)
			}
		}

		if !b.IgnoreOtherNBTBlock {
			for nbtBlockPos, nbt := range nbtList {
				nbtWorldBlockPosX := chunkWorldPosX + nbtBlockPos.X()
				nbtWorldBlockPosY := worldChunkY + nbtBlockPos.Y() + 64
				nbtWorldBlockPosZ := chunkWorldPosZ + nbtBlockPos.Z()

				blockRuntimeID := c.Block(uint8(nbtBlockPos.X()), int16(nbtBlockPos.Y()), uint8(nbtBlockPos.Z()), 0)
				if b.everyNBTBlockBuildingCallback != nil {
					b.everyNBTBlockBuildingCallback(defines.BlockPos{
						nbtWorldBlockPosX,
						nbtWorldBlockPosY,
						nbtWorldBlockPosZ,
					}, blockRuntimeID, nbt)
				}
				if err := b.placeNBTBlock(blockRuntimeID, nbt, nbtWorldBlockPosX, nbtWorldBlockPosY, nbtWorldBlockPosZ); err != nil {
					return fmt.Errorf("failed to place nbt block: %w", err)
				}
			}
		}
	}

	return nil
}

// buildChunk 通用的区块构建函数
// 参数：
//   - chunk: 要构建的区块数据
//   - worldChunkX: 区块的世界X坐标（16的倍数）
//   - worldChunkY: 区块的世界Y坐标（16的倍数）
//   - worldChunkZ: 区块的世界Z坐标（16的倍数）
//
// 返回：
//   - error: 如果构建过程中出现错误则返回错误信息
func (b *BuildTask) buildChunk(c *chunk.Chunk, worldChunkX, worldChunkY, worldChunkZ int32) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}

	subChunks := c.Sub()
	// 清理区块（如果需要）
	if b.AutoCleanBlock {
		for subChunkY := range subChunks {
			cmdStr := fmt.Sprintf(
				"fill %d %d %d %d %d %d air",
				worldChunkX,
				worldChunkY+int32(subChunkY)*16,
				worldChunkZ,
				worldChunkX+15,
				worldChunkY+int32(subChunkY)*16+15,
				worldChunkZ+15,
			)
			if err := b.sendSettingsCommand(cmdStr, false); err != nil {
				return fmt.Errorf("failed to clean chunk: %w", err)
			}
		}
	}

	if b.AutoPlaceBorderBlock {
		if err := b.placeBorderRingNearChunk(worldChunkX, worldChunkZ); err != nil {
			return err
		}
	}

	if b.AutoPlaceDenyBlock {
		if err := b.placeDenyLayerForChunk(worldChunkX, worldChunkZ); err != nil {
			return err
		}
	}

	if b.EnableDeltaMode {
		for command := range chunk_fill.GenerateChunkCommand(
			c,
			defines.BlockPos{
				worldChunkX,
				worldChunkY,
				worldChunkZ,
			},
		) {
			if err := b.sendSettingsCommand(command, false); err != nil {
				return fmt.Errorf("failed to send command: %w", err)
			}
		}
	} else {
		for subChunkY, subChunk := range subChunks {
			if subChunk.Empty() {
				continue
			}

			if err := b.moveBot(defines.BlockPos{
				worldChunkX,
				worldChunkY + int32(subChunkY)*16,
				worldChunkZ,
			}); err != nil {
				return fmt.Errorf("failed to move bot: %w", err)
			}
			layer := subChunk.Layer(0)
			for x := range byte(16) {
				for y := range byte(16) {
					for z := range byte(16) {
						blockRuntimeID := layer.At(x, y, z)
						if blockRuntimeID == block.AirRuntimeID {
							continue
						}

						// 获取方块名称和属性
						name, properties, _ := block.RuntimeIDToState(blockRuntimeID)
						stateStr := utils.PropertiesToStateStr(properties)

						// 构建setblock命令
						cmdStr := fmt.Sprintf(
							"setblock %d %d %d %s %s",
							worldChunkX+int32(x),
							worldChunkY+int32(subChunkY)*16+int32(y),
							worldChunkZ+int32(z),
							name,
							stateStr,
						)

						// 执行命令
						if err := b.sendSettingsCommand(cmdStr, false); err != nil {
							return fmt.Errorf("failed to build block at [%d,%d,%d]: %w", x, y, z, err)
						}
					}
				}
			}
		}
	}

	if !b.IgnoreCommandBlock || !b.IgnoreOtherNBTBlock {
		buildStartPos := b.getBuildStartPos()
		startPosX := buildStartPos.X()
		startPosZ := buildStartPos.Z()
		realStartChunkX := startPosX - mod(startPosX, 16)
		realStartChunkZ := startPosZ - mod(startPosZ, 16)
		chunkPosX := worldChunkX - realStartChunkX
		chunkPosZ := worldChunkZ - realStartChunkZ
		chunkX := chunkPosX / 16
		chunkZ := chunkPosZ / 16
		chunkPos := define.ChunkPos{
			chunkX,
			chunkZ,
		}
		chunksNBTs, err := b.reader.GetChunksNBT([]define.ChunkPos{
			chunkPos,
		})
		if err != nil {
			return fmt.Errorf("failed to get chunks nbt: %w", err)
		}
		nbts := chunksNBTs[chunkPos]
		if !b.IgnoreCommandBlock {
			if err := b.buildCommandBlockNBTs(
				c,
				nbts,
				worldChunkX,
				worldChunkY,
				worldChunkZ,
			); err != nil {
				return fmt.Errorf("failed to build command block nbt: %w", err)
			}
		}
		if !b.IgnoreOtherNBTBlock {
			for nbtBlockPos, nbt := range nbts {
				nbtWorldBlockPosX := worldChunkX + nbtBlockPos.X()
				nbtWorldBlockPosY := worldChunkY + nbtBlockPos.Y() + 64
				nbtWorldBlockPosZ := worldChunkZ + nbtBlockPos.Z()
				blockRuntimeID := c.Block(uint8(nbtBlockPos.X()), int16(nbtBlockPos.Y()), uint8(nbtBlockPos.Z()), 0)
				if b.everyNBTBlockBuildingCallback != nil {
					b.everyNBTBlockBuildingCallback(defines.BlockPos{
						nbtWorldBlockPosX,
						nbtWorldBlockPosY,
						nbtWorldBlockPosZ,
					}, blockRuntimeID, nbt)
				}
				if err := b.placeNBTBlock(blockRuntimeID, nbt, nbtWorldBlockPosX, nbtWorldBlockPosY, nbtWorldBlockPosZ); err != nil {
					return fmt.Errorf("failed to place nbt block: %w", err)
				}
			}
		}
	}

	// 清理物品（如果需要）
	if b.AutoCleanItem {
		cmdStr := fmt.Sprintf(
			"kill @e[type=Item,x=%d,y=%d,z=%d,dx=16,dy=385,dz=16]",
			worldChunkX,
			-64,
			worldChunkZ,
		)
		if err := b.sendSettingsCommand(cmdStr, false); err != nil {
			return fmt.Errorf("failed to clean items: %w", err)
		}
	}

	return nil
}

// fixChunkByRebuild 修补失败的区块，通过重新构建整个区块
// 参数：
//   - failedChunkPos: 失败的区块位置
//   - unverifyChunks: 未验证的区块映射表
//
// 返回：
//   - error: 如果修补过程中出现错误则返回错误信息
func (b *BuildTask) fixChunkByRebuild(failedChunkPos define.ChunkPos, unverifyChunks map[define.ChunkPos]*chunk.Chunk) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	// 计算区块的世界坐标（16的倍数）
	worldChunkX := failedChunkPos.X() * 16
	worldChunkZ := failedChunkPos.Z() * 16
	// 使用起始Y坐标作为基准
	buildStartPos := b.getBuildStartPos()
	worldChunkY := buildStartPos.Y() - mod(buildStartPos.Y(), 16)

	// 移动机器人到目标区块位置
	targetPos := defines.BlockPos{
		worldChunkX,
		buildStartPos.Y(), // 使用起始Y坐标
		worldChunkZ,
	}
	if err := b.moveBot(targetPos); err != nil {
		return fmt.Errorf("failed to move bot to fix chunk position: %w", err)
	}

	// 等待区块加载
	if b.WaitChunkLoad {
		if err := b.waitChunkLoad(failedChunkPos); err != nil {
			return fmt.Errorf("failed to wait for fix chunk to load: %w", err)
		}
		defer b.afterChunkLoaded(failedChunkPos)
	}

	// 查找对应的原始区块数据
	originalChunk := unverifyChunks[failedChunkPos]
	if originalChunk == nil {
		return fmt.Errorf("original chunk data not found for chunk %v, cannot perform fix", failedChunkPos)
	}

	// 使用通用构建函数重新构建整个区块
	if err := b.buildChunk(originalChunk, worldChunkX, worldChunkY, worldChunkZ); err != nil {
		return err
	}

	return nil
}

// StringSimilarity 计算两个字符串的相似百分比（0-100）
// 基于Levenshtein编辑距离：1 - (距离 / 较长字符串长度)，再转换为百分比
func StringSimilarity(a, b string) float64 {
	if a == b {
		return 100.0
	}

	lenA, lenB := len(a), len(b)
	if lenA == 0 {
		return float64(lenB) * 100.0 // 实际应为0，此处仅作边界处理
	}
	if lenB == 0 {
		return float64(lenA) * 100.0 // 实际应为0，此处仅作边界处理
	}

	// 初始化编辑距离矩阵
	dp := make([][]int, lenA+1)
	for i := range dp {
		dp[i] = make([]int, lenB+1)
		dp[i][0] = i
	}
	for j := 0; j <= lenB; j++ {
		dp[0][j] = j
	}

	// 计算编辑距离
	for i := 1; i <= lenA; i++ {
		for j := 1; j <= lenB; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[i][j] = min3(
				dp[i-1][j]+1,      // 删除
				dp[i][j-1]+1,      // 插入
				dp[i-1][j-1]+cost, // 替换
			)
		}
	}

	// 计算相似度百分比
	maxLen := math.Max(float64(lenA), float64(lenB))
	distance := float64(dp[lenA][lenB])
	similarity := (1 - distance/maxLen) * 100

	// 确保结果在0-100之间（处理边界情况）
	return math.Max(0, math.Min(100, similarity))
}

// 辅助函数：返回三个数中的最小值
func min3(a, b, c int) int {
	minVal := a
	if b < minVal {
		minVal = b
	}
	if c < minVal {
		minVal = c
	}
	return minVal
}

func convertStructureFileToMCWorld(srcPath, destPath string, startCallback func(int), progressCallback func()) error {
	if startCallback == nil {
		startCallback = func(int) {}
	}
	if progressCallback == nil {
		progressCallback = func() {}
	}

	tempDir, err := os.MkdirTemp("", "fatalder-mcworld-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	worldDir := filepath.Join(tempDir, "world")
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		return err
	}

	bedrockWorld, err := world.Open(worldDir, nil)
	if err != nil {
		return err
	}

	file, err := os.Open(srcPath)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}
	defer file.Close()

	structure, err := structure.StructureFromFile(file)
	if err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}

	defer structure.Close()

	size := structure.GetSize()

	if err := structure.ToMCWorld(bedrockWorld, bwo_define.SubChunkPos{0, -4, 0}, startCallback, progressCallback); err != nil {
		_ = bedrockWorld.CloseWorld()
		return err
	}

	structureName := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	worldName := fmt.Sprintf("%s@[0,-64,0]~[%d,%d,%d]",
		structureName,
		size.Width-1,
		size.Height-64-1,
		size.Length-1,
	)
	bedrockWorld.LevelDat().LevelName = worldName

	if err := bedrockWorld.CloseWorld(); err != nil {
		return err
	}

	entries, err := os.ReadDir(worldDir)
	if err != nil {
		return err
	}

	var archiveInputs []string
	for _, entry := range entries {
		archiveInputs = append(archiveInputs, filepath.Join(worldDir, entry.Name()))
	}

	tmpZip := filepath.Join(tempDir, "world.zip")
	z := archiver.Zip{}
	if err := z.Archive(archiveInputs, tmpZip); err != nil {
		return err
	}

	if err := os.RemoveAll(destPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(tmpZip, destPath); err != nil {
		// 重命名失败（可能跨设备），改用复制+删除
		if err := copyAndRemoveFile(tmpZip, destPath); err != nil {
			return fmt.Errorf("rename failed, copy fallback also failed: %w", err)
		}
	}

	return nil
}

// 跨设备文件移动：复制后删除原文件
func copyAndRemoveFile(src, dst string) error {
	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src file: %w", err)
	}
	defer srcFile.Close()

	// 创建目标文件（保留源文件权限）
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("get src file info: %w", err)
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create dst file: %w", err)
	}
	defer dstFile.Close()

	// 复制文件内容
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		// 复制失败，清理目标文件
		os.Remove(dst)
		return fmt.Errorf("copy file content: %w", err)
	}

	// 复制成功，删除源文件
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove src file after copy: %w", err)
	}

	return nil
}

// handleChunkVerificationAndFix 处理区块验证和循环修补逻辑
func (b *BuildTask) handleChunkVerificationAndFix(unverifyChunks map[defines.ChunkPos]*chunk.Chunk, initialChunkStatusMap map[define.ChunkPos]bool) error {
	if err := b.waitForControl(); err != nil {
		return err
	}
	// 获取最大修补深度配置
	maxFixDepth := utils.GetConfig().GetBuildTaskConfig().MaxFixDepth
	if maxFixDepth < 1 {
		maxFixDepth = 1 // 确保至少修补一次
	}

	// 跟踪每个区块的修补次数
	fixAttempts := make(map[define.ChunkPos]int)

	// 需要修补的区块（从初始验证结果开始）
	chunksNeedFix := make(map[define.ChunkPos]bool)
	for chunkPos, needFix := range initialChunkStatusMap {
		if needFix {
			chunksNeedFix[chunkPos] = true
			fixAttempts[chunkPos] = 0
		}
	}

	// 循环修补直到所有区块验证通过或达到最大修补深度
	for len(chunksNeedFix) > 0 {
		if err := b.waitForControl(); err != nil {
			return err
		}
		// 收集本轮需要修补的区块
		currentFixList := make([]define.ChunkPos, 0)
		for chunkPos := range chunksNeedFix {
			if fixAttempts[chunkPos] < maxFixDepth {
				currentFixList = append(currentFixList, chunkPos)
			}
		}

		// 如果没有区块可以继续修补，退出循环
		if len(currentFixList) == 0 {
			break
		}

		// 修补当前批次的区块
		for _, failedChunkPos := range currentFixList {
			if b.everyChunkAutoFixCallback != nil {
				b.everyChunkAutoFixCallback(defines.ChunkPos(failedChunkPos))
			}
			if err := b.fixChunkByRebuild(failedChunkPos, unverifyChunks); err != nil {
				return err
			}
			fixAttempts[failedChunkPos]++
		}

		// 重新验证修补过的区块
		reVerifyResult, err := b.reVerifyChunks(currentFixList, unverifyChunks)
		if err != nil {
			return err
		}

		// 更新需要修补的区块列表
		for _, chunkPos := range currentFixList {
			if !reVerifyResult[chunkPos] {
				// 验证仍然失败
				if fixAttempts[chunkPos] >= maxFixDepth {
					// 调用修补次数超过限制的回调
					if b.chunkFixExceededLimitCallback != nil {
						b.chunkFixExceededLimitCallback(defines.ChunkPos(chunkPos), maxFixDepth)
					}

					delete(chunksNeedFix, chunkPos)
				}
				// 否则保持在需要修补列表中
			} else {
				// 验证通过，从需要修补列表中移除
				delete(chunksNeedFix, chunkPos)
			}
		}
	}

	return nil
}

// reVerifyChunks 重新验证指定的区块列表
func (b *BuildTask) reVerifyChunks(chunkPosList []define.ChunkPos, unverifyChunks map[defines.ChunkPos]*chunk.Chunk) (map[define.ChunkPos]bool, error) {
	if err := b.waitForControl(); err != nil {
		return nil, err
	}
	if len(chunkPosList) == 0 {
		return make(map[define.ChunkPos]bool), nil
	}

	game_interface := b.control.GameInterface()
	subChunkRequest := game_interface.SubChunkRequest()
	dimensionID, _ := b.control.Resources().UQHolder().Micro().GetExtendInfo().GetBotDimension()
	dimension := bwo_define.Dimension(dimensionID)

	// 构建需要验证的SubChunk位置列表
	targetSubChunkPosList := make([]protocol.SubChunkPos, 0)
	buildStartPos := b.getBuildStartPos()
	startPosY := buildStartPos.Y()
	realStartChunkY := startPosY - mod(startPosY, 16)

	for _, chunkPos := range chunkPosList {
		realChunkPos := defines.ChunkPos{chunkPos.X(), chunkPos.Z()}
		unverifyChunk := unverifyChunks[realChunkPos]
		if unverifyChunk == nil {
			continue
		}

		noAirSubChunkYList := make([]int, 0)
		for subChunkPosY, subChunk := range unverifyChunk.Sub() {
			if subChunk.Empty() {
				continue
			}
			noAirSubChunkYList = append(noAirSubChunkYList, int(unverifyChunk.SubIndex(int16(subChunkPosY*16))))
		}
		if len(noAirSubChunkYList) == 0 {
			continue
		}
		for _, subChunkPosY := range noAirSubChunkYList {
			targetSubChunkPosList = append(targetSubChunkPosList, protocol.SubChunkPos{
				chunkPos.X(),
				int32(realStartChunkY)/16 + int32(subChunkPosY),
				chunkPos.Z(),
			})
		}
	}

	if len(targetSubChunkPosList) == 0 {
		// 没有需要验证的SubChunk，所有区块都通过
		result := make(map[define.ChunkPos]bool)
		for _, chunkPos := range chunkPosList {
			result[chunkPos] = true
		}
		return result, nil
	}

	// 发送SubChunk请求
	request := subChunkRequest.MakeSubChunkRequestBySubChunkPos(dimension, targetSubChunkPosList)
	response, err := subChunkRequest.SendSubChunkRequestWithResp(request)
	if err != nil {
		return nil, err
	}

	// 分析验证结果
	chunkStatusMap := make(map[define.ChunkPos]bool)
	// 初始化所有目标区块为"验证通过"
	for _, chunkPos := range chunkPosList {
		chunkStatusMap[chunkPos] = true
	}

	// 使用公共验证方法（注意：reVerifyChunks返回相反的逻辑，true表示通过）
	b.verifySubChunkEntries(response, unverifyChunks, chunkStatusMap)

	return chunkStatusMap, nil
}

// verifySubChunkEntries 公共的SubChunk验证方法
// 参数：
//   - response: SubChunk响应
//   - unverifyChunks: 未验证的区块映射
//   - chunkStatusMap: 区块状态映射，true表示验证通过，false表示验证失败
func (b *BuildTask) verifySubChunkEntries(response *packet.SubChunk, unverifyChunks map[defines.ChunkPos]*chunk.Chunk, chunkStatusMap map[define.ChunkPos]bool) {
	for _, entity := range response.SubChunkEntries {
		chunkPos := define.ChunkPos{
			response.Position[0] + int32(entity.Offset[0]),
			response.Position[2] + int32(entity.Offset[2]),
		}

		realChunkPos := defines.ChunkPos{chunkPos.X(), chunkPos.Z()}
		unverifyChunk := unverifyChunks[realChunkPos]
		if unverifyChunk == nil {
			continue
		}
		if !chunkStatusMap[chunkPos] {
			continue // 已经标记为验证失败，跳过
		}

		// 检查空气子区块
		if b.VerifyChunkLevel >= VerifyChunkLevelAir {
			if entity.Result != protocol.SubChunkResultSuccess {
				chunkStatusMap[chunkPos] = false // 验证失败
				continue
			}
		}

		// 检查相似度
		if b.VerifyChunkLevel >= VerifyChunkLevelSimilar {
			chunkRawPayload := entity.RawPayload
			// 计算子区块索引并进行边界检查
			subChunkIndex := int16(response.Position[1] + int32(entity.Offset[1]))
			chunkRange := unverifyChunk.Range()

			// 检查子区块索引是否在有效范围内
			if subChunkIndex < int16(chunkRange[0]) || subChunkIndex >= int16(chunkRange[1]) {
				// 子区块索引超出范围，跳过此次验证
				chunkStatusMap[chunkPos] = false // 验证失败
				continue
			}

			unverifySubChunk := unverifyChunk.SubChunk(subChunkIndex)
			unverifyChunkRawPayload := chunk.EncodeSubChunk(
				unverifySubChunk,
				chunkRange,
				int(((response.Position[1]+int32(entity.Offset[1]))<<4-int32(chunkRange[0]))>>4),
				chunk.NetworkEncoding,
			)
			chunkPayload := string(chunkRawPayload)
			unverifyChunkPayload := string(unverifyChunkRawPayload)
			if len(unverifyChunkRawPayload) > len(chunkRawPayload) {
				unverifyChunkPayload = string(unverifyChunkRawPayload[:len(chunkRawPayload)])
			} else {
				chunkPayload = string(chunkRawPayload[:len(unverifyChunkRawPayload)])
			}

			MatchingDegree := StringSimilarity(chunkPayload, unverifyChunkPayload)
			// 从全局配置获取匹配度阈值
			minMatchingDegree := utils.GetConfig().GetBuildTaskConfig().MinMatchingDegree
			if MatchingDegree < minMatchingDegree {
				chunkStatusMap[chunkPos] = false // 验证失败
				continue
			}
		}
		// 检查区块数据
		if b.VerifyChunkLevel >= VerifyChunkLevelComplete {
			s, _, _, err := NEMCSubChunkDecode(entity.RawPayload, bwo_define.Dimension(bwo_define.DimensionIDOverworld).Range())
			if err != nil {
				chunkStatusMap[chunkPos] = false // 验证失败
				continue
			}
			buildStartPos := b.getBuildStartPos()
			startPosY := buildStartPos.Y()
			realStartChunkY := startPosY - mod(startPosY, 16)
			realStartChunkPosY := realStartChunkY / 16
			subChunkPosY := int16(response.Position[1] + int32(entity.Offset[1]) - realStartChunkPosY)
			unverifySubChunk := unverifyChunk.SubChunk(subChunkPosY)
			for x := range byte(16) {
				for y := range byte(16) {
					for z := range byte(16) {
						if s.Block(x, y, z, 0) != unverifySubChunk.Block(x, y, z, 0) {
							chunkStatusMap[chunkPos] = false // 验证失败
							break
						}
					}
					if !chunkStatusMap[chunkPos] {
						break
					}
				}
				if !chunkStatusMap[chunkPos] {
					break
				}
			}
		}
	}
}
