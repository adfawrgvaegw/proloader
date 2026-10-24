"""Build environment for offline native helpers. Never changes Go global settings."""
import os


def go_build_environment():
    environment = dict(os.environ)
    # One pinned upstream module is no longer served by the default proxy.
    # Retain explicit user overrides and Go's normal checksum verification.
    environment.setdefault("GOPROXY", "https://proxy.golang.org,https://goproxy.cn,direct")
    return environment
