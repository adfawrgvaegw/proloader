package base_container_cache

import (
	"bytes"
	"strings"

	"github.com/Yeah114/FunShuttler/core/minecraft/protocol"
	"github.com/Yeah114/FunShuttler/nbt_assigner/block_helper"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

// BaseContainer 描述一个空的容器，
// 并且我们将这样的容器称为“基容器”
type BaseContainer struct {
	BlockName         string
	BlockStatesString string
	CustomeName       string
	ShulkerFacing     uint8
}

// StructureBaseContainer 指示了一个保存在结构中的基容器
type StructureBaseContainer struct {
	UniqueID  uuid.UUID                           // 该容器所在结构的唯一标识符
	Container block_helper.ContainerBlockOpenInfo // 该容器应当如何打开
}

// Hash 给出这个基容器的唯一哈希校验和。
// 校验和不包括该容器所在结构的唯一标识，
// 这意味着来自两个不同结构的相同基容器
// 具有完全相同的哈希校验和
func (b BaseContainer) Hash() uint64 {
	buf := bytes.NewBuffer(nil)
	w := protocol.NewWriter(buf, 0)

	name := strings.ToLower(b.BlockName)
	if !strings.HasPrefix(name, "minecraft:") {
		name = "minecraft:" + name
	}

	w.String(&name)
	w.String(&b.BlockStatesString)
	w.String(&b.CustomeName)
	w.Uint8(&b.ShulkerFacing)

	return xxhash.Sum64(buf.Bytes())
}
