# CaseMagica

An AI creative platform for novel writing and AI role-playing games.

English | [中文](README.md)

## Overview

CaseMagica provides:

- Writing workspace: chapter management, lore library, style references, version control
- Game mode: interactive stories, story director, memory system, image generation
- AI integration: Agents, Skills, Subagent Workflows, automation tasks

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
