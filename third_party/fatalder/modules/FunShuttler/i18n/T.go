package i18n

const (
	LanguageEnglish_US        = "en_US"
	LanguageSimplifiedChinese = "zh_CN"
	LanguageSpecialChinese    = "zh_CN_special"
	LanguageSpecial2Chinese   = "zh_CN_special2"
)

const (
	Frame_Bootstarp = iota
	Frame_StartLoginSequence
	Frame_ConnectingToAuthenticationServer
	Frame_Error_ParseUrl
	Utils_Error_DetailedError
	Auth_BackendError
	Auth_FailedToRequestEntry
	Auth_HelperNotCreated
	Auth_InvalidVersion
	Auth_InvalidHelperUsername
	Auth_InvalidToken
	Auth_InvalidUser
	Auth_ServerNotFound
	Auth_UnauthorizedRentalServerNumber
	Auth_UserCombined
	Auth_FailedToRequestEntry_TryAgain
	Auth_MessageFromAuthServer
	Frame_Error_ContactWithAPI
	Frame_Error_APIServerDown
	Frame_Success_ConnectedToAuthenticationServer
	Frame_CreateNewAccessWrapper
	Frame_InitializingConnection
	Frame_GeneratingClientKeyPair
	Frame_RetrievingBotInfo
	Frame_CreatingRakNetConnection
	Frame_Encapsulating_the_data_frame_connection_layer
	Frame_Encapsulating_the_data_packet_connection_layer
	Frame_GeneratingKeyLoginInfo
	Frame_Error_InitializingConnection
	Frame_LoginSequenceCompleted
	Frame_SendingAdditionalInfo
	Frame_PackingKeyData
	Frame_MCPCheckChallenge
	Frame_MCPCheckChallengeDone
	Frame_LoginDone
	Frame_CrashedNotice
	Frame_ConnectionClosed_RebootTip
	Frame_LoginFailed_RebootTip
	Cmd_Error_SendSettingsCommand
	Cmd_Error_SendCommand
	Cmd_Error_SendWSCommand
	Cmd_Error_SendCommandWithResponse
	Cmd_Error_SendCommandWithResponse_ParseOptions
	Cmd_Error_SendCommandWithResponse_ParseResponse
	Cmd_Error_SendWSCommandWithResponse
	Client_Error_GetClientInfo
)

var I18nDict_en_US map[uint16]string = map[uint16]string{
	Frame_Bootstarp:                                      "[Fun-Core] Bootstrapping..",
	Frame_StartLoginSequence:                             "Begin to execute the login sequence (Server number: %s Password protection: %t)",
	Frame_ConnectingToAuthenticationServer:               "Connecting to authentication server: %s",
	Frame_Error_ParseUrl:                                 "Failed to resolve the address of the authentication server",
	Utils_Error_DetailedError:                            "Detailed error: ",
	Auth_BackendError:                                    "Backend Error",
	Auth_FailedToRequestEntry:                            "Failed to request entry for your server, please check whether the password is correct and please turn off the level limitation",
	Auth_HelperNotCreated:                                "Helper user haven't been created, please go create it on FastBuilder User Center",
	Auth_InvalidVersion:                                  "Invalid version, please update",
	Auth_InvalidHelperUsername:                           "Invalid username for helper user, please set it on FastBuilder User Center",
	Auth_InvalidToken:                                    "Invalid login token",
	Auth_InvalidUser:                                     "Invalid user for FastBuilder User Center",
	Auth_ServerNotFound:                                  "Server not found, please check your server's public state",
	Auth_UnauthorizedRentalServerNumber:                  "Unauthorized rental server number, please add it on your FastBuilder User Center",
	Auth_UserCombined:                                    "Given user has been combined to another account, please login using another account's information",
	Auth_FailedToRequestEntry_TryAgain:                   "Failed to request server entry, please try again later",
	Auth_MessageFromAuthServer:          "Message from auth server:",
	Frame_Error_ContactWithAPI:                           "Failed to contact with API",
	Frame_Error_APIServerDown:                            "API server is down",
	Frame_Success_ConnectedToAuthenticationServer:        "Successfully connected to authentication server",
	Frame_CreateNewAccessWrapper:                         "Creating access wrapper",
	Frame_InitializingConnection:                         "Initializing the connection with Minecraft",
	Frame_GeneratingClientKeyPair:                        "Generating the client key pair",
	Frame_RetrievingBotInfo:                              "Retrieving bot information from the authentication server",
	Frame_CreatingRakNetConnection:                       "Establishing a RakNet connection",
	Frame_Encapsulating_the_data_frame_connection_layer:  "Encapsulating the data frame connection layer",
	Frame_Encapsulating_the_data_packet_connection_layer: "Encapsulating the data packet connection layer",
	Frame_GeneratingKeyLoginInfo:                         "Generating key login information",
	Frame_Error_InitializingConnection:                   "Failed to initialize the connection with Minecraft",
	Frame_LoginSequenceCompleted:                         "The login sequence is completed",
	Frame_SendingAdditionalInfo:                          "Sending additional information",
	Frame_PackingKeyData:                                 "Packing the key data",
	Frame_MCPCheckChallenge:                              "Resolving the MCP check challenge",
	Frame_MCPCheckChallengeDone: "MCP check complete",
	Frame_LoginDone:                                      "Successfully connected to the rented server",
	Frame_CrashedNotice:                                  "Oh no! FunCore Crashed",
	Frame_ConnectionClosed_RebootTip:                     "[Fun-Core] Currently, there have been %d consecutive crashes. FunCore will attempt to reconnect after %.2f seconds",
	Frame_LoginFailed_RebootTip:                          "[Fun-Core] Currently, there have been %d consecutive login failures. FunCore will attempt to reconnect immediately",
	Cmd_Error_SendSettingsCommand:                        "Failed to send settings command",
	Cmd_Error_SendCommand:                                "Failed to send command",
	Cmd_Error_SendWSCommand:                              "Failed to send websocket command",
	Cmd_Error_SendCommandWithResponse:                    "Failed to send command with response",
	Cmd_Error_SendCommandWithResponse_ParseOptions:       "Failed to parse the options",
	Cmd_Error_SendCommandWithResponse_ParseResponse:      "Failed to parse the response",
	Cmd_Error_SendWSCommandWithResponse:                  "Failed to send websocket command with response",
	Client_Error_GetClientInfo:                           "Failed to parse the client info",
}

var I18nDict_zh_CN map[uint16]string = map[uint16]string{
	Frame_Bootstarp:                                      "[Fun-Core] 启动中..",
	Frame_StartLoginSequence:                             "开始执行登录序列 (服务器号: %s 密码保护: %t)",
	Frame_ConnectingToAuthenticationServer:               "正在连接到验证服务器: %s",
	Frame_Error_ParseUrl:                                 "解析验证服务器地址失败",
	Utils_Error_DetailedError:                            "详细错误: ",
	Auth_BackendError:                                    "后端错误",
	Auth_FailedToRequestEntry:                            "未能请求租赁服入口, 请检查租赁服等级设置是否关闭及租赁服密码是否正确",
	Auth_HelperNotCreated:                                "辅助用户尚未创建, 请前往用户中心进行创建",
	Auth_InvalidVersion:                                  "版本无效, 请更新",
	Auth_InvalidHelperUsername:                           "辅助用户的用户名无效, 请前往用户中心进行设置",
	Auth_InvalidToken:                                    "无效Token, 请重新登录",
	Auth_InvalidUser:                                     "无效用户, 请重新登录",
	Auth_ServerNotFound:                                  "租赁服未找到, 请检查租赁服是否对所有人开放",
	Auth_UnauthorizedRentalServerNumber:                  "对应租赁服号尚未授权, 请前往用户中心进行授权",
	Auth_UserCombined:                                    "该用户已经合并到另一个账户中, 请使用新账户登录",
	Auth_FailedToRequestEntry_TryAgain:                   "未能请求租赁服入口, 请稍后再试",
	Auth_MessageFromAuthServer:  "来自验证服务器的消息:",
	Frame_Error_ContactWithAPI:                           "未能与API进行通信",
	Frame_Error_APIServerDown:                            "API服务器已关闭",
	Frame_Success_ConnectedToAuthenticationServer:        "成功连接到验证服务器",
	Frame_CreateNewAccessWrapper:                         "正在创建访问包装器",
	Frame_InitializingConnection:                         "正在初始化与Minecraft的连接",
	Frame_GeneratingClientKeyPair:                        "正在生成客户端密钥对",
	Frame_RetrievingBotInfo:                              "正在从验证服务器获取机器人信息",
	Frame_CreatingRakNetConnection:                       "正在建立 RakNet 连接",
	Frame_Encapsulating_the_data_frame_connection_layer:  "正在封装数据帧连接层",
	Frame_Encapsulating_the_data_packet_connection_layer: "正在封装数据包连接层",
	Frame_GeneratingKeyLoginInfo:                         "正在生成关键登录信息",
	Frame_Error_InitializingConnection:                   "未能初始化与Minecraft的连接",
	Frame_LoginSequenceCompleted:                         "登录序列已完成",
	Frame_SendingAdditionalInfo:                          "正在发送附加信息",
	Frame_PackingKeyData:                                 "正在打包关键数据",
	Frame_MCPCheckChallenge:                              "正在解决 MCP 检查挑战",
	Frame_MCPCheckChallengeDone: "成功解决 MCP 检查挑战",
	Frame_LoginDone:                                      "成功连接到租赁服",
	Frame_CrashedNotice:                                  "哦不! FunCore 崩溃了",
	Frame_ConnectionClosed_RebootTip:                     "[Fun-Core] 目前连续崩溃 %d 次, FunCore 将在 %.2f 秒后尝试重新连接",
	Frame_LoginFailed_RebootTip:                          "[Fun-Core] 目前连续登录失败 %d 次, FunCore 将在 %.2f 秒尝试重新连接",
	Cmd_Error_SendSettingsCommand:                        "发送设置命令失败",
	Cmd_Error_SendCommand:                                "发送命令失败",
	Cmd_Error_SendWSCommand:                              "发送 Websocket 命令失败",
	Cmd_Error_SendCommandWithResponse:                    "发送命令并等待响应失败",
	Cmd_Error_SendCommandWithResponse_ParseOptions:       "解析选项失败",
	Cmd_Error_SendCommandWithResponse_ParseResponse:      "解析响应失败",
	Cmd_Error_SendWSCommandWithResponse:                  "发送 Websocket 命令并等待响应失败",
	Client_Error_GetClientInfo:                           "解析客户端信息失败",
}

