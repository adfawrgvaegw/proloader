package utils

import (
	"github.com/Yeah114/WaterStructure/utils/nbt"
)

// DeepCopyNBT 深拷贝 src 所指示的 NBT 数据，
// 并返回深拷贝产物 dst
func DeepCopyNBT(src map[string]any) (dst map[string]any) {
	srcBytes, _ := nbt.MarshalEncoding(src, nbt.LittleEndian)
	nbt.UnmarshalEncoding(srcBytes, &dst, nbt.LittleEndian)
	if dst == nil {
		dst = make(map[string]any)
	}
	return
}
