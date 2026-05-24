---
name: add-gmail
description: Give the agent Gmail tools (read, send, label, archive, trash) brokered through the harness, gated per chat and by granted scope. Requires configure-google-auth first. Runnable before or after add-gcalendar.
---

## Add Gmail

This skill guides you through giving a Pitú agent the ability to work with Gmail — reading, searching, sending, and organizing mail — on demand or on a schedule. Like the Google auth foundation it builds on, this is discretionary per-user functionality, so it lives in the operator's own instance and is installed via this skill rather than shipped in the upstream core.

This is implementation guidance, not a turnkey script. Write the code in the operator's own fork following the architecture and constraints below.

### Prerequisite: the Google auth foundation

This skill depends on `configure-google-auth`. Before doing anything else, check that the Google credential foundation is in place: the host credential file exists with owner-only permissions, and the harness can load it and mint access tokens.

It is fine to run this skill before or after `add-gcalendar` — they are independent of each other and both depend only on the shared foundation. If the foundation is missing, stop and direct the operator to run `configure-google-auth` first, then return here.

Then check that the granted scopes recorded by the foundation actually cover the Gmail access the operator wants. If the operator wants to send mail but only the read scope was granted, the send tools must not be created — direct the operator to re-run the foundation's consent step and grant a broader Gmail tier, then return. Surfacing this gap now, at install time, is far better than letting the agent discover at call time that a tool it sees does not actually work.

### The brokered model (unchanged from the foundation)

Every Gmail tool follows the broker principle established by `configure-google-auth`: the tool the agent calls does not talk to Google. It sends a capability request through Pitú's existing request/response channel; the harness, where the credentials live, performs the Gmail API call and returns the result. Credentials never enter the container. Keep this boundary — do not let a Gmail tool read credentials or call Google directly from inside the agent's environment.

### The tool surface to implement

Provide tools that cover the breadth of Gmail, not just reading and sending. Group them by the scope tier they require, and only create the tools the granted scope actually permits:

- Available with the **read** scope (and above): search messages with Gmail's query syntax, fetch a specific message's content, and list the account's labels.
- Available with the **read + modify** scope (and above): send a message, create or update a draft, apply and remove labels on a message, archive a message, move a message to trash, and restore a message from trash, and mark messages read or unread.
- Available only with the **full** scope: permanently delete a message. Because this is irreversible, keep it behind the full tier and treat it as the exceptional, explicitly-granted operation it is — never as part of the ordinary toolset.

Whatever the granted scope permits should be expressed as distinct, well-described tools so the agent can choose precisely. Each tool's description is load-bearing: a tightly written description improves the agent's tool selection more than adding more tools does, so describe each operation and its parameters clearly and honestly, including what it does not do (for example, that "trash" is reversible and is not permanent deletion).

### Scope gating

The set of tools the agent sees must be derived from the scopes recorded in the credential file — not from a separate switch. If the operator later narrows the Gmail scope, the corresponding tools should simply stop being offered on the next run, with no other change needed. This keeps the agent's actual abilities and the operator's granted permissions from ever drifting apart, and it means the destructive tools are absent unless the operator deliberately enabled the tier that includes them.

The authoritative check of whether an operation is allowed happens in the harness, against the granted scopes — the same place the API call is made. Tool gating in the agent's view is for ergonomics and honesty; the scope check on the harness side is the real enforcement.

### Per-chat enablement

Building the tools does not turn them on. Gmail is enabled for a specific chat by the operator through Pitú's existing per-chat capability controls — the same mechanism `view-capabilities` lists and that enables or disables a capability for a chat by name. The agent never enables Gmail for itself; this is always an operator decision. A chat with Gmail enabled sees the Gmail tools on its next message; a chat without it sees none of them and carries none of their context cost.

### How the agent should handle failures

Provide the runtime agent with guidance (through its context, not as part of this operator skill) for the errors brokered Gmail calls can return, so it behaves sensibly:

- When an operation is refused for lack of scope, or the stored authorization has expired or been revoked, the agent should not retry. It should tell the user the capability needs operator attention and name the relevant step (re-running the auth consent with a broader tier, or re-authorizing).
- When Google rate-limits a request, the agent should respect the indicated wait; if the wait is long, surface it to the user rather than blocking the turn.
- For ordinary "not found" or invalid-argument outcomes, the agent should treat them as expected data conditions and adjust, rather than as failures to report.

### Verify

With Gmail enabled for a test chat, confirm the agent can perform a read operation (for example, summarizing recent mail) and, if the modify tier was granted, a reversible write (such as applying a label or moving a message to trash). Confirm that an operation outside the granted scope is cleanly refused by the harness, not silently attempted. Confirm that disabling the capability for the chat removes the Gmail tools on the next message.
