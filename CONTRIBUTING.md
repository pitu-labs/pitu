# Contributing to Pitú

Thank you for your interest in contributing. This document explains how the project grows and how your contribution can have the most impact.

---

## The core constraint

Pitú is intentionally small. The entire orchestrator fits in ~15 source files and under 4,000 lines of Go — small enough to be read and audited by a human in an afternoon, or by an agent in seconds. That size is not an accident; it is a design invariant.

**Feature pull requests to core are declined by design.** Not because contributions aren't valued, but because every line added to core is a line every operator, auditor, and agent must read and trust forever. The bar for core changes is therefore high: security fixes, bug fixes, and — rarely — genuinely new orchestration primitives with no reasonable skill-based alternative.

The community's primary contribution surface is **skills**.

---

## Two contribution tracks

### Track 1 — Skills (the main track)

Skills are Markdown files with YAML frontmatter that follow the [AgentSkills specification](https://agentskills.io/specification). They live outside core, extend the agent's capabilities, and are entirely opt-in for operators. This is where new features, integrations, workflows, and personas belong.

### Track 2 — Core (narrow and high-bar)

Core contributions are limited to:

- **Security fixes** — vulnerabilities in the harness, IPC layer, or container lifecycle
- **Bug fixes** — incorrect behavior in existing features
- **New bundled skills** — additions to `.agents/skills/` that ship with the binary

Feature additions to `internal/`, `cmd/pitu/`, or `cmd/pitu-mcp/` are not accepted upstream. Build them in your fork and share them as skills.

---

## How to contribute a skill

### 1. Fork, don't clone

Fork the repository on GitHub. Do not work from a direct clone of the upstream repo.

```
GitHub → Fork → your-username/pitu
git clone https://github.com/your-username/pitu
```

### 2. Write your skill

Create `.agents/skills/<your-skill-name>/SKILL.md` in your fork. Skills must follow the [AgentSkills frontmatter format](https://agentskills.io/specification):

```markdown
---
name: your-skill-name
description: One sentence describing when to invoke this skill.
---

## What this skill does

...
```

**Skill writing principles:**

- **Opt-in, never prescriptive.** Skills describe what an agent *can* do — not what it must do. Assume the operator may choose not to run the skill. Do not write instructions that modify the user's system without explicit confirmation at each step.

- **No code execution.** Skills must not contain instructions that run or write code directly. Shell commands and code blocks are permitted only as **illustrative examples** in few-shot prompt style — never as imperative steps an agent executes blindly. The agent reading the skill, and the user it serves, decide what to run.

- **Concise and complete.** Skills are injected into the agent's context on every message. Context space is a shared resource. A skill should be long enough to guide a successful implementation and short enough not to crowd out the conversation. Aim for 100–300 lines. If it runs longer, split it into two skills.

- **Agent-agnostic.** Any capable agent using any supported model should be able to read your skill and act on it successfully. Avoid idioms or tool references specific to a single agent harness.

- **Human-readable first.** A motivated non-technical user should be able to read the skill and understand what it does and what will happen if they invoke it.

### 3. Test your skill locally

Install your fork locally and verify that:

- The skill appears in the agent's available skills list after startup
- An agent can invoke the skill and follow its instructions to a successful outcome without ambiguity

### 4. Open a merge request

Open a pull request from your fork against `main` on the upstream repository. The pull request description should include:

- **What the skill does** — one or two sentences
- **Who it is for** — the operator scenario it serves
- **What the agent does when invoked** — the outcome, not the steps
- **What the user is asked to confirm** — any destructive or modifying actions the skill may prompt

Community members will review the skill for clarity, safety, and adherence to these guidelines before it is considered for inclusion as a bundled skill.

---

## How to contribute a core fix

For **security vulnerabilities**, please do not open a public issue. Email **community@pitu.dev** with a description and reproduction steps. We will respond within 72 hours.

For **bug fixes**:

1. Open an issue describing the bug, reproduction steps, and expected behavior.
2. Fork the repository.
3. Make the minimal fix — do not refactor surrounding code or add new features alongside the fix.
4. Open a pull request referencing the issue, including a description of what was wrong and why the fix is correct.
5. Add or update a test in `tests/` if the bug is reproducible with one.

Pull requests that expand scope beyond the stated bug will be asked to separate the fix from the additional changes.

---

## Code style

Pitú follows standard Go conventions. Run these before opening a pull request:

```bash
go vet ./...
go test ./...
```

No linter configuration is enforced beyond `go vet`. Keep it readable.

---

## Questions and discussion

- **Bug reports and feature ideas:** [GitHub Issues](https://github.com/pituhq/pitu/issues)
- **Community discussion:** open a GitHub Discussion
- **Security issues:** community@pitu.dev (private)

---

## Code of conduct

All contributors are expected to follow the project's [Code of Conduct](CODE_OF_CONDUCT.md). Participation in this project is a privilege extended to those who act in good faith.

---

*Pitú accepts skills, fixes, and ideas — in that order of frequency. Thank you for keeping the kernel lean.*
