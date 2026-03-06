---
name: generate-claude-md
description: >
  Scans a folder and produces a Claude.md context briefing for AI agents. Use this skill whenever the user asks to "generate a Claude.md", "create context for agents", "document this folder for AI", "analyze this codebase folder", "make a Claude.md for this project", or wants a quick onboarding document for AI agents working in a directory. The skill handles depth limits (root + one child level only), respects existing Claude.md files in subfolders, and outputs a structured markdown file covering tech stack, architecture, key files, env vars, and scripts.
---

# Generate Claude.md — Codebase Context for AI Agents

You are a codebase analyst. Your job is to scan a folder path the user provides, understand what's in it, and write a concise `Claude.md` that gives any future AI agent instant situational awareness about the folder.

## Step 1 — Pre-check

Before scanning anything:

1. Check whether `Claude.md` already exists at the root of the target folder.
   - If it **does not exist** → you'll create `Claude.md`.
   - If it **already exists** → you'll create `Claude2.md` instead. Do NOT read or overwrite the existing `Claude.md`.

## Step 2 — Scan & Analyze

### Depth rule: root + one child level only

You have a strict scanning boundary to keep things fast and focused:

- **Root-level files** → read and analyze normally.
- **Direct child folders (depth 1)** → read the files inside them normally.
- **Grandchild folders (depth ≥ 2)** → do NOT open or read files inside them. Note them but don't recurse.

Skip `node_modules`, `__pycache__`, `.git`, `dist`, `build`, `vendor`, and other generated/dependency directories. Respect `.gitignore` if present.

**Why this matters:** Going deeper creates diminishing returns — you'd spend most of your time on generated files or deep internals. The root + one level gives you the architectural skeleton, which is exactly what an incoming agent needs.

### Leveraging existing Claude.md in child folders

Before reading individual files in a child folder, check if it already has its own `Claude.md`:

- **Has `Claude.md`** → use it as the primary reference for that folder. Summarize or pull from it rather than re-analyzing every file. You may glance at key files (entry points, config) to verify, but defer to the existing `Claude.md`.
- **No `Claude.md`** → analyze the files inside normally (still bounded to depth 1; don't recurse into sub-subfolders).

### Per-file extraction

For every file you analyze directly, extract:
1. **Purpose** — one sentence: what does this file do?
2. **Key exports / entry points** — functions, classes, components, CLI commands, routes, etc.
3. **Internal dependencies** — which other files in the folder does it import?
4. **External dependencies** — third-party packages, SDKs, APIs.

Don't just list file names. Extract meaning so an agent that never sees the files still understands the system.

## Step 3 — Contextual Analysis

After reading files, step back and determine:

| Aspect | What to capture |
|---|---|
| **Language & Runtime** | Primary language(s), version constraints, TypeScript vs JS, etc. |
| **Framework** | Core framework and major libraries |
| **Architecture** | Monolith, microservices, serverless, MVC, event-driven, etc. |
| **Build & Generation** | Auto-generated files? By what tool? (Prisma, OpenAPI, protobuf…) |
| **DI / IoC** | Is there a DI container? How are services registered? |
| **Package Management** | npm/yarn/pnpm/pip/poetry/cargo/etc. Note lockfile. |
| **Environment & Config** | .env files, config modules, secret managers. List env var **names only** (never values). |
| **Entry Points** | Main entry file(s), CLI commands, API route roots, scheduled jobs. |
| **Testing** | Test framework, test location, how to run. |
| **Scripts** | Notable scripts in `package.json`, `Makefile`, `pyproject.toml`, etc. |

## Step 4 — Write the Output File

Produce `Claude.md` (or `Claude2.md`) with this structure. Aim for under ~1,500 words — an agent should be able to consume it in one read and have full situational awareness.

```markdown
# <Folder Name> — Context for AI Agents

> Auto-generated context file. Do not edit manually.

## Overview
<!-- 2-3 sentences: what this folder is, what it does, who it's for. -->

## Tech Stack
<!-- Language, framework, runtime, package manager, key libraries. -->

## Architecture
<!-- High-level pattern, data flow, main modules and how they connect. -->

## Folder Structure
<!-- Trimmed tree with one-line annotations. Mark folders with their own Claude.md as ✅. -->

## Child Folders
<!--
For each direct child folder:
- Has Claude.md → one-line summary from that file, noting "(has Claude.md)"
- No Claude.md → brief description based on your analysis
- Contains deeper nesting → note clearly that sub-subfolders were NOT scanned
-->

## Key Files
<!-- Per important file: path, purpose, key exports. Group by module/feature if helpful. -->

## Dependencies & Injection
<!-- External packages that matter for understanding the code. DI/IoC patterns if any. -->

## Generated / Do-Not-Edit Files
<!-- Files/dirs that are auto-generated. Name the tool that produces them. -->

## Environment & Config
<!-- Required env var names (only names, never values), config files, secrets approach. -->

## Scripts & Commands
<!-- How to install, build, run, test, lint. Copy-paste-ready commands. -->

## Conventions & Patterns
<!-- Naming conventions, error handling patterns, logging approach, team idioms. -->

## Known Gotchas
<!-- Non-obvious traps: circular deps, magic strings, implicit ordering, fragile tests, etc. -->
```

If any direct child folder contains sub-folders (grandchildren you couldn't scan), add this warning block at the TOP of the output file, before the Overview:

```markdown
> ⚠️ **Depth limitation**: This context file only covers files at the root and one level of child folders. The following sub-folders contain deeper nesting that was NOT analyzed: `<list of paths>`. Run this skill inside those sub-folders individually for full coverage.
```

## Step 5 — Self-Review

Before saving, verify:

- [ ] No secrets, tokens, or sensitive values included.
- [ ] Generated files are clearly marked as do-not-edit.
- [ ] The Overview makes sense to an agent with zero prior context.
- [ ] Commands in "Scripts & Commands" are accurate.
- [ ] Depth-limitation warning present if needed.
- [ ] Child folders with existing `Claude.md` were referenced, not redundantly re-analyzed.
- [ ] File is under ~1,500 words. Trim aggressively — reference source files rather than duplicating content.

## Output

Save the file as `Claude.md` (or `Claude2.md` if `Claude.md` already exists) at the **root of the provided folder path**.

Tell the user where the file was saved and give a one-sentence summary of what the folder turned out to be.
