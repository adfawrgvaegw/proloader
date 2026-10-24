package protocol

const (
	ContainerAnvilInput = iota // 0: 铁砧的输入槽
	ContainerAnvilMaterial // 1: 铁砧的材料槽
	ContainerAnvilResultPreview // 2: 铁砧的结果预览槽
	ContainerSmithingTableInput // 3: 锻造台的输入槽
	ContainerSmithingTableMaterial // 4: 锻造台的材料槽
	ContainerSmithingTableResultPreview // 5: 锻造台的结果预览槽
	ContainerArmor // 6: 装备槽（盔甲）
	ContainerLevelEntity // 7: 等级实体容器
	ContainerBeaconPayment // 8: 信标的支付槽
	ContainerBrewingStandInput // 9: 酿造台的输入槽
	ContainerBrewingStandResult // 10: 酿造台的结果槽
	ContainerBrewingStandFuel // 11: 酿造台的燃料槽
	ContainerCombinedHotBarAndInventory // 12: 合并的快捷栏和 inventory
	ContainerCraftingInput // 13:  crafting（合成）的输入槽
	ContainerCraftingOutputPreview // 14:  crafting（合成）的结果预览槽
	ContainerRecipeConstruction // 15: 建筑类配方
	ContainerRecipeNature // 16: 自然类配方
	ContainerRecipeCustom // 17: 自定义配方
	ContainerRecipeItems // 18: 物品类配方
	ContainerRecipeSearch // 19: 配方搜索
	ContainerRecipeSearchBar // 20: 配方搜索栏
	ContainerRecipeEquipment // 21: 装备类配方
	ContainerRecipeBook // 22: 配方书
	ContainerEnchantingInput // 23: 附魔台的输入槽
	ContainerEnchantingMaterial // 24: 附魔台的材料槽
	ContainerFurnaceFuel // 25: 熔炉的燃料槽
	ContainerFurnaceIngredient // 26: 熔炉的原料槽
	ContainerFurnaceResult // 27: 熔炉的结果槽
	ContainerHorseEquip // 28: 马的装备槽
	ContainerHotBar // 29: 快捷栏
	ContainerInventory // 30: 背包
	ContainerShulkerBox // 31: 潜影盒
	ContainerTradeIngredientOne // 32: 交易的第一个原料槽
	ContainerTradeIngredientTwo // 33: 交易的第二个原料槽
	ContainerTradeResultPreview // 34: 交易的结果预览槽
	ContainerOffhand // 35: 副手槽
	ContainerCompoundCreatorInput // 36: 化合物制造机的输入槽
	ContainerCompoundCreatorOutputPreview // 37: 化合物制造机的结果预览槽
	ContainerElementConstructorOutputPreview // 38: 元素构造器的结果预览槽
	ContainerMaterialReducerInput // 39: 材料还原器的输入槽
	ContainerMaterialReducerOutput // 40: 材料还原器的输出槽
	ContainerLabTableInput // 41: 实验室工作台的输入槽
	ContainerLoomInput // 42: 织布机的输入槽
	ContainerLoomDye // 43: 织布机的染料槽
	ContainerLoomMaterial // 44: 织布机的材料槽
	ContainerLoomResultPreview // 45: 织布机的结果预览槽
	ContainerBlastFurnaceIngredient // 46: 高炉的原料槽
	ContainerSmokerIngredient // 47: 烟熏炉的原料槽
	ContainerTradeTwoIngredientOne // 48: 第二个交易的第一个原料槽
	ContainerTradeTwoIngredientTwo // 49: 第二个交易的第二个原料槽
	ContainerTradeTwoResultPreview // 50: 第二个交易的结果预览槽
	ContainerGrindstoneInput // 51: 砂轮的输入槽
	ContainerGrindstoneAdditional // 52: 砂轮的附加槽
	ContainerGrindstoneResultPreview // 53: 砂轮的结果预览槽
	ContainerStonecutterInput // 54: 切石机的输入槽
	ContainerStonecutterResultPreview // 55: 切石机的结果预览槽
	ContainerCartographyInput // 56: 制图台的输入槽
	ContainerCartographyAdditional // 57: 制图台的附加槽
	ContainerCartographyResultPreview // 58: 制图台的结果预览槽
	ContainerBarrel // 59: 木桶
	ContainerCursor // 60: 光标（鼠标选中的物品）
	ContainerCreatedOutput // 61: 已创建的输出槽
	ContainerSmithingTableTemplate // 62: 锻造台的模板槽
	ContainerCrafterLevelEntity // 63: 工匠等级实体容器
)

const (
	ContainerTypeInventory = iota - 1 // -1: 背包类型
	ContainerTypeContainer // 0: 容器类型
	ContainerTypeWorkbench // 1: 工作台类型
	ContainerTypeFurnace // 2: 熔炉类型
	ContainerTypeEnchantment // 3: 附魔台类型
	ContainerTypeBrewingStand // 4: 酿造台类型
	ContainerTypeAnvil // 5: 铁砧类型
	ContainerTypeDispenser // 6: 发射器类型
	ContainerTypeDropper // 7: 投掷器类型
	ContainerTypeHopper // 8: 漏斗类型
	ContainerTypeCauldron // 9: 炼药锅类型
	ContainerTypeCartChest // 10: 箱子矿车类型
	ContainerTypeCartHopper // 11: 漏斗矿车类型
	ContainerTypeHorse // 12: 马（容器）类型
	ContainerTypeBeacon // 13: 信标类型
	ContainerTypeStructureEditor // 14: 结构编辑器类型
	ContainerTypeTrade // 15: 交易类型
	ContainerTypeCommandBlock // 16: 命令方块类型
	ContainerTypeJukebox // 17: 唱片机类型
	ContainerTypeArmour // 18: 盔甲（容器）类型
	ContainerTypeHand // 19: 手持（容器）类型
	ContainerTypeCompoundCreator // 20: 化合物制造机类型
	ContainerTypeElementConstructor // 21: 元素构造器类型
	ContainerTypeMaterialReducer // 22: 材料还原器类型
	ContainerTypeLabTable // 23: 实验室工作台类型
	ContainerTypeLoom // 24: 织布机类型
	ContainerTypeLectern // 25: 讲台类型
	ContainerTypeGrindstone // 26: 砂轮类型
	ContainerTypeBlastFurnace // 27: 高炉类型
	ContainerTypeSmoker // 28: 烟熏炉类型
	ContainerTypeStonecutter // 29: 切石机类型
	ContainerTypeCartography // 30: 制图台类型
	ContainerTypeHUD // 31: HUD（ Heads-Up Display，平视显示）类型
	ContainerTypeJigsawEditor // 32: 拼图编辑器类型
	ContainerTypeSmithingTable // 33: 锻造台类型
	ContainerTypeChestBoat // 34: 箱子船类型
	ContainerTypeDecoratedPot // 35: 装饰罐类型
	ContainerTypeCrafter // 36: 工匠容器类型
)
