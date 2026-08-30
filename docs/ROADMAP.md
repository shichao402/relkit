# Roadmap

尚未排期的意向，不等于已接受的 ADR。落地前再写决策。

## 公网 browse：访问计数用 51.la

- **状态**：意向（方向已拍，未嵌代码）
- **背景**：公网给人看的目录在 EdgeOne Makers（契约 `sites/updates-index/`，现纯静态）。曾评估用 Edge Function + KV 做点击/下载计数。KV 是最终一致（边缘缓存最长 60s）、没有原子 `INCR`，全国并发会大量丢数；Blob 强一致仍是 `get` 再 `put`，又慢又不准；运行时拿不到边缘节点 ID，按节点分片做不成。自建 Redis/API 对个人目录页不划算。腾讯分析（MTA）已停服；灯塔偏腾讯系 App/游戏，个人静态站开不顺、也不把数字画回页面。
- **目标**：要计数，但只在统计后台看，不把次数画在产品卡片上。browse dump 的 HTML 嵌 51.la JS，PV 与下载点击用自定义事件上报到 51.la。请求打他们的服务，不打自己的 CVM / COS / Makers 函数。
- **不做**：Makers KV / Blob 热路径 `+1`；为计数开 `edge-functions/`；腾讯分析 / 灯塔；自建 Redis 或计数 API；把实时次数写进 `catalog.json`。
- **边缘函数**：继续留给无状态改写（鉴权、跳转、geo、功能开关）。KV 只当偶尔更新的配置盘，不当账本。
- **落点（规划）**：`internal/browse` 生成的 HTML 嵌入 51.la；契约 README 写明。内网数据面同一份 dump 会带上脚本；精确下载次数仍以 serve 面板已有计数为准，不在此重复造账本。
