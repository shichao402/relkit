# relkit 现行架构

本文记的是 relkit 现在长什么样、哪些规矩不能破。ADR 记的是"当时为什么这么定"，两者冲突时以本文为准，并把 ADR 标为 Superseded。

改动本文任何一条规矩，先写一份 ADR 说明为什么破，再改本文。两者不得矛盾。

依据：[ADR 0023](adr/0023-desktop-updater-epoch.md)（桌面更新断代）。

## 一、定位

relkit 只做 Windows 与 macOS 桌面应用的更新。

范围内的平台：`windows`、`macos`。范围外且明确不支持的：Linux、Android、iOS、移动端沙箱环境、浏览器内自更新。selector 与 target 维度里不得为范围外平台保留取值。

`relkit-updater` 是唯一引擎：`cmd/relkit-updater` + `internal/updater`。更新状态机只实现一次，宿主与 relkit 之间唯一的客户端契约是 `api/updater/v1` protobuf 帧 IPC（4 字节大端长度前缀 + protobuf 载荷）。当前 IPC 基线是 4。

各语言只有薄 IPC 客户端，职责限于：找到引擎可执行文件、启动子进程、编解码帧、把事件交给 UI。

| 宿主 | 语言 | 薄客户端 |
| --- | --- | --- |
| SvnMergeTool | Dart | `sdk/dart/lib/src/updater_facade.dart` |
| Loom-Launcher | Rust | `sdk/rust/src/lib.rs` |
| Loom | TypeScript | `bindings/ts`（仅生成类型） |

薄客户端不做这些事：解析 bootstrap、解析 index/manifest、验签、算摘要、决定版本、管理安装目录。这些全部在引擎里。发现任何一处客户端开始解析协议文档，就是架构被破坏，应删除而不是补齐。

Go 侧 facade 在 `internal/updaterfacade`，属仓内实现，不对外发布。

## 二、信任

每台 agent 服务器一把根密钥（Ed25519）。根密钥的私钥只在这台服务器上，离线备份至少一份且位于所有 git 仓库之外，开发机与 CI 不持有，不同服务器之间不共用。

根密钥只证明一件事：这份文档或文件是这台服务器发出的、没被改过。它不承担任何鉴权职责。

它签这些对象：

- Runtime Profile（服务器是谁、端点在哪、允许哪些渠道）
- Catalog（服务器提供哪些 relkit 组件、各版本摘要、当前标记哪个版本）
- 各产品的 index 与 manifest（有哪些版本、每个文件的摘要）
- 工具清单与工具文件的 SSH 签名

文件本身只校验摘要，不另外签名。也就是说：文档靠根签名认证，文档里点名的文件靠摘要绑定。两层就够，不需要第三层。

明确没有的东西：

- 没有产品级签名密钥。同一台服务器上的私钥一起泄露，分开放不产生隔离效果。
- 没有第二层签名文档。index 路径已经表达"这份文档属于这个产品"。
- 没有 Distribution Root、Candidate Root、Runtime Profile Root。一台服务器一把根。
- 没有 candidate 与 certified 的二分，没有 fleet 认证登记，没有 watermark。

一个应用只认一台服务器的根，不同时信任多把。根公钥来自它注册的那台服务器产出的接入配置，原样写进宿主项目配置，CI 用它验工具，宿主构建把它写进 Runtime Bootstrap 供 updater 使用。

Runtime Bootstrap（`relkit.runtime-bootstrap/2`）内嵌这一把根公钥，本身不签名：它携带的 Runtime Profile 由同一把根签名，updater 用内嵌公钥验证。bootstrap 是构建输入，不是信任对象。

不设计换根流程，不做新旧根重叠窗口。保管是唯一手段：服务器一份、离线备份一份、登记在凭据清单、启动自检与定期自检都做签名往返确认归属。

## 三、凭据

token 只表示身份和权限：这个宿主项目能在 agent 上操作哪些产品。

- 服务器只存 token 的 `hex(sha256)`，明文只在签发时显示一次。
- 每条凭据绑定唯一产品，越权返回明确错误，同时点名 token 授权的产品与请求的产品。
- token 不做任何验证服务器或验证内容的事。验证服务器只靠根公钥。
- 宿主 CI 只需要注入这一个 token。下载 relkit 组件不需要 token，也不需要 mirrors 凭据。

所有程序与脚本只按 keyId 经清单读取模块取钥匙，不直接读密钥文件或凭据环境变量。

清单 schema 是 `relkit.custody-manifest/1`，每条登记 keyId、种类、用途、载体（`file` 或 `env`）、路径、公钥指纹。清单本身不存任何秘密。

