# ADR 0025: releaseCode 由服务器分配

- 状态：Accepted（2026-11-04 用户裁决）
- 日期：2026-11-04
- 关联：取代 [ADR 0021](0021-vnext-artifact-runtime-registry.md) 中“产品生成的单调 `releaseCode`”一条；延续 [ADR 0024](0024-server-ssot-registration.md) 的服务器单一事实源原则

## 背景

一个发布版本有两部分身份：

- `x.y.z`（以及渠道）：人的决定，表达兼容承诺、何时发 stable。属于产品意图。
- `releaseCode`：只用来给同一产品的发布排序，客户端用它比较新旧、给安装目录命名。没有人的含义。

ADR 0021 规定 releaseCode 由产品生成，Operator 不得从 tag 或仓库规则二次猜测。宿主于是各自拿 `git rev-list --count HEAD`、CI 序号之类的东西充当来源。但判定它是否合法的唯一依据在服务器上：`publish.Run` 要求新 code 大于该渠道已有的最大 code。比较基准在服务器，数字却由宿主猜。

2026-11-04 的实际事故：novoard-agent 在同一提交上第二次手动发 dev，提交数仍为 37，与已发布的 `0.3.3-dev.37+37` 撞号。服务器正确拒绝，但拒绝发生在约 760MB 的 CAS 全部上传之后。用提交数还会在 squash、force push、改写历史时倒退。

## 决策

1. releaseCode 是服务器事实。每个注册产品在 agent admin state 里有一个 `lastReleaseCode` 计数器，与注册记录放在一起。
2. 发号接口 `POST /v1/release-code {"product":"<id>"}`，持该产品的发布 token。新号 = `max(lastReleaseCode, 该产品所有服务器渠道已发布的最大 code) + 1`。递增与落盘在同一把锁里完成，落盘后才返回，所以并发领号不会重号，重启不会重发。
3. 一个产品跨渠道共用一条号序。dev 与 stable 的号互不重叠，任何两个发布都能直接比较。
4. 领了没用的号只留下空号。code 只用于排序，不要求连续。
5. 已发布的历史就是下限。服务器接管前已经发过版本的产品不需要迁移，第一次领号自动接在已发布最大值之后。
6. `GET /v1/release-code?product=<id>` 只读返回计数器和已发布最大值。`relkit release --execute` 在上传前先查一次，staged code 不大于已发布最大值就立刻失败，不再先传完产物。旧 agent 没有这个接口时跳过预检，回到原来只在提交时检查的行为。
7. CLI：`relkit release-code claim` 在 stdout 只打印号码，供 CI 捕获；`relkit release-code show` 打印状态。服务器地址和 token 的取法与 `relkit release` 相同。
8. 提交时的单调检查保留，作为最后一道防线；报错带稳定码 `release.code_not_monotonic`，并指向领号命令。`--allow-backfill` 逃生门不变。

宿主的版本号 `x.y.z` 仍由宿主仓库的 `VERSION.json` 决定（ADR 0004），服务器不接管。

## 宿主接入方式

CI 在构建前领一次号，把它和 `VERSION.json` 的 `x.y.z` 组合成本次发布版本，再交给 producer 编进二进制：

```bash
code="$(relkit release-code claim)"
# dev:    <x.y.z>-dev.<code>+<code>
# stable: <x.y.z>+<code>
```

一次发布只领一次。多平台矩阵共用同一个号，所以必须在矩阵之前的单个 job 里领。

## 影响

- 服务器需要升级到含本 ADR 的 relkit；admin state 增加 `lastReleaseCode` 字段，旧文件读入时视为 0，不需要迁移。
- 宿主不再从 git 历史或 CI 计数器推导 releaseCode，同一提交可以反复发布。
- 领号需要发布 token，所以必须在持有 token 的 job 里进行。
