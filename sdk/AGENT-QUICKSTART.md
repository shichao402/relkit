# Go SDK 开箱（Agent）

面向要在 **Go 进程**里接入 RUP 检查/下载的 Agent。  
协议细节见上游 SPEC；发版见 `relkit agent-guide`。本文只保证「依赖装上 → check/download 跑通」。

上级入口：[`../docs/agent/README.md`](../docs/agent/README.md) · 级联表：[`../docs/agent/sdk-cascade.md`](../docs/agent/sdk-cascade.md)  
Dart 同级 SDK：[`dart/AGENT-QUICKSTART.md`](dart/AGENT-QUICKSTART.md)

## G0. 何时用这份文档

- 宿主是 Go 服务/工具/带 Go 更新器的桌面壳
- 已有或即将有可用的 index URL 与内嵌公钥

**不要**在这里实现签名、选路、第二套版本解析。

## G1. 安装

```bash
go get cnb.cool/shichao402/relkit/sdk@latest
```

模块路径：`cnb.cool/shichao402/relkit/sdk`。

## G2. 最小可运行片段

把尖括号换成与 `relkit.json` / 发布侧一致的值：

```go
package update

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"cnb.cool/shichao402/relkit/sdk"
)

func CheckOnce(ctx context.Context) error {
	pub, err := base64.StdEncoding.DecodeString("<base64-32-byte-ed25519-pubkey>")
	if err != nil {
		return err
	}
	supportDir := os.TempDir() // 换成应用 support/config 目录
	u := &sdk.Updater{
		Product:     "<product>",
		Channel:     "stable",
		CurrentCode: 0, // 换成真实已装 code；可用 sdk.SemverCode(version)
		EntryURLs: []string{
			"https://updates.example.com/rup/directory/<product>.pb",
		},
		TrustedKeys: sdk.TrustedKeys{
			"<key-id>": pub,
		},
		ClientSelectors: map[string]string{
			"os":   "windows", // 或 linux / darwin —— 必须与 stage 时 selectors 一致
			"arch": "x64",     // SPEC §11.1：x64 / arm64 / x86 / armv7，不要写 amd64
		},
		StateStore: sdk.NewFileStateStore(supportDir, "<product>", "stable"),
		Recovery: &sdk.RecoveryHelp{
			Message: "Automatic updates are unavailable. Install from an official page.",
			Links: []sdk.RecoveryLink{
				{Label: "GitHub Releases", URL: "https://github.com/example/app/releases"},
				{Label: "CNB Releases", URL: "https://cnb.cool/example/app/-/releases"},
			},
		},
	}

	res := u.CheckForce(ctx, true)
	if res.Err != nil {
		return fmt.Errorf("check: %w (attempts=%v)", res.Err, res.Attempts)
	}
	if res.UpToDate || res.Available == nil {
		fmt.Println("up to date")
		return nil
	}

	dest := "update-download.bin"
	if err := u.Download(ctx, res.Available, dest); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	fmt.Println("verified file:", dest)
	// 单二进制自替换见 sdk/apply；目录型应用仍由宿主处理。
	_ = os.Remove
	return nil
}
```

## G3. 必须对齐的字段

| SDK 字段 | 发布侧来源 |
|----------|------------|
| `Product` | `relkit.json` `product` |
| `Channel` | index 路径中的 channel |
| `CurrentCode` | 已安装版本的 RUP code（可用 `sdk.SemverCode`） |
| `EntryURLs` | 自有域名上的 `directory/<product>.pb`（优先于 IndexURLs） |
| `IndexURLs` | 无 directory 时的直连兜底 |
| `TrustedKeys` | `keygen` 公钥；**编译进二进制**，禁止运行时下载 |
| `ClientSelectors` | 与 `relkit stage --add … os=…,arch=…` 一致 |
| `Recovery` | `relkit.json` `recovery`；编译期内嵌，远程全失败时展示 |

## G4. 能力边界（避免重复造轮）

当前 Go SDK（与 Dart 对齐矩阵见 [`README.md`](README.md)）：

- 有：`Check` / `CheckForce`、`EntryURLs` directory 引导、StateStore 节流、源学习排序、`Download`（Range 续传 / 并行分块 / 进度）、`sdk/apply` 单二进制自替换、`UpdateScheduler`
- 未对齐 Dart：便携目录 swap / macOS DMG 解包（留给宿主或 Dart SDK）

## G5. 开箱 Done

- [ ] `go build` 含 sdk 导入通过
- [ ] 对真实或 `local` 发布的 directory/index：`CheckForce` 返回预期
- [ ] `Download` 后文件 hash 与 manifest 一致
- [ ] README/内部文档写明公钥轮换：先双钥并存发一版，再删旧钥
- [ ] Apply 策略有书面说明（`sdk/apply` 或宿主自定义）
- [ ] 远程 check 失败时 `result.Recovery` 仍能给出内嵌文案与官方链接

## G6. 排障

| 现象 | 方向 |
|------|------|
| 验签失败 | keyId/公钥是否与签名 index 一致；是否用了错误环境的 key |
| 总是 up-to-date | `CurrentCode` 是否已经 ≥ head |
| 无 artifact | selectors 与 stage 不匹配；或该版本只发了单平台 |
| HTTP 200 但业务失败 | index 被缓存；COS/CDN 须对 `directory/`/`index/` 短缓存 |
| Throttled | `CheckForce(ctx, true)` 强制检查；或清 StateStore |

发布/serve/agent 问题改读 `relkit agent-guide` / `docs/design/publish-agent.md`。

## Fallback (SPEC section 12.6)

Set `Updater.FallbackURLs`（或由 directory `services[].fallbackUrl` 自动带上）。`Check` merges
results so `Available` beats `Fallback`. Call `CheckFallback` after a download/apply
failure to urge a manual update page.
