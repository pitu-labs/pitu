---
name: set-container-ttl
description: Change how long idle agent containers stay warm before being stopped. Use when tuning memory usage vs. response latency on the host machine.
---

## Set Container TTL

In `~/.pitu/config.toml`, update:

```toml
[container]
ttl = "5m"   # e.g. "2m", "10m", "30m"
```

Restart Pitu for the change to take effect. Shorter TTL = less RAM used by idle containers. Longer TTL = faster response when a quiet chat resumes.
