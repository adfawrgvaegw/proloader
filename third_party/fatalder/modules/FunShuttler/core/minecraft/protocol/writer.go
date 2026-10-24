package protocol

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"reflect"
	"sort"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"

	// PhoenixBuilder specific changes.
	// A Python library which named "msgpack"
	"github.com/ugorji/go/codec"
)

import nbt "github.com/Yeah114/FunShuttler/core/minecraft/nbt"

// Writer implements writing methods for data types from Minecraft packets.
// Writer 使用指针传递值，使 Reader 和 Writer 具有一致的接口
type Writer struct {
	w interface {
		io.Writer
		io.ByteWriter
	}
	shieldID int32
}

// NewWriter 创建新的 Writer 实例
func NewWriter(w interface {
	io.Writer
	io.ByteWriter
}, shieldID int32) *Writer {
	return &Writer{w: w, shieldID: shieldID}
}

// Uint8 向底层缓冲区写入 uint8
func (w *Writer) Uint8(x *uint8) {
	_ = w.w.WriteByte(*x)
}

// Int8 向底层缓冲区写入 int8
func (w *Writer) Int8(x *int8) {
	_ = w.w.WriteByte(byte(*x) & 0xff)
}

// Bool 以 0 或 1 写入布尔值
func (w *Writer) Bool(x *bool) {
	_ = w.w.WriteByte(*(*byte)(unsafe.Pointer(x)))
}

// StringUTF 写入带 int16 长度前缀的 UTF 字符串
func (w *Writer) StringUTF(x *string) {
	l := int16(len(*x))
	w.Int16(&l)
	_, _ = w.w.Write([]byte(*x))
}

// String 写入带 varuint32 长度前缀的字符串
func (w *Writer) String(x *string) {
	l := uint32(len(*x))
	w.Varuint32(&l)
	_, _ = w.w.Write([]byte(*x))
}

// ByteSlice 写入带 varuint32 长度前缀的字节切片
func (w *Writer) ByteSlice(x *[]byte) {
	l := uint32(len(*x))
	w.Varuint32(&l)
	_, _ = w.w.Write(*x)
}

// Bytes 直接向缓冲区追加字节切片
func (w *Writer) Bytes(x *[]byte) {
	_, _ = w.w.Write(*x)
}

// ByteFloat 将旋转角度 float32 转换为单字节写入
func (w *Writer) ByteFloat(x *float32) {
	_ = w.w.WriteByte(byte(*x / (360.0 / 256.0)))
}

// Vec3 写入 mgl32.Vec3 作为 3 个 float32
func (w *Writer) Vec3(x *mgl32.Vec3) {
	w.Float32(&x[0])
	w.Float32(&x[1])
	w.Float32(&x[2])
}

// Vec2 写入 mgl32.Vec2 作为 2 个 float32
func (w *Writer) Vec2(x *mgl32.Vec2) {
	w.Float32(&x[0])
	w.Float32(&x[1])
}

// BlockPos 以 3 个 varint32 写入区块位置
func (w *Writer) BlockPos(x *BlockPos) {
	w.Varint32(&x[0])
	w.Varint32(&x[1])
	w.Varint32(&x[2])
}

// UBlockPos 写入区块位置（Y 坐标为 varuint32）
func (w *Writer) UBlockPos(x *BlockPos) {
	w.Varint32(&x[0])
	y := uint32(x[1])
	w.Varuint32(&y)
	w.Varint32(&x[2])
}

// ChunkPos 以 2 个 varint32 写入区块坐标
func (w *Writer) ChunkPos(x *ChunkPos) {
	w.Varint32(&x[0])
	w.Varint32(&x[1])
}

// SubChunkPos 以 3 个 varint32 写入子区块位置
func (w *Writer) SubChunkPos(x *SubChunkPos) {
	w.Varint32(&x[0])
	w.Varint32(&x[1])
	w.Varint32(&x[2])
}

// USubChunkPos 写入网易版子区块位置（Y 坐标为 varuint32）
func (w *Writer) USubChunkPos(x *SubChunkPos) {
	y := uint32(x[1])
	w.Varint32(&x[0])
	w.Varuint32(&y)
	w.Varint32(&x[2])
}

// SoundPos 写入声音位置（转换为区块坐标）
func (w *Writer) SoundPos(x *mgl32.Vec3) {
	b := BlockPos{int32((*x)[0] * 8), int32((*x)[1] * 8), int32((*x)[2] * 8)}
	w.BlockPos(&b)
}

