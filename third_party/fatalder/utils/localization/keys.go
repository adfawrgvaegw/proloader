package l10n

// 翻译键常量定义
// 这些常量提供代码自动补全，避免硬编码字符串，方便管理所有翻译文本

// 语言包元信息
const (
	KeyPackName = "pack.name" // 语言包显示名称
)

// 应用程序信息
const (
	KeyAppName        = "app.name"        // 应用程序名称
	KeyAppDescription = "app.description" // 应用程序描述
	KeyAppUsage       = "app.usage"       // 应用程序用法说明
)

// 命令行参数相关
const (
	// 存储路径参数
	KeyArgsStoragePathName        = "args.storage_path.name"        // 存储路径参数名
	KeyArgsStoragePathUsage       = "args.storage_path.usage"       // 存储路径参数用法
	KeyArgsStoragePathDescription = "args.storage_path.description" // 存储路径参数详细描述

	// 语言参数
	KeyArgsLanguageName  = "args.language.name"  // 语言参数名
	KeyArgsLanguageUsage = "args.language.usage" // 语言参数用法

	// 帮助参数
	KeyArgsHelpName  = "args.help.name"  // 帮助参数名
	KeyArgsHelpUsage = "args.help.usage" // 帮助参数用法

	// 版本参数
	KeyArgsVersionName  = "args.version.name"  // 版本参数名
	KeyArgsVersionUsage = "args.version.usage" // 版本参数用法
)

// 通用术语
const (
	KeyCommonOptions  = "common.options"  // "选项"
	KeyCommonExamples = "common.examples" // "示例"
	KeyCommonError    = "common.error"    // "错误"
)

// 版本信息
const (
	KeyVersionLicense     = "version.license"      // "许可证"
	KeyVersionAuthors     = "version.authors"      // "作者"
	KeyVersionCommit      = "version.commit"       // "提交"
	KeyVersionCommitNum   = "version.commit_num"   // "提交次数"
	KeyVersionBuildDate   = "version.build_date"   // "构建日期"
	KeyVersionStoragePath = "version.storage_path" // "存储路径"
	KeyVersionLanguage    = "version.language"     // "界面语言"
)

// 使用示例
const (
	KeyExamplesDefault     = "examples.default"      // 使用默认设置
	KeyExamplesStoragePath = "examples.storage_path" // 使用自定义存储目录
	KeyExamplesLanguage    = "examples.language"     // 设置语言
	KeyExamplesVersion     = "examples.version"      // 显示版本信息
	KeyExamplesHelp        = "examples.help"         // 显示帮助信息
)

// UI 界面文本
const (
	KeyUIInputValidInteger = "ui.input.valid_integer" // 请输入有效的整数
	KeyUIInputValidNumber  = "ui.input.valid_number"  // 请输入有效的数字
	KeyUISelectPrompt      = "ui.select.prompt"       // 请选择
	KeyUISelectRange       = "ui.select.range"        // 请输入 1 到 %d 之间的数字
	KeyUIMultiInputEnd     = "ui.multi_input.end"     // 输入空行结束
	KeyUIConfirmPrompt     = "ui.confirm.prompt"      // (y/n)
)

// 主菜单
const (
	KeyMenuMain = "menu.main" // 主菜单

	KeyMenuStartExport  = "menu.start_export"  // 启动导出
	KeyMenuStartBuild   = "menu.start_build"   // 启动构建
	KeyMenuConfigTasks  = "menu.config_tasks"  // 配置任务
	KeyMenuManageConfig = "menu.manage_config" // 管理配置
	KeyMenuBuildingOps  = "menu.building_ops"  // 建筑操作
	KeyMenuExit         = "menu.exit"          // 退出程序
)

// 建筑操作（WaterStructure 工具集成）
const (
	KeyBuildingOpsTitle                     = "building_ops.title"                        // 建筑操作
	KeyBuildingOpsConvertMCWorldSlow        = "building_ops.convert_mcworld_slow"         // 慢速转换 MCWorld
	KeyBuildingOpsConvertMCWorldFast        = "building_ops.convert_mcworld_fast"         // 快速转换 MCWorld
	KeyBuildingOpsAnyToAny                  = "building_ops.any_to_any"                   // 任意结构互转
	KeyBuildingOpsNeteaseEncrypt            = "building_ops.netease_encrypt"              // 网易地图加密
	KeyBuildingOpsNeteaseDecrypt            = "building_ops.netease_decrypt"              // 网易地图解密
	KeyBuildingOpsMapArt                    = "building_ops.map_art"                      // 转换地图画
	KeyBuildingOpsPromptStructurePath       = "building_ops.prompt.structure_path"        // 请输入结构文件路径
	KeyBuildingOpsPromptOutputMCWorldPath   = "building_ops.prompt.output_mcworld"        // 请输入输出 MCWorld 路径
	KeyBuildingOpsDetectedStructureType     = "building_ops.detected_structure_type"      // 检测到结构类型
	KeyBuildingOpsSelectTargetFormat        = "building_ops.select_target_format"         // 选择目标结构格式
	KeyBuildingOpsPromptOutputStructurePath = "building_ops.prompt.output_structure"      // 请输入输出结构路径
	KeyBuildingOpsAnyToAnyDone              = "building_ops.any_to_any_done"              // 互转完成
	KeyBuildingOpsPromptWorldOrDBPath       = "building_ops.prompt.world_or_db"           // 请输入世界/DB路径
	KeyBuildingOpsPromptImagePath           = "building_ops.prompt.image_path"            // 请输入图片路径
	KeyBuildingOpsConfirmModifyDir          = "building_ops.confirm_modify_dir"           // 确认修改目录
	KeyBuildingOpsNeteaseDone               = "building_ops.netease_done"                 // 加解密完成
	KeyBuildingOpsProgressWriteWorld        = "building_ops.progress.write_world"         // 写入世界
	KeyBuildingOpsProgressExportStructure   = "building_ops.progress.export_structure"    // 导出结构
	KeyBuildingOpsPromptStartSubChunkX      = "building_ops.prompt.start_subchunk_x"      // 起始子区块X
	KeyBuildingOpsPromptStartSubChunkY      = "building_ops.prompt.start_subchunk_y"      // 起始子区块Y
	KeyBuildingOpsPromptStartSubChunkZ      = "building_ops.prompt.start_subchunk_z"      // 起始子区块Z
	KeyBuildingOpsPromptMapWidth            = "building_ops.prompt.map_width"             // 地图宽
	KeyBuildingOpsPromptMapHeight           = "building_ops.prompt.map_height"            // 地图高
	KeyBuildingOpsPromptMapForce2D          = "building_ops.prompt.map_force_2d"          // 强制2D
	KeyBuildingOpsPromptMapDisableReference = "building_ops.prompt.map_disable_reference" // 禁用参考列
	KeyBuildingOpsPromptMapMax3DHeight      = "building_ops.prompt.map_max_3d_height"     // 3D高度限制
	KeyBuildingOpsMapArtNeedsWorldDir       = "building_ops.map_art_needs_world_dir"      // 地图画需要世界目录
	KeyBuildingOpsMapArtRange               = "building_ops.map_art_range"                // 写入范围
	KeyBuildingOpsMapArtDone                = "building_ops.map_art_done"                 // 地图画完成
)

