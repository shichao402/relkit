# 内网 update.devcloud.woa.com 黄金路径验证（2026-09-04 / 09-07）

> 用户口头说的 `update.devops.woa.com` 无 DNS。现网与仓库文档统一为 `update.devcloud.woa.com`（`9.134.240.235:36000`）。

## 同构结论

与外网 COS 架构同构于：

- 控制面：`relkit-agent`（`127.0.0.1:8787`）+ nginx `/v1/`
- 数据面：路径型 RUP 树匿名 GET
- 协议：同一套 stage / staged-put / publish / directory / index / manifest / artifact

差异只在基础设施 adapter：

| 层 | 外网 | 内网 |
|---|---|---|
| 存储 | COS `s3-compatible` | 磁盘 `local` + `relkit-serve` |
| 人页 | Makers | `browse/` |
| TLS | 本机 / COS 自定义域名 | WOA 入口终止 TLS，箱上 `:80` |
| 证书续期 | `relkit-cos-cert-renew.timer` | N/A |

## 已落地（发布机）

- `relkit-agent` 升级为 `0.1.2+8a0b312`
- `-migrate-profile`：`/etc/relkit-agent/products/svn-auto-merge.json`（`local` → `/data/relkit-serve`，`https://update.devcloud.woa.com/`）
- 产品根 `relkit.json` → `relkit.json.migrated`
- 为 staged `0.2.0+112` 补上 `release-policy.json`
- `relkit-agent onboard check`：全部 passed / N/A（local 无第二 entryUrl、无 COS 证书项属正常）
- 因操作失误曾短暂暴露旧 upload token → 已轮换并重启；旧 token 失效
- nginx：非 `/v1/` 的 PUT = 403
- 2026-09-07：已吊销 `svn-auto-merge` 产品 token，并去掉运营方 `uploadTokenFile`；`relkit-serve` 只读，直连 PUT = **405**

## 已验证（只读）

- 本机 HTTPS：`/-/health`、`directory`、`index/dev`、`browse/`、artifact Range 均通过
- 客户端视角：`dart run scripts/verify_rup_release.dart 0 --channel dev --download` 成功拿到 `0.2.0+112` 并校验哈希

## 宿主仓（SvnMergeTool）

HTTPS URL、`recovery` 编译期保底、去掉 CI 私钥依赖、钉 `relkit` `8a0b312`。发布仍只走 `relkit-agent`，不走 serve PUT。

## 仍待本机操作

1. Console Authenticate 解锁后，把新 `RELKIT_UPLOAD_TOKEN` `dec push` 到项目私密资产
2. 更新蓝盾 `relkit_upload_token`（轮换后旧值会 401）

## 探针备忘

箱上没有 443 监听；从箱内测公网域名会失败。用本机 HTTPS，或箱内 `http://127.0.0.1[:8080]`。
