# ADR 0005: 签名的更新服务目录（bootstrap directory）

- Status: Accepted
- Date: 2026-08-09

## 背景

多区域部署 `relkit-serve`（例如中国 + 欧洲）时，付费 Geo-DNS / Anycast / CDN 性价比不合现阶段。  
需要一个**稳定、跟项目走、极少变更**的客户端入口，再动态发现当前可用的更新服务；SDK 用历史下载体验排序，而不做独立连通性探测。

## 决策

1. 引入 **bootstrap directory**（更新服务目录）作为对外稳定入口协议面。
2. 客户端内嵌 `entryUrls`（一主两备，串行）：**自有域名绑定 COS（主）** → CNB raw 备 → GitHub raw 备。单台 CVM 上的 HTTP 服务不作默认主入口。
3. 目录文档与 RUP index / fallback **共用同一把（组）ed25519 发布钥**签名；签名一次，字节相同后分别上传三处。
4. 目录内列出的各 `indexUrl` **必须**指向**字节完全相同**的已签名 index（与 SPEC「多镜像同一份字节」一致）；区域机房差异只通过 index/manifest/artifact 的 `urls[]` 镜像，**禁止**为各区域签发内容不同的 index。
5. SDK **禁止**为选源单独发起探测/测速下载；仅根据真实更新流程中的成功/失败与吞吐记录，决定下一次尝试顺序。

细节见 [docs/design/bootstrap-directory.md](../design/bootstrap-directory.md)、[docs/design/update-ingress-cos.md](../design/update-ingress-cos.md) 与 [`SPEC.md` §16](../../SPEC.md)。

## 后果

- App 内嵌入口长期稳定；加减机房主要改目录与 publish 双写，不必改客户端常量。
- 发布链必须能更新三处目录镜像，并保证各数据面（COS / serve / raw）上的 index/产物字节一致。
- 首次安装无历史记录时，仅按目录默认序；接受冷启动不最优。
- 目录验签失败与 index 一样：丢弃该 entry，试下一个；禁止降级为明文信任。
- 推荐拓扑下：发布控制面在可信 CVM；客户端读路径走自有域名 COS（可选 CDN）。