// 导入导出功能
const (
	KeyExportTitle                = "export.title"                  // 导出功能
	KeyExportTaskPreparingIndexed = "export.task_preparing_indexed" // [%d/%d] 导出文件: %s
	KeyExportTaskStartSummary     = "export.task_start_summary"     // 开始导出任务 (%d/%d)，区域尺寸：宽=%d，高=%d，长=%d
	KeyExportEstimatedTotalChunks = "export.estimated_total_chunks" // 预计总区块数：%d
	KeyExportProgressBarDesc      = "export.progress_bar_desc"      // 导出区块
	KeyExportSubChunkDecodeFailed = "export.subchunk_decode_failed" // 解析子区块NBT失败
	KeyExportSubChunkSaveFailed   = "export.subchunk_save_failed"   // 保存子区块失败
	KeyExportChunkSaveFailed      = "export.chunk_save_failed"      // 保存区块NBT失败
	KeyExportFinishedPath         = "export.finished_path"          // 导出完成，文件保存在：%s
	KeyExportTaskRunning          = "export.task_running"           // 正在执行导出任务...
	KeyExportTaskFailed           = "export.task_failed"            // 导出任务失败: %w
	KeyExportTaskCompleted        = "export.task_completed"         // 导出任务完成
	KeyExportTaskExecuteFailed    = "export.task_execute_failed"    // 导出任务执行失败: %v
	KeyServerConnectFailed        = "server.connect_failed"         // 连接服务器失败: %v
	KeyBuildInterruptRemoveWarn   = "build.interrupt_remove_warn"   // 警告: 删除中断文件失败: %v
	KeyBuildTitle                 = "build.title"                   // 构建功能
	KeyServerConfigSelect         = "server.config.select"          // 选择进服配置
	KeyServerConfigUseLastTitle   = "server.config.use_last"        // 使用上次配置
	KeyServerConfigUseLast        = "server.config.use_last_desc"   // 是否使用上次的进服配置
	KeyServerConfigCreateNew      = "server.config.create_new"      // 创建新的配置
	KeyServerConfigSelectExist    = "server.config.select_exist"    // 选择已有配置
	KeyServerConfigNoFiles        = "server.config.no_files"        // 没有找到配置文件
)

// 服务器配置
const (
	KeyServerNumber      = "server.number"       // 服务器号
	KeyServerPassword    = "server.password"     // 服务器密码
	KeyServerAddress     = "server.address"      // 验证服务器地址
	KeyServerToken       = "server.token"        // 验证服务器Token
	KeyServerSaveConfig  = "server.save_config"  // 是否保存配置
	KeyServerFileName    = "server.file_name"    // 配置文件名
	KeyServerConfigSaved = "server.config_saved" // 配置已保存
)

// 任务配置
const (
	KeyTaskStartCoord           = "task.start_coord"             // 起始坐标
	KeyTaskEndCoord             = "task.end_coord"               // 终止坐标
	KeyTaskDimensionName        = "task.dimension_name"          // 请输入维度名（默认 overworld）
	KeyTaskSpeed                = "task.speed"                   // 速度
	KeyTaskChunkGroupSide       = "task.chunk_group_side"        // 每组构建的区块边长
	KeyTaskClearBlocks          = "task.clear_blocks"            // 自动清除原方块
	KeyTaskClearDrops           = "task.clear_drops"             // 清除掉落物
	KeyTaskVerifyInterval       = "task.verify_interval"         // 验证间隔
	KeyTaskVerifyLevel          = "task.verify_level"            // 验证等级
	KeyTaskProgress             = "task.progress"                // 起始进度
	KeyTaskSaveGroup            = "task.save_group"              // 保存任务组
	KeyTaskExecuteNow           = "task.execute_now"             // 立即执行任务
	KeyTaskExecuteComplete      = "task.execute_complete"        // 任务执行完成
	KeyServerConnecting         = "server.connecting"            // 正在连接服务器
	KeyServerConnected          = "server.connected"             // 已连接到服务器
	KeyTaskExecuting            = "task.executing"               // 正在执行任务
	KeyTaskGroupName            = "task.group_name"              // 任务组名称
	KeyTaskAutoDisableCommand   = "task.auto_disable_command"    // 自动关闭命令方块启用
	KeyTaskAutoPlaceDenyBlock   = "task.auto_place_deny_block"   // 自动铺设 deny 方块（底部铺 deny 并抬高）
	KeyTaskAutoPlaceBorderBlock = "task.auto_place_border_block" // 自动铺设 border_block 边框（外围描边，长宽+2）
	KeyTaskEnableDeltaMode      = "task.enable_delta_mode"       // 是否启用增量导入
	KeyTaskAutoUpgradeCommand   = "task.auto_upgrade_command"    // 自动升级命令方块命令
	KeyTaskIgnoreCommandBlock   = "task.ignore_command_block"    // 忽略命令方块
	KeyTaskIgnoreOtherNBTBlock  = "task.ignore_other_nbt_block"  // 忽略其他NBT方块
	KeyTaskSetConsolePos        = "task.set_console_pos"         // 设置nbt制作位置
	KeyTaskConsoleCoordinates   = "task.console_coordinates"     // nbt制作位置
	KeyTaskShowNBTProgress      = "task.show_nbt_progress"       // 显示NBT导入进度
	KeyTaskShowProgressInGame   = "task.show_progress_in_game"   // 在游戏内显示进度条（actionbar）
	KeyTaskNBTImportProgress    = "task.nbt_import_progress"     // NBT导入进度消息
	KeyTaskWaitChunkLoad        = "task.wait_chunk_load"         // 是否等待区块加载
	KeyTaskConvertToMCWorld     = "task.convert_to_mcworld"      // 是否在导入前转换为 MCWorld
	KeyTaskAutoEnterFixMode     = "task.auto_enter_fix_mode"     // 构建完成后自动进入修补模式
	KeyTaskEnterFixModeDirectly = "task.enter_fix_mode_directly" // 跳过构建直接进入修补模式
)

// 导出任务配置
const (
	KeyExportModePrompt     = "export.mode.prompt"      // 选择导出任务配置方式
	KeyExportAddAnotherTask = "export.mode.add_another" // 继续添加导出任务
	KeyExportFilePrompt     = "export.file.prompt"      // 导出文件路径
)

// 构建任务配置
const (
	KeyBuildModePrompt     = "build.mode.prompt"      // 选择构建任务配置方式
	KeyBuildAddAnotherTask = "build.mode.add_another" // 继续添加构建任务
	KeyBuildAppendNewTask  = "build.mode.append_new"  // 额外配置新的构建任务
	KeyBuildFilePrompt     = "build.file.prompt"      // 选择要构建的文件
)

