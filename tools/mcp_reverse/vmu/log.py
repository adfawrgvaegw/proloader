from __future__ import annotations
import logging

logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s.%(msecs)d] %(levelname)s: %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)

logger = logging.getLogger("default")

def get_logger(name: str) -> logging.Logger:
    return logging.getLogger(name)