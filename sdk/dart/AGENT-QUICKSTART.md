# Dart `rup_client` 开箱（Agent）

面向要在 **Dart / Flutter** 进程里接入 RUP 的 Agent。  
上级入口（工具链 + 级联）：[`../../docs/agent/README.md`](../../docs/agent/README.md)。

本文保证：依赖就位 → `check` / `download` 可跑 → 宿主知道 apply 边界。

## D0. 包位置

**SSOT：** 本文件位于 `relkit/sdk/dart/`（与 Go `relkit/sdk` 并列）。

```yaml
# pubspec.yaml — 推荐
dependencies:
  rup_client:
    git:
      url: https://github.com/shichao402/relkit.git
      path: sdk/dart
```

内网 CI 若拉不到 GitHub，可在产品仓保留镜像副本，但改动必须先合入本目录再同步。

## D1. 安全与职责（先读再写代码）

| 做 | 不做 |
|----|------|
| 内嵌公钥、喂 product/channel/code/selectors | 联网下载公钥 |
| 调用 `RupUpdater.check` / `download` | 自己再比一次版本号「防呆」 |
| UI / 节流 / 是否安装 / apply | 重写验签、选路、sha256 |
| 保留用户数据目录 | 把用户数据放进会被整包替换的安装目录却不进 preserve 列表 |

## D2. 最小接入清单

### D2.1 常量（与发布侧逐字一致）

在宿主服务文件中集中定义（参考 SvnMergeTool `lib/services/update_service.dart`）：

```dart
const product = 'your-product';
const indexUrls = [
  'http://your-host/index/your-product/stable.pb',
];
const publicKeys = {
  'your-key-id': '<base64-ed25519-pubkey>',
};
```

### D2.2 构造 Updater

```dart
import 'package:rup_client/rup_client.dart';

final updater = RupUpdater(
  product: product,
  channel: 'stable',
  currentCode: currentInstalledCode, // 常来自 VERSION / package_info 的 build
  indexUrls: indexUrls.map(Uri.parse).toList(),
  trustedKeys: TrustedKeys.fromBase64(publicKeys),
  clientSelectors: {
    'os': Platform.isWindows
        ? 'windows'
        : Platform.isMacOS
            ? 'macos'
            : 'linux',
    'arch': 'x64', // 按真实 ABI 填写，并与 stage selectors 一致
  },
  stateStore: FileUpdateStateStore(
    // 每 product+channel 一个文件；勿与用户文档混放成可被清错的路径
    directory: supportDir,
    product: product,
    channel: 'stable',
  ),
  // policy: 可调 downloadConcurrency / retries；默认已启用 Range 多连接 + 续传
);
```

### D2.3 check → download

```dart
final result = await updater.check(force: true); // UI「检查更新」用 force
switch (result) {
  case UpToDate():
    break;
  case UpdateAvailable():
    final file = await updater.download(
      result,
      destinationDir: stagingDownloadDir,
      onProgress: (p) {
        // p.received / p.total / p.bytesPerSecond / p.eta
      },
    );
    // file.file 已通过 sha256；再交给 apply 或提示用户
  case CheckFailed(:final reason, :final attempts):
    // 展示 reason；细节打日志
  case CheckThrottled():
    break;
}
updater.close();
```

### D2.3b 启动 + 周期自动检查（SDK 能力）

```dart
final scheduler = UpdateScheduler(
  check: ({bool force = false}) => updater.check(force: force),
  policy: updater.policy, // afterSuccess=24h, afterFailure=1h
  onResult: (result) {
    if (result is UpdateAvailable) {
      // 打开 UI；不要在这里 force check
    }
  },
);
scheduler.start(checkOnStart: true); // 启动立刻 throttled check
// …应用退出或关掉能力时：
scheduler.stop();
```

`CheckThrottled` 只会重臂定时器，不回调 `onResult`。手动「检查更新」仍用 `check(force: true)`，不要走 scheduler。

### D2.4 Apply（可选）

便携桌面目录替换可用 `package:rup_client` 的 `apply/`（Windows 无锁换档等）。  
安装器型 / 商店分发：不要用目录替换，自行处理。

## D3. UpdatePolicy（下载）

默认已适合大包：

- `downloadConcurrency: 8`（`1` = 强制单连接）
- `downloadChunkSize: 4 MiB`
- `downloadRetries: 3` + 指数退避
- 断点：`.part` + `.part.meta`（换 URL/hash 会丢弃）

服务端需支持 HTTP Range（`relkit-serve` 已支持）；否则自动降级。

## D4. 开箱 Done

- [ ] `dart pub get` 成功，`import 'package:rup_client/rup_client.dart'` 无分析错误
- [ ] product / indexUrl / keyId / selectors 与现网发布一致
- [ ] 强制 check 对旧 code 能看到更新（或明确 up-to-date）
- [ ] download 进度能显示 % 与速度；完成后 hash 校验通过
- [ ] 公钥仅编译期常量；轮换流程写进宿主文档
- [ ] （若 apply）preserve 列表覆盖日志与重成本运行时数据

## D5. 排障

| 现象 | 方向 |
|------|------|
| rejected product | product 字符串不一致 |
| 验签失败 | 公钥/keyId 错误或 index 被篡改/截断 |
| no artifact | os/arch 与 stage 不一致；或缺平台包 |
| 下载慢且无 Range | 反代吞掉 Range；对照 serve AGENT-GUIDE |
| 节流 | UI 检查应用 `force: true` |

命令行冒烟：`dart run example/check_update.dart --index … --product … --key id=…`

发版与 serve 问题 → relkit `docs/agent` / `relkit agent-guide`，不要在 Dart 里「修协议」。
