#!/bin/bash
# Launch the actual rental CLI; keep private bot data in ~/.proloader.
set -u
PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)" || exit 1
cd "$PROJECT_DIR" || exit 1
if [ -x "$PROJECT_DIR/.venv/bin/python" ]; then
    PYTHON_BIN="$PROJECT_DIR/.venv/bin/python"
else
    PYTHON_BIN="$(command -v python3 || true)"
fi
if [ -z "$PYTHON_BIN" ]; then
    echo "未找到 Python 3，请先安装 Python 3.9 或更新版本。"
    exit 1
fi
if ! "$PYTHON_BIN" -c 'import numpy, rich, requests, msgpack, cryptography' >/dev/null 2>&1; then
    echo "正在项目独立环境中安装运行依赖…"
    "$PYTHON_BIN" -m venv "$PROJECT_DIR/.venv" || exit 1
    PYTHON_BIN="$PROJECT_DIR/.venv/bin/python"
    "$PYTHON_BIN" -m pip install -e '.[cli,bedrock,netease]' || exit 1
fi
"$PYTHON_BIN" -m proloader "$@"
RESULT=$?
if [ "$#" -eq 0 ] && [ -t 0 ]; then
    printf '\nproloader 已退出，按 Enter 关闭窗口。'
    read -r
fi
exit "$RESULT"