// 验证等级
const (
	KeyVerifyNone     = "verify.none"     // 不进行验证
	KeyVerifyAir      = "verify.air"      // 只验证区块是否有可疑空气
	KeyVerifySimilar  = "verify.similar"  // 对比区块数据是否相似
	KeyVerifyComplete = "verify.complete" // 完整对比区块数据
)

// 文件操作
const (
	KeyFileNotFound    = "file.not_found"    // 文件不存在
	KeyFileCustomPath  = "file.custom_path"  // 自定义文件路径
	KeyFileEnterPath   = "file.enter_path"   // 请输入文件路径
	KeyFileInvalidPath = "file.invalid_path" // 无效的文件路径
)

// 通用操作
const (
	KeyOperationSuccess = "operation.success" // 操作成功
	KeyOperationFailed  = "operation.failed"  // 操作失败
	KeyOperationCancel  = "operation.cancel"  // 操作取消
	KeyOperationAborted = "operation.aborted" // 用户中止
	KeyBackToMenu       = "back_to_menu"      // 返回菜单
	KeyPressEnter       = "press_enter"       // 按回车继续
)

// 构建进度和中断
const (
	KeyBuildProgressTitle     = "build.progress_title"     // 构建进度
	KeyBuildProgressCurrent   = "build.progress_current"   // 当前进度: %d/%d (%.1f%%)
	KeyBuildProgressSpeed     = "build.progress_speed"     // 构建速度: %d 方块/秒
	KeyBuildInterruptFound    = "build.interrupt_found"    // 发现中断文件
	KeyBuildInterruptResume   = "build.interrupt_resume"   // 是否从中断点继续构建？
	KeyBuildInterruptNew      = "build.interrupt_new"      // 开始新的构建
	KeyBuildInterruptDelete   = "build.interrupt_delete"   // 删除中断文件
	KeyBuildInterruptSaved    = "build.interrupt_saved"    // 中断信息已保存
	KeyBuildInterruptRestored = "build.interrupt_restored" // 已从中断点恢复
	KeyBuildInterruptDeleted  = "build.interrupt_deleted"  // 中断文件已删除
	KeyBuildInterruptFile     = "build.interrupt_file"     // 中断文件: %s
	KeyBuildInterruptProgress = "build.interrupt_progress" // 中断进度: %d/%d (%.1f%%)

	// 构建任务详细反馈
	KeyBuildTaskStart                = "build.task_start"                  // 开始构建任务
	KeyBuildStructureType            = "build.structure_type"              // 结构类型
	KeyBuildParsingFile              = "build.parsing_file"                // 正在解析结构文件
	KeyBuildFileParsed               = "build.file_parsed"                 // 结构文件解析完成
	KeyBuildStructureSize            = "build.structure_size"              // 结构尺寸
	KeyBuildWidth                    = "build.width"                       // 宽
	KeyBuildHeight                   = "build.height"                      // 高
	KeyBuildLength                   = "build.length"                      // 长
	KeyBuildTotalChunks              = "build.total_chunks"                // 总区块数
	KeyBuildStartImport              = "build.start_import"                // 开始进行导入
	KeyBuildProgressBarDesc          = "build.progress_bar_desc"           // 区块构建
	KeyBuildAutoFixingChunk          = "build.auto_fixing_chunk"           // 自动修补区块
	KeyBuildEnterFixMode             = "build.enter_fix_mode"              // 进入修补模式
	KeyBuildFixModeInstructions      = "build.fix_mode_instructions"       // 修补模式操作提示
	KeyBuildExitFixMode              = "build.exit_fix_mode"               // 退出修补模式
	KeyBuildFixChunkStart            = "build.fix_chunk_start"             // 开始修补区块
	KeyBuildFixChunkEnd              = "build.fix_chunk_end"               // 结束修补区块
	KeyBuildTaskPreparingIndexed     = "build.task_preparing_indexed"      // [%d/%d] 准备执行任务: %s
	KeyBuildControlHint              = "build.control_hint"                // 控制: p=暂停  r=继续  s=停止  n=跳过  q=退出
	KeyBuildTaskFailed               = "build.task_failed"                 // 任务执行失败: %w
	KeyBuildTaskCompleted            = "build.task_completed"              // 任务完成
	KeyBuildPauseFailed              = "build.pause_failed"                // 暂停失败: %v
	KeyBuildPaused                   = "build.paused"                      // 已暂停
	KeyBuildContinueFailed           = "build.continue_failed"             // 继续失败: %v
	KeyBuildResumed                  = "build.resumed"                     // 已继续
	KeyBuildStopSuccess              = "build.stop_success"                // 已停止当前任务
	KeyBuildSkipSuccess              = "build.skip_success"                // 跳过当前任务
	KeyBuildUnknownCommand           = "build.unknown_command"             // 未知指令，支持: p r s n q
	KeyBuildChunkFixExceeded         = "build.chunk_fix_exceeded"          // 区块 (%d, %d) 修补 %d 次后仍然失败，已跳过
	KeyBuildInterruptCleaned         = "build.interrupt_cleaned"           // 中断文件已自动清理: %s
	KeyBuildInterruptMigrated        = "build.interrupt_migrated"          // 中断文件已迁移到新位置: %s
	KeyBuildResumeTitle              = "build.resume_title"                // 从中断点恢复构建
	KeyBuildResumeInfo               = "build.resume_info"                 // 恢复任务信息:
	KeyBuildResumeFile               = "build.resume_file"                 //   文件: %s
	KeyBuildResumeStartPos           = "build.resume_start_pos"            //   起始位置: (%d, %d, %d)
	KeyBuildResumeSpeed              = "build.resume_speed"                //   速度: %d
	KeyBuildResumeInProgress         = "build.resume_in_progress"          // 恢复构建中...
	KeyBuildResumeExecuteFailed      = "build.resume_execute_failed"       // 恢复构建失败: %v
	KeyBuildResumeRestoreWarning     = "build.resume_restore_warning"      // 警告: 恢复中断状态失败: %v
	KeyBuildResumeCancelled          = "build.resume_cancelled"            // 用户取消了恢复操作
	KeyBuildConvertToMCWorldStart    = "build.convert_to_mcworld_start"    // 正在转换结构文件为 MCWorld...
	KeyBuildConvertToMCWorldExists   = "build.convert_to_mcworld_exists"   // 检测到已存在 MCWorld 文件: %s
	KeyBuildConvertToMCWorldSuccess  = "build.convert_to_mcworld_success"  // 转换完成: %s
	KeyBuildConvertToMCWorldFailed   = "build.convert_to_mcworld_failed"   // 转换结构文件失败: %v
	KeyBuildConvertToMCWorldProgress = "build.convert_to_mcworld_progress" // MCWorld 转换进度
)

// 配置管理
const (
	KeyConfigManageServer    = "config.manage_server"    // 管理进服配置
	KeyConfigManageFramework = "config.manage_framework" // 管理框架配置
	KeyConfigCreateNew       = "config.create_new"       // 创建新配置
	KeyConfigSelectExisting  = "config.select_existing"  // 选择已有配置
	KeyConfigNoFiles         = "config.no_files"         // 没有找到配置文件
	KeyConfigSaved           = "config.saved"            // 配置已保存
	KeyConfigRenamed         = "config.renamed"          // 配置已重命名
	KeyConfigDeleted         = "config.deleted"          // 配置已删除
)

