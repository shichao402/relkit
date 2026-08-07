# relkit (archived monorepo)

> **This repository is archived.** RUP has been split into separate repositories.

| Piece | Repository |
|-------|------------|
| Protocol SSOT (SPEC / proto / shared Go) | https://github.com/shichao402/rup |
| Publisher CLI (`relkit`) | https://github.com/shichao402/relkit-cli |
| Distribution server (`relkit-serve`) | https://github.com/shichao402/relkit-server |
| Go client SDK | https://github.com/shichao402/sdk-go |
| Dart client SDK (`rup_client`) | https://github.com/shichao402/sdk-dart |

## Install (post-split)

```bash
go install github.com/shichao402/relkit-cli/cmd/relkit@latest
go install github.com/shichao402/relkit-server/cmd/relkit-serve@latest
go get github.com/shichao402/sdk-go@latest
```

History before the split remains here for reference. Prefer the sibling repos above for new work.

> Note: a future move under the `relkit` GitHub org may rebrand module paths; until then modules live under `github.com/shichao402/…`.
