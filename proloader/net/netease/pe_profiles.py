"""Verified official Android 3.9.15 authentication metadata, not an arbitrary version fallback."""
OFFICIAL_ENGINE='3.9.15.297907'
# Public official manifests + downloaded MCP MD5, selected auth methods compared
# offline (not executed). New patch pairs require a new evidence-backed entry.
OFFICIAL_PATCHES={
    ('3.9.41.300665','2d2833df3518783e52130aa8951c548f'),
    ('3.9.42.301055','65ce57607cdb2f70a040a1db7149279d'),
}


class PEMetadataMismatchError(ValueError):
    def __init__(self,engine,patch):
        self.engine,self.patch=engine,patch
        super().__init__('official PE profile does not match inspected metadata')


class PEAuthenticationError(RuntimeError):
    def __init__(self,code):
        self.code=code if type(code) is int else None
        super().__init__(f'PE 认证失败 (code: {self.code})')
