# Built-in Runtime Skills

This directory holds Pitú's built-in runtime skills. They are embedded into the
`pitu` binary at build time via `go:embed` and unpacked at startup into
`~/.pitu/data/skills-builtin/`.

Each subdirectory must contain a `SKILL.md` file with valid AgentSkills
frontmatter. See `docs/ARCHITECTURE.md` for the discovery and merge rules.

This file (`README.md`) is intentionally not a `SKILL.md`. Discovery only scans
for `SKILL.md` files, so this serves as documentation without being registered
as a skill.
