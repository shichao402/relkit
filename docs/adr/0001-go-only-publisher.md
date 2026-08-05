# ADR 0001: 发布工具只用 Go，不再使用 Python

- Status: Accepted
- Date: 2026-08-01

## 决策

本仓库是 RUP 发布工具的正式实现。早期曾用 Python（标准库 / zipapp）做过评审与原型，**后续不再使用，也不再保留该实现**。

## 后果

- 业务接入：下载 / `go install` 二进制，不再 vendor Python 源码。
- 协议夹具与权威行为以 Go 实现 + 本仓 `conformance/` JSON 为准。
- 文档与仓库中不保留 Python 发布工具代码路径。
