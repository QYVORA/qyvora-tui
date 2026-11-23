# Third-Party Licenses

This file lists major third-party components shipped or vendored in this
repository. License texts are preserved in-repo at the referenced paths.

## OpenTUI (upstream fork base)

- Component: OpenTUI
- Upstream: https://github.com/anomalyco/opentui
- License: MIT
- Local license: `LICENSE`
- Notice: `NOTICE`

## Vendored Zig libraries (`packages/native/vendor`)

| Component | License | Local license file |
|---|---|---|
| libvaxis | MIT | `packages/native/vendor/libvaxis/LICENSE` |
| uucode | MIT | `packages/native/vendor/uucode/LICENSE` |
| zigimg | MIT | `packages/native/vendor/zigimg/LICENSE` |
| zg | MIT | `packages/native/vendor/zg/LICENSE` |

## Package/distribution notes

- This repository contains workspace package metadata and source for OpenTUI/QYVORA TUI packages; dependency lockfiles and installed `node_modules` are not treated as vendored source in this inventory.
- When adding or updating vendored third-party code, copy the upstream license text into the repository and update this file.
