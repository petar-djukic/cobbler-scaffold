<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Git Tagging Convention

We use a semver-inspired tagging scheme: `v[REL].[DATE].[REVISION]`.

| Segment  | Description                          | Example    |
|----------|--------------------------------------|------------|
| REL      | Release number, incremented manually | 0, 1, 2   |
| DATE     | Date in YYYYMMDD format              | 20260213   |
| REVISION | Revision within the same date        | 0, 1, 2   |

Examples: `v0.20260213.0`, `v0.20260213.1`, `v1.20260301.0`.

The REVISION starts at 0 for each new date and increments if multiple tags are created on the same date.

When the user asks to tag, use this format. The orchestrator also uses this scheme when completing a generation (GeneratorStop): the merged code is tagged `v0.YYYYMMDD.N` and the requirements state is tagged `v0.YYYYMMDD.N-requirements`.
