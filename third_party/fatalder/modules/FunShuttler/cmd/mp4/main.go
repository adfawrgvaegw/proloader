package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	"github.com/TriM-Organization/bedrock-world-operator/chunk"
	"github.com/TriM-Organization/bedrock-world-operator/define"
	"github.com/Yeah114/FunShuttler/client"
	"github.com/Yeah114/FunShuttler/core/bunker/auth"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/core/minecraft/protocol/packet"
	"github.com/Yeah114/FunShuttler/game_control/game_interface"
	"github.com/Yeah114/FunShuttler/game_control/resources_control"
	"github.com/Yeah114/FunShuttler/game_control/special_command"
	"github.com/disintegration/imaging"
	"github.com/pterm/pterm"
)

// VideoPlayer 用于播放视频到Minecraft地图
type VideoPlayer struct {
	api           *game_interface.GameInterface
	startPos      [3]int32
	endPos        [3]int32
	videoPath     string
	mapsPerRow    int
	mapsPerCol    int
	targetFPS     int
	playbackSpeed float64
	mapIDs        [][]int64
	lastFrames    [][][]color.RGBA
	frameBuffer   chan *image.Image
	stopChan      chan struct{}
	wg            sync.WaitGroup
	ffmpegCmd     *exec.Cmd
	ffmpegOutput  io.ReadCloser
	width         int
	height        int
	planeType     string // 区域类型：horizontal, x_fixed, z_fixed
	xDirection    int    // 1: 递增, -1: 递减
	zDirection    int    // 1: 递增, -1: 递减
}

// ItemFrameData 存储物品框信息
type ItemFrameData struct {
	PosX  int32
	PosY  int32
	PosZ  int32
	MapID int64
	Item  map[string]interface{}
}

// NewVideoPlayer 创建新的视频播放器实例
func NewVideoPlayer(api *game_interface.GameInterface, startPos, endPos [3]int32, videoPath string,
	targetFPS int, playbackSpeed float64, xDirection, zDirection int) *VideoPlayer {

	if xDirection == 0 {
		xDirection = 1
	} // 默认递增
	if zDirection == 0 {
		zDirection = 1
	} // 默认递增

	return &VideoPlayer{
		api:           api,
		startPos:      startPos,
		endPos:        endPos,
		videoPath:     videoPath,
		targetFPS:     targetFPS,
		playbackSpeed: playbackSpeed,
		frameBuffer:   make(chan *image.Image, 10),
		stopChan:      make(chan struct{}),
		xDirection:    xDirection,
		zDirection:    zDirection,
	}
}

// 锁定地图
func (vp *VideoPlayer) LockMap(mapID int64) error {
	mapCreateLockedCopy := &packet.MapCreateLockedCopy{
		OriginalMapID: mapID,
		NewMapID:      mapID,
	}

	if err := vp.api.Resources().WritePacket(mapCreateLockedCopy); err != nil {
		return err
	}

	return nil
}

