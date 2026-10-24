"""诊断 bootstrap 每一步，定位 PE 认证 resources_hash 计算问题。

对齐 g79client/globals.go:fetchG79PatchMetadata + fetchAndroidResourcesHash
"""

import hashlib
import io
import json
import sys
import zipfile

import requests

PACK_LIST_URL = "https://g79.update.netease.com/pack_list/production/g79_packlist_2"
PATCH_LIST_URL = "https://g79.update.netease.com/patch_list/production/g79_rn_patchlist"


def step(msg: str) -> None:
    print(f"\n{'='*60}\n{msg}\n{'='*60}")


def main() -> int:
    sess = requests.Session()
    sess.headers.update({
        "User-Agent": "libhttpclient/1.0.0.0",
        "Accept-Encoding": "gzip",
        "content-type": "text/plain",
        "cache-control": "no-cache",
    })

    # ── 1. pack_list → engine_version ──
    step("1. 获取 pack_list")
    resp = sess.get(PACK_LIST_URL, timeout=15)
    print(f"status={resp.status_code} len={len(resp.content)}")
    pack_list = resp.json()
    print("channels:", list(pack_list.keys()))
    for ch in ("netease", "android", "ios"):
        if ch in pack_list:
            print(f"  {ch}: version={pack_list[ch].get('version')!r} "
                  f"text={pack_list[ch].get('text')!r}")
    engine_version = pack_list.get("netease", {}).get("version", "")
    if not engine_version:
        print("✗ netease 渠道无 version")
        return 1
    print(f"→ engine_version = {engine_version}")

    # ── 2. patch_list → android 版本列表 + urlNew ──
    step("2. 获取 patch_list")
    resp = sess.get(PATCH_LIST_URL, timeout=15)
    print(f"status={resp.status_code} len={len(resp.content)}")
    patch_info = resp.json()
    android_versions = patch_info.get("android", [])
    print(f"android versions ({len(android_versions)}): {android_versions}")
    print(f"urlNew = {patch_info.get('urlNew')!r}")
    print(f"url    = {patch_info.get('url')!r}")

    # 选择匹配版本（对齐 selectAndroidPatchVersion）
    parts = engine_version.split(".")
    base = ".".join(parts[:2]) if len(parts) >= 2 else ""
    print(f"engine base prefix = {base!r}")
    selected = None
    for v in reversed(android_versions):
        if v.startswith(base):
            selected = v
            break
    if not selected:
        print(f"✗ 无匹配 {base} 的 android patch 版本")
        return 1
    print(f"→ selected patch version = {selected}")

    # ── 3. manifest.zip ──
    base_url = (patch_info.get("urlNew") or patch_info.get("url") or "").rstrip("/")
    pre = f"{base_url}/android_{selected}/{selected}/android"
    manifest_url = f"{pre}/manifest.zip"
    step(f"3. 获取 manifest.zip\n   {manifest_url}")
    resp = sess.get(manifest_url, timeout=15)
    print(f"status={resp.status_code} len={len(resp.content)}")
    if resp.status_code != 200:
        print(f"✗ manifest 获取失败: {resp.status_code}")
        print(f"  body[:200]: {resp.text[:200]}")
        return 1
    try:
        zf = zipfile.ZipFile(io.BytesIO(resp.content))
        names = zf.namelist()
        print(f"zip entries: {names}")
        manifest_data = json.loads(zf.read(names[0]))
    except Exception as e:
        print(f"✗ manifest 解析失败: {e}")
        return 1
    assets = manifest_data.get("assets", {})
    vanilla_md5 = assets.get("vanilla.mcp", {}).get("md5", "")
    vanilla_patch_md5 = assets.get("vanilla_patch.mcp", {}).get("md5", "")
    print(f"vanilla.mcp md5       = {vanilla_md5!r}")
    print(f"vanilla_patch.mcp md5 = {vanilla_patch_md5!r}")
    hash_base = vanilla_md5 + vanilla_patch_md5
    print(f"→ hash_base = {hash_base!r}")

    # ── 4. RN bundle ──
    rn_url = f"{pre}/rn/index.bundle.backup"
    step(f"4. 获取 RN bundle\n   {rn_url}")
    rn_hash = ""
    try:
        resp = sess.get(rn_url, timeout=15)
        print(f"status={resp.status_code} len={len(resp.content)}")
        if resp.status_code == 200:
            payload = resp.content
            try:
                with zipfile.ZipFile(io.BytesIO(payload)) as zf:
                    payload = zf.read(zf.namelist()[0])
                    print(f"  (解压 zip 后 len={len(payload)})")
            except Exception:
                pass
            rn_hash = hashlib.md5(payload).hexdigest()
            print(f"→ rn_hash = {rn_hash!r}")
        else:
            print(f"  RN bundle 失败 status={resp.status_code}")
            # 尝试备选路径
            for alt in ("rn/index.bundle", "rn/index.android.bundle"):
                alt_url = f"{pre}/{alt}"
                print(f"  尝试 {alt_url}")
                r2 = sess.get(alt_url, timeout=15)
                print(f"    status={r2.status_code} len={len(r2.content)}")
                if r2.status_code == 200:
                    payload = r2.content
                    try:
                        with zipfile.ZipFile(io.BytesIO(payload)) as zf:
                            payload = zf.read(zf.namelist()[0])
                    except Exception:
                        pass
                    rn_hash = hashlib.md5(payload).hexdigest()
                    print(f"    → rn_hash = {rn_hash!r}")
                    break
    except Exception as e:
        print(f"  RN bundle 异常: {e}")

    # ── 5. 最终 resources_hash ──
    final = hashlib.md5((hash_base + rn_hash).encode()).hexdigest()
    step("5. 最终结果")
    print(f"engine_version        = {engine_version}")
    print(f"patch_version         = {selected}")
    print(f"hash_base (vanilla)   = {hash_base}")
    print(f"rn_hash               = {rn_hash!r}")
    print(f"→ resources_hash      = {final}")

    # 对齐 versionMessage
    G79_LIBRARY_HASH = "815873bdd82cb7a4ce422da361f7d60d"
    G79_SIGNATURE_HASH = "2b3e7ca013bb30a74d822579860c042b"
    seed = "00000000-0000-0000-0000-000000000000"
    version_message = engine_version + G79_LIBRARY_HASH + selected + final + G79_SIGNATURE_HASH + seed
    print(f"\nversionMessage (示例 seed) = {version_message}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
