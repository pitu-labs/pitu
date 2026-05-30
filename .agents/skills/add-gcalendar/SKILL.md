---
name: add-gcalendar
description: Give the agent Google Calendar tools (list, read, free/busy, create, update, delete events) brokered through the harness, gated per chat and by granted scope. Owns the Calendar scope choice and runs the Google auth foundation on demand. Runnable before or after add-gmail.
---

## Add Google Calendar

This skill guides you through giving a Pitú agent the ability to work with Google Calendar — listing calendars, reading and searching events, checking availability, and creating or changing events — on demand or on a schedule. Like the Google auth foundation it builds on, this is discretionary per-user functionality, so it lives in the operator's own instance and is installed via this skill rather than shipped in the upstream core.

This is implementation guidance, not a turnkey script. Write the code in the operator's own fork following the architecture and constraints below.

### Phase 0 — Ensure the Google auth foundation, on demand

This skill depends on `configure-google-auth`, but the operator does not need to have run it first. Begin by checking whether the foundation is bootstrapped — the GCP OAuth client is configured and the host-side broker mechanism and "authorize scopes" operation are in place.

- If the foundation is not bootstrapped, run `configure-google-auth`'s setup now (on demand) to bootstrap it, then continue here.
- If it is already bootstrapped, continue.

It is fine to run this skill before or after `add-gmail` — they are independent of each other and both depend only on the shared foundation.

### Phase 1 — Choose the Calendar access tier (this skill owns this)

The foundation is deliberately ignorant of Calendar. This skill owns the Calendar scope vocabulary. Offer the operator these tiers and map the choice to the corresponding Google scope:

| Tier | Google scope | What it allows |
|------|--------------|----------------|
| read | calendar.readonly | Read calendars, events, and free/busy. |
| read + write | calendar | Read plus create, update, and delete events. |

Unlike Gmail, Calendar has no separate "permanent delete" distinction — deleting an event is a single write operation, included in the read + write tier.

### Phase 2 — Authorize the chosen scope through the foundation

Hand the chosen Calendar scope to the foundation's "authorize scopes" operation. Because that operation uses incremental authorization, this consent **adds** Calendar's scope to whatever the operator previously granted (for example, Gmail) without disturbing it — the operator sees a consent screen for the Calendar access and approves it. The foundation persists the accumulated union of granted scopes and the newest refresh token; you do not manage credentials here.

Also remind the operator to enable the Calendar API for the project if they have not already. If the operator later wants to change Calendar's access level, they re-run this skill and pick a different tier; the foundation re-authorizes incrementally.

### The brokered model (unchanged from the foundation)

Every Calendar tool follows the broker principle: the tool the agent calls does not talk to Google. It sends a capability request through Pitú's existing request/response channel; the harness, where the credentials live, performs the Calendar API call and returns the result. Credentials never enter the container. Keep this boundary.

### Phase 3 — The tool surface to implement

Provide tools that cover the breadth of Calendar, grouped by the scope tier they require, creating only those the granted scope permits:

- Available with the **read** scope (and above): list the account's calendars, list and search events within a calendar and time range, fetch a specific event's details, and query free/busy availability across calendars.
- Available with the **read + write** scope: create an event, update an existing event, delete an event, respond to an invitation, and propose or find a suitable meeting time.

Express each operation as a distinct, clearly described tool; as with any capability, a precise description of what each tool does and what parameters it takes improves the agent's tool selection more than a larger, vaguer toolset would.

### Scope gating

Derive the set of tools the agent sees from the Calendar scope name strings the harness injects into the container at `podman exec` time (see `configure-google-auth`'s Phase 3) — not by reading any credential file from inside the container, and not from a separate switch. The container has no access to the credential file and is never told its path; the scope strings (for example `calendar.readonly`, `calendar`) are the only Calendar-related information it holds. If the operator later narrows Calendar to read-only, the write tools should simply stop being offered on the next run. The authoritative check that an operation is allowed happens on the host, in the harness's Calendar handler, against the granted scopes, at the point the API call is made; the container-side tool gating is for ergonomics and honesty.

### Per-chat enablement

Building the tools does not turn them on. Calendar is enabled for a specific chat by the operator through Pitú's existing per-chat capability controls — the same mechanism `view-capabilities` lists. The agent never enables Calendar for itself. A chat with Calendar enabled sees the tools on its next message; a chat without it sees none of them.

Because the agent can also schedule recurring or one-shot work through Pitú's existing scheduling, a Calendar-enabled chat can be asked to do things like check the day's agenda each morning — the agent schedules itself, and when it fires it uses these brokered Calendar tools. No separate scheduling mechanism is needed for Calendar.

### How the agent should handle failures

Provide the runtime agent with guidance (through its context, not as part of this operator skill) for the errors brokered Calendar calls can return:

- When an operation is refused for lack of scope, or the stored authorization has expired or been revoked, the agent should not retry; it should tell the user the capability needs operator attention and name the relevant step.
- When a request is rate-limited, respect the indicated wait, surfacing long waits to the user rather than blocking the turn.
- Treat "not found" (for example, an event that no longer exists) and invalid-argument outcomes as expected data conditions to adjust to, not failures to report.
- For write operations on shared calendars where the account lacks permission, report the permission problem plainly rather than retrying.

### Verify

With Calendar enabled for a test chat, confirm the agent can perform a read operation (such as listing today's events or checking free/busy) and, if the write tier was granted, a reversible-in-practice write such as creating a clearly-marked test event and then deleting it. Confirm that an operation outside the granted scope is cleanly refused by the harness. Confirm that disabling the capability for the chat removes the Calendar tools on the next message.