// PrepareMaps 准备播放视频所需的地图
func (vp *VideoPlayer) PrepareMaps() error {
	// 1. 确定平面类型
	minX, maxX := min(vp.startPos[0], vp.endPos[0]), max(vp.startPos[0], vp.endPos[0])
	minY, maxY := min(vp.startPos[1], vp.endPos[1]), max(vp.startPos[1], vp.endPos[1])
	minZ, maxZ := min(vp.startPos[2], vp.endPos[2]), max(vp.startPos[2], vp.endPos[2])

	if minY == maxY {
		vp.planeType = "horizontal"
	} else if minX == maxX {
		vp.planeType = "x_fixed"
	} else if minZ == maxZ {
		vp.planeType = "z_fixed"
	} else {
		return errors.New("区域不是二维平面")
	}

	pterm.Info.Printf("检测到平面类型: %s\n", vp.planeType)

	// 2. 获取区域内的物品框
	itemFrames, err := vp.scanItemFrames(minX, minY, minZ, maxX, maxY, maxZ)
	if err != nil {
		return err
	}

	if len(itemFrames) == 0 {
		return errors.New("区域内没有找到地图物品框")
	}

	// 3. 根据平面类型对地图物品框进行排序
	switch vp.planeType {
	case "horizontal":
		// 水平面：按X和Z排序（考虑方向）
		sort.Slice(itemFrames, func(i, j int) bool {
			if itemFrames[i].PosZ != itemFrames[j].PosZ {
				if vp.zDirection > 0 {
					return itemFrames[i].PosZ < itemFrames[j].PosZ
				}
				return itemFrames[i].PosZ > itemFrames[j].PosZ
			}
			if vp.xDirection > 0 {
				return itemFrames[i].PosX < itemFrames[j].PosX
			}
			return itemFrames[i].PosX > itemFrames[j].PosX
		})
	case "x_fixed":
		// X固定：按Y和Z排序（考虑方向）
		sort.Slice(itemFrames, func(i, j int) bool {
			if itemFrames[i].PosY != itemFrames[j].PosY {
				return itemFrames[i].PosY > itemFrames[j].PosY // Y总是从上到下
			}
			if vp.zDirection > 0 {
				return itemFrames[i].PosZ < itemFrames[j].PosZ
			}
			return itemFrames[i].PosZ > itemFrames[j].PosZ
		})
	case "z_fixed":
		// Z固定：按Y和X排序（考虑方向）
		sort.Slice(itemFrames, func(i, j int) bool {
			if itemFrames[i].PosY != itemFrames[j].PosY {
				return itemFrames[i].PosY > itemFrames[j].PosY // Y总是从上到下
			}
			if vp.xDirection > 0 {
				return itemFrames[i].PosX < itemFrames[j].PosX
			}
			return itemFrames[i].PosX > itemFrames[j].PosX
		})
	}

	// 4. 计算地图网格大小
	xSet := make(map[int32]struct{})
	ySet := make(map[int32]struct{})
	zSet := make(map[int32]struct{})

	for _, frame := range itemFrames {
		xSet[frame.PosX] = struct{}{}
		ySet[frame.PosY] = struct{}{}
		zSet[frame.PosZ] = struct{}{}
	}

	switch vp.planeType {
	case "horizontal":
		vp.mapsPerRow = len(xSet)
		vp.mapsPerCol = len(zSet)
	case "x_fixed":
		vp.mapsPerRow = len(zSet)
		vp.mapsPerCol = len(ySet)
	case "z_fixed":
		vp.mapsPerRow = len(xSet)
		vp.mapsPerCol = len(ySet)
	}

	if len(itemFrames) != vp.mapsPerRow*vp.mapsPerCol {
		return fmt.Errorf("地图物品框数量(%d)与网格(%dx%d=%d)不匹配",
			len(itemFrames), vp.mapsPerRow, vp.mapsPerCol, vp.mapsPerRow*vp.mapsPerCol)
	}

	pterm.Info.Printf("检测到地图网格: %d行 x %d列\n", vp.mapsPerCol, vp.mapsPerRow)

	// 5. 初始化地图ID和上一帧存储
	vp.mapIDs = make([][]int64, vp.mapsPerCol)
	vp.lastFrames = make([][][]color.RGBA, vp.mapsPerCol)

	for i := range vp.mapIDs {
		vp.mapIDs[i] = make([]int64, vp.mapsPerRow)
		vp.lastFrames[i] = make([][]color.RGBA, vp.mapsPerRow)
	}

	// 6. 填充地图ID
	for i := 0; i < vp.mapsPerCol; i++ {
		for j := 0; j < vp.mapsPerRow; j++ {
			idx := i*vp.mapsPerRow + j
			if idx >= len(itemFrames) {
				return fmt.Errorf("索引超出范围: %d >= %d", idx, len(itemFrames))
			}

			frame := itemFrames[idx]
			vp.mapIDs[i][j] = frame.MapID
			vp.lastFrames[i][j] = make([]color.RGBA, 128*128)

			// 初始化为黑色
			for k := range vp.lastFrames[i][j] {
				vp.lastFrames[i][j][k] = color.RGBA{0, 0, 0, 255}
			}

			pterm.Debug.Printf("地图[%d,%d]: ID=%d (位置: %d,%d,%d)\n",
				i, j, frame.MapID, frame.PosX, frame.PosY, frame.PosZ)
		}
	}

	pterm.Success.Println("地图准备完成")
	return nil
}

