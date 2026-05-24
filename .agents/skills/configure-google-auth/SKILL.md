---
name: configure-google-auth
description: Set up the shared Google credential foundation (OAuth, scopes, host-side brokering) that the add-gmail and add-gcalendar skills build on. Use this first, before enabling any Google service for a chat.
---

## Configure Google Auth

This skill guides you through adding a Google credential foundation to a Pitú instance so the agent can act on Google services (Gmail, Calendar) without ever holding the credentials itself. It is the prerequisite for `add-gmail` and `add-gcalendar`.

This is implementation guidance, not a turnkey script. You are expected to write the code in the operator's own fork, following the architecture and constraints below. Pitú's contribution model keeps discretionary features like this out of the upstream core — they live in each user's instance, installed via skills like this one.

### What you are building, and why this shape

The capability mechanism already in Pitú's core (the synchronous request/response IPC channel and per-chat capability gating) is the transport. Your job is to add a Google **broker** on top of it. The defining principle, which every decision below serves, is:

> Credentials live only on the host. The harness makes the Google API calls. The agent's container never sees a token.

Concretely, that means the agent calls a tool (for example, "search my inbox"); the tool does not talk to Google. It writes a capability request through the existing IPC channel; the harness — running outside the container, where the credentials live — performs the actual Google API call and writes the result back. This mirrors how every other Pitú capability already works (the agent describes intent, the harness performs I/O), and it is what makes credential exfiltration from a prompt-injected agent structurally impossible rather than merely discouraged: the secret is never in a place the agent can reach.

Do not take shortcuts that put credentials, refresh tokens, or access tokens inside the container, in the skills mount, or in any file the agent process can read. If you find yourself mounting a credential path into the container, stop — that breaks the entire security model and there is no configuration that makes it safe.

### Phase 0 — Confirm the prerequisite and the environment

Confirm the instance already has the capability mechanism (per-chat capabilities and the request/response channel). If it does not, that core support must be present first; this skill assumes it.

Ask the operator whether they are setting this up on a machine with a web browser, or remotely over SSH on a headless host. This determines which kind of OAuth client they create and which consent flow you will use. Record the answer.

### Phase 1 — Google Cloud project and OAuth client

Walk the operator through this in the Google Cloud console, waiting for confirmation at each step. Do not assume any step is already done.

- Create or select a Google Cloud project.
- Enable the APIs for the services they intend to use — the Gmail API and the Google Calendar API.
- Configure the OAuth consent screen as an external app and add the operator's own Google account as a test user. This keeps the app in testing mode and avoids Google's full verification review, which is appropriate for a personal, single-operator instance.
- Create an OAuth client credential of the type that matches their environment: a desktop-application client for a machine with a browser, or a limited-input-device client for a headless or SSH setup. The two client types support different consent flows, so this choice must match Phase 0.
- Download the client credential file Google provides and note its path on the host.

Explain to the operator that the client identifier and secret these steps produce are not themselves the sensitive long-lived secret — the refresh token obtained after consent is. The credential file you will create in Phase 3 must therefore be protected accordingly.

### Phase 2 — Decide the access scope per service, at consent time

The access the agent will have is decided once, here, when the operator grants consent — not later at call time. Request only what the operator wants. Offer these tiers per service and map the choice to the corresponding Google scope:

| Service | Tier | What it allows |
|---------|------|----------------|
| Gmail | none | The service is not requested at all. |
| Gmail | read | Read messages and labels only. |
| Gmail | read + modify | Read, send, draft, label, archive, and move to trash. Trash is reversible. Does **not** allow permanent deletion. This is the recommended default. |
| Gmail | full | Everything above plus permanent, irreversible deletion. Offer this only if the operator explicitly asks; present it as the visibly larger, riskier choice it is. |
| Calendar | none | The service is not requested at all. |
| Calendar | read | Read calendars, events, and free/busy. |
| Calendar | read + write | Read plus create, update, and delete events. |

The reason "modify" rather than "full" is the recommended Gmail default: moving a message to trash is recoverable, so an agent mistake (or a manipulated instruction) is reversible, whereas permanent deletion is not. Make the safe tier the easy default and the destructive tier a deliberate opt-in.

Record exactly which scopes were granted. That recorded set is the single source of truth for what the agent is later allowed to do — the service skills (`add-gmail`, `add-gcalendar`) must gate their tools against it, so that a tool the operator did not grant scope for simply does not exist for the agent rather than failing at call time.

### Phase 3 — The one-time host-side consent and credential file

Build a host-side, operator-run command that performs the OAuth consent once and stores the result. It must:

- Accept the path to the client credential file from Phase 1.
- Let the operator choose the per-service tiers from Phase 2 and request exactly those scopes.
- Run the consent flow appropriate to the environment, auto-detected with an override: the browser-based loopback flow when a local browser is available, and the device flow — which prints a short URL and code the operator opens on any other device, such as a phone — when the host is headless or reached over SSH. Detect "headless" from the absence of a local display or the presence of an SSH session, and allow the operator to force either flow explicitly.
- Request offline access and force a fresh consent prompt, because Google only returns the long-lived refresh token under those conditions. If consent succeeds but no refresh token comes back (which happens when the app was authorized before), tell the operator to revoke the app's access in their Google account security settings and run the command again.
- Write the resulting credentials — the client identifier and secret, the refresh token, the exact set of granted scopes, and a creation timestamp — to a single host file under the operator's Pitú configuration directory, readable and writable only by the owner. Write it atomically. This file is the sensitive artifact; treat its permissions as a hard requirement, and have the loader refuse to use it if its permissions are looser than owner-only.

This command runs entirely on the host. Nothing about it touches a container.

### Phase 4 — Harness-side credential loading and token brokering

In the harness (the host-side process, not the container), build the component that the service skills will call into:

- A loader that reads and validates the credential file, rejecting it if its permissions are too loose or its refresh token is missing.
- A token source that turns the stored refresh token into short-lived access tokens, refreshing automatically and caching only in memory. The refresh token must never be written anywhere else or logged, and access tokens must never be persisted or sent through the IPC channel.
- A single dispatch entry point that the harness routes Google capability requests to. In this foundation skill it can return a clear "not implemented yet" result for the individual service operations; `add-gmail` and `add-gcalendar` fill in the real behavior. Wire the harness so that capability requests for the Google services reach this entry point, preserving the request correlation so the agent gets its response back.

When you check whether a requested operation is permitted, check it here, against the granted scopes recorded in the credential file — never trust the container's claim about what it is allowed to do.

### Phase 5 — Verify

Confirm the credential file exists with owner-only permissions and contains the expected granted scopes. Confirm the harness can load it and mint an access token. Report to the operator which scopes are active and remind them that this only establishes the foundation: turning specific Gmail or Calendar tools on for a given chat is a separate step handled by the service skills and Pitú's per-chat capability controls.

### Security constraints to preserve (non-negotiable)

- Credentials stay on the host. Never mount, copy, or expose the credential file, refresh token, or access tokens to a container, the skills mount, or any agent-readable location.
- Scope minimization is the primary defense. Because a read-only mount can still be read and its contents relayed out, the real protection is granting the narrowest scopes the operator actually needs — which is why scope is chosen at consent time and enforced on every call.
- The chat identity is derived by the harness from the filesystem path, never taken from the container's payload. Keep that invariant when wiring Google requests.
- The agent can never grant itself a capability. Enabling Google access for a chat is always an operator action through Pitú's existing capability controls; the agent may, at most, observe and report what it currently has.
