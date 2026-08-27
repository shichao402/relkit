# 给人看的更新索引（EdgeOne Makers）

这不是门户 CMS。发布完成后，人用浏览器打开一页：有哪些产品、哪个 channel 最新、点哪里下安装包。协议客户端不读这些文件。

公网 **COS 只放协议对象**（`directory/`、`index/`、`.pb`、产物）。HTML 不进桶：COS 根路径 403 是 ListBucket，不要为此打开静态网站源站。

## 文件从哪来

`relkit publish`（经 agent 或本机 CLI）会在产品 root 写出：

```text
.relkit/browse/index.html
.relkit/browse/<product>.html
.relkit/browse/catalog.json
```

把这两份 HTML 拷进本目录（可覆盖示例），提交后交给 EdgeOne Makers 部署。

内网 `local` 后端会另外把同一套文件写到数据面的 `browse/`，GET 即可；不必走 Makers。

## 部署（腾讯云 EdgeOne Makers）

与腾讯云 EdgeOne **个人版 CDN（约 29.9 元/月）不是同一产品**。Makers 是 Git 部署静态站；官方免费档长期 0 元/月（额度以 [定价页](https://pages.edgeone.ai/zh/pricing) 为准）。

1. 新建 Makers 项目，连本仓库或只含本目录的小仓。
2. 构建输出目录设为本文件夹（无构建命令，纯静态）。
3. 自定义域先不要抢 `updates.firoyang.com`。等 `raw.firoyang.com` 已双挂 COS、新客户端 `entryUrls` 走 `raw.`、旧客户端都升完，再把 `updates` CNAME 指到 Makers。

`entryUrls` 改不起；这页的域名改得起。

## 页上必须有的

产品卡片、channel 行、版本 / code / 日期、下载链、单产品页。不要把 `.pb` 当导航，不要外链字体或图。
