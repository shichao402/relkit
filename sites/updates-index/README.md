# 给人看的更新索引（EdgeOne Makers）

这是 **relkit 仓库里的 Makers 站点契约**：占位页、本 README、以后若要函数再加 `edge-functions/`。部署时不要把整个 Go 仓库当站点。

协议客户端不读这些文件。公网 COS 只放协议对象（`directory/`、`index/`、`.pb`、产物）。HTML **不**进桶。

拓扑与 BrowseSink（Makers 只是其中一个实现）：[`docs/design/publish-topology.md`](../../docs/design/publish-topology.md)。

**不要**在发版时把 dump 拷进本目录再提交。本目录不是每次发布的拷贝目标。

## 发布怎么上去

`relkit publish` 总会在产品 root 写出：

```text
.relkit/browse/index.html
.relkit/browse/<product>.html
.relkit/browse/catalog.json
```

| | `HostsBrowse` 数据面（`local` / `http-put`） | `site.makers`（现网外网） |
|---|---|---|
| 站点根 | 数据面上的 `browse/` | EdgeOne Makers（现网项目 `relkit-updates-index`） |
| 要不要 Makers | 不要：本轮若只有 HostsBrowse 后端，即使配了 makers 也跳过 | 要：本轮有协议专用后端（如 COS）且配了 `site.makers` |
| 文件从哪来 | 同一份 dump | 同一份 dump |

产品仓库示例：

```json
"site": {
  "title": "Demo App",
  "makers": {
    "projectId": "makers-xxxxxxxx",
    "tokenEnv": "EDGEONE_PAGES_API_TOKEN"
  }
}
```

`tokenEnv` 默认 `EDGEONE_PAGES_API_TOKEN`。token 只进发布机环境，禁止写入仓库。`--to local` 或纯内网 `publishTo` 不会打 Makers。公网 COS 发布若没配 `site.makers`，publish 会警告：协议已提交，人页不会更新。

Makers 失败与 `site` / `latest` / `browse` 指针失败同类：协议 index 可能已经 live，要用 `--allow-partial` 才接受人页落后。

## 现在：纯静态

本目录 **没有** `edge-functions/`。Makers 就当普通静态站。不要为了目录页去开 KV。

以后若要函数，在本目录加 `edge-functions/`；内网发布不受影响，只要 publish 仍写出 `browse/`。动态方案若要减部署次数，优先静态壳 + COS 上的 `catalog.json`，不要 KV。

## 部署

与 EdgeOne **个人版 CDN** 不是同一产品。大陆加速区的 `*.edgeone.cool` 预设域名会 401，要给人打开必须绑已备案自定义域。

**不要动 `updates.firoyang.com`。** 等旧客户端都改认 `raw.` 之后，再把 `updates` CNAME 指到 Makers。

页上要有：产品卡片、channel、版本 / code / 日期、下载链、单产品页。不要把 `.pb` 当导航，不要外链字体或图。
