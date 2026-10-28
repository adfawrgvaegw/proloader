# Proloader maintenance

Use the repository skill at `.agents/skills/proloader-maintenance/SKILL.md` for
Minecraft/NetEase version updates, Fatalder synchronization, acceptance tests,
and preparation of a source release. Install or link that directory into the
agent's skill directory when automatic discovery is desired.

## Agent-operated version maintenance

Ask the agent to read `.agents/skills/proloader-maintenance/SKILL.md` and maintain
the current version. The agent runs the internal automation, reads its compact
JSON result, investigates changed authentication when needed, and reports evidence.
Users do not need to execute a terminal command or a desktop launcher.

The existing `proloader维护.command` remains an optional legacy shortcut; it is
not required by the skill workflow. Local credentials stay in place. See the
skill's `references/automation.md` for internal flags and evidence boundaries.

## Runtime and source

Python owns the rental connection, private bot store, task checkpoints and CLI.
`third_party/fatalder` contains the complete available upstream source plus our
native adapter at `cmd/proloader-plan`. Its original `chunk_fill` package is called
directly. Native fill grouping remains 48×48, ordered inside source-aligned
144×144 coverage tiles. Main-layer placement uses one owned ticking area of at
most 100 world chunks, covering the whole command footprint when possible;
otherwise tiles replace one another after draining. No placement teleports are
needed. Positive server predicates prove readiness; unknown columns retry within
a bound. Native full readback and NBT interactions may move. The command token
bucket defaults to rate=4000 and burst=10 by user choice. With block verification
disabled, processing probes run every 256 commands with a bounded 768-command
scheduling window, drained before moves/NBT/phase completion. Missing probe replies
are recorded and reduce pacing to 250; they never certify placement. Position
confirmation remains required; the following fixed wait is 0.25 seconds. Block
verification modes retain 32-command checks with a bounded 512-command response
pipeline, drained before area changes and phase completion. Interrupted imports
roll back their full saved processing window (including checked imports).
Current real-session manual repairs scan only the bot's confirmed 16x16 column
clipped to the source; external teleports trigger the next local action.

The combined distribution is AGPL-3.0-only. Keep original MIT terms in
LICENSE-MIT, upstream notices, source changes and dependency lockfiles.
`third_party/fatalder-source.json` records the supplied snapshot; an unknown
upstream commit must remain explicitly unknown.

Command-block configuration staging is connected through the Python session:
`--command-blocks stage` writes redstone-required, first-tick-disabled settings
and requires exact shell states plus SubChunk NBT readback even with `--verify none`.
The source activation settings remain part of the plan fingerprint. External
redstone can still trigger staged commands. The mode is explicit; default reject
stops before connection, and explicit skip reports shells only. Java commands
are not converted. The native `cmd/proloader-nbt` helper has no game connection;
it reuses upstream packet, block-actor and subchunk codecs.

Non-command Bedrock data is planned by `proloader/block_entities.py` and applied
after shell delivery through the same Python session. Plain container slots use
upstream-style replaceitem commands; sign text uses the independent
`tools/nbtcodec` adapter importing the unmodified FunShuttler NetEase click/auth
packet codecs. This path has offline tests, not live acceptance. Sign execution
is gated to the current NetEase860 session, but that gate alone does not prove
wire compatibility. Unsupported fields/items are explicitly inventoried in the
plan; supported-subset completion must not be presented as arbitrary NBT import.
See `docs/OTHER_NBT.md`. Local repairs have separate NBT cursors and resume
must read before writing; failed readback cannot advance the durable cursor.

Command-block gamerules are managed manually by the user. The importer must
not query or toggle commandblocksenabled. Wizard and CLI imports disable
automatic reconnection by default; preserve checkpoints for manual resume.

Configuration readback progress is independent of shell placement. Resume after
entering the NBT phase must not replay the first shell probe. Manual repair
uses separate local NBT cursors and rechecks only actors in the target column.
Global cursors and pending differences outside the column remain unchanged.
Packet delivery alone never counts as NBT success. Historic NBT runtime fields
are retained in parsed mcstructure source but not restored by CommandBlockUpdate.
Other block entities are connected for preflight-supported plain containers and
sign text/formatting. Arbitrary item tags, history and source automatic activation
remain outside this delivery contract; keep unsupported counts visible.
Live permission/switch/protocol/trigger acceptance and a fresh ten-round connection
regression remain pending; offline results must not be described as live success.

## Build and verify

Use a source checkout with Python 3.9+ and a Go toolchain satisfying the vendored
go.mod. A normal wheel does not currently package the native source resources.

```sh
python3 -m pip install -e '.[dev]'
python3 -m proloader doctor --json
python3 -m pytest -q
```

`python3 -m proloader` opens the guided menu. `plan` is offline, `import` targets
real rental servers, `tasks` reads checkpoints, `resume` validates the saved plan,
and `simulate` contains older development-only tools. Never mistake a successful
simulation for online acceptance.

With world verification enabled, record mismatches without aborting the initial
send. Queue the entire suspect batch and final sample differences, then replay
after all initial commands have been processed. At most two repair rounds run
per invocation; unresolved commands remain in `repair_pending` and the outcome
is `verification_failed`. Keep pending work durable until its recheck succeeds,
including across interruption/resume. `--verify none` skips block checks and
repairs, while keeping position confirmation and server-processing barriers.
Connection failures still preserve the checkpoint; no default auto-reconnect. Healthy-connection NBT failures receive bounded local recovery, then the interactive CLI retains the connection and accepts repair/done instead of throwing away the task.

See `docs/ACCEPTANCE_20261028.md` for upstream SubChunk/hash comparison, bounded scan caching, merged differential repair, and the shared 0.5-second game/terminal phase counters.

The planner builds automatically on first use. To verify both native targets:

```sh
cd third_party/fatalder
go build -o ../../bin/fatalder-plan ./cmd/proloader-plan
go build -o ../../bin/fatalder-upstream ./cmd/cli
go build -o ../../bin/fatalder-nbt ./cmd/proloader-nbt
go test ./cmd/proloader-nbt ./cmd/proloader-world
```

Create the output bin directory first if absent. Do not run the upstream CLI
against the same identity as the Python connection.

Network-affecting changes require an authorized sequential ten-round first-attempt
acceptance run: full initialization, two read-only probes, at least 30 seconds
online per round, and verified shutdown. A failed round resets that experiment;
record failures and diagnose rather than repeatedly reconnecting until lucky.

## Private data and publication

Keep local identities in `~/.proloader` and diagnostic material in the local
working copy. Never delete or move them as part of preparing open source.
Generate a separate source directory using the maintenance skill's export script,
then review it before any upload. Public source must not include local cache,
private handoff documents, personal AGENTS instructions, credentials, packet
captures, private building assets, or machine-specific absolute paths.

Large-building tests precede release preparation. Account/server/coordinate
examples must be placeholders. GitHub creation, push and release publication
follow the owner's authorization; no scheduled upload is implied.