// 任务管理
const (
	KeyTaskGroupCreate  = "task_group.create"   // 创建新任务组
	KeyTaskGroupManage  = "task_group.manage"   // 管理任务组
	KeyTaskGroupNoFiles = "task_group.no_files" // 没有找到任务组文件
	KeyTaskGroupRenamed = "task_group.renamed"  // 任务组已重命名
	KeyTaskGroupDeleted = "task_group.deleted"  // 任务组已删除

	KeyTaskGroupCreateExport      = "task_group.create_export"       // 创建导出任务组
	KeyTaskGroupCreateBuild       = "task_group.create_build"        // 创建构建任务组
	KeyTaskGroupCreateMixed       = "task_group.create_mixed"        // 创建混合任务组
	KeyTaskGroupSelectType        = "task_group.select_type"         // 选择任务组类型
	KeyTaskGroupEmpty             = "task_group.empty"               // 任务组为空
	KeyTaskGroupRunOption         = "task_group.run_option"          // 运行任务组
	KeyTaskGroupRunTitle          = "task_group.run_title"           // 运行任务组: %s
	KeyTaskGroupSkipInvalid       = "task_group.skip_invalid"        // 跳过无效任务: %v
	KeyTaskGroupSkipInvalidBuild  = "task_group.skip_invalid_build"  // 跳过无效构建任务: %v
	KeyTaskGroupSkipInvalidExport = "task_group.skip_invalid_export" // 跳过无效导出任务: %v
	KeyTaskGroupSummary           = "task_group.summary"             // 共找到 %d 个任务 (构建: %d, 导出: %d)
	KeyTaskGroupExecuteConfirm    = "task_group.execute_confirm"     // 确定要执行这些任务吗？
	KeyTaskExecutionCancelled     = "task.execution_cancelled"       // 任务执行已取消
	KeyBuildTaskExecuteFailed     = "build.task_execute_failed"      // 构建任务执行失败: %v

	KeyTaskAddExportConfirm = "task.add_export_confirm" // 是否添加导出任务
	KeyTaskAddBuildConfirm  = "task.add_build_confirm"  // 是否添加构建任务
	KeyTaskAddNew           = "task.add_new"            // 添加新任务

	KeyTaskAddExport = "task.add_export" // 添加导出任务
	KeyTaskAddBuild  = "task.add_build"  // 添加构建任务
	KeyTaskAdded     = "task.added"      // 任务已添加
	KeyTaskDeleted   = "task.deleted"    // 任务已删除
	KeyTaskModify    = "task.modify"     // 修改任务组
	KeyTaskRename    = "task.rename"     // 重命名任务组
	KeyTaskDelete    = "task.delete"     // 删除任务组
)

// 导入导出模式
const (
	KeyModeSingleImport   = "mode.single_import"   // 启动单导入
	KeyModeMultipleImport = "mode.multiple_import" // 启动多导入
	KeyModeSingleExport   = "mode.single_export"   // 启动单导出
	KeyModeMultipleExport = "mode.multiple_export" // 启动多导出
	KeyModeSingleBuild    = "mode.single_build"    // 启动单构建
	KeyModeMultipleBuild  = "mode.multiple_build"  // 启动多构建
)

// 通用文本
const (
	KeyReturn           = "common.return"             // 返回
	KeyCancel           = "common.cancel"             // 取消
	KeyConfirmDelete    = "common.confirm_delete"     // 确定要删除吗？此操作不可恢复
	KeyNameUnchanged    = "common.name_unchanged"     // 名称未更改
	KeyEnterNewName     = "common.enter_new_name"     // 请输入新名称
	KeyCurrentName      = "common.current_name"       // 当前
	KeySelectOperation  = "common.select_operation"   // 选择操作
	KeySelectTaskType   = "common.select_task_type"   // 选择任务类型
	KeySelectToModify   = "common.select_to_modify"   // 选择要修改的任务
	KeySelectConfigType = "common.select_config_type" // 选择配置类型
	KeySelectLanguage   = "common.select_language"    // 选择语言
	KeyCurrent          = "common.current"            // 当前
	KeyEmpty            = "common.empty"              // 空
)

// 错误和状态消息
const (
	KeyErrorReadTaskDir     = "error.read_task_dir"     // 读取任务目录失败
	KeyErrorReadConfigFile  = "error.read_config_file"  // 读取配置文件失败
	KeyErrorParseConfigFile = "error.parse_config_file" // 解析配置文件失败
	KeyErrorParseTask       = "error.parse_task"        // 解析错误
	KeyErrorRenameFailed    = "error.rename_failed"     // 重命名失败
	KeyErrorDeleteFailed    = "error.delete_failed"     // 删除失败
	KeyErrorInvalidPath     = "error.invalid_path"      // 无效的存储路径
)

// JSON 和调试信息
const (
	KeyJSONConfigAndTasks = "json.config_and_tasks" // 配置和任务 JSON
	KeyJSONServerConfig   = "json.server_config"    // 服务器配置
	KeyJSONTaskList       = "json.task_list"        // 任务列表
	KeyJSONDescription    = "json.description"      // 说明
	KeyJSONSeparateNote   = "json.separate_note"    // 服务器配置和任务是分开的
	KeyJSONNoServerConfig = "json.no_server_config" // 任务不包含服务器配置
	KeyJSONTaskGroupFile  = "json.task_group_file"  // 任务组文件只包含任务列表
	KeyJSONFileNameNote   = "json.file_name_note"   // 文件名就是任务组名称
)

// 任务显示
const (
	KeyTaskGroupInfo   = "task.group_info"   // 任务组: %s (共 %d 个任务)
	KeyTaskTypeImport  = "task.type_import"  // 导入
	KeyTaskTypeExport  = "task.type_export"  // 导出
	KeyTaskTypeBuild   = "task.type_build"   // 构建
	KeyTaskTypeUnknown = "task.type_unknown" // 未知类型
	KeyTaskSpeedLabel  = "task.speed_label"  // 速度
	KeyTaskTypeLabel   = "task.type_label"   // 类型
)