// scanItemFrames 扫描区域内的物品框并提取地图ID
func (vp *VideoPlayer) scanItemFrames(minX, minY, minZ, maxX, maxY, maxZ int32) ([]*ItemFrameData, error) {
	dimensionID, ok := vp.api.Resources().UQHolder().Micro().GetExtendInfo().GetBotDimension()
	if !ok {
		return nil, errors.New("无法获取机器人所在维度")
	}

	dimension := define.Dimension(dimensionID)
	start := protocol.BlockPos{minX, minY, minZ}
	end := protocol.BlockPos{maxX, maxY, maxZ}

	subChunkResp, err := vp.api.SubChunkRequest().GetSubChunksInArea(dimension, start, end)
	if err != nil {
		return nil, fmt.Errorf("子区块请求失败: %w", err)
	}

	subChunks := make(map[[3]int32]*chunk.SubChunk)
	blockEntities := make(map[[3]int32]map[string]interface{})
	basePos := subChunkResp.Position
	worldRange := dimension.Range()

	for _, entry := range subChunkResp.SubChunkEntries {
		if entry.Result == protocol.SubChunkResultChunkNotFound || entry.Result == protocol.SubChunkResultSuccessAllAir {
			continue
		}

		subChunk, _, nbts, decodeErr := special_command.NEMCSubChunkDecode(entry.RawPayload, worldRange)
		if decodeErr != nil {
			pterm.Warning.Printf("子区块解码失败(%d,%d,%d): %v\n", entry.Offset[0], entry.Offset[1], entry.Offset[2], decodeErr)
			continue
		}

		chunkX := basePos.X() + int32(entry.Offset[0])
		chunkZ := basePos.Z() + int32(entry.Offset[2])
		subY := int32(entry.Offset[1])

		subChunks[[3]int32{chunkX, subY, chunkZ}] = subChunk

		for _, nbtData := range nbts {
			x, xOk := toInt32(nbtData["x"])
			y, yOk := toInt32(nbtData["y"])
			z, zOk := toInt32(nbtData["z"])
			if xOk && yOk && zOk {
				blockEntities[[3]int32{x, y, z}] = nbtData
			}
		}
	}

	var itemFrames []*ItemFrameData

	for y := minY; y <= maxY; y++ {
		for z := minZ; z <= maxZ; z++ {
			for x := minX; x <= maxX; x++ {
				subChunkKey := [3]int32{x >> 4, y >> 4, z >> 4}
				subChunk := subChunks[subChunkKey]
				if subChunk == nil {
					continue
				}

				runtimeID := subChunk.Block(byte(x&15), byte(y&15), byte(z&15), 0)
				blockName, _, found := block.RuntimeIDToState(runtimeID)
				if !found {
					continue
				}

				if blockName != "minecraft:glow_frame" && blockName != "minecraft:frame" {
					continue
				}

				entityData, exists := blockEntities[[3]int32{x, y, z}]
				if !exists {
					continue
				}

				item, ok := entityData["Item"].(map[string]interface{})
				if !ok {
					continue
				}

				itemName, _ := item["Name"].(string)
				if itemName != "minecraft:filled_map" {
					continue
				}

				tagData, ok := item["tag"].(map[string]interface{})
				if !ok {
					continue
				}

				mapID, ok := toInt64(tagData["map_uuid"])
				if !ok {
					continue
				}

				itemFrames = append(itemFrames, &ItemFrameData{
					PosX:  x,
					PosY:  y,
					PosZ:  z,
					MapID: mapID,
					Item:  item,
				})

				pterm.Success.Printf("找到地图物品框(%d,%d,%d): ID=%d\n", x, y, z, mapID)
			}
		}
	}

	if len(itemFrames) == 0 {
		return nil, errors.New("区域内没有找到包含地图的物品框")
	}

	pterm.Info.Printf("找到 %d 个地图物品框\n", len(itemFrames))
	return itemFrames, nil
}

