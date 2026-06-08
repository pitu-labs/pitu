---
name: configure-google-auth
description: Establish the scope-agnostic Google credential foundation — OAuth setup, host-side token brokering, and a reusable "authorize scopes" operation. Google service skills depend on this and run it on demand; running it alone only bootstraps.
---

## Configure Google Auth

This skill guides you through adding a Google credential **foundation** to a Pitú instance: the host-side machinery that lets the agent act on Google services without ever holding the credentials itself. It is the shared prerequisite that any Google service skill builds on.

Crucially, this foundation knows nothing about which Google services exist — not even the ones that exist today. It is pure mechanism. Each Google service skill declares the scopes it needs and asks the foundation to authorize them. That keeps the dependency pointing one way — services depend on the foundation, never the reverse — so any Google service, present or future, can build on this with no change here. (This skill intentionally names no specific service, to keep that independence honest.)

This is implementation guidance, not a turnkey script. Write the code in the operator's own fork following the architecture and constraints below. Pitú's contribution model keeps discretionary features out of the upstream core — they live in each user's instance, installed via skills like this one.

### Division of labor — who does what

Some steps genuinely require the operator; most do not. Be deliberate about the difference, and do not hand the operator a checklist of steps you are capable of performing yourself.

**The operator must do (you cannot):** the Google Cloud console work — create the project, configure the OAuth consent screen, create and download the OAuth client credential — and approve the consent screen in their browser when the authorize operation runs. Walk them through these and wait at each one.

**You do directly (do not offload):** writing and editing configuration, building the binaries, wiring the harness, exposing the operator-facing command, and rebuilding the container image. The operator chose to run this skill; that choice is their consent for the mechanical setup. Pause only for the console steps above, for the browser consent, and for any genuinely destructive or ambiguous decision — never to delegate a copy-paste, a config edit, or a build you could run. Offloading automatable work back to the operator is the most common failure mode of this skill; treat it as a defect, not a courtesy.

### When this runs

Google service skills run this foundation **on demand**: when the operator chooses to install a service, that service's skill ensures the foundation exists (running this skill's setup first if needed), then asks it to authorize that service's scopes. The operator does not need to run this skill explicitly first.

Running this skill on its own is also fine — the only outcome is a **bootstrap**: the GCP client and the host-side broker mechanism get set up, but no consent runs and no credentials are stored yet, because no service has asked for any scopes. Consent happens lazily, the first time a service skill authorizes its scopes.

### What you are building, and why this shape

The capability mechanism already in Pitú's core (the synchronous request/response IPC channel and per-chat capability gating) is the transport. Your job is to add a Google **broker** on top of it. The defining principle, which every decision below serves, is:

> Credentials live only on the host. The harness makes the Google API calls. The agent's container never sees a token.

Concretely: the agent calls a tool (for example, "search my inbox"); the tool does not talk to Google. It writes a capability request through the existing IPC channel; the harness — running outside the container, where the credentials live — performs the actual Google API call and writes the result back. This mirrors how every other Pitú capability already works, and it is what makes credential exfiltration from a prompt-injected agent structurally impossible rather than merely discouraged: the secret is never in a place the agent can reach.

Do not take shortcuts that put credentials, refresh tokens, or access tokens inside the container, in the skills mount, or in any file the agent process can read. If you find yourself mounting a credential path into the container, stop — that breaks the entire security model and there is no configuration that makes it safe.

### Phase 0 — Confirm the prerequisite and the environment

Confirm the instance already has the capability mechanism (per-chat capabilities and the request/response channel). If it does not, that core support must be present first; this skill assumes it.

Ask the operator whether they are setting this up on a machine with a web browser, or remotely over SSH on a headless host. This determines which kind of OAuth client they create and which consent flow the authorize operation uses. Record the answer.

### Phase 1 — Google Cloud project and OAuth client (the bootstrap)

Walk the operator through this in the Google Cloud console, waiting for confirmation at each step. Do not assume any step is already done.

