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
directly. Native construction grouping is 48×48, and the default command token
bucket defaults to rate=4000 and burst=10 by user choice. With block verification
disabled, processing probes run every 256 commands with a bounded 768-command
scheduling window, drained before moves/NBT/phase completion. Missing probe replies
are recorded and reduce pacing to 250; they never certify placement. Position
confirmation remains required; the following fixed wait is 0.25 seconds. Block
verification modes retain their existing 32-command checks. Interrupted unchecked
imports and manual repairs roll back the full saved processing window.

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

Configuration readback progress is independent of shell placement. Resume after
entering the NBT phase must not replay the first shell probe. Manual repair
resets the NBT cursor and reapplies/readbacks configuration after shell replay.
Packet delivery alone never counts as NBT success. Historic NBT runtime fields
are retained in parsed mcstructure source but not restored by CommandBlockUpdate.
Remaining NBT console/assigner, exports and other block entities are not connected.
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
Connection/teleport failures still stop safely and preserve the checkpoint.

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
