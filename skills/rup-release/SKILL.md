---
name: rup-release
description: >
  用 RUP 协议与 relkit 工具执行更新分发：固化产物、生成签名清单、上传到多个后端、维护版本链条与升级路径约束（minFrom）。
  当用户要求发布新版本、发版、release、撤回某个版本、yank、设置强制更新下限、为项目接入自动更新能力，
  或排查客户端收不到更新时使用。仅适用于存在 relkit.json 的项目。
---

# RUP 发布

## 第一步：读操作手册

**执行任何写操作之前，必须先完整读一遍操作手册。** 它是操作性知识的唯一来源，本 skill 只声明能力边界，不复制任何操作细节 —— 手册更新后 skill 无需同步。

按顺序查找：

1. 本仓库 [`embed/AGENT-GUIDE.md`](../../embed/AGENT-GUIDE.md)
2. `relkit agent-guide`（工具内嵌副本，工具已安装时可用）

两处都找不到就**停下来告知用户**，说明手册缺失。**禁止**凭记忆或推测执行发布操作 —— 这套流程里有若干发布后无法补救的错误，手册的主要价值就是标出它们。

## 能力范围

| 用户意图 | 手册章节 |
|---|---|
| 发布一个新版本 | §3 流程 A |
| 为项目接入 RUP | §4 流程 B |
| 撤回坏版本 | §5 流程 C |
| 客户端收不到更新 | §8 流程 D |
| `code` / `minFrom` / 后端怎么选 | §6 决策表 |
| 校验报错了怎么办 | §7 错误码处置 |

不属于本 skill 的范围：修改协议本身（改 `SPEC.md` / `proto/` 必须同步更新 `schema/` 与 `conformance/`，跑 `scripts/gen-proto.ps1`，并重跑用例）、实现客户端运行时（照 `SPEC.md` §12 实现并用 `conformance/` 验证）。

## 先实测工具是否可用

跑 `relkit --version`，不要假设。失败就直接告知用户工具不可用并询问是先安装还是先实现，**禁止**声称执行了 `relkit` 命令，**禁止**编造命令输出。

已实现：`init` `keygen` `stage` `inspect` `simulate` `publish` `verify` `agent-guide` `backends`；后端有 `local`（离线全流程）、`static-http`（任何按路径提供 HTTP 下载的托管，不配 `stageDir` 则为只读审计用）、`http-put`（带鉴权 PUT 上传）。要自托管下载，用同仓的 `relkit-serve`（`cmd/relkit-serve`）。

**未实现：`s3-compatible`（COS）/ `github-release` / `cnb-release` 后端，以及 `yank` / `unyank` / `min-supported`。** 用 `relkit backends` 确认当前构建实到哪一步。需要工具自带凭据上传到对象存储的场景得先实现对应后端；**禁止**用 `local` 后端加手工上传冒充正式发布 —— 那样绕过了「指针最后写」的保证，改用 `static-http` + `stageDir` 或 `http-put`。

自检：在本仓库跑 `go test ./internal/chain ./internal/selectors ./internal/envelope`。

## 与其他发布 skill 的分工

判据是项目根目录有没有 `relkit.json`：

- **有** → 用本 skill，发布经由 relkit 完成。
- **没有** → 本 skill 不适用。若项目是「改版本文件触发 CI 打 tag」那种流程，用 `cli-release-workflow`。
- **判断不了** → 问用户，不要猜。把 RUP 的概念硬套到用别的机制发布的项目上，只会产生一堆无法执行的建议。

## 完成后

按手册 §10 汇报。最关键的一条：若中途失败，必须说明失败发生在写 `index` 指针**之前**还是**之后** —— 这决定了新版本对客户端是否已经可见，也决定了直接重跑是否安全。
