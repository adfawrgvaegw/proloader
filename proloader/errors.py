"""统一异常体系。

所有 proloader 内部异常都继承 ProloaderError，方便上层统一捕获。
"""

from __future__ import annotations


class ProloaderError(Exception):
    """所有 proloader 异常的基类。"""


# ── 解析相关 ──────────────────────────────────────────────────────────────────
class ParseError(ProloaderError):
    """建筑文件解析失败（格式错误、不支持的版本等）。"""


class UnsupportedFormatError(ParseError):
    """不支持的文件格式（如未知扩展名）。"""


# ── 后端相关 ──────────────────────────────────────────────────────────────────
class BackendError(ProloaderError):
    """后端操作失败。"""


class NotSupportedError(BackendError):
    """后端不支持该操作（如 SimulatedBackend 不需要 teleport，或后端不支持读区域）。

    调度器捕获此异常后会退化为更简单的策略（如全量放置）。
    """


class BackendDisconnectedError(BackendError):
    """后端连接断开（网络问题、被踢出等）。"""


# ── 认证相关 ──────────────────────────────────────────────────────────────────
class AuthError(ProloaderError):
    """网易认证失败。"""


class CredentialsRequiredError(AuthError):
    """需要用户提供凭证（token 文件 / cookie 文件 / 账号密码）。

    网易认证无法全自动（涉及风控、验证码），必须用户配合。
    """


# ── 调度相关 ──────────────────────────────────────────────────────────────────
class SchedulerError(ProloaderError):
    """调度器错误。"""


class RegionLockTimeoutError(SchedulerError):
    """区域锁获取超时（可能死锁）。"""


# ── 快照/会话相关 ─────────────────────────────────────────────────────────────
class SnapshotError(ProloaderError):
    """快照读写失败。"""


class SessionNotFound(ProloaderError):
    """导入会话不存在（resume/rollback 时）。"""


class SessionStateError(ProloaderError):
    """会话状态错误（如对已 DONE 的会话 resume）。"""