var I18nDict_zh_CN_Special map[uint16]string = map[uint16]string{
	Frame_Bootstarp:                                      "[Fun-Core] 杂鱼服务启动中喵～哼哼，别急嘛！",
	Frame_StartLoginSequence:                             "要开始调教登录序列了哦～（服务器号:%s 密码保护:%t）杂鱼服务器准备好颤抖了吗？",
	Frame_ConnectingToAuthenticationServer:               "正在用尾巴戳戳验证服务器: %s ...呜～好慢！",
	Frame_Error_ParseUrl:                                 "杂鱼连验证服务器地址都解析不了吗？真是的～",
	Utils_Error_DetailedError:                            "错误详情什么的...才、才不想告诉你呢！哼！(＞﹏＜)",
	Auth_BackendError:                                    "后端什么的坏掉了啦～杂鱼程序员快修好！",
	Auth_FailedToRequestEntry:                            "连租赁服入口都找不到的杂鱼～快检查密码和等级设置啦笨蛋！(╯‵□′)╯︵┻━┻",
	Auth_HelperNotCreated:                                "辅助用户都没创建还想登录？杂鱼快去用户中心报到啦～",
	Auth_InvalidVersion:                                  "版本太旧了啦～杂鱼都不更新的吗？快给咱升级！(ﾉ≧∀≦)ﾉ",
	Auth_InvalidHelperUsername:                           "用户名设置得乱七八糟的～杂鱼快去用户中心重新设置啦！",
	Auth_InvalidToken:                                    "Token过期了啦～杂鱼连重新登录都不会吗？(￣^￣)ゞ",
	Auth_InvalidUser:                                     "无效用户什么的～杂鱼是哪里来的冒牌货呀？(¬_¬ )",
	Auth_ServerNotFound:                                  "找不到租赁服什么的～杂鱼服务器是对全宇宙开放的吗？(눈_눈)",
	Auth_UnauthorizedRentalServerNumber:                  "没授权的服务器还想进？杂鱼快去用户中心求许可啦～( ͡°ᴥ ͡° ʋ)",
	Auth_UserCombined:                                    "这个账号已经被吃掉啦～杂鱼要用新账号登录懂不懂？(´• ω •`)",
	Auth_FailedToRequestEntry_TryAgain:                   "入口请求失败什么的～杂鱼等会再试啦～（戳脸）",
	Auth_MessageFromAuthServer:  "来自验证服务器的消息:",
	Frame_Error_ContactWithAPI:                           "和API通信失败啦～杂鱼服务器在装高冷吗？(◔_◔)",
	Frame_Error_APIServerDown:                            "API服务器宕机了啦～管理员快去修理啦笨蛋！＞︿＜",
	Frame_Success_ConnectedToAuthenticationServer:        "和验证服务器贴贴成功～杂鱼还不快夸夸我！(✿◕‿◕✿)",
	Frame_CreateNewAccessWrapper:                         "正在给杂鱼制作访问包装器～要好好感恩哦～",
	Frame_InitializingConnection:                         "初始化连接什么的...杂鱼就乖乖等着吧～（甩尾巴）",
	Frame_GeneratingClientKeyPair:                        "在给杂鱼生成密钥对啦～转圈圈～(～￣▽￣)～",
	Frame_RetrievingBotInfo:                              "正在从服务器偷取机器人信息～杂鱼看不见看不见～",
	Frame_CreatingRakNetConnection:                       "用魔法搭建RakNet连接中～杂鱼不要偷看！(⁄ ⁄•⁄ω⁄•⁄ ⁄)",
	Frame_Encapsulating_the_data_frame_connection_layer:  "给数据帧穿小裙子封装中～欸嘿嘿～",
	Frame_Encapsulating_the_data_packet_connection_layer: "数据包也要戴上猫耳哦～完美封装完成喵！(ฅ^•ω•^ฅ)",
	Frame_GeneratingKeyLoginInfo:                         "生成超～重要的登录信息～杂鱼心跳加速了吗？",
	Frame_Error_InitializingConnection:                   "连接初始化失败什么的～杂鱼网络是土豆做的吗？(╬◣д◢)",
	Frame_LoginSequenceCompleted:                         "登录序列完成啦～杂鱼服务器准备好迎接本大人了吗？",
	Frame_SendingAdditionalInfo:                          "正在发送附加信息～杂鱼要心怀感激地收下哦～",
	Frame_PackingKeyData:                                 "把重要数据打包成礼物盒～蝴蝶结系好了喵～",
	Frame_MCPCheckChallenge:                              "正在欺负MCP检查挑战～轻松解决啦笨蛋！(￣▽￣)/",
	Frame_MCPCheckChallengeDone: "成功解决 MCP 检查挑战",
	Frame_LoginDone:                                      "成功闯入租赁服啦～杂鱼们快列队欢迎！ヽ(✿ﾟ▽ﾟ)ノ",
	Frame_CrashedNotice:                                  "呜哇！FunCore被杂鱼玩坏了啦～（蹲墙角画圈圈）",
	Frame_ConnectionClosed_RebootTip:                     "[Fun-Core] 已经坏掉%d次了啦～%.2f秒后再来挑战哦杂鱼～",
	Frame_LoginFailed_RebootTip:                          "[Fun-Core] 登录失败%d次了喵～%.2f秒后本大人会再来的！",
	Cmd_Error_SendSettingsCommand:                        "设置命令都发不出去～杂鱼通道堵塞了吗？(´-ω-`)",
	Cmd_Error_SendCommand:                                "命令发送失败～杂鱼没对准接口吧？(¬▂¬)",
	Cmd_Error_SendWSCommand:                              "Websocket命令发送失败～杂鱼的网络在打瞌睡吗？(－ｏ⌒)",
	Cmd_Error_SendCommandWithResponse:                    "命令和回应私奔了～杂鱼快去把他们抓回来！(╯°Д°)╯",
	Cmd_Error_SendCommandWithResponse_ParseOptions:       "解析选项失败～杂鱼写的什么奇怪东西呀！(╬￣皿￣)",
	Cmd_Error_SendCommandWithResponse_ParseResponse:      "回应解析不了～杂鱼的代码需要调教！(◣_◢)",
	Cmd_Error_SendWSCommandWithResponse:                  "Websocket私聊失败～杂鱼被拉黑了吗？( ˘•ω•˘ )",
	Client_Error_GetClientInfo:                           "连客户端信息都拿不到～杂鱼设备是史前古董吗？(；¬д¬)",
}

