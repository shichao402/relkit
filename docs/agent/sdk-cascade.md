# SDK 开箱级联

工具链开箱（[`toolchain-onboard.md`](toolchain-onboard.md)）解决「怎么发布」。  
本页解决「客户端怎么接」。

**唯一调用面**是生成的 `Updater` facade（ADR 0010 / `relkit.updater.v1`）。不要按语言各写一套 check/apply。Range、节流、skip、选路全部由 `relkit-updater` 引擎决定，**禁止**「以各 SDK 实现为准」。

## 级联规则（给 Agent）

1. 探测宿主语言。
2. 打开该语言 facade（Go `sdk/updaterfacade`，Dart `updater_facade.dart`，Node `updater_facade.ts`）。
3. 把同 SHA 的 `relkit-updater` 打进产品包（`scripts/consume.py`）。
4. 宿主只做：何时检查、UI、是否退出进程。

## 当前已登记 SDK

| 语言 | 包 | 开箱 |
|------|-----|------|
| Go | `cnb.cool/shichao402/relkit/sdk/updaterfacade` | 本文 + ADR 0010 |
| Dart | `rup_client` `Updater.open` | 同上 |
| Node | `rup-client` `Updater.open` | 同上 |

旧的 `AGENT-QUICKSTART.md` 描述冻结中的完整 SDK，新接入走 facade。

## 最小片段

```
opened = Updater.open(profile, runtime)
switch opened.updater.check(force: userInitiated)
  updateAvailable a: download(a.planId); apply(a.planId)
  fallbackRequired f: openBrowser(f.manualUrl)
  upToDate / throttled: quiet
  failed e: show(e.error.message)
```

方法名、变体名、错误码三语言逐字相同。TS 的 `code`/`sequence` 是 `bigint`。