// USoundPos 写入网易版声音位置（使用 UBlockPos）
func (w *Writer) USoundPos(x *mgl32.Vec3) {
	b := BlockPos{int32((*x)[0] * 8), int32((*x)[1] * 8), int32((*x)[2] * 8)}
	w.UBlockPos(&b)
}

// RGB 写入 color.RGBA 作为 3 个 float32
func (w *Writer) RGB(x *color.RGBA) {
	red := float32(x.R) / 255
	green := float32(x.G) / 255
	blue := float32(x.B) / 255
	w.Float32(&red)
	w.Float32(&green)
	w.Float32(&blue)
}

// RGBA 写入 color.RGBA 作为 uint32
func (w *Writer) RGBA(x *color.RGBA) {
	val := uint32(x.R) | uint32(x.G)<<8 | uint32(x.B)<<16 | uint32(x.A)<<24
	w.Uint32(&val)
}

// VarRGBA 写入 color.RGBA 作为 varuint32
func (w *Writer) VarRGBA(x *color.RGBA) {
	val := uint32(x.R) | uint32(x.G)<<8 | uint32(x.B)<<16 | uint32(x.A)<<24
	w.Varuint32(&val)
}

// Netease
func (w *Writer) NeteasePixels(x *[]color.RGBA) {
	isEmpty := len(*x) == 0
	w.Bool(&isEmpty)
	if isEmpty {
		return
	}

	marshalType := uint8(3)
	w.Uint8(&marshalType)

	FuncSlice(w, x, w.RGBA)
}

// UUID 写入 UUID（调整字节顺序）
func (w *Writer) UUID(x *uuid.UUID) {
	b := append((*x)[8:], (*x)[:8]...)
	for i, j := 0, 15; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	_, _ = w.w.Write(b)
}

// PlayerInventoryAction 写入玩家物品栏操作数据
func (w *Writer) PlayerInventoryAction(x *UseItemTransactionData) {
	w.Varint32(&x.LegacyRequestID)
	if x.LegacyRequestID < -1 && (x.LegacyRequestID&1) == 0 {
		Slice(w, &x.LegacySetItemSlots)
	}
	Slice(w, &x.Actions)
	w.Varuint32(&x.ActionType)

	// 网易版使用 UBlockPos
	w.UBlockPos(&x.BlockPosition)

	w.Varint32(&x.BlockFace)
	w.Varint32(&x.HotBarSlot)
	w.ItemInstance(&x.HeldItem)
	w.Vec3(&x.Position)
	w.Vec3(&x.ClickedPosition)
	w.Varuint32(&x.BlockRuntimeID)
}

// GameRule 写入游戏规则数据
func (w *Writer) GameRule(x *GameRule) {
	w.String(&x.Name)
	w.Bool(&x.CanBeModifiedByPlayer)

	switch v := x.Value.(type) {
	case bool:
		id := uint32(1)
		w.Varuint32(&id)
		w.Bool(&v)
	case uint32:
		id := uint32(2)
		w.Varuint32(&id)
		w.Varuint32(&v)
	case float32:
		id := uint32(3)
		w.Varuint32(&id)
		w.Float32(&v)
	default:
		w.UnknownEnumOption(fmt.Sprintf("%T", v), "game rule type")
	}
}

// EntityMetadata 写入实体元数据
func (w *Writer) EntityMetadata(x *map[uint32]any) {
	l := uint32(len(*x))
	w.Varuint32(&l)

	// 实体元数据需按顺序排序（如 NPC 文本显示需求）
	keys := make([]int, 0, l)
	for k := range *x {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		key := uint32(k)
		value := (*x)[uint32(k)]
		w.Varuint32(&key)
		switch v := value.(type) {
		case byte:
			entityDataTypeByte := EntityDataTypeByte
			w.Varuint32(&entityDataTypeByte)
			w.Uint8(&v)
		case int16:
			entityDataTypeInt16 := EntityDataTypeInt16
			w.Varuint32(&entityDataTypeInt16)
			w.Int16(&v)
		case int32:
			entityDataTypeInt32 := EntityDataTypeInt32
			w.Varuint32(&entityDataTypeInt32)
			w.Varint32(&v)
		case float32:
			entityDataTypeFloat32 := EntityDataTypeFloat32
			w.Varuint32(&entityDataTypeFloat32)
			w.Float32(&v)
		case string:
			entityDataTypeString := EntityDataTypeString
			w.Varuint32(&entityDataTypeString)
			w.String(&v)
		case map[string]any:
			entityDataTypeCompoundTag := EntityDataTypeCompoundTag
			w.Varuint32(&entityDataTypeCompoundTag)
			w.NBT(&v, nbt.NetworkLittleEndian)
		case BlockPos:
			entityDataTypeBlockPos := EntityDataTypeBlockPos
			w.Varuint32(&entityDataTypeBlockPos)
			w.BlockPos(&v)
		case int64:
			entityDataTypeInt64 := EntityDataTypeInt64
			w.Varuint32(&entityDataTypeInt64)
			w.Varint64(&v)
		case mgl32.Vec3:
			entityDataTypeVec3 := EntityDataTypeVec3
			w.Varuint32(&entityDataTypeVec3)
			w.Vec3(&v)
		default:
			w.UnknownEnumOption(reflect.TypeOf(value), "entity metadata")
		}
	}
}