清单不是一份全局文件，而是每个持有钥匙的地方各有一份，格式相同、读取代码相同：服务器（根私钥与 token 存储位置）、宿主 CI（只写 keyId 与环境变量名）、离线备份处。

取值路径统一：Go 程序用 `internal/custody`；Python 与 shell 脚本调用 `relkit custody get` / `check` / `public`，不各写一份解析。

启动自检必须覆盖每个要用的 keyId：清单里有登记、载体可读、私钥与登记的公钥指纹一致（签名往返）。任一不满足立即退出，报出 keyId 与原因，不降级、不跳过。

token 的具体实例以管理面板数据为准；清单只登记 token 存储的位置，不逐个登记 token。

这条规矩靠评审遵守，不做自动检查。

## 四、分发

宿主项目只持有四样东西：服务器地址、根公钥（公钥本身，不只是指纹）、产品名、token（配在 CI 密钥里）。

宿主仓库不提交 `relkitw`、`relkit.lock.json`、`.relkit/` 下的指纹与 resolution 文件，不钉任何 relkit 版本或组件摘要。

工具一律从服务器取。宿主 CI 需要 relkit 工具时：下载工具与工具的根签名 → 用项目里的根公钥验签 → 验签通过才执行。验签失败立即失败，不降级到 GitHub、mirrors 或其他来源。

验签用构建机自带程序：服务器为每个工具额外产出一份 SSH 签名（namespace 固定 `relkit-tool`），CI 用 `ssh-keygen -Y verify` 对照 `allowed_signers` 文件校验。这一步固定为"下载、验签、执行"三条命令，脚本内不含版本号、文件名、摘要或 bundle 地址，所以可以写进流水线 yaml 并冻结。哪个版本、哪个文件名由服务器已签名文档决定。

工具启动后再从同一服务器取 Profile 与 Catalog，同样用根公钥验签。

服务器是唯一事实源：relkit 的版本、组件、摘要只在服务器上维护一份，内网与外网同一套流程，只有地址和根密钥实例不同。

宿主项目配置（`relkit.project/1`）只有这些字段：`product`、`server`、`rootPublicKey`、`channels`、`selectorDimensions`、`artifactProducer`、`versionProvider`、`site`。读到控制面字段明确报错，不保留读取旧字段的兼容路径。

选择器维度由项目自由声明（`SelectorDimensions`），代码里没有平台白名单。只注入 `os`/`arch`（取 `runtime.GOOS`/`GOARCH`）并强制 `apply=relkit-payload`。

## 五、已接受的代价

这两条是明知有代价仍然接受的，不是待修缺陷。看到它们时不要"顺手修掉"，那会重新引入已被删除的复杂度。

换根要已装应用重装。没有重叠窗口、没有双签过渡，换根当天那把服务器上所有已装应用失去信任链。缓解手段只有妥善保管根私钥，没有技术兜底。这也是"私钥找不到先跑自检、脚本说没有才算真的没有"这条硬规则的由来。

CI 可能被重放旧 Catalog。宿主 CI 构建机不保留任何状态，理论上人可以重放一份旧的、签名有效的 Catalog，让 CI 用上旧版工具。最终用户不受影响：updater 对产品 index 的防回滚保留。引入 watermark 能让 CI 察觉重放，但会让 CI 持有跨构建状态，而 CI 持有状态正是上一代"修了又坏"的来源之一，因此选择接受重放风险。

## 六、改规矩的流程

要引入新的签名层、新的密钥种类、新的工具来源，或者要突破本文任何一条规矩：

1. 先写一份 ADR，说明为什么现在的规矩不够、新方案怎么解决、代价是什么。
2. 该 ADR 必须明确废止或修订 [ADR 0023](adr/0023-desktop-updater-epoch.md) 的对应条目。
3. ADR 通过后再改本文。

两者不得矛盾。本文与 ADR 不一致时，说明有一处该更新了，不存在"以哪份为准"的模糊地带。

不在 CI 里放检查这些规矩的步骤或守卫脚本。规则写得严会拦住正常改动，写得松又拦不住，而检查本身会变成新的拦路虎。规矩靠评审遵守，靠本文与 ADR 的显式记录传递。

## 相关

- [ADR 0023](adr/0023-desktop-updater-epoch.md)：桌面更新断代决策
- [ADR 0019-0022 驳斥](adr/0019-0022-refutation.md)：上一代设计为何被拆、哪些部分保留
- [ADR 0010](adr/0010-single-updater-engine.md)：单引擎决策的前身
- [ADR 0013](adr/0013-internal-update-payload.md)：payload 文件表形态
- [信任根密钥托管规范](guide/trust-root-custody.md)：密钥放哪、怎么确认在、丢了怎么办
