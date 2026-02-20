<\!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Command: Bootstrap Project

I'm starting a new project and need you to help me create the initial epics and issues to structure the work.

First, read **docs/CONSTITUTION-design.yaml** to understand the documentation format rules and available document types (VISION, ARCHITECTURE, PRD, use case, test suite, etc.).

Then ask me questions to understand:
1. What problem I'm trying to solve
2. What the solution will do
3. What success looks like
4. What the major components are
5. How those components fit together
6. Key design decisions and why

Based on my answers, create epics and issues using the bead system:

## Epic Structure
Create a main epic that captures the overall project vision and scope.

## Child Issues

Break down the work into specific issues for:

- **Documentation**: VISION.yaml, ARCHITECTURE.yaml, PRDs
- **Core Implementation**: Major components and features
- **Infrastructure**: Build, test, deployment setup
- **Integration**: Component wiring and data flow

## Issue Creation
Follow the detailed process in `.claude/commands/make-work.md` for:
- Issue format and structure
- Using `bd create` and dependency management
- Proper syncing and committing

Start by asking me questions to understand the project.