// 服务器配置管理
const (
	KeyServerConfigTitle        = "server.config_title"         // 服务器配置: %s
	KeyServerConfigNumber       = "server.config_number"        // 服务器号
	KeyServerConfigPassword     = "server.config_password"      // 服务器密码
	KeyServerConfigAuthAddress  = "server.config_auth_address"  // 验证服务器地址
	KeyServerConfigAuthToken    = "server.config_auth_token"    // 验证服务器Token
	KeyServerConfigModifyNumber = "server.config_modify_number" // 修改服务器号
	KeyServerConfigModifyPass   = "server.config_modify_pass"   // 修改服务器密码
	KeyServerConfigModifyAddr   = "server.config_modify_addr"   // 修改验证服务器地址
	KeyServerConfigModifyToken  = "server.config_modify_token"  // 修改验证服务器Token
	KeyServerConfigInputNumber  = "server.config_input_number"  // 请输入新的服务器号
	KeyServerConfigInputPass    = "server.config_input_pass"    // 请输入新的服务器密码
	KeyServerConfigInputAddr    = "server.config_input_addr"    // 请输入新的验证服务器地址
	KeyServerConfigInputToken   = "server.config_input_token"   // 请输入新的验证服务器Token
	KeyServerConfigInputName    = "server.config_input_name"    // 请输入新名称 (当前: %s)
	KeyServerConfigConfirmDel   = "server.config_confirm_del"   // 确定要删除配置 '%s' 吗？此操作不可恢复
	KeyServerConfigRenamed      = "server.config_renamed"       // 配置已重命名为: %s
	KeyServerConfigDeleted      = "server.config_deleted"       // 配置 '%s' 已删除
)

// 框架配置管理
const (
	KeyFrameworkConfigTitle           = "framework.config_title"             // 管理框架配置
	KeyFrameworkConfigCurrent         = "framework.config_current"           // 当前框架配置
	KeyFrameworkConfigLanguage        = "framework.config_language"          // 语言
	KeyFrameworkConfigStorage         = "framework.config_storage"           // 存储路径
	KeyFrameworkConfigModifyLang      = "framework.config_modify_lang"       // 修改语言设置
	KeyFrameworkConfigModifyPath      = "framework.config_modify_path"       // 修改存储路径
	KeyFrameworkConfigModifyBuildTask = "framework.config_modify_build_task" // 修改构建任务配置
	KeyFrameworkConfigModifyUpdate    = "framework.config_modify_update"     // 修改更新配置
	KeyFrameworkConfigModifyExtra     = "framework.config_modify_extra"      // 修改扩展配置
	KeyFrameworkConfigReset           = "framework.config_reset"             // 重置为默认配置
	KeyFrameworkConfigConfirmReset    = "framework.config_confirm_reset"     // 确定要重置为默认配置吗？
	KeyFrameworkConfigCurrentLang     = "framework.config_current_lang"      // 当前语言: %s
	KeyFrameworkConfigLangSet         = "framework.config_lang_set"          // 语言已设置为: %s
	KeyFrameworkConfigCurrentPath     = "framework.config_current_path"      // 当前存储路径: %s
	KeyFrameworkConfigInputPath       = "framework.config_input_path"        // 请输入新的存储路径 (直接回车保持不变)
	KeyFrameworkConfigPathSet         = "framework.config_path_set"          // 存储路径已设置为: %s
	KeyFrameworkConfigResetDone       = "framework.config_reset_done"        // 框架配置已重置为默认值
	KeyLanguageChinese                = "language.chinese"                   // 中文
	KeyLanguageEnglish                = "language.english"                   // English
)

// 扩展配置
const (
	KeyExtraConfigTitle              = "extra.config_title"                 // 扩展配置
	KeyExtraConfigTanLobbyMode       = "extra.config_tan_lobby_mode"        // TanLobby 房间运营模式
	KeyExtraConfigTanLobbyModePrompt = "extra.config_tan_lobby_mode_prompt" // 是否启用 TanLobby 房间运营模式
)

// 构建任务配置管理
const (
	KeyBuildTaskConfigTitle                      = "build_task.config_title"                           // 构建任务配置
	KeyBuildTaskConfigCurrent                    = "build_task.config_current"                         // 当前构建任务配置
	KeyBuildTaskConfigMinOnceParseChunkNum       = "build_task.config_min_once_parse_chunk_num"        // 单次解析区块数量最小值
	KeyBuildTaskConfigMinMatchingDegree          = "build_task.config_min_matching_degree"             // 匹配度最小值
	KeyBuildTaskConfigMaxFixDepth                = "build_task.config_max_fix_depth"                   // 最大修补深度
	KeyBuildTaskConfigModifyMinOnceParseChunkNum = "build_task.config_modify_min_once_parse_chunk_num" // 修改单次解析区块数量最小值
	KeyBuildTaskConfigModifyMinMatchingDegree    = "build_task.config_modify_min_matching_degree"      // 修改匹配度最小值
	KeyBuildTaskConfigModifyMaxFixDepth          = "build_task.config_modify_max_fix_depth"            // 修改最大修补深度
	KeyBuildTaskConfigInputMinOnceParseChunkNum  = "build_task.config_input_min_once_parse_chunk_num"  // 请输入新的单次解析区块数量最小值
	KeyBuildTaskConfigInputMinMatchingDegree     = "build_task.config_input_min_matching_degree"       // 请输入新的匹配度最小值
	KeyBuildTaskConfigInputMaxFixDepth           = "build_task.config_input_max_fix_depth"             // 请输入新的最大修补深度
	KeyBuildTaskConfigMinOnceParseChunkNumSet    = "build_task.config_min_once_parse_chunk_num_set"    // 单次解析区块数量最小值已设置为: %d
	KeyBuildTaskConfigMinMatchingDegreeSet       = "build_task.config_min_matching_degree_set"         // 匹配度最小值已设置为: %.1f
	KeyBuildTaskConfigMaxFixDepthSet             = "build_task.config_max_fix_depth_set"               // 最大修补深度已设置为: %d
	KeyBuildTaskConfigErrorMinOnceParseChunkNum  = "build_task.config_error_min_once_parse_chunk_num"  // 单次解析区块数量最小值不能小于1
	KeyBuildTaskConfigErrorMinMatchingDegree     = "build_task.config_error_min_matching_degree"       // 匹配度必须在0-100之间
	KeyBuildTaskConfigErrorMaxFixDepth           = "build_task.config_error_max_fix_depth"             // 最大修补深度不能小于1
	KeyBuildTaskConfigErrorSetFailed             = "build_task.config_error_set_failed"                // 设置失败: %v
)

