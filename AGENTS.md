# Public contributor instructions

Read README.md and MAINTAINERS.md before making changes. Use the repository proloader-maintenance skill for version maintenance.

- Python exclusively owns each live game connection. Native Go helpers perform offline work only.
- Keep account tokens, cookies, phone numbers, server passwords, logs, packet captures, buildings and local runtime data out of commits. Use synthetic test identities.
- Do not connect, send SMS, change a world or publish without the user's task authorizing it.
- Run offline tests and relevant native builds. Separate sent commands, block-name verification and complete states/NBT acceptance in reports.
- Preserve AGPL-3.0, original MIT, upstream licenses and dependency lockfiles.
