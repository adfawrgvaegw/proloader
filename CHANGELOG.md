# Changelog

## 0.1.2 - 2026-11-11

- Adapt exact 3.10.5 engine and 3.10.12/3.10.15 patch metadata with engine-specific native PE signatures.
- Support rental protocol 898 with GameVersion 1.21.130, Certificate login, modern teleport acknowledgements and reviewed NBT wire layouts.
- Add bounded public-manifest GET recovery and safe failure diagnostics while preserving first-attempt game connection acceptance.
- Expand the maintenance skill with static APK, certificate and bytecode inspection tools and complete recovery guidance.
- Record a 60-second read-only hold and an independent ten-round connection series for the development build. See docs/MAINTENANCE_20261111.md.

## 0.1.1 - 2026-10-28

- Adapt the reviewed 3.9.43 patch pair and statically decode the eight-field dynamic MCP challenge; reject unknown templates.
- Wait for explicitly starting rental worlds; preserve transport close reasons and stop on failure without default reconnect.
- Add BDX import, stationary loading windows, differential local repair, and persistent offline codecs for supported NBT delivery.
- Verify three 168-block house imports with full readback and a 60-second hold each; complex NBT and local repair optimizations still need live acceptance.
- Preserve existing public-copy parser, privacy and fresh-runner fixes. See docs/ACCEPTANCE_20261028.md.

## 0.1.0 - 2026-10-24

Initial experimental source release: rental-server login, building import, interactive CLI, task recovery, AI command-block tools, and offline native planning.

- Include Fatalder source and notices under the combined AGPL-3.0-only license.
- Sanitize personal examples, machine paths and upstream hardcoded GitHub tokens.
- Reject truncated mcstructure primary layers, invalid palette references, and duplicate block entity coordinates.
- Add GitHub CI for Python tests and native build checks.

General block entity NBT, secondary liquids, full world cloning and standalone wheel packaging remain incomplete. See README.md and RELEASE_REVIEW.md.

Build portability fixes: use a checksum-verified public Go mirror fallback for an unavailable upstream dependency, and preserve the absence of a default event loop in the Python 3.11 CLI test.