var I18nDict_zh_CN_Special2 map[uint16]string = map[uint16]string{
	Frame_Bootstarp:                                      "你妈骨灰拌饭呢这么急？启动...启动nmlgb！",
	Frame_StartLoginSequence:                             "狗东西听好！服务器号%s 密码保护带死妈buff%t",
	Frame_ConnectingToAuthenticationServer:               "给验证服务器%s全家上香（物理）",
	Frame_Error_ParseUrl:                                 "你妈今晚必死！地址都nm解析不来？",
	Utils_Error_DetailedError:                            "你妈死因：",
	Auth_BackendError:                                    "后端司马程序员在灵堂打胶是吧？",
	Auth_FailedToRequestEntry:                            "透你妈个批！密码设你妈防沉迷呢？",
	Auth_HelperNotCreated:                                "没辅助用户？你妈灵车漂移登记没？",
	Auth_InvalidVersion:                                  "版本比你妈寿衣还旧！更！给老子更！",
	Auth_InvalidHelperUsername:                           "辅助用户ID是你妈墓碑二维码？重刻！",
	Auth_InvalidToken:                                    "透批通行证烧给你妈了？重登！",
	Auth_InvalidUser:                                     "用户骨灰都扬了还登录nmb！",
	Auth_ServerNotFound:                                  "找个几把服！全员透批权限开没开？",
	Auth_UnauthorizedRentalServerNumber:                  "没给你妈交墓地管理费？速去上贡！",
	Auth_UserCombined:                                    "用户被狗粉丝拿去配阴婚了，换号！",
	Auth_FailedToRequestEntry_TryAgain:                   "透批闪退！过会给你妈坟头蹦迪再试",
	Auth_MessageFromAuthServer:  "来自验证服务器的消息:",
	Frame_Error_ContactWithAPI:                           "API在给你妈哭丧？通讯中断！",
	Frame_Error_APIServerDown:                            "API服务器灵堂麦片是吧？",
	Frame_Success_ConnectedToAuthenticationServer:        "成了！透穿验证服务器祖坟！",
	Frame_CreateNewAccessWrapper:                         "手搓你妈批发的透批许可证",
	Frame_InitializingConnection:                         "跟MC服务器对砍（典藏版）",
	Frame_GeneratingClientKeyPair:                        "手搓你妈棺材钉密钥对！",
	Frame_RetrievingBotInfo:                              "扒机器人裤衩（带经血版）",
	Frame_CreatingRakNetConnection:                       "给RakNet批道灌水泥",
	Frame_Encapsulating_the_data_frame_connection_layer:  "数据帧塞你妈裹尸袋",
	Frame_Encapsulating_the_data_packet_connection_layer: "数据包压成你妈骨灰砖",
	Frame_GeneratingKeyLoginInfo:                         "搓登录密钥？你妈今晚的批密码！",
	Frame_Error_InitializingConnection:                   "透MC服务器失败！你妈生你时没开指？",
	Frame_LoginSequenceCompleted:                         "透批圣遗物集齐！开灵车！",
	Frame_SendingAdditionalInfo:                          "空投你妈前列腺钙化报告",
	Frame_PackingKeyData:                                 "骨灰拌屎打包中",
	Frame_MCPCheckChallenge:                              "和MCP检查对砍武士刀（开刃）",
	Frame_MCPCheckChallengeDone: "成功解决 MCP 检查挑战",
	Frame_LoginDone:                                      "透烂！租赁服祖坟青烟！",
	Frame_CrashedNotice:                                  "我日你妈！FunCore 炸灵堂啦！",
	Frame_ConnectionClosed_RebootTip:                     "[Fun-Core] 连炸你妈%d次坟 %.2f秒后借尸还魂",
	Frame_LoginFailed_RebootTip:                          "[Fun-Core] 透批翻车%d次 %.2f秒后棺材冲浪",
	Cmd_Error_SendSettingsCommand:                        "设置指令被你妈吞了？剖腹产取！",
	Cmd_Error_SendCommand:                                "指令发射失败！闪现进你妈坟？",
	Cmd_Error_SendWSCommand:                              "Websocket指令空降你妈天灵盖！",
	Cmd_Error_SendCommandWithResponse:                    "指令对线被狗粉丝爆破！",
	Cmd_Error_SendCommandWithResponse_ParseOptions:       "解析选项？我看你妈像选项！",
	Cmd_Error_SendCommandWithResponse_ParseResponse:      "对面批话比你妈裹脚布还臭",
	Cmd_Error_SendWSCommandWithResponse:                  "Websocket对线被你妈棺材板夹断",
	Client_Error_GetClientInfo:                           "客户端信息？我看你像殡葬信息！",
}

var LangDict map[string]map[uint16]string = map[string]map[uint16]string{
	LanguageEnglish_US:        I18nDict_en_US,
	LanguageSimplifiedChinese: I18nDict_zh_CN,
	LanguageSpecialChinese:    I18nDict_zh_CN_Special,
	LanguageSpecial2Chinese:   I18nDict_zh_CN_Special2,
}

var I18nDict map[uint16]string

func Init(lang string) {
/*
	if lang != "en_US" && lang != "zh_CN" {
		I18nDict = LangDict["en_US"]
	}
*/
	I18nDict = LangDict[lang]
}

func T(code uint16) string {
	r, has := I18nDict[code]
	if !has {
		r, has = I18nDict_en_US[code]
		if !has {
			return "???"
		}
	}
	return r
}