// StartDecoding 启动视频解码
func (vp *VideoPlayer) StartDecoding() error {
	// 使用ffmpeg命令行工具获取视频信息
	probeCmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
		vp.videoPath,
	)

	output, err := probeCmd.Output()
	if err != nil {
		return fmt.Errorf("获取视频尺寸失败: %w", err)
	}

	_, err = fmt.Sscanf(string(output), "%d,%d", &vp.width, &vp.height)
	if err != nil {
		return fmt.Errorf("解析视频尺寸失败: %w", err)
	}

	pterm.Info.Printf("视频尺寸: %dx%d\n", vp.width, vp.height)

	// 启动ffmpeg进程
	vp.ffmpegCmd = exec.Command("ffmpeg",
		"-i", vp.videoPath,
		"-f", "image2pipe",
		"-pix_fmt", "rgba",
		"-vcodec", "rawvideo",
		"-",
	)

	vp.ffmpegOutput, err = vp.ffmpegCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建ffmpeg输出管道失败: %w", err)
	}

	if err := vp.ffmpegCmd.Start(); err != nil {
		return fmt.Errorf("启动ffmpeg失败: %w", err)
	}

	vp.wg.Add(1)
	go func() {
		defer vp.wg.Done()
		defer vp.ffmpegCmd.Wait()
		defer vp.ffmpegOutput.Close()

		frameSize := vp.width * vp.height * 4
		buffer := make([]byte, frameSize)

		for {
			select {
			case <-vp.stopChan:
				vp.ffmpegCmd.Process.Kill()
				return
			default:
				// 读取一帧数据
				_, err := io.ReadFull(vp.ffmpegOutput, buffer)
				if err != nil {
					if err == io.EOF {
						pterm.Info.Println("视频播放完成")
						return
					}
					pterm.Warning.Println("读取帧数据失败:", err)
					continue
				}

				// 创建图像对象
				img := image.NewRGBA(image.Rect(0, 0, vp.width, vp.height))
				copy(img.Pix, buffer)

				// 调整图像大小并发送到帧缓冲区
				resizedImg := vp.resizeImage(img)
				vp.frameBuffer <- &resizedImg
			}
		}
	}()

	return nil
}

// resizeImage 调整图像大小以适应地图布局（正确填充黑条版本）
func (vp *VideoPlayer) resizeImage(img image.Image) image.Image {
	targetWidth := vp.mapsPerRow * 128  // 目标区域宽度（例如128*列数）
	targetHeight := vp.mapsPerCol * 128 // 目标区域高度（例如128*行数）
	srcBounds := img.Bounds()
	srcWidth, srcHeight := srcBounds.Dx(), srcBounds.Dy()
	srcAspect := float64(srcWidth) / float64(srcHeight)          // 原始宽高比
	targetAspect := float64(targetWidth) / float64(targetHeight) // 目标宽高比

	var scaledWidth, scaledHeight int
	var padLeft, padTop int

	// 计算缩放尺寸，确保完全包含在目标区域内
	if srcAspect > targetAspect {
		// 原始视频更宽（宽屏），按目标宽度缩放，上下填充黑条
		scaledWidth = targetWidth
		scaledHeight = int(float64(targetWidth) / srcAspect)
		padTop = (targetHeight - scaledHeight) / 2
	} else {
		// 原始视频更高或等比，按目标高度缩放，左右填充黑条
		scaledHeight = targetHeight
		scaledWidth = int(float64(targetHeight) * srcAspect)
		padLeft = (targetWidth - scaledWidth) / 2
	}

	// 缩放视频到合适尺寸
	scaledImg := imaging.Resize(img, scaledWidth, scaledHeight, imaging.Lanczos)

	// 创建目标尺寸的黑色背景
	background := imaging.New(targetWidth, targetHeight, color.Black)

	// 居中粘贴视频到背景上
	pastePos := image.Pt(padLeft, padTop) // 计算粘贴坐标
	background = imaging.Paste(background, scaledImg, pastePos)

	return background
}