// 自动更新
const (
	KeyAutoUpdateError       = "auto_update.error"        // 自动更新过程中发生错误，程序将继续启动
	KeyAutoUpdateFailed      = "auto_update.failed"       // 自动更新失败
	KeyAutoUpdateCannotStart = "auto_update.cannot_start" // 程序无法继续启动，请检查网络连接

	// 更新流程
	KeyAutoUpdateChecking           = "auto_update.checking"             // 🔍 检查更新中...
	KeyAutoUpdateCheckFailed        = "auto_update.check_failed"         // 检查更新失败
	KeyAutoUpdateLatest             = "auto_update.latest"               // ✅ 当前已是最新版本
	KeyAutoUpdateNewVersion         = "auto_update.new_version"          // 🆕 发现新版本，开始下载...
	KeyAutoUpdateLocalVersion       = "auto_update.local_version"        // 本地版本
	KeyAutoUpdateRemoteVersion      = "auto_update.remote_version"       // 远程版本
	KeyAutoUpdateDownloadFailed     = "auto_update.download_failed"      // 下载更新失败
	KeyAutoUpdateDownloadDone       = "auto_update.download_done"        // 📦 下载完成，正在安装更新...
	KeyAutoUpdateInstallFailed      = "auto_update.install_failed"       // 安装更新失败
	KeyAutoUpdateInstallDone        = "auto_update.install_done"         // ✅ 更新安装完成，正在重启程序...
	KeyAutoUpdateRestartFailed      = "auto_update.restart_failed"       // ⚠️  重启失败，请手动重启程序
	KeyAutoUpdateManualRestart      = "auto_update.manual_restart"       // 程序将退出，请手动重新启动以使用新版本
	KeyAutoUpdateDisabled           = "auto_update.disabled"             // 自动更新已禁用
	KeyAutoUpdateDevMode            = "auto_update.dev_mode"             // 🔧 开发模式检测：跳过自动更新
	KeyAutoUpdateNoticeDownloadOnly = "auto_update.notice_download_only" // 自动更新提示：仅下载，不自动替换
	KeyAutoUpdateSavedPath          = "auto_update.saved_path"           // 已保存到
	KeyAutoUpdateManualRunTip       = "auto_update.manual_run_tip"       // 请手动运行最新文件

	// 客户端错误
	KeyAutoUpdateNoExecutable     = "auto_update.no_executable"     // 无法获取当前可执行文件路径
	KeyAutoUpdateCalcSHAFailed    = "auto_update.calc_sha_failed"   // 计算本地文件SHA512失败
	KeyAutoUpdateGetSHAFailed     = "auto_update.get_sha_failed"    // 获取远程文件SHA512失败
	KeyAutoUpdateServerError      = "auto_update.server_error"      // 服务器返回错误
	KeyAutoUpdateDownloadError    = "auto_update.download_error"    // 下载失败
	KeyAutoUpdateVerifyFailed     = "auto_update.verify_failed"     // 验证下载文件SHA512失败
	KeyAutoUpdateCheckSumFailed   = "auto_update.checksum_failed"   // 下载文件SHA512校验失败
	KeyAutoUpdateBackupFailed     = "auto_update.backup_failed"     // 备份当前文件失败
	KeyAutoUpdateReplaceFailed    = "auto_update.replace_failed"    // 替换文件失败
	KeyAutoUpdatePermissionFailed = "auto_update.permission_failed" // 设置可执行权限失败

	// 平台相关
	KeyAutoUpdateUnsupportedPlatform = "auto_update.unsupported_platform" // 不支持的平台
	KeyAutoUpdateScriptCreateFailed  = "auto_update.script_create_failed" // 创建重启脚本失败
	KeyAutoUpdateScriptStartFailed   = "auto_update.script_start_failed"  // 启动重启脚本失败

	// 进度条
	KeyAutoUpdateProgressUnknown = "auto_update.progress_unknown" // 未知
	KeyAutoUpdateProgressTitle   = "auto_update.progress_title"   // 下载进度
)

// 扣费与余额提示
const (
	KeyCostChargeSuccess        = "cost.charge.success"         // 扣费成功：已扣除 %d 点，余额 %s
	KeyCostChargeDuplicate      = "cost.charge.duplicate"       // 检测到重复文件，已跳过扣费
	KeyCostChargeFailed         = "cost.charge.failed"          // 扣费失败：%s
	KeyCostChargeBalanceUnknown = "cost.charge.balance_unknown" // 未知
	KeyCostBlockStats           = "cost.block_stats"            // 方块统计：普通=%d, 命令=%d, NBT=%d
	KeyUserCenterNotInit        = "user_center.not_init"        // 用户中心未初始化
)

