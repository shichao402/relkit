# Go SDK 开箱（Agent）

面向要在 **Go 进程**里接入 RUP 检查/下载的 Agent。  
协议细节见上游 SPEC；发版见 `relkit agent-guide`。本文只保证「依赖装上 → check/download 跑通」。

上级入口：[`../docs/agent/README.md`](../docs/agent/README.md) · 级联表：[`../docs/agent/sdk-cascade.md`](../docs/agent/sdk-cascade.md)

## G0. 何时用这份文档

- 宿主是 Go 服务/工具/带 Go 更新器的桌面壳
- 已有或即将有可用的 index URL 与内嵌公钥

**不要**在这里实现签名、选路、第二套版本解析。

## G1. 安装

```bash
go get github.com/shichao402/relkit/sdk@latest
```

模块路径：`github.com/shichao402/relkit/sdk`。

## G2. 最小可运行片段

把尖括号换成与 `relkit.json` / 发布侧一致的值：

```go
package update

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/shichao402/relkit/sdk"
)

func CheckOnce(ctx context.Context) error {
	pub, err := base64.StdEncoding.DecodeString("<base64-32-byte-ed25519-pubkey>")
	if err != nil {
		return err
	}
	u := &sdk.Updater{
		Product:     "<product>",
		Channel:     "stable",
		CurrentCode: 0, // 换成真实已装 code；version-build 时通常等于 VERSION +build
		IndexURLs: []string{
			"http://<host>/index/<product>/stable.pb",
		},
		TrustedKeys: sdk.TrustedKeys{
			"<key-id>": pub,
		},
		ClientSelectors: map[string]string{
			"os":   "windows", // 或 linux / darwin —— 必须与 stage 时 selectors 一致
			"arch": "x64",
		},
	}

	res := u.Check(ctx)
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
	// Apply：宿主自己负责（换文件、重启服务等）。本 SDK 故意不包含。
	_ = os.Remove // 占位提醒：失败清理由宿主定策略
	return nil
}
```

## G3. 必须对齐的字段

| SDK 字段 | 发布侧来源 |
|----------|------------|
| `Product` | `relkit.json` `product` |
| `Channel` | index 路径中的 channel |
| `CurrentCode` | 已安装版本的 RUP code（常 = `relkit version code`） |
| `IndexURLs` | `http(s)://…/index/<product>/<channel>.pb` |
| `TrustedKeys` | `keygen` 公钥；**编译进二进制**，禁止运行时下载 |
| `ClientSelectors` | 与 `relkit stage --add … os=…,arch=…` 一致 |

## G4. 能力边界（避免重复造轮）

当前 Go SDK：

- 有：`Check`、镜像串行、`Download` + size/sha256
- 无 / 弱：进度回调、多线程 Range、断点续传、Apply（Dart 侧已更强；Go 对齐见 roadmap，未完成前不要假称支持）

若产品需要强下载体验且是 Dart/Flutter UI，优先 Dart `rup_client`。

## G5. 开箱 Done

- [ ] `go build` 含 sdk 导入通过
- [ ] 对真实或 `local` 发布的 index：`Check` 返回预期
- [ ] `Download` 后文件 hash 与 manifest 一致
- [ ] README/内部文档写明公钥轮换：先双钥并存发一版，再删旧钥
- [ ] Apply 策略有书面说明（哪怕是「仅下载人工安装」）

## G6. 排障

| 现象 | 方向 |
|------|------|
| 验签失败 | keyId/公钥是否与签名 index 一致；是否用了错误环境的 key |
| 总是 up-to-date | `CurrentCode` 是否已经 ≥ head |
| 无 artifact | selectors 与 stage 不匹配；或该版本只发了单平台 |
| HTTP 200 但业务失败 | index 被缓存；`relkit-serve` 须对 `index/` no-cache |

发布/serve 问题改读 `relkit agent-guide` / `relkit-serve agent-guide`。