// ItemDescriptorCount 写入物品描述符计数
func (w *Writer) ItemDescriptorCount(i *ItemDescriptorCount) {
	var id byte
	switch i.Descriptor.(type) {
	case *InvalidItemDescriptor:
		id = ItemDescriptorInvalid
	case *DefaultItemDescriptor:
		id = ItemDescriptorDefault
	case *MoLangItemDescriptor:
		id = ItemDescriptorMoLang
	case *ItemTagItemDescriptor:
		id = ItemDescriptorItemTag
	case *DeferredItemDescriptor:
		id = ItemDescriptorDeferred
	case *ComplexAliasItemDescriptor:
		id = ItemDescriptorComplexAlias
	default:
		w.UnknownEnumOption(fmt.Sprintf("%T", i.Descriptor), "item descriptor type")
		return
	}
	w.Uint8(&id)

	i.Descriptor.Marshal(w)
	w.Varint32(&i.Count)
}

// ItemInstance 写入物品实例数据
func (w *Writer) ItemInstance(i *ItemInstance) {
	x := &i.Stack
	w.Varint32(&x.NetworkID)

	// 处理空气物品或未更改的槽位（NetworkID 为 0 或 -1）
	switch x.NetworkID {
	case 0, -1:
		return
	}

	w.Uint16(&x.Count)
	w.Varuint32(&x.MetadataValue)

	hasNetID := i.StackNetworkID != 0
	w.Bool(&hasNetID)

	if hasNetID {
		w.Varint32(&i.StackNetworkID)
	}

	w.Varint32(&x.BlockRuntimeID)

	buf := new(bytes.Buffer)
	bufWriter := NewWriter(buf, w.shieldID)

	var length int16
	if len(x.NBTData) != 0 {
		length = int16(-1)
		version := uint8(1)

		bufWriter.Int16(&length)
		bufWriter.Uint8(&version)
		bufWriter.NBT(&x.NBTData, nbt.LittleEndian)
	} else {
		bufWriter.Int16(&length)
	}

	FuncSliceUint32Length(bufWriter, &x.CanBePlacedOn, bufWriter.StringUTF)
	FuncSliceUint32Length(bufWriter, &x.CanBreak, bufWriter.StringUTF)

	if x.NetworkID == bufWriter.shieldID {
		var blockingTick int64
		bufWriter.Int64(&blockingTick)
	}

	b := buf.Bytes()
	w.ByteSlice(&b)
}

// Item 写入物品栈数据
func (w *Writer) Item(x *ItemStack) {
	w.Varint32(&x.NetworkID)
	if x.NetworkID == 0 {
		// 空气物品，无后续数据
		return
	}

	w.Uint16(&x.Count)
	w.Varuint32(&x.MetadataValue)
	w.Varint32(&x.BlockRuntimeID)

	var extraData []byte
	buf := bytes.NewBuffer(extraData)
	bufWriter := NewWriter(buf, w.shieldID)

	var length int16
	if len(x.NBTData) != 0 {
		length = int16(-1)
		version := uint8(1)

		bufWriter.Int16(&length)
		bufWriter.Uint8(&version)
		bufWriter.NBT(&x.NBTData, nbt.LittleEndian)
	} else {
		bufWriter.Int16(&length)
	}

	FuncSliceUint32Length(bufWriter, &x.CanBePlacedOn, bufWriter.StringUTF)
	FuncSliceUint32Length(bufWriter, &x.CanBreak, bufWriter.StringUTF)

	if x.NetworkID == bufWriter.shieldID {
		var blockingTick int64
		bufWriter.Int64(&blockingTick)
	}

	extraData = buf.Bytes()
	w.ByteSlice(&extraData)
}