// 所有翻译键的列表，用于验证和管理
var AllKeys = []string{
	// 语言包元信息
	KeyPackName,

	// 应用程序信息
	KeyAppName,
	KeyAppDescription,
	KeyAppUsage,

	// 命令行参数
	KeyArgsStoragePathUsage,
	KeyArgsStoragePathDescription,
	KeyArgsLanguageUsage,
	KeyArgsHelpUsage,
	KeyArgsVersionUsage,

	// 通用术语
	KeyCommonOptions,
	KeyCommonExamples,
	KeyCommonError,

	// 版本信息
	KeyVersionLicense,
	KeyVersionAuthors,
	KeyVersionCommit,
	KeyVersionCommitNum,
	KeyVersionBuildDate,
	KeyVersionStoragePath,
	KeyVersionLanguage,

	// 使用示例
	KeyExamplesDefault,
	KeyExamplesStoragePath,
	KeyExamplesLanguage,
	KeyExamplesVersion,
	KeyExamplesHelp,

	// UI 界面文本
	KeyUIInputValidInteger,
	KeyUIInputValidNumber,
	KeyUISelectPrompt,
	KeyUISelectRange,
	KeyUIMultiInputEnd,
	KeyUIConfirmPrompt,

	// 主菜单
	KeyMenuMain,

	KeyMenuStartExport,
	KeyMenuStartBuild,
	KeyMenuConfigTasks,
	KeyMenuManageConfig,
	KeyMenuBuildingOps,
	KeyMenuExit,

	// 建筑操作
	KeyBuildingOpsTitle,
	KeyBuildingOpsConvertMCWorldSlow,
	KeyBuildingOpsConvertMCWorldFast,
	KeyBuildingOpsAnyToAny,
	KeyBuildingOpsNeteaseEncrypt,
	KeyBuildingOpsNeteaseDecrypt,
	KeyBuildingOpsMapArt,
	KeyBuildingOpsPromptStructurePath,
	KeyBuildingOpsPromptOutputMCWorldPath,
	KeyBuildingOpsDetectedStructureType,
	KeyBuildingOpsSelectTargetFormat,
	KeyBuildingOpsPromptOutputStructurePath,
	KeyBuildingOpsAnyToAnyDone,
	KeyBuildingOpsPromptWorldOrDBPath,
	KeyBuildingOpsPromptImagePath,
	KeyBuildingOpsConfirmModifyDir,
	KeyBuildingOpsNeteaseDone,
	KeyBuildingOpsProgressWriteWorld,
	KeyBuildingOpsProgressExportStructure,
	KeyBuildingOpsPromptStartSubChunkX,
	KeyBuildingOpsPromptStartSubChunkY,
	KeyBuildingOpsPromptStartSubChunkZ,
	KeyBuildingOpsPromptMapWidth,
	KeyBuildingOpsPromptMapHeight,
	KeyBuildingOpsPromptMapForce2D,
	KeyBuildingOpsPromptMapDisableReference,
	KeyBuildingOpsPromptMapMax3DHeight,
	KeyBuildingOpsMapArtNeedsWorldDir,
	KeyBuildingOpsMapArtRange,
	KeyBuildingOpsMapArtDone,

	// 导入导出功能

	KeyExportTitle,
	KeyExportTaskPreparingIndexed,
	KeyExportTaskStartSummary,
	KeyExportEstimatedTotalChunks,
	KeyExportProgressBarDesc,
	KeyExportSubChunkDecodeFailed,
	KeyExportSubChunkSaveFailed,
	KeyExportChunkSaveFailed,
	KeyExportFinishedPath,
	KeyExportTaskRunning,
	KeyExportTaskFailed,
	KeyExportTaskCompleted,
	KeyExportTaskExecuteFailed,
	KeyServerConnectFailed,
	KeyBuildInterruptRemoveWarn,
	KeyBuildTitle,
	KeyServerConfigSelect,
	KeyServerConfigUseLastTitle,
	KeyServerConfigUseLast,
	KeyServerConfigCreateNew,
	KeyServerConfigSelectExist,
	KeyServerConfigNoFiles,

	// 服务器配置
	KeyServerNumber,
	KeyServerPassword,
	KeyServerAddress,
	KeyServerToken,
	KeyServerSaveConfig,
	KeyServerFileName,
	KeyServerConfigSaved,

	// 任务配置
	KeyTaskStartCoord,
	KeyTaskEndCoord,
	KeyTaskDimensionName,
	KeyTaskSpeed,
	KeyTaskClearBlocks,
	KeyTaskClearDrops,
	KeyTaskVerifyInterval,
	KeyTaskVerifyLevel,
	KeyTaskProgress,
	KeyTaskSaveGroup,
	KeyTaskExecuteNow,
	KeyTaskExecuteComplete,
	KeyServerConnecting,
	KeyServerConnected,
	KeyTaskExecuting,
	KeyTaskGroupName,
	KeyTaskAutoDisableCommand,
	KeyTaskAutoUpgradeCommand,
	KeyTaskIgnoreCommandBlock,
	KeyTaskIgnoreOtherNBTBlock,
	KeyTaskSetConsolePos,
	KeyTaskConsoleCoordinates,
	KeyTaskShowNBTProgress,
	KeyTaskNBTImportProgress,
	KeyTaskWaitChunkLoad,
	KeyTaskConvertToMCWorld,

	// 导出任务配置
	KeyExportModePrompt,
	KeyExportAddAnotherTask,
	KeyExportFilePrompt,

	// 构建任务配置
	KeyBuildModePrompt,
	KeyBuildAddAnotherTask,
	KeyBuildAppendNewTask,
	KeyBuildFilePrompt,

	// 验证等级
	KeyVerifyNone,
	KeyVerifyAir,
	KeyVerifySimilar,
	KeyVerifyComplete,

	// 文件操作
	KeyFileNotFound,
	KeyFileCustomPath,
	KeyFileEnterPath,
	KeyFileInvalidPath,

	// 通用操作
	KeyOperationSuccess,
	KeyOperationFailed,
	KeyOperationCancel,
	KeyOperationAborted,
	KeyBackToMenu,
	KeyPressEnter,

	// 构建进度和中断
	KeyBuildProgressTitle,
	KeyBuildProgressCurrent,
	KeyBuildProgressSpeed,
	KeyBuildInterruptFound,
	KeyBuildInterruptResume,
	KeyBuildInterruptNew,
	KeyBuildInterruptDelete,
	KeyBuildInterruptSaved,
	KeyBuildInterruptRestored,
	KeyBuildInterruptDeleted,
	KeyBuildInterruptFile,
	KeyBuildInterruptProgress,
	KeyBuildTaskStart,
	KeyBuildStructureType,
	KeyBuildParsingFile,
	KeyBuildFileParsed,
	KeyBuildStructureSize,
	KeyBuildWidth,
	KeyBuildHeight,
	KeyBuildLength,
	KeyBuildTotalChunks,
	KeyBuildStartImport,
	KeyBuildProgressBarDesc,
	KeyBuildAutoFixingChunk,
	KeyBuildTaskPreparingIndexed,
	KeyBuildControlHint,
	KeyBuildTaskFailed,
	KeyBuildTaskCompleted,
	KeyBuildPauseFailed,
	KeyBuildPaused,
	KeyBuildContinueFailed,
	KeyBuildResumed,
	KeyBuildStopSuccess,
	KeyBuildSkipSuccess,
	KeyBuildUnknownCommand,
	KeyBuildChunkFixExceeded,
	KeyBuildInterruptCleaned,
	KeyBuildInterruptMigrated,
	KeyBuildResumeTitle,
	KeyBuildResumeInfo,
	KeyBuildResumeFile,
	KeyBuildResumeStartPos,
	KeyBuildResumeSpeed,
	KeyBuildResumeInProgress,
	KeyBuildResumeExecuteFailed,
	KeyBuildResumeRestoreWarning,
	KeyBuildResumeCancelled,
	KeyBuildConvertToMCWorldStart,
	KeyBuildConvertToMCWorldExists,
	KeyBuildConvertToMCWorldSuccess,
	KeyBuildConvertToMCWorldFailed,
	KeyBuildConvertToMCWorldProgress,

	// 配置管理
	KeyConfigManageServer,
	KeyConfigManageFramework,
	KeyConfigCreateNew,
	KeyConfigSelectExisting,
	KeyConfigNoFiles,
	KeyConfigSaved,
	KeyConfigRenamed,
	KeyConfigDeleted,

	// 任务管理
	KeyTaskGroupCreate,
	KeyTaskGroupManage,
	KeyTaskGroupNoFiles,
	KeyTaskGroupRenamed,
	KeyTaskGroupDeleted,

	KeyTaskGroupCreateExport,
	KeyTaskGroupCreateBuild,
	KeyTaskGroupCreateMixed,
	KeyTaskGroupSelectType,
	KeyTaskGroupEmpty,
	KeyTaskGroupRunOption,
	KeyTaskGroupRunTitle,
	KeyTaskGroupSkipInvalid,
	KeyTaskGroupSkipInvalidBuild,
	KeyTaskGroupSkipInvalidExport,
	KeyTaskGroupSummary,
	KeyTaskGroupExecuteConfirm,
	KeyTaskExecutionCancelled,
	KeyBuildTaskExecuteFailed,

	KeyTaskAddExportConfirm,
	KeyTaskAddBuildConfirm,
	KeyTaskAddNew,

	KeyTaskAddExport,
	KeyTaskAddBuild,
	KeyTaskAdded,
	KeyTaskDeleted,
	KeyTaskModify,
	KeyTaskRename,
	KeyTaskDelete,

	// 导入导出模式
	KeyModeSingleImport,
	KeyModeMultipleImport,
	KeyModeSingleExport,
	KeyModeMultipleExport,
	KeyModeSingleBuild,
	KeyModeMultipleBuild,

	// 通用文本
	KeyReturn,
	KeyCancel,
	KeyConfirmDelete,
	KeyNameUnchanged,
	KeyEnterNewName,
	KeyCurrentName,
	KeySelectOperation,
	KeySelectTaskType,
	KeySelectToModify,
	KeySelectConfigType,
	KeySelectLanguage,
	KeyCurrent,
	KeyEmpty,

	// 错误和状态消息
	KeyErrorReadTaskDir,
	KeyErrorReadConfigFile,
	KeyErrorParseConfigFile,
	KeyErrorParseTask,
	KeyErrorRenameFailed,
	KeyErrorDeleteFailed,
	KeyErrorInvalidPath,

	// JSON 和调试信息
	KeyJSONConfigAndTasks,
	KeyJSONServerConfig,
	KeyJSONTaskList,
	KeyJSONDescription,
	KeyJSONSeparateNote,
	KeyJSONNoServerConfig,
	KeyJSONTaskGroupFile,
	KeyJSONFileNameNote,

	// 任务显示
	KeyTaskGroupInfo,
	KeyTaskTypeImport,
	KeyTaskTypeExport,
	KeyTaskTypeBuild,
	KeyTaskTypeUnknown,
	KeyTaskSpeedLabel,
	KeyTaskTypeLabel,

	// 服务器配置管理
	KeyServerConfigTitle,
	KeyServerConfigNumber,
	KeyServerConfigPassword,
	KeyServerConfigAuthAddress,
	KeyServerConfigAuthToken,
	KeyServerConfigModifyNumber,
	KeyServerConfigModifyPass,
	KeyServerConfigModifyAddr,
	KeyServerConfigModifyToken,
	KeyServerConfigInputNumber,
	KeyServerConfigInputPass,
	KeyServerConfigInputAddr,
	KeyServerConfigInputToken,
	KeyServerConfigInputName,
	KeyServerConfigConfirmDel,
	KeyServerConfigRenamed,
	KeyServerConfigDeleted,

	// 框架配置管理
	KeyFrameworkConfigTitle,
	KeyFrameworkConfigCurrent,
	KeyFrameworkConfigLanguage,
	KeyFrameworkConfigStorage,
	KeyFrameworkConfigModifyLang,
	KeyFrameworkConfigModifyPath,
	KeyFrameworkConfigModifyBuildTask,
	KeyFrameworkConfigModifyUpdate,
	KeyFrameworkConfigModifyExtra,
	KeyFrameworkConfigReset,
	KeyFrameworkConfigConfirmReset,
	KeyFrameworkConfigCurrentLang,
	KeyFrameworkConfigLangSet,
	KeyFrameworkConfigCurrentPath,
	KeyFrameworkConfigInputPath,
	KeyFrameworkConfigPathSet,
	KeyFrameworkConfigResetDone,
	KeyLanguageChinese,
	KeyLanguageEnglish,

	// 扩展配置
	KeyExtraConfigTitle,
	KeyExtraConfigTanLobbyMode,
	KeyExtraConfigTanLobbyModePrompt,

	// 构建任务配置管理
	KeyBuildTaskConfigTitle,
	KeyBuildTaskConfigCurrent,
	KeyBuildTaskConfigMinOnceParseChunkNum,
	KeyBuildTaskConfigMinMatchingDegree,
	KeyBuildTaskConfigMaxFixDepth,
	KeyBuildTaskConfigModifyMinOnceParseChunkNum,
	KeyBuildTaskConfigModifyMinMatchingDegree,
	KeyBuildTaskConfigModifyMaxFixDepth,
	KeyBuildTaskConfigInputMinOnceParseChunkNum,
	KeyBuildTaskConfigInputMinMatchingDegree,
	KeyBuildTaskConfigInputMaxFixDepth,
	KeyBuildTaskConfigMinOnceParseChunkNumSet,
	KeyBuildTaskConfigMinMatchingDegreeSet,
	KeyBuildTaskConfigMaxFixDepthSet,
	KeyBuildTaskConfigErrorMinOnceParseChunkNum,
	KeyBuildTaskConfigErrorMinMatchingDegree,
	KeyBuildTaskConfigErrorMaxFixDepth,
	KeyBuildTaskConfigErrorSetFailed,

	// 自动更新
	KeyAutoUpdateError,
	KeyAutoUpdateFailed,
	KeyAutoUpdateCannotStart,

	// 更新流程
	KeyAutoUpdateChecking,
	KeyAutoUpdateCheckFailed,
	KeyAutoUpdateLatest,
	KeyAutoUpdateNewVersion,
	KeyAutoUpdateLocalVersion,
	KeyAutoUpdateRemoteVersion,
	KeyAutoUpdateDownloadFailed,
	KeyAutoUpdateDownloadDone,
	KeyAutoUpdateInstallFailed,
	KeyAutoUpdateInstallDone,
	KeyAutoUpdateRestartFailed,
	KeyAutoUpdateManualRestart,
	KeyAutoUpdateDisabled,
	KeyAutoUpdateDevMode,
	KeyAutoUpdateNoticeDownloadOnly,
	KeyAutoUpdateSavedPath,
	KeyAutoUpdateManualRunTip,

	// 客户端错误
	KeyAutoUpdateNoExecutable,
	KeyAutoUpdateCalcSHAFailed,
	KeyAutoUpdateGetSHAFailed,
	KeyAutoUpdateServerError,
	KeyAutoUpdateDownloadError,
	KeyAutoUpdateVerifyFailed,
	KeyAutoUpdateCheckSumFailed,
	KeyAutoUpdateBackupFailed,
	KeyAutoUpdateReplaceFailed,
	KeyAutoUpdatePermissionFailed,

	// 平台相关
	KeyAutoUpdateUnsupportedPlatform,
	KeyAutoUpdateScriptCreateFailed,
	KeyAutoUpdateScriptStartFailed,

	// 进度条
	KeyAutoUpdateProgressUnknown,
	KeyAutoUpdateProgressTitle,

	// 扣费与余额提示
	KeyCostChargeSuccess,
	KeyCostChargeDuplicate,
	KeyCostChargeFailed,
	KeyCostChargeBalanceUnknown,
	KeyCostBlockStats,
	KeyUserCenterNotInit,
}

