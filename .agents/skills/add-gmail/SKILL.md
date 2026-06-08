---
name: add-gmail
description: Give the agent Gmail tools (read, send, label, archive, trash) brokered through the harness, gated per chat and by granted scope. Owns the Gmail scope choice and runs the Google auth foundation on demand. Runnable before or after add-gcalendar.
---

## Add Gmail

This skill guides you through giving a Pitú agent the ability to work with Gmail — reading, searching, sending, and organizing mail — on demand or on a schedule. Like the Google auth foundation it builds on, this is discretionary per-user functionality, so it lives in the operator's own instance and is installed via this skill rather than shipped in the upstream core.

This is implementation guidance, not a turnkey script. Write the code in the operator's own fork following the architecture and constraints below.

The same division of labor as `configure-google-auth` applies: the operator does the Google Cloud console steps (enabling the Gmail API, approving consent in their browser) and you do the rest directly — building the tools, wiring the handler, rebuilding the image, and running the enablement command on the operator's behalf. Do not hand the operator a list of commands to run that you could run yourself.

### Phase 0 — Ensure the Google auth foundation, on demand

This skill depends on `configure-google-auth`, but the operator does not need to have run it first. Begin by checking whether the foundation is bootstrapped — the GCP OAuth client is configured and the host-side broker mechanism and "authorize scopes" operation are in place.

- If the foundation is not bootstrapped, run `configure-google-auth`'s setup now (on demand) to bootstrap it, then continue here.
- If it is already bootstrapped, continue.

This is the only thing this skill needs from the foundation's setup; everything else about Gmail — which scopes, which tools — is owned here.

### Phase 1 — Choose the Gmail access tier (this skill owns this)

The foundation is deliberately ignorant of Gmail. This skill owns the Gmail scope vocabulary. Offer the operator these tiers and map the choice to the corresponding Google scope:

| Tier | Google scope | What it allows |
|------|--------------|----------------|
| read | gmail.readonly | Read messages and labels only. |
| read + modify | gmail.modify | Read, send, draft, label, archive, and move to trash. Trash is reversible. Does **not** allow permanent deletion. Recommended default. |
| full | mail.google.com | Everything above plus permanent, irreversible deletion. Offer only if explicitly asked; present it as the visibly larger, riskier choice it is. |

The reason "modify" is the recommended default: moving a message to trash is recoverable, so an agent mistake (or a manipulated instruction) is reversible, whereas permanent deletion is not. Make the safe tier the easy default and the destructive tier a deliberate opt-in.

### Phase 2 — Authorize the chosen scope through the foundation

Hand the chosen Gmail scope to the foundation's "authorize scopes" operation. Because that operation uses incremental authorization, this consent **adds** Gmail's scope to whatever the operator previously granted (for example, Calendar) without disturbing it — the operator sees a consent screen for the Gmail access and approves it. The foundation persists the accumulated union of granted scopes and the newest refresh token; you do not manage credentials here.

Also remind the operator to enable the Gmail API for the project if they have not already.

If the operator later wants to change Gmail's access level, they re-run this skill and pick a different tier; the foundation re-authorizes incrementally.

### The brokered model (unchanged from the foundation)

Every Gmail tool follows the broker principle: the tool the agent calls does not talk to Google. It sends a capability request through Pitú's existing request/response channel; the harness, where the credentials live, performs the Gmail API call and returns the result. Credentials never enter the container. Keep this boundary — do not let a Gmail tool read credentials or call Google directly from inside the agent's environment.

### Phase 3 — The tool surface to implement

Provide tools that cover the breadth of Gmail, not just reading and sending. Group them by the scope tier they require, and only create the tools the granted scope actually permits:

- Available with the **read** scope (and above): search messages with Gmail's query syntax, fetch a specific message's content, and list the account's labels.
- Available with the **read + modify** scope (and above): send a message, create or update a draft, apply and remove labels on a message, archive a message, move a message to trash, restore a message from trash, and mark messages read or unread.
- Available only with the **full** scope: permanently delete a message. Because this is irreversible, keep it behind the full tier and treat it as the exceptional, explicitly-granted operation it is — never as part of the ordinary toolset.

Express whatever the granted scope permits as distinct, well-described tools. Each tool's description is load-bearing: a tightly written description improves the agent's tool selection more than adding more tools does, so describe each operation and its parameters clearly and honestly, including what it does not do (for example, that "trash" is reversible and is not permanent deletion).

### Scope gating

The set of tools the agent sees must be derived from the Gmail scope name strings the harness injects into the container at `podman exec` time (see `configure-google-auth`'s Phase 3) — not by reading any credential file from inside the container, and not from a separate switch. The container has no access to the credential file and is never told its path; the scope strings (for example `gmail.readonly`, `gmail.modify`) are the only Gmail-related information it holds. If the operator later narrows the Gmail scope, the new strings the harness injects cause the corresponding tools to simply stop being offered on the next run, with no other change needed. This keeps the agent's actual abilities and the operator's granted permissions from ever drifting apart, and it means the destructive tools are absent unless the operator deliberately enabled the tier that includes them.

The authoritative check of whether an operation is allowed happens on the host, in the harness's Gmail handler, against the granted scopes — the same place the API call is made. Tool gating in the agent's view is for ergonomics and honesty; the scope check on the harness side is the real enforcement.

### Per-chat enablement

Building the tools does not turn them on. Gmail is enabled for a specific chat through Pitú's existing per-chat capability controls — the same mechanism `view-capabilities` lists, which enables or disables a capability for a chat by name. Keep two distinct agents separate here: the **runtime agent inside the container can never enable Gmail for itself** — that security invariant stands. But you, the operator's coding agent running this install skill, should perform the enablement command yourself once the operator confirms which chat to enable it for, rather than printing the command for them to paste. Confirm the target chat (an authorization decision), then run it. A chat with Gmail enabled sees the Gmail tools on its next message; a chat without it sees none of them and carries none of their context cost.

### How the agent should handle failures

Provide the runtime agent with guidance (through its context, not as part of this operator skill) for the errors brokered Gmail calls can return:

- When an operation is refused for lack of scope, or the stored authorization has expired or been revoked, the agent should not retry. It should tell the user the capability needs operator attention and name the relevant step (re-running this skill with a broader tier, or re-authorizing).
- When Google rate-limits a request, the agent should respect the indicated wait; if the wait is long, surface it to the user rather than blocking the turn.
- For ordinary "not found" or invalid-argument outcomes, the agent should treat them as expected data conditions and adjust, rather than as failures to report.

Install this guidance as a runtime skill — a `SKILL.md` the running agent loads, either shipped built-in (under `internal/skills/builtin/assets/`) or written to `~/.pitu/skills/`. Note the consequence: a **built-in runtime skill only reaches containers after you rebuild the image and restart**, so the guidance (and any new tools) will not appear until that rebuild — which is a step you perform as part of this install, not one you delegate or skip.

### Verify

With Gmail enabled for a test chat, confirm the agent can perform a read operation (for example, summarizing recent mail) and, if the modify tier was granted, a reversible write (such as applying a label or moving a message to trash). Confirm that an operation outside the granted scope is cleanly refused by the harness, not silently attempted. Confirm that disabling the capability for the chat removes the Gmail tools on the next message.
