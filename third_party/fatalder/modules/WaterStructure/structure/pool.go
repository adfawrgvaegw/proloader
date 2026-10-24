package structure

const (
	// Java 的建筑格式
	// .schematic
	IDSchematic uint8 = iota

	// Java 的建筑格式
	// .schem
	IDSchemV1
	
	// V2 相较于 V1，仅仅是修改了几个标签的位置
	IDSchemV2
	
	// Java 的投影文件
	// .litematic
	IDLitematic

	// 基岩版的结构文件
	// .mcstructure
	IDMCStructure

	// 基岩版的存档文件
	// .mcworld
	IDMCWorld

	// PhoenixBuilder 的建筑格式
	// .bdx
	IDBDX

	// 疑似 MOJANG 废弃的建筑格式
	// 目前该文件的来源有争议
	// 可能是 Amulet 的自创格式
	// 因为如果该文件真被遗弃
	// Amulet 导出版本为什么有高版本呢？
	// .construction
	IDConstruction

	// Axiom 的投影格式
	// .bp
	IDAxiomBP

	// no standard
	// they are not Water

	// BuildTool 的产出
	// .mcfunction, .txt
	IDMCFunction

	// 万花筒的建筑格式
	// .kbdx
	IDKBDX

	// InfiniteBot 的建筑格式
	// .ibi
	// 其本质为 MCFunction
	IDIBImport

	// 绵羊 JSON
	// .json
	IDMianYangV1

	// .json
	IDMianYangV2
	
	// .building
	// V3 是 V2 的压缩版本
	IDMianYangV3
	
	// .buildingX
	// 解析未完善
	// V4 是 V2 的二进制压缩版本
	IDMianYangV4

	// 钢板 JSON
	// .json
	IDGangBanV1
	IDGangBanV2
	IDGangBanV3
	IDGangBanV4
	IDGangBanV5
	IDGangBanV6
	// .reb
	// V7 是 V6 的压缩版本
	IDGangBanV7

	// 跑路 JSON
	// .json
	IDRunAway

	// 情绪 JSON
	// .json
	IDQingXuV1

	// TimeBuilder JSON
	// .json
	IDTimeBuilderV1

	// 浮鸿 JSON
	// .json
	IDFuHongV1
	IDFuHongV2
	IDFuHongV3
	IDFuHongV4
	// .fhbuild
	// V5 是 V4 的压缩加密版本
	IDFuHongV5

	// Consensus 的建筑格式
	// 使用 MsgPack 编码
	// .bds
	IDBDS

	// 虹欠 的 InfiniteBot 分支建筑格式
	// .sibi
	IDSIBI

	// bxy 的建筑格式
	// .bcf
	IDBCF

	// InfiniteBot 的建筑格式
	// 目前疑似废稿
	// .tibi
	IDTIBI

	// Eggylan 的 建筑格式，MCStructure 的 JSON 版
	// .covstructure
	IDCovStructure

	// Nexus 的建筑格式
	// 使用 MsgPack 编码
	// .np
	IDNexusNP
)

var NameSchematic = "Schematic"
var NameSchemV1 = "SchemV1"
var NameSchemV2 = "SchemV2"
var NameLitematic = "Litematic"
var NameMCStructure = "MCStructure"
var NameMCWorld = "MCWorld"
var NameBDX = "BDX"
var NameConstruction = "Construction"
var NameAxiomBP = "AxiomBP"
var NameMCFunction = "MCFunction"
var NameKBDX = "KBDX"
var NameIBImport = "IBImport"
var NameMianYangV1 = "MianYangV1"
var NameMianYangV2 = "MianYangV2"
var NameMianYangV3 = "MianYangV3"
var NameMianYangV4 = "MianYangV4"
var NameGangBanV1 = "GangBanV1"
var NameGangBanV2 = "GangBanV2"
var NameGangBanV3 = "GangBanV3"
var NameGangBanV4 = "GangBanV4"
var NameGangBanV5 = "GangBanV5"
var NameGangBanV6 = "GangBanV6"
var NameGangBanV7 = "GangBanV7"
var NameRunAway = "RunAway"
var NameQingXuV1 = "QingXuV1"
var NameTimeBuilderV1 = "TimeBuilderV1"
var NameFuHongV1 = "FuHongV1"
var NameFuHongV2 = "FuHongV2"
var NameFuHongV3 = "FuHongV3"
var NameFuHongV4 = "FuHongV4"
var NameFuHongV5 = "FuHongV5"
var NameBDS = "BDS"
var NameSIBI = "SIBI"
var NameBCF = "BCF"
var NameTIBI = "TIBI"
var NameCovStructure = "CovStructure"
var NameNexusNP = "NexusNP"

type StructureFunc func() Structure

var StructureIDPool = map[uint8]StructureFunc{
	IDSchematic:     func() Structure { return &Schematic{} },
	IDSchemV1:       func() Structure { return &SchemV1{} },
	IDSchemV2:       func() Structure { return &SchemV2{} },
	IDLitematic:     func() Structure { return &Litematic{} },
	IDMCStructure:   func() Structure { return &MCStructure{} },
	IDMCWorld:       func() Structure { return &MCWorld{} },
	IDBDX:           func() Structure { return &BDX{} },
	IDConstruction:  func() Structure { return &Construction{} },
	IDAxiomBP:       func() Structure { return &AxiomBP{} },
	IDMCFunction:    func() Structure { return &MCFunction{} },
	IDKBDX:          func() Structure { return &KBDX{} },
	IDIBImport:      func() Structure { return &IBImport{} },
	IDMianYangV1:    func() Structure { return &MianYangV1{} },
	IDMianYangV2:    func() Structure { return &MianYangV2{} },
	IDMianYangV3:    func() Structure { return &MianYangV3{} },
	IDMianYangV4:    func() Structure { return &MianYangV4{} },
	IDGangBanV1:     func() Structure { return &GangBanV1{} },
	IDGangBanV2:     func() Structure { return &GangBanV2{} },
	IDGangBanV3:     func() Structure { return &GangBanV3{} },
	IDGangBanV4:     func() Structure { return &GangBanV4{} },
	IDGangBanV5:     func() Structure { return &GangBanV5{} },
	IDGangBanV6:     func() Structure { return &GangBanV6{} },
	IDGangBanV7:     func() Structure { return &GangBanV7{} },
	IDRunAway:       func() Structure { return &RunAway{} },
	IDQingXuV1:      func() Structure { return &QingXuV1{} },
	IDTimeBuilderV1: func() Structure { return &TimeBuilderV1{} },
	IDFuHongV1:      func() Structure { return &FuHongV1{} },
	IDFuHongV2:      func() Structure { return &FuHongV2{} },
	IDFuHongV3:      func() Structure { return &FuHongV3{} },
	IDFuHongV4:      func() Structure { return &FuHongV4{} },
	IDFuHongV5:      func() Structure { return &FuHongV5{} },
	IDBDS:           func() Structure { return &BDS{} },
	IDSIBI:          func() Structure { return &SIBI{} },
	IDBCF:           func() Structure { return &BCF{} },
	IDTIBI:          func() Structure { return &TIBI{} },
	IDCovStructure:  func() Structure { return &CovStructure{} },
	IDNexusNP:       func() Structure { return &NexusNP{} },
}

var StructureNamePool = map[string]StructureFunc{}

func init() {
	for _, structureFunc := range StructureIDPool {
		name := structureFunc().Name()
		StructureNamePool[name] = structureFunc
	}
}
