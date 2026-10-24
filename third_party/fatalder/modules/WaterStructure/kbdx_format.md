# KBDX Binary Structure Format

## Overview
- Implemented by `Kbdx` in `package/MCStructureManage/StructureRUNAWAY/classic.py` with codec support in `package/MCStructureManage/codec.py`.
- File extension: `.kbdx` (binary, little-endian).
- Purpose: compact RunAway-official structure format bundling a fixed-width block table and trailing JSON metadata for palette entries and block entities.
- Content includes:
  - Absolute block coordinates and palette references.
  - Optional block-entity payloads (command blocks, etc.).
  - Palette map from block identifiers to palette indices.

## File Layout
```
+---------------------------+
| uint32 block_count        | 4 bytes little-endian
+---------------------------+
| block table               | block_count entries × 20 bytes
+---------------------------+
| JSON metadata             | UTF-8 text until EOF
+---------------------------+
```

### Block Table Entry (`<iiiII`)
Each entry stores five values in little-endian order:
| Offset | Field | Type | Description |
| ------ | ----- | ---- | ----------- |
| 0 | `x` | int32 | Absolute X coordinate. |
| 4 | `y` | int32 | Absolute Y coordinate. |
| 8 | `z` | int32 | Absolute Z coordinate. |
| 12 | `palette_index` | uint32 | Palette index defined in the JSON tail. Must be ≥ 1. |
| 16 | `aux` | uint32 | Block metadata / data value. |

`Kbdx.from_buffer` reads entries via `struct.Struct('<iiiII').iter_unpack(...)`; `save_as` writes them with the same format.

### JSON Metadata Section
The remainder of the file is UTF-8 JSON with the shape:
```json
{
  "BlockEntityData": [
    {
      "id": "command_block", "x": 0, "y": 64, "z": 0,
      "Command": "/say hi", "CustomName": "@",
      "TickDelay": 0, "isConditional": 0,
      "redstone": false, "TrackOutput": true,
      "ExecuteOnFirstTick": true, "LastOutput": "",
      "Mode": 0
    }
  ],
  "minecraft:stone": 1,
  "minecraft:command_block": 2
}
```
- `BlockEntityData` *(optional array)*: list of dictionaries describing block-entity payloads. Known fields include:
  - `id`: block identifier without namespace (e.g. `command_block`).
  - `x`, `y`, `z`: absolute coordinates (int).
  - Command-block specific: `Command` (string), `CustomName` (string), `TickDelay` (int), `isConditional` (int/bool), `redstone` (bool), `ExecuteOnFirstTick` (bool), `TrackOutput` (bool), `LastOutput` (string), `Mode` (0 command, 1 repeating, 2 chain).
  - Other tile entities may provide additional keys (e.g., `Text` for signs, inventory arrays for containers). The current codec only materialises command-block data when decoding.
- Remaining key/value pairs define the **palette map**: JSON keys are block identifiers (strings) and values are palette indices (ints) referenced by the block table.

#### Block Entity Payload Types
| Block entity type | Expected fields | Notes |
| ----------------- | --------------- | ----- |
| Command blocks (`id`: `command_block`, `repeating_command_block`, `chain_command_block`) | `Command`, `CustomName`, `TickDelay`, `isConditional`, `redstone`, `ExecuteOnFirstTick`, `TrackOutput`, `LastOutput`, `Mode`, `x`, `y`, `z` | `Mode` maps to Bedrock IDs (`0` command, `1` repeating, `2` chain). The codec converts these entries back into NBT when decoding. |
| Signs (`id` ends with `_sign` or `hanging_sign`) | `Text` or `FrontText` (object containing `Text`), positional fields | Present in some exports; current codec preserves the dictionary in `block_nbt` but does not rehydrate NBT automatically. |
| Containers / inventories | `Items`: array of item dicts (`Name`, `Damage`, `Count`, `Slot`), plus positional fields and optional `id` | Not emitted by the bundled encoder yet, but original `.kbdx` files may include them. Consumers should iterate `Items` to rebuild chest contents. |
| Other tile entities | Arbitrary keys defined by the exporter, along with `id`, `x`, `y`, `z` | Loader keeps these dictionaries untouched inside `block_nbt`; custom tooling must interpret them. |

