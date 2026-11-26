# QYVORA TUI

QYVORA TUI is QYVORA's fork of [OpenTUI](https://github.com/anomalyco/opentui), used as the terminal UI foundation for QYVORA products. It preserves upstream OpenTUI architecture and MIT licensing while carrying QYVORA package identity and release controls.

**Readiness:** `INTERNAL` supporting terminal UI component, not a standalone public product and no public readiness claim. See the [Product Registry](../../../../knowledge/qyvora-docs/07-products/PRODUCT-REGISTRY.md).

OpenTUI is a library to build terminal user interfaces.

- It is written in Zig.
- You write TypeScript directly or through React and Solid.
- You arrange boxes and text with flexbox.
- You add selects, inputs, and scroll boxes with keyboard and mouse controls.
- You can play sounds, show images, play videos, and render 3D graphics.

Upstream: [anomalyco/opentui](https://github.com/anomalyco/opentui) | [OpenTUI docs](https://opentui.com/docs)

## Packages

- [`@opentui/core`](packages/core) - Native terminal UI library with TypeScript bindings.
- [`@opentui/react`](packages/react) - React renderer for building terminal user interfaces with OpenTUI Core.
- [`@opentui/solid`](packages/solid) - Solid renderer for building terminal user interfaces with OpenTUI Core.
- [`@opentui/keymap`](packages/keymap) - Key binding and command routing library for terminal and browser applications.
- [`@opentui/qrcode`](packages/qrcode) - QR code encoder and renderable with React and Solid surfaces.
- [`@opentui/three`](packages/three) - Three.js WebGPU renderer for OpenTUI.
- [`@opentui/ssh`](packages/ssh) - SSH server integration for OpenTUI applications.

**Private packages**

- [`@opentui/native`](packages/native) - Private workspace with the Zig implementation, native tests, and benchmarks.
- [`@opentui/examples`](packages/examples) - Private workspace with example applications and a standalone examples executable.
- [`@opentui/web`](packages/web) - Private workspace with the documentation website and the AI agent skill.
- [`packages/rust`](packages/rust) - Unpublished Rust bindings for the native C ABI and a framework built on them.

## Upstream AI agent skill

Upstream OpenTUI documentation can be installed as a skill with [`npx skills`](https://skills.sh):

```bash
npx skills add https://opentui.com --skill opentui
```

This installs upstream OpenTUI docs; QYVORA-specific behavior should be documented in QYVORA repositories. Disable Astro/web telemetry in local docs workflows with `ASTRO_TELEMETRY_DISABLED=1` or `DO_NOT_TRACK=1`.

## Development

Development requires Bun 1.4.1 or later and Zig 0.17.1.

See [AGENTS.md](AGENTS.md) for engineering, tooling, and verification conventions.

```bash
bun install
```

Run package scripts from their package directory. Run `bun run build` from the repository root after you change
native code or cross-package build outputs. Ordinary TypeScript changes do not require a root build.

Run repository tests and static checks from the repository root:

```bash
bun run test
bun run fmt:check
bun run lint
```

To link the source packages into a Bun-managed project, install that project's dependencies, then run
`./scripts/link-opentui-dev.sh /path/to/project` from the repository root. Add `--react` or `--solid` when needed.

## Contributing

This repository is a QYVORA-maintained fork. Preserve upstream MIT notices and attribution when porting or adapting upstream OpenTUI changes.

## License

MIT License. Copyright (c) 2026 QYVORA and upstream OpenTUI contributors. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