// GetAllKeys 获取所有翻译键
func GetAllKeys() []string {
	return AllKeys
}

// ValidateKey 验证翻译键是否存在
func ValidateKey(key string) bool {
	for _, k := range AllKeys {
		if k == key {
			return true
		}
	}
	return false
}

// GetKeysByCategory 按分类获取翻译键
func GetKeysByCategory() map[string][]string {
	return map[string][]string{
		"app": {
			KeyAppDescription,
			KeyAppUsage,
		},
		"args": {
			KeyArgsStoragePathUsage,
			KeyArgsStoragePathDescription,
			KeyArgsLanguageUsage,
			KeyArgsHelpUsage,
			KeyArgsVersionUsage,
		},
		"common": {
			KeyCommonOptions,
			KeyCommonExamples,
			KeyCommonError,
		},
		"version": {
			KeyVersionLicense,
			KeyVersionAuthors,
			KeyVersionCommit,
		},
		"examples": {
			KeyExamplesDefault,
			KeyExamplesStoragePath,
			KeyExamplesLanguage,
			KeyExamplesVersion,
			KeyExamplesHelp,
		},
		"ui": {
			KeyUIInputValidInteger,
			KeyUIInputValidNumber,
			KeyUISelectPrompt,
			KeyUISelectRange,
			KeyUIMultiInputEnd,
			KeyUIConfirmPrompt,
		},
	}
}