- Create or select a Google Cloud project.
- Enable the APIs for the services they intend to use. (Each service skill will remind the operator to enable its specific API; enabling them here is fine if known, but not required by the foundation itself.)
- Configure the OAuth consent screen as an external app and add the operator's own Google account as a test user. This keeps the app in testing mode and avoids Google's full verification review, which is appropriate for a personal, single-operator instance.
- Create an OAuth client credential of the type that matches their environment: a desktop-application client for a machine with a browser, or a limited-input-device client for a headless or SSH setup. The two client types support different consent flows, so this choice must match Phase 0.
- Download the client credential file Google provides and store its path where the foundation can read it (alongside the foundation's own configuration on the host). The client identifier and secret are not the sensitive long-lived secret — the refresh token obtained after consent is.

The client identifier, the client secret, the credential file Google supplies, and any path or config pointer to any of them are host-only artefacts. They are not passed into the container — not as env vars, not via mounts, not embedded in any context file — even though they are not the long-lived secret. Keeping them host-only preserves the rule that the container learns nothing about how host credentials are stored; carving an exception for "the non-sensitive parts" reintroduces the very metadata leak the boundary forbids.

This is the entire bootstrap. After it, the foundation is ready to authorize scopes on request, but holds no user grant yet.

### Phase 2 — The reusable "authorize scopes" operation (the heart of the foundation)

Build a single operation the service skills call, with this contract:

> Given a set of OAuth scopes, obtain the operator's consent for them and persist a credential that grants the **union** of those scopes and any previously granted ones.

Implement it using Google's **incremental authorization**: request the new scopes together with a signal to include already-granted scopes (`include_granted_scopes`), so the resulting grant is additive rather than replacing what came before. This is what lets the foundation stay agnostic — it never needs the full scope list up front; scopes accrue as services are added, each with its own explicit consent.

The operation must:

- Run the consent flow appropriate to the environment, auto-detected with an override: the browser-based loopback flow when a local browser is available, and the device flow — which prints a short URL and code the operator opens on any other device, such as a phone — when the host is headless or reached over SSH. Detect "headless" from the absence of a local display or the presence of an SSH session, and allow the operator to force either flow. **Both flows must exist; do not collapse this to device-flow-only.** On a machine with a browser the loopback flow is the default, and it is the robust path: Google's device flow is a restricted grant that pairs poorly with sensitive service scopes (Gmail in particular) and requires the limited-input client type from Phase 1, so a device-only implementation both regresses the common browser case and risks failing consent outright. The flow is selected by detection, never by dropping one of the two.
- Request offline access and force a fresh consent prompt, because Google only returns the long-lived refresh token under those conditions. If consent succeeds but no refresh token comes back (which happens when the app was authorized before), tell the operator to revoke the app's access in their Google account security settings and run the operation again.
- Persist the result to a single host file under the operator's Pitú configuration directory, readable and writable only by the owner, written atomically. Store the client identifier and secret, the **newest** refresh token returned, the accumulated **union** of granted scopes, and a timestamp. Always treat that stored union as the source of truth for what the agent is allowed to do.

Two real-world cautions to handle: Google caps the number of refresh tokens per client and user, so re-consenting repeatedly can rotate older tokens out — always persist the newest token you receive. And the credential loader must refuse to use the file if its permissions are looser than owner-only.

The foundation does **not** define any service tiers or pick any scopes. It only authorizes the scopes it is handed.

**Where the operator-facing command lives (interface).** Expose "authorize scopes" and a way to inspect the current grant as a `pitu auth` subcommand — for example `pitu auth authorize` and `pitu auth scopes` — dispatched inline in `cmd/pitu/main.go` exactly as the existing `pitu service` and `pitu capabilities` subcommands already are, and reusing `internal/config` to read the credential and grant paths. Do **not** create a second binary for this. A separate `cmd/pitu-auth` would have to duplicate config loading, and it adds an install artefact that every build, ship, and service-management step then has to track or silently drop. One binary, one subcommand surface, consistent with what is already there.

### Before implementing — a boundary self-check

Before writing any code that crosses the host↔container line, answer this question honestly: **does any part of your design require the container to know how host credentials are stored — their path, filename, presence flag, or shape?** If yes, the design is wrong even if no token bytes actually cross. The container is allowed to learn three things and three things only: the chat id, the enabled capability names, and the granted scope name strings (for example `gmail.readonly` — the *name* of the scope, not the file it came from). Anything else — a path, a filename, an `exists?` boolean, a hash, a config snippet pointing at credential storage — is drift back toward "the container is a trusted partner," which is the model this skill exists to refuse.

The drift case to recognise: scope-gating logic placed *inside* the container needs scope information, and the path of least resistance is to point the container at the credential file (a path env var, a small mount, "just a read-only peek"). Don't take that path. Do the scope-gating on the host and inject only the resulting allow-list. Phase 3 makes that placement explicit.

### Phase 3 — Harness-side credential loading and token brokering

In the harness (the host-side process, not the container), build:

- A loader that reads and validates the credential file, rejecting it if its permissions are too loose or its refresh token is missing.
- A token source that turns the stored refresh token into short-lived access tokens, refreshing automatically and caching only in memory. The refresh token must never be written anywhere else or logged, and access tokens must never be persisted or sent through the IPC channel.
- A single dispatch entry point the harness routes Google capability requests to. The foundation provides the wiring; each service skill fills in the behavior for its own operations. When you check whether a requested operation is permitted, check it here, against the granted scopes recorded in the credential file — never trust the container's claim about what it is allowed to do.
- A scope-gating placement that lives **on the host**, not in the container. At `podman exec` time, the harness derives the agent's tool allow-list from the credential file's granted scopes and injects only the resulting strings — capability names alongside per-capability scope name lists (for example `PITU_GMAIL_SCOPES=gmail.readonly,gmail.modify`) — into the container's environment, alongside the per-chat `PITU_CAPABILITIES` list the core already passes. The container's MCP server uses those strings to decide which tools to register, and that is the only Google-related information it ever holds. The container has no knowledge of the credential file's existence, path, or contents. The authoritative permission check still happens on the host at the point of each API call, so a tool the harness should refuse is refused even if the container somehow registered it; the container-side gate is for ergonomics and honesty, not enforcement.

### Phase 4 — Verify

Verify by behavior, not by forcing a grant: confirm the bootstrap is complete (the OAuth client is configured and the broker mechanism and authorize operation are in place) and that the harness can load and refresh a credential *if one exists*. A standalone run legitimately ends here with no credential file — that is the lazy design working as intended. The first service skill to authorize its scopes produces the first credential.

### Security constraints to preserve (non-negotiable)

- Credentials stay on the host. Never mount, copy, or expose the credential file, refresh token, or access tokens to a container, the skills mount, or any agent-readable location. The same prohibition extends to *metadata about* those credentials — their path, filename, existence flag, hash, or shape — which must never be injected into the container as env vars, config, or context. Only the *result* of using the credentials (the granted scope name strings) crosses the boundary.
- Scope minimization is the primary defense, and the incremental-authorization design makes it the default: the stored grant only ever contains scopes for services the operator actually enabled. Never broaden a request beyond what the calling service asked for.
- The chat identity is derived by the harness from the filesystem path, never taken from the container's payload. Keep that invariant when wiring Google requests.
- The agent can never grant itself a capability or a scope. Authorizing scopes and enabling a service for a chat are always operator actions; the agent may, at most, observe and report what it currently has.
