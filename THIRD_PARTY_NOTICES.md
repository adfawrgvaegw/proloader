# Third-party components

The isolated Java→Bedrock palette bridge in `tools/blockmapper` uses
[`github.com/Yeah114/blocks`](https://github.com/Yeah114/blocks), pinned to
`v0.0.0-20251025181709-54ab0b294dfe` (AGPL-3.0). Its license is retained in
`tools/blockmapper/COPYING`; dependencies and checksums are recorded in its
`go.mod` and `go.sum`. This is the mapper used by the cached
Fatalder/WaterStructure reference. It performs conversion locally and has no
game/account network access. Preserve the component's license and corresponding
source when distributing this helper. The repository's MIT declaration does not
replace this component's license.

The Python import workflow and terminal interface are project code. Complete
upstream Fatalder source, including its original UI and assets, is included for
source availability; the Python interface uses the local native adapters.

## Fatalder integration (2026-10-20)

The combined program is distributed under AGPL-3.0-only. Original project MIT
terms are retained in LICENSE-MIT. The complete local Fatalder archive, including
FunShuttler, WaterStructure, bdump, and restored missing archive files, is retained
under third_party/fatalder with its license notices and dependency lockfiles.
The missing g79client submodule is supplied from the existing UCKETX fork cache.
Exact file hashes and the original archive SHA-256 are in
third_party/fatalder-source.json; the archive's upstream commit is unknown.

The native worker directly calls the unmodified Fatalder chunk_fill package.
cmd/proloader-plan/main.go is our adapter. The Python process remains the sole
game connection owner. The complete upstream CLI builds as bin/fatalder-upstream,
but is not launched alongside that connection. Source presence and successful
compilation do not establish live acceptance of every upstream feature. NBT
console/assigner and export integrations require separate runtime adapters and
acceptance; their original source is included, not advertised as working here.

When distributing, retain AGPL text, upstream notices, corresponding source,
local modifications, build scripts and required dependency sources/availability.
Subprocess boundaries do not remove the combined program's license obligations.

## Public source snapshot modifications

Two upstream files were sanitized for this source snapshot: `build-and-release.sh`
now reads GH_PAT/GH_TOKEN from the environment, and
`utils/auto_updater/server/constants.py` reads GITHUB_API_KEY from the environment.
Hardcoded GitHub credentials are not distributed. Fixed protocol PEM constants
remain upstream public protocol material; see SECURITY.md.

`third_party/fatalder-source.json` preserves provenance and hashes of the supplied
upstream archive. Those hashes describe that input, not subsequent local adapters
or sanitization. RELEASE_MANIFEST.json records the public snapshot file hashes.

The pinned `github.com/Happy2018new/nemc-tan-lobby-solver v0.2.5` dependency remains externally resolved through a public Go module mirror. It is not vendored into this snapshot. The resolved module checksum is retained in the Go lockfiles.
