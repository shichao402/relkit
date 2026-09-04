# ADR 0007: entryUrls 备援必须国内可达且缓存可控

- Status: Accepted
- Date: 2026-09-04
- Updated: 2026-09-04
- Supersedes: [ADR 0005](0005-signed-bootstrap-directory.md) 决策 2 中「CNB raw 备 → GitHub raw 备」的选型部分。ADR 0005 的其余决策（bootstrap directory 本身、同一把钥签名、镜像字节相同、禁止测速探测）继续有效。

## 背景

ADR 0005 定下 `entryUrls` 一主两备：自有域名 COS（主）→ CNB raw → GitHub raw。当时看重的是「免费、跟项目走、不必再买一套托管」。

两件事让这个选型站不住：

1. **`entryUrls` 写进二进制后几乎不可变**（SPEC §1.1、本 ADR 与 ADR 0005 共同前提）。选错备援不是配置错误，是要按 `update-ingress-cos.md` §8 双写迁移才能收场的错误。
2. **`directory/` 是可变指针**，`update-ingress-cos.md` §5 要求它短缓存（≤ 60s）或 no-cache，客户端还要能带缓存击穿参数（SPEC §3.1 / §12）。

按这两条重看原选型：

- **`raw.githubusercontent.com` 在中国大陆不可达或被污染。** 主入口是 COS 广州，用户主要在国内。主可达时备援用不上；主不可达时备援同样拿不到。失效域不正交，等于没有备援。
- **git 托管的 raw 端点缓存不可配。** `Cache-Control` 由平台决定，客户端也无法令其 revalidate。这正好命中 `update-ingress-cos.md` §5 点名的故障现场「发布成功但客户端几分钟内看不到更新」，且落在一个既查不了也调不了的地方。

## 决策

1. **entryUrls 备援的准入条件**，三条全部满足才可写进客户端：
   - **目标用户所在网络可达**（当前即中国大陆直连可达）。
   - **`Cache-Control` 由我方可配**，能满足 `directory/` 的短缓存 / no-cache 要求。
   - **失效域与主入口尽量正交**：不与主共用同一个桶、同一地域，也不共用同一张证书。
2. **备援选型：第二个 COS 桶（异地域）+ 独立自有二级域名，登记为第二个 `s3-compatible` backend。** 写入走 `publishTo`（将来可拆 `pointerTo`）。**禁止**把同一桶上的多个自定义域名配成多个 backend。
3. **git 托管的 raw 端点不再作为 `entryUrls` 备援。**
4. **全网崩坏保底不是远程页。** 宿主必须编译期内嵌 `recovery` 文案与官方手动入口（GitHub / CNB 等）。不走 Makers、不走 COS。这是 relkit 开箱硬门槛。
5. 本 ADR 约束 `entryUrls` 的 directory 镜像。签名 `fallback/<product>.pb`（SPEC §12.6）仍可写在主桶，用于「数据面还在、升级链断了」；它不是全网崩坏保底。

## backend 与域名

| | 一个 backend | 两个 backend |
|---|---|---|
| 存储 | 同一个 bucket | 两个 bucket（如广州 + 成都） |
| 写 | `publish.Run` 写一次 | 各写一遍 |
| 读 | 可选多个 GET 主机名（同桶别名，**不算**第二 backend） | `entryUrls` 各指各家 `baseUrl` |

验证期曾用成都桶 / `ap-chengdu` / `raw2.firoyang.com` 作第二 backend。2026-09-04 已按单独指令拆除（先从 `publishTo`/`entryUrls` 去掉，再解 DNS/证书续期/清空对象；桶由发布机凭据删除）。

## 后果

- 第二桶有存储与证书续期成本，所以验证期后端必须能按单独指令拆除。
- 账号级故障仍不正交。
- 已装客户端若只有一条 `entryUrls`，远程补不进第二条，只能自然升级或走内嵌保底后手动安装。
