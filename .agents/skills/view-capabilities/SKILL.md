---
name: view-capabilities
description: List which external capabilities (e.g. Gmail, Calendar) are enabled for each chat. Use when diagnosing why an agent can or cannot access a capability in a given chat.
---

## View Enabled Capabilities

This skill lists the capabilities enabled per chat, reading the authoritative
`chat_capabilities` table via the Pitú CLI. It is read-only and makes no changes.

### Steps

1. Run the capabilities listing yourself (do not ask the operator to run it):

   ```bash
   pitu capabilities list
   ```

   This prints one line per chat with its enabled capabilities, e.g.:

   ```
   Chat 123456: [gmail gcalendar]
   Chat 789012: [gmail]
   ```

   If it prints "No capabilities enabled for any chat.", none are configured yet.

2. Interpret the output for the operator:
   - A chat absent from the list has no capabilities enabled.
   - If an expected capability is missing, enable it with the relevant skill
     (`add-gmail`, `add-gcalendar`) or directly with
     `pitu capabilities enable --chat <id> --capability <name>`.

3. Report the findings plainly to the operator.
