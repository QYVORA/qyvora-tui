# Telemetry

QYVORA TUI should not send telemetry during local development, CI, or automated
validation unless explicitly configured for an approved external service.

## Astro / docs tooling

When running docs or web tooling that may include Astro telemetry, disable it:

```bash
export ASTRO_TELEMETRY_DISABLED=1
export DO_NOT_TRACK=1
```

Use either variable supported by the toolchain; setting both is safe.