When the current codec decodes a `.kbdx`, it only transforms command-block entries into Bedrock NBT; all other entity dictionaries remain available in `Kbdx.block_nbt` for external handling.

## Loading Workflow (`Kbdx.from_buffer`)
1. Read the 4-byte little-endian `block_count` header.
2. Consume `block_count * 20` bytes and unpack sequential `<iiiII>` entries into tuples `(x, y, z, palette_index, aux)`.
3. Parse the remaining bytes as UTF-8 JSON:
   - `BlockEntityData` → `self.block_nbt` (list of dicts, defaults to `[]`).
   - Remaining pairs → `self.block_palette` (string → int mapping).
4. Helper methods:
   - `get_volume()` derives min/max coordinates directly from `self.blocks`.
   - `error_check()` enforces tuple length, integer types, `palette_index >= 1`, and palette value types.

## Saving Workflow (`Kbdx.save_as`)
1. (Optional) call `error_check()`.
2. Write `len(self.blocks)` as `uint32` little-endian.
3. Pack each tuple `(x, y, z, palette_index, aux)` with `<iiiII>` and append.
4. Construct JSON payload:
   ```python
   payload = {"BlockEntityData": list(self.block_nbt)}
   for block_id, index in self.block_palette.items():
       payload[block_id] = index
   ```
5. Serialise payload using `json.dumps(..., separators=(',', ':'))`, encode as UTF-8, and append to the file.

## Codec Behaviour (`KBDX` in `codec.py`)
- `verify` ensures the stream is binary: reads `block_count`, skips the block table, then attempts `json.load` on the remainder.
- `decode` performs:
  1. `Kbdx.from_buffer` → loads blocks, palette, and block entities.
  2. Calls `get_volume()` to size the in-memory structure.
  3. Builds a reverse palette map (`index → block_id`) and populates world blocks. Command blocks receive generated NBT from `BlockEntityData` entries; other entities are currently ignored.
- `encode` writes `.kbdx` by:
  1. Building a palette map (`block.runawayID` → index) from the internal block palette.
  2. Appending block tuples `(x, y, z, palette_index, aux)`.
  3. Translating command-block NBT into `BlockEntityData` dictionaries. (Other tile entities are not exported yet.)

## Parsing Checklist
1. Read header to obtain `block_count`.
2. Unpack block table entries, caching `(x, y, z, palette_index, aux)`.
3. Parse JSON tail to build palette (`str → int`) and optional `BlockEntityData` list.
4. Reverse palette if needed (`index → block_id`) to resolve actual block names.
5. Combine block tuples and palette to place blocks; use `aux` for metadata.
6. Handle block entities according to your needs (rebuild NBT for command blocks, parse sign/container fields, etc.).

## Example
```python
import json, struct

def read_kbdx(path):
    with open(path, 'rb') as f:
        count = int.from_bytes(f.read(4), 'little')
        entry = struct.Struct('<iiiII')
        blocks = list(entry.iter_unpack(f.read(count * entry.size)))
        meta = json.loads(f.read().decode('utf-8'))
    palette = {k: v for k, v in meta.items() if k != 'BlockEntityData'}
    block_entities = meta.get('BlockEntityData', [])
    return blocks, palette, block_entities
```

## Notes & Caveats
- Coordinates are absolute; subtract the minima if you need to reposition the structure origin.
- Palette indices in current exports start at 1. Index `0` is unused and causes loader validation to fail.
- `BlockEntityData` is optional. Absence implies no tile entities.
- JSON keys beyond recognised palette entries are ignored but preserved when round-tripping.
- Always read/write using explicit `<` in struct format strings to maintain portability.
- Ensure the file pointer sits at the start of the JSON tail before invoking `json.load` when implementing custom readers.
- Command block metadata maps `Mode` (`0/1/2`) to `minecraft:command_block`, `minecraft:repeating_command_block`, and `minecraft:chain_command_block` respectively when rehydrating NBT.

This expanded guide mirrors the behaviour of the RunAway KBDX tooling bundled with the project and should help you implement compatible readers and writers.