// Play 播放视频到地图
func (vp *VideoPlayer) Play() error {
	// 等待用户准备
	pterm.Info.Println("视频即将开始播放，请准备...")
	time.Sleep(10 * time.Second)

	// 计算帧间隔
	frameDuration := time.Duration(float64(time.Second) / (float64(vp.targetFPS) * vp.playbackSpeed))
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	frameCount := 0
	startTime := time.Now()

	for {
		select {
		case <-vp.stopChan:
			return nil
		case img := <-vp.frameBuffer:
			select {
			case <-ticker.C:
				// 处理并显示帧
				vp.processFrame(*img)
				frameCount++

				// 每10帧显示状态
				if frameCount%10 == 0 {
					elapsed := time.Since(startTime)
					fps := float64(frameCount) / elapsed.Seconds()
					pterm.Info.Printf("已播放 %d 帧 (%.1f FPS)\n", frameCount, fps)
				}
			default:
				// 跳过帧以维持目标FPS
				pterm.Debug.Println("跳过一帧以维持目标FPS")
			}
		}
	}
}

// processFrame 处理单帧图像并更新地图
func (vp *VideoPlayer) processFrame(frame image.Image) {
	// 处理每个地图块
	for row := 0; row < vp.mapsPerCol; row++ {
		for col := 0; col < vp.mapsPerRow; col++ {
			// 计算当前地图块的偏移量
			xOffset := col * 128
			yOffset := row * 128

			// 获取该地图的上一帧数据
			lastFrame := vp.lastFrames[row][col]

			// 处理图片块并获取变化的像素
			changedPixels, newFrame := processImagePartWithChanges(frame, xOffset, yOffset, lastFrame)

			// 更新该地图的上一帧数据
			vp.lastFrames[row][col] = newFrame

			// 如果有变化的像素，发送更新
			if len(changedPixels) > 0 {
				if err := sendMapPixels(vp.api, vp.mapIDs[row][col], changedPixels); err != nil {
					pterm.Warning.Printf("更新地图(%d,%d)失败: %v\n", row, col, err)
				}
			}
		}
	}
}

// Stop 停止视频播放
func (vp *VideoPlayer) Stop() {
	close(vp.stopChan)
	vp.wg.Wait()

	if vp.ffmpegCmd != nil && vp.ffmpegCmd.Process != nil {
		vp.ffmpegCmd.Process.Kill()
	}

	pterm.Info.Println("视频播放已停止")
}

// processImagePartWithChanges 处理图片块并返回变化的像素
func processImagePartWithChanges(img image.Image, xOffset, yOffset int, lastFrame []color.RGBA) ([]protocol.PixelRequest, []color.RGBA) {
	bounds := img.Bounds()
	changedPixels := make([]protocol.PixelRequest, 0)
	newFrame := make([]color.RGBA, 128*128)

	// 处理128x128块
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			index := y*128 + x

			// 计算实际坐标
			actualX := xOffset + x
			actualY := yOffset + y

			var rgba color.RGBA

			// 检查是否在图片范围内
			if actualX >= bounds.Max.X || actualY >= bounds.Max.Y {
				// 超出范围使用黑色
				rgba = color.RGBA{0, 0, 0, 255}
			} else {
				// 获取原始颜色
				c := img.At(actualX, actualY)
				r, g, b, a := c.RGBA()
				rgba = color.RGBA{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
					A: uint8(a >> 8),
				}
			}

			// 存储到新帧
			newFrame[index] = rgba

			// 与上一帧比较，只记录变化的像素
			if lastFrame[index] != rgba {
				changedPixels = append(changedPixels, protocol.PixelRequest{
					Colour: rgba,
					Index:  uint16(index),
				})
			}
		}
	}

	return changedPixels, newFrame
}

// sendMapPixels 发送地图像素数据
func sendMapPixels(api *game_interface.GameInterface, mapID int64, pixels []protocol.PixelRequest) error {
	mapInfoRequest := &packet.MapInfoRequest{
		MapID:        mapID,
		ClientPixels: pixels,
	}

	if err := api.Resources().WritePacket(mapInfoRequest); err != nil {
		return err
	}

	return nil
}