// StackRequestAction 写入物品栈请求动作
func (w *Writer) StackRequestAction(x *StackRequestAction) {
	var id byte
	if !lookupStackRequestActionType(*x, &id) {
		w.UnknownEnumOption(fmt.Sprintf("%T", *x), "stack request action type")
	}
	w.Uint8(&id)
	(*x).Marshal(w)
}

// MaterialReducer 写入材料还原器数据
func (w *Writer) MaterialReducer(m *MaterialReducer) {
	mix := (m.InputItem.NetworkID << 16) | int32(m.InputItem.MetadataValue)
	w.Varint32(&mix)
	Slice(w, &m.Outputs)
}

// Recipe 写入配方数据
func (w *Writer) Recipe(x *Recipe) {
	var recipeType int32
	if !lookupRecipeType(*x, &recipeType) {
		w.UnknownEnumOption(fmt.Sprintf("%T", *x), "crafting recipe type")
	}
	w.Varint32(&recipeType)
	(*x).Marshal(w)
}

// EventType 写入事件类型
func (w *Writer) EventType(x *Event) {
	var t int32
	if !lookupEventType(*x, &t) {
		w.UnknownEnumOption(fmt.Sprintf("%T", x), "event packet event type")
	}
	w.Varint32(&t)
}

// TransactionDataType 写入库存交易数据类型
func (w *Writer) TransactionDataType(x *InventoryTransactionData) {
	var id uint32
	if !lookupTransactionDataType(*x, &id) {
		w.UnknownEnumOption(fmt.Sprintf("%T", x), "inventory transaction data type")
	}
	w.Varuint32(&id)
}

// MapPixelsDataType 写入地图像素数据类型（PhoenixBuilder 特有）
func (w *Writer) MapPixelsDataType(x *MapPixelsData) {
	var id uint8
	if !lookupMapPixelsType(*x, &id) {
		w.UnknownEnumOption(fmt.Sprintf("%T", x), "map pixels data type")
	}
	w.Uint8(&id)
}

// AbilityValue 写入能力值数据
func (w *Writer) AbilityValue(x *any) {
	switch val := (*x).(type) {
	case bool:
		valType, defaultVal := uint8(1), float32(0)
		w.Uint8(&valType)
		w.Bool(&val)
		w.Float32(&defaultVal)
	case float32:
		valType, defaultVal := uint8(2), false
		w.Uint8(&valType)
		w.Bool(&defaultVal)
		w.Float32(&val)
	default:
		w.InvalidValue(*x, "ability value type", "must be bool or float32")
	}
}

// CompressedBiomeDefinitions 写入压缩的生物群系定义
// Minecraft 使用字典压缩算法，遇到 0xff 时后续两字节为字典索引
func (w *Writer) CompressedBiomeDefinitions(x *map[string]any) {
	decompressed, err := nbt.Marshal(x)
	if err != nil {
		w.panicf("error marshaling nbt: %v", err)
	}

	var compressed []byte
	buf := bytes.NewBuffer(compressed)
	bufWriter := NewWriter(buf, w.shieldID)

	header := []byte("COMPRESSED")
	bufWriter.Bytes(&header)

	// TODO: 实现完整的字典压缩算法
	var dictionaryLength uint16
	bufWriter.Uint16(&dictionaryLength)
	for _, b := range decompressed {
		bufWriter.Uint8(&b)
		if b == 0xff {
			dictionaryIndex := int16(1)
			bufWriter.Int16(&dictionaryIndex)
		}
	}

	compressed = buf.Bytes()
	length := uint32(len(compressed))
	w.Varuint32(&length)
	w.Bytes(&compressed)
}

// MsgPack 写入网易版特有的 MsgPack 数据
func (w *Writer) MsgPack(x *any) {
	var msgPackBytes []byte
	if err := codec.NewEncoderBytes(&msgPackBytes, &codec.MsgpackHandle{}).Encode(x); err != nil {
		panic(fmt.Sprintf("(w *Writer) MsgPack: %v", err))
	}
	w.ByteSlice(&msgPackBytes)
}

// Varint64 以 1-10 字节写入 int64（Varint 编码）
func (w *Writer) Varint64(x *int64) {
	u := *x
	ux := uint64(u) << 1
	if u < 0 {
		ux = ^ux
	}
	for ux >= 0x80 {
		_ = w.w.WriteByte(byte(ux) | 0x80)
		ux >>= 7
	}
	_ = w.w.WriteByte(byte(ux))
}

