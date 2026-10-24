#!/bin/zsh
cd -- "${0:A:h}/.." || exit 1
python3 -m tools.diagnostics.login_netease_mobile_and_verify
result=$?
printf '\n登录检查结束（退出码 %s）。按回车关闭。\n' "$result"
read
