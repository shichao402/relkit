# 给人看的更新索引（EdgeOne Makers）

这是 **relkit 仓库里的 Makers 站点契约**：占位页、本 README、以后若要函数再加 `edge-functions/`。部署时不要把整个 Go 仓库当站点。

协议客户端不读这些文件。公网 COS 只放协议对象（`directory/`、`index/`、`.pb`、产物）。HTML **不**进桶。

拓扑与 BrowseSink（Makers 只是其中一个实现）：[`docs/design/publish-topology.md`](../../docs/design/publish-topology.md)。

**不要**在发版时把 dump 拷进本目录再提交。本目录不是每次发布的拷贝目标。

## 站点怎么上去

`relkit publish` 只更新 `site/<product>.json` 与 `latest/<product>/<channel>.json`。agent 随后运行与 `relkit-agent site-rebuild` 相同的全量重建：读取全部注册产品，静态渲染 `index.html`、所有产品页和 `catalog.json`。渲染产物从不读回合并。

| | `HostsBrowse` 数据面（`relkit-compatible`） | agent `site.makers`（现网外网） |
|---|---|---|
| 站点根 | 数据面上的 `browse/` | EdgeOne Makers（现网项目 `relkit-updates-index`） |
| 配置归属 | 产品 publish profile 的后端 | `/etc/relkit-agent/relkit-agent.json` 顶层 |
| 文件从哪来 | agent 全量 rebuild | 同一份完整 dump |

agent 配置示例：

```json
"site": {
  "makers": {
    "projectId": "makers-xxxxxxxx",
    "tokenEnv": "EDGEONE_PAGES_API_TOKEN",
    "region": "china"
  }
}
```

`tokenEnv` 默认 `EDGEONE_PAGES_API_TOKEN`。token 只进发布机环境，禁止写入仓库。rebuild 失败不回滚已提交的协议 index；修好站点配置后单独重跑 `relkit-agent site-rebuild`，不要重发产品版本。

## 现在：纯静态

本目录 **没有** `edge-functions/`。Makers 就当普通静态站。不要为了目录页去开 KV。

访问计数（未落地）：嵌 51.la，后台看报表，不把次数画在卡片上。不要用 Edge Function + KV/Blob 做 `+1`，不要自建 Redis，不要腾讯分析 / 灯塔。结论见 [`docs/ROADMAP.md`](../../docs/ROADMAP.md)。

以后若要函数，在本目录加 `edge-functions/`（无状态改写，不当计数账本）；内网不受影响。当前保持纯静态，不做页面 fetch/CORS/KV。

## 部署

与 EdgeOne **个人版 CDN** 不是同一产品。大陆加速区的 `*.edgeone.cool` 预设域名会 401，要给人打开必须绑已备案自定义域。

**不要动 `updates.firoyang.com`。** 等旧客户端都改认 `raw.` 之后，再把 `updates` CNAME 指到 Makers。

页上要有：产品卡片、channel、版本 / code / 日期、下载链、单产品页。不要把 `.pb` 当导航，不要外链字体或图。
