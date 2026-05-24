![Pitú logo](docs/assets/pitu_big.png)
``` 
                        ██╗
                       ██╔╝
██████╗ ██╗████████╗   ╚═╝
██╔══██╗██║╚══██╔══╝██╗   ██╗
██████╔╝██║   ██║   ██║   ██║
██╔═══╝ ██║   ██║   ██║   ██║
██║     ██║   ██║   ╚██████╔╝
╚═╝     ╚═╝   ╚═╝    ╚═════╝
```
**/peeh-tooh/** · _Make it yours_

---

## A lean lobster

While other claw agents grow heavier with every release — runtimes, registries, cloud dashboards, proprietary skill stores, vendor-locked model choices — Pitú stays small on purpose.

Pitú is a **bootstrap agent kernel**: a local-first harness that receives messages from a frontend (Telegram by default), dispatches them to AI agents running inside rootless Podman containers, and delivers responses back through the same channel. Every interaction is mediated through filesystem-based IPC — no daemons, no sockets, no cloud accounts required.

It is built entirely on open source foundations: [Podman](https://podman.io) for rootless containers, [OpenCode](https://opencode.ai) as the in-container agent runtime, and standard Go tooling for the harness itself. You choose your model provider — Anthropic, OpenAI, Ollama, or any OpenAI-compatible endpoint. No proprietary harness, no forced subscription, no telemetry.

Pitú is a **thin orchestrator** — OpenCode does the agent heavy lifting (model calls, tool use, session state). The harness only does what agents cannot: manage containers, route IPC, enforce rate limits, and persist state. That division of responsibility is what keeps the codebase small.

Out of the box, a default installation includes a minimal but complete set of capabilities: **Telegram frontend** for sending and receiving messages, **cron-style scheduled tasks** agents can create at runtime, **multi-agent spawning** so an agent can delegate work to specialised sub-agents, and **two-tier memory** — short-term context kept alive within a warm container session, and a persistent `CONTEXT.md` scratch-pad the agent writes to and reads from across sessions. All of this works without touching configuration.

The entire core orchestrator fits in **~15 source files and under 4,000 lines of Go**. You can read it in an afternoon. Your companion agent can read it in seconds. Both of you can understand it, modify it, and extend it — because that is exactly the point.

---

## Skill-based extensibility

New capabilities come as **skills** — Markdown files with YAML frontmatter that follow the [AgentSkills specification](https://agentskills.io/specification). Pitú distinguishes two skill audiences:

- **Operator skills** live in `.agents/skills/` (project root) and are run by the operator's own coding agent to install features, manage the running instance, or bootstrap new behaviors. They are never injected into the runtime agent's context.
- **Runtime skills** are mounted into every agent container and listed in the running agent's context automatically. They come from two sources: built-ins embedded in the `pitu` binary, and operator-installed skills under `~/.pitu/skills/`. Operator-installed skills win on name conflict.

A particularly powerful pattern: an operator skill can *install* a runtime skill. For example, an `add-socratic-reasoning` operator skill instructs your coding agent to write a runtime skill into `~/.pitu/skills/`, which then shapes how the running agent thinks. The operator chooses which behaviors to install; the running agent gains them on the next container start.

For the four-location model, discovery rules, and merge semantics, see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

---

## Capabilities

Where skills shape *how the agent thinks*, **capabilities** give the agent *things it can do* — agent-callable tools whose work is performed by the harness, not the container. The agent calls a tool (e.g. searching email or creating a calendar event); the harness makes the actual external call and returns the result over filesystem IPC. Credentials and API access live on the host and never enter the agent's container.

Two properties make capabilities safe to grant:

- **Per-chat gating.** A capability is enabled for specific chats by the operator (`pitu capabilities enable --chat <id> --capability <name>`). The agent sees only the tools for capabilities its chat has enabled — context stays lean and a chat can't use what it wasn't granted.
- **Operator-controlled, never self-granted.** The agent can *list* its capabilities (to honestly report what it can and cannot do) but cannot enable them. Granting is an operator action, stored in Pitú's database and injected per message.

Capabilities ride a synchronous request/response IPC primitive: the agent's tool call becomes a request file the harness fulfills and answers. Adding a new capability (Gmail, Calendar, …) means registering its tools and a harness-side handler — no new transport, no credentials in the container. See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the request/response protocol and the "Adding a new capability" guide.

---

## Security by smallness

A small codebase is an auditable codebase. Pitú's security posture is a direct consequence of its size constraint:

- Agents run in **rootless Podman containers** — a container escape lands a non-root user, not root.
- All harness ↔ agent communication is **filesystem IPC** scoped to per-chat directories. Agents cannot reach arbitrary chats, sockets, or localhost ports.
- **Path-derived identity**: the harness overwrites the `chat_id` in every IPC payload with the value it controls — a prompt-injected agent cannot forge routing.
- **No secrets in transit**: API keys are written to a `0600` temp file, read by Podman at start, then deleted from the host.
- Warm containers are bounded by a configurable **TTL** and a per-container **memory limit**.

Every one of these decisions is deliberate and documented. For the full threat model and rationale, see [`docs/SECURITY.md`](docs/SECURITY.md).

---

## Contributing: skills, not PRs

Pitú's contribution model mirrors its extensibility model — new features live as skills, not as commits to the core.

Want WhatsApp integration? Write an operator skill that instructs users to have their own agent implement a companion bridge process. Want a new way for the runtime agent to reason or summarise? Write an operator skill that installs a runtime skill describing the behavior. Want to bundle a persona template, a memory workflow, or a deployment helper? Write a skill.

This keeps the core repo at ~4,000 lines and every operator's fork clean and auditable. **Pitú accepts:**

- Security fixes
- Bug fixes
- New bundled skill proposals (as `.agents/skills/<name>/SKILL.md`)

**Feature pull requests to the core are declined by design.** If you want to contribute a skill or a fix:

1. **Fork** the repository — do not clone directly.
2. Make your changes in your fork.
3. Open a **merge request** against `main`.

This keeps the upstream repo lean and the contribution history meaningful. Feature work belongs in forks, operator configs, and skill files — not in the shared kernel.

---

## Quick start

The recommended way to install Pitú is to let your agent do it. The bundled `setup` skill contains instructions clear enough that any capable model can execute them end-to-end without hand-holding — **any agent harness works**. Below are reference examples for three common ones:

**[OpenCode](https://opencode.ai) (recommended)**

```sh
opencode
> skills          # open the skill picker
> select: setup   # run the setup skill
```

**[Claude Code](https://claude.ai/code)**

```sh
claude
> /setup
```

**[Gemini CLI](https://github.com/google-gemini/gemini-cli)**

```sh
gemini
> /setup
```

[Kilo Code](https://kilocode.ai), [Crush](https://github.com/charmbracelet/crush), [Goose](https://github.com/block/goose), and any other agent that can read Markdown and run shell commands will work equally well — just point it at the `setup` skill and let it run.

Your agent will clone the repository, build the binaries, scaffold the config, and install the system service — asking you only for the things it cannot infer (your model provider, API key, and Telegram bot token).

---

## Agent-first development

Pitú is built to be explored, debugged, and extended by you _with_ your agent — not by reading API docs or joining a Discord server. Any capable agent and model works; the codebase is small enough to fit entirely in context.

- **Want to understand the code?** Point your agent at `internal/` and ask it to walk you through a request's lifecycle from Telegram poll to container exec.
- **Building a new feature?** Have your agent read `docs/ARCHITECTURE.md`, then draft the implementation together against the documented invariants.
- **Debugging something unexpected?** Ask your agent to tail the logs, trace the IPC files, and explain what each component is doing.
- **Exploring new ideas?** Share the source with your agent and brainstorm. The codebase is small enough that your agent holds the whole thing in context at once.

This is the _make it yours_ ethos in practice — not a plugin marketplace, but a shared workspace between you and a capable collaborator.

---

### A note on where this is heading

Pitú is a small bet on a larger idea.

If LLMs continue to improve at the current pace, the relationship between developers and code will shift. Source code will increasingly become a _side effect_ of functional and non-functional requirements — the artefact a system produces to satisfy a spec, rather than the thing humans author directly. Two instances of the same application could behave identically but be implemented in completely different languages, with different abstractions, generated fresh for different runtime environments.

In that world, what matters is the spec, the behaviour, and the tests that validate the behaviour as a black box. The implementation language, the framework, even the operating system become incidental.

Pitú is built for that direction: keep the kernel small enough to specify precisely, keep the extension surface in plain text (skills, Markdown, TOML), and let agents handle the rest. The goal is not a better bot platform — it is a working example of what software looks like when it is built to be understood and modified by agents as naturally as by humans.

---

## License

[MIT](LICENSE)
