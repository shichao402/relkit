# ADR 0024: 服务器单一事实源——渠道与发布配置在服务器注册，首份 Runtime Profile 随注册签发

- 状态：Accepted（2026-11-03 用户裁决）
- 日期：2026-11-03
- 关联：[ADR 0023](0023-desktop-updater-epoch.md)（服务器单一事实源原则的前身，本 ADR 把它推到产品注册这一步）、[`docs/architecture.md`](../architecture.md)

## 背景

宿主接入 relkit 时，`relkit bootstrap build` 需要读到服务器根签名的 Runtime Profile，才能把根公钥和 Profile 打进安装包。而 Profile 原本只在第一次 release 时签发，release 又依赖编译出的安装包，形成死结。宿主只能手动调用 `POST /v1/runtime-profile`，并且要把整份 `relkit.project.json` 发过去。

逐个看 Profile 的输入，没有一项需要宿主提供：

- `product`：注册时已经给出。
- `allowedChannels`：所有现役产品都是 `dev`、`stable`，宿主自定渠道从未被用过。
- `directoryBootstrap.entryUrls`：由服务器发布配置的 `baseUrl` 推出。
- `profileVersion`：取 directory 序号，是服务器状态。
- 签名：服务器根。

每产品的发布配置（`/etc/relkit-agent/products/<id>.json`）在实际部署里只有 `product` 字段不同，其余都是机器事实。

## 决策

原则：能在服务器注册保存的东西，不放在宿主项目里。项目只声明服务器无法知道的东西（产物怎么产出、版本号从哪来、人页文案）。

1. 渠道是服务器事实。标准渠道集定义在 `internal/model/channels.go`，值为 `["dev","stable"]`。`relkit.project.json` 的 `channels` 改为兼容字段：省略，或者写成与服务器完全相同的值，写其他值直接报错。staged 文档的渠道必须属于服务器渠道集。
2. 发布配置由服务器级默认值派生。`relkit-agent.json` 新增顶层 `publish`（`backends`、`publishTo`、`directory.publishTo`），每个注册产品都从它派生发布配置，只替换 `product`。`directory.entryUrls` 和 `directory.services` 带产品路径，不能放进默认值。已存在的每产品文件仍然优先，旧服务器行为不变。
3. 首份 Runtime Profile 随注册签发。`POST /v1/admin/products` 在同一请求里签发 Profile 并发布 directory，响应带 `runtimeProfile.signed`。签发失败不回滚注册，agent 每次启动会对所有没有 directory 的产品补签。注册表单只需填产品名。
4. `POST /v1/runtime-profile` 保留，用于按需重签，请求只需 `product`。`project` 字段仍然接受，只做身份校验。

## 渠道顺序

updater 把 `allowedChannels[0]` 当作默认渠道，并按位置比较新旧 Profile 的渠道列表。因此标准渠道集保持现有顺序 `dev`、`stable`，不调整。改顺序会改变所有现役客户端的默认渠道。

## 影响

- 新产品接入流程：注册产品（只填 id），然后直接 `relkit bootstrap build`。
- 服务器部署需要 `relkit-agent.json` 的 `publish` 块和 agent 环境里的 `RELKIT_SERVE_TOKEN`。已有机器用 `upgrade --public-upload-url ... --publish-read-url ...` 写入一次；读地址由操作者明确给出，不从域名推断，因为它会进入客户端信任的 Runtime Profile。没配时 agent 启动打印 WARNING，注册仍会成功，但不会签发 Profile。
- 宿主 `relkit.project.json` 里的 `channels` 可以删除。
