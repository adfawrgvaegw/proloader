# 凭据与本地数据

不要在 issue、PR 或日志附件中提交 Cookie、sauth_json、PE user_token、短信码、服务器密码或完整认证响应。报告登录故障前先脱敏。

身份默认保存在 `~/.proloader/bots`，任务位于 `~/.proloader/imports`。源码目录的忽略规则无法保护该目录的手动打包；分享时只分享已审查的源码。若凭据意外公开，应撤销相关会话并重新登录。

上游源码包含协议使用的固定 PEM 密钥常量（`utils/user_center/keys.go` 与 FunShuttler 的 shuttler 入口）。它们属于上游公开协议实现，不是本地用户登录私钥；保留源码用途与许可说明，不能把这些公开常量作为应用自身的秘密。
