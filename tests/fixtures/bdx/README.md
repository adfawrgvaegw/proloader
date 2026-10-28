# BDX fixtures

These tiny generated fixtures are test data, not user builds. Each starts with
`BD@` plus Brotli-compressed `BDX\0test-author\0`, a manually specified BDX
command stream, and opcode 88 (except the deliberately unterminated fixtures).
They were compressed with github.com/andybalholm/brotli v1.1.1; runtime-pool 117
uses entry 1 from the bundled upstream table. Tests require no Python Brotli
dependency and decode bytes through the actual unmodified bdump dependency.

Cases cover legacy and state placements, overwrite semantics, negative and
multi-width movement, command metadata and standard little-endian named-root
NBT, chest slots, undefined constants, unknown blocks/opcodes/pools, truncated
commands, oversized bounds and a bogus debug-data allocation length.
