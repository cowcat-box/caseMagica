# CaseMagica

Current version: <strong>v0.4.1</strong> · Beta

An AI creative platform for novel writing and AI role-playing games.

English | [中文](README.md)

## Overview

CaseMagica provides:

- Writing workspace: chapter management, Works Catalog export/import, book settings (outline/rules/group outlines) export/import, lore library, style references, version control
- Continuation exploration: explore multiple continuation candidates from the current chapter (direction/excerpt), editable and commitable as new chapters or appends
- Game mode: interactive stories, story director, memory system, image generation
- AI integration: Agents, Skills (with AI Chat crafting), Subagent Workflows, automation tasks

## Install & Run

Download the archive for your platform from [Releases](https://github.com/cowcat-box/caseMagica/releases), extract it, and run:

```bash
# macOS / Linux
./casemagica

# Windows
casemagica.exe
```

Your browser will open `http://localhost:8080`.

## Build from Source

Requires Go 1.26+, Node.js 20+, and pnpm:

```bash
git clone https://github.com/cowcat-box/caseMagica.git
cd caseMagica

# Development mode (frontend hot-reload)
./scripts/bootstrap.sh

# Or production build
./scripts/build.sh
cd output && ./casemagica
```

## License

[Apache-2.0](LICENSE)
