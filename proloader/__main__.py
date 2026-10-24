"""允许 `python -m proloader` 启动 CLI。"""

from __future__ import annotations

from proloader.cli.main import app


if __name__ == "__main__":
    app()