func toInt32(v interface{}) (int32, bool) {
	switch val := v.(type) {
	case int32:
		return val, true
	case int64:
		return int32(val), true
	case int:
		return int32(val), true
	case int16:
		return int32(val), true
	case int8:
		return int32(val), true
	case uint32:
		return int32(val), true
	case uint64:
		return int32(val), true
	case uint16:
		return int32(val), true
	case uint8:
		return int32(val), true
	case float32:
		return int32(val), true
	case float64:
		return int32(val), true
	default:
		return 0, false
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int32:
		return int64(val), true
	case int:
		return int64(val), true
	case int16:
		return int64(val), true
	case int8:
		return int64(val), true
	case uint64:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint16:
		return int64(val), true
	case uint8:
		return int64(val), true
	case float32:
		return int64(val), true
	case float64:
		return int64(val), true
	default:
		return 0, false
	}
}

// 辅助函数
func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

var (
	c         *client.Client
	resources *resources_control.Resources
	api       *game_interface.GameInterface
)

func SystemTestingLogin() {
	var err error
	tA := time.Now()

	cfg := client.Config{
		AuthServerAddress:    "http://127.0.0.1:8080",
		AuthServerToken:      "",
		RentalServerCode:     "LobbyGame:4682670425802129213",
		RentalServerPasscode: "114514",
	}

	authClient, err := auth.CreateClient(&auth.ClientOptions{AuthServer: cfg.AuthServerAddress})
	if err != nil {
		panic(err)
	}
	cfg.AccessWrapper = auth.NewAccessWrapper(
		authClient,
		cfg.RentalServerCode,
		cfg.RentalServerPasscode,
		cfg.AuthServerToken,
		"", "",
	)

	c, err = client.LoginRentalServer(cfg)
	if err != nil {
		panic(err)
	}
	resources = resources_control.NewResourcesControl(c)
	api = game_interface.NewGameInterface(resources)

	pterm.Success.Printfln("SystemTestingLogin: PASS (Time used = %v)", time.Since(tA))
}

func main() {
	tA := time.Now()

	// 使用login.go中初始化的全局变量
	SystemTestingLogin()
	defer func() {
		time.Sleep(time.Second * 30)
	}()
	go func() {
		for {
			pk, err := c.Conn().ReadPacket()
			if err != nil {
				panic(err)
			}
			_ = resources.HandlePacket(&pk)
		}
	}()

	// 定义两个坐标点
	startPos := [3]int32{73, 76, -40}
	endPos := [3]int32{73, 77, -38} // 修改为你需要的结束坐标

	videoPath := "/storage/emulated/0/Download/Bad Apple but it's in 4k 60fps.mp4" // 修改为你的视频路径

	// 如果物品框的X坐标是从左到右递增（默认），则xDirection=1
	// 如果物品框的X坐标是从右到左递增（即墙面背后），则xDirection=-1
	xDirection := -1 // 根据实际排列设置

	// 如果物品框的Z坐标是从北到南递增（默认），则zDirection=1
	// 如果物品框的Z坐标是从南到北递增，则zDirection=-1
	zDirection := 1 // 根据实际排列设置
	// 创建视频播放器
	player := NewVideoPlayer(api, startPos, endPos, videoPath,
		10,  // 目标FPS
		1.2, // 播放速度
		xDirection,
		zDirection,
	)

	// 准备地图
	if err := player.PrepareMaps(); err != nil {
		pterm.Error.Printfln("准备地图失败: %v", err)
		return
	}

	// 开始解码视频
	if err := player.StartDecoding(); err != nil {
		pterm.Error.Printfln("启动视频解码失败: %v", err)
		return
	}

	// 播放视频
	go func() {
		if err := player.Play(); err != nil {
			pterm.Error.Printfln("播放失败: %v", err)
		}
	}()

	// 等待用户输入停止
	pterm.Info.Println("视频播放中... 按Enter键停止")
	fmt.Scanln()
	player.Stop()

	pterm.Success.Printfln("视频播放完成 (总耗时 %v)", time.Since(tA))
}
