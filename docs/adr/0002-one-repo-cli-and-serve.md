# ADR 0002: 发布 CLI 与分发服务放在同一个仓库

- Status: Accepted
- Date: 2026-08-01

## 决策

`relkit`（发布 CLI）与 `relkit-serve`（自托管分发服务）合并为单一仓库 [`cnb.cool/shichao402/relkit`](https://cnb.cool/shichao402/relkit)。

- CLI：`cmd/relkit`
- 服务：`cmd/relkit-serve`，部署脚本在 `deploy/`

早期曾拆成两个 GitHub 仓库；二者同属一套 RUP 发布栈、发布节奏一致，分开维护没有收益。旧仓库 `relkit-serve` 归档，不再使用。

## 后果

- 业务方与 CI 只跟踪一个上游；Release 同时产出两类二进制。
- `go install cnb.cool/shichao402/relkit/cmd/relkit@latest` 与 `.../cmd/relkit-serve@latest` 均可。
- 文档与接入方引用改为指向本仓库，不再写「独立仓库」。