// Varuint64 以 1-10 字节写入 uint64（Varint 编码）
func (w *Writer) Varuint64(x *uint64) {
	u := *x
	for u >= 0x80 {
		_ = w.w.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	_ = w.w.WriteByte(byte(u))
}

// Varint32 以 1-5 字节写入 int32（Varint 编码）
func (w *Writer) Varint32(x *int32) {
	u := *x
	ux := uint32(u) << 1
	if u < 0 {
		ux = ^ux
	}
	for ux >= 0x80 {
		_ = w.w.WriteByte(byte(ux) | 0x80)
		ux >>= 7
	}
	_ = w.w.WriteByte(byte(ux))
}

// Varuint32 以 1-5 字节写入 uint32（Varint 编码）
func (w *Writer) Varuint32(x *uint32) {
	u := *x
	for u >= 0x80 {
		_ = w.w.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	_ = w.w.WriteByte(byte(u))
}

// Varint16 以 1-2 字节写入 int16（Varint 编码，PhoenixBuilder 特有）
func (w *Writer) Varint16(x *int16) {
	u := *x
	ux := uint16(u) << 1
	if u < 0 {
		ux = ^ux
	}
	for ux >= 0x80 {
		_ = w.w.WriteByte(byte(ux) | 0x80)
		ux >>= 7
	}
	_ = w.w.WriteByte(byte(ux))
}

// Varuint16 以 1-2 字节写入 uint16（Varint 编码，PhoenixBuilder 特有）
func (w *Writer) Varuint16(x *uint16) {
	u := *x
	for u >= 0x80 {
		_ = w.w.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	_ = w.w.WriteByte(byte(u))
}

// NBT 以指定编码写入 NBT 数据
func (w *Writer) NBT(x *map[string]any, encoding nbt.Encoding) {
	if err := nbt.NewEncoderWithEncoding(w.w, encoding).Encode(*x); err != nil {
		panic(err)
	}
}

// NBTWithLength 写入带长度前缀的 NBT 数据（网易版特有）
func (w *Writer) NBTWithLength(x *map[string]any) {
	var length uint32
	if x == nil || len(*x) == 0 {
		w.Varuint32(&length)
		return
	}

	buffer := bytes.NewBuffer([]byte{})
	if err := nbt.NewEncoderWithEncoding(buffer, nbt.NetworkLittleEndian).Encode(*x); err != nil {
		panic(err)
	}
	nbtBytes := buffer.Bytes()
	length = uint32(len(nbtBytes))

	w.Varuint32(&length)
	w.w.Write(nbtBytes)
}

// NBTList 以指定编码写入 NBT 列表数据
func (w *Writer) NBTList(x *[]any, encoding nbt.Encoding) {
	if err := nbt.NewEncoderWithEncoding(w.w, encoding).Encode(*x); err != nil {
		panic(err)
	}
}

// EnchantList 写入附魔列表（PhoenixBuilder 特有）
func (w *Writer) EnchantList(x *[]Enchant) {
	var bufferLength uint16

	if x == nil || len(*x) == 0 {
		w.Varuint16(&bufferLength)
		return
	}

	buffer := bytes.NewBuffer([]byte{})
	SliceVarint16Length(NewWriter(buffer, 0), x)

	bufferLength = uint16(buffer.Len())
	w.Varuint16(&bufferLength)
	w.w.Write(buffer.Bytes())
}

// ItemList 写入带槽位的物品列表（PhoenixBuilder 特有）
func (w *Writer) ItemList(x *[]ItemWithSlot) {
	var length uint32
	if x == nil || len(*x) == 0 {
		w.Varuint32(&length)
		return
	}

	buffer := bytes.NewBuffer([]byte{})
	SliceOfLen(NewWriter(buffer, 0), uint32(len(*x)), x)

	length = uint32(buffer.Len())
	w.Varuint32(&length)
	w.w.Write(buffer.Bytes())
}

// ShieldID 返回 Writer 的 ShieldID
func (w *Writer) ShieldID() int32 {
	return w.shieldID
}

// UnknownEnumOption 处理未知枚举值（panic）
func (w *Writer) UnknownEnumOption(value any, enum string) {
	w.panicf("unknown value '%v' for enum type '%v'", value, enum)
}

// InvalidValue 处理无效值（panic）
func (w *Writer) InvalidValue(value any, forField, reason string) {
	w.panicf("invalid value '%v' for %v: %v", value, forField, reason)
}

// panicf 格式化 panic
func (w *Writer) panicf(format string, a ...any) {
	panic(fmt.Errorf(format, a...))
}
