// SIM-S1: 独立应用自更新（模拟 SvnMergeTool 形态）
//
// Dart facade 驱动 updater，与 SvnMergeTool lib/services/update_service.dart
// 同构：编译期注入 runtime bootstrap（--define SIM_BOOTSTRAP_B64），
// Runtime 带 os/arch selectors + InstallSpec（windows: library retain=2，
// darwin: in-place）。程序由 e2e driver 循环驱动 check→apply→重启。
//
// hello-world 业务: 打印版本与平台。

import "dart:convert";
import "dart:io";

import "package:rup_client/rup_client.dart" show Int64, runtimeBootstrapFromJson;
import "package:rup_client/updater_v1.dart" as upv1;

const product = "sim-s1";

// 由 dart compile --define 注入（base64 的 relkit.runtime-bootstrap/1 JSON）。
const bootstrapB64 = String.fromEnvironment("SIM_BOOTSTRAP_B64");

const simVersion = String.fromEnvironment("SIM_VERSION", defaultValue: "0.1.0+1");

Future<void> main(List<String> args) async {
  final command = args.isNotEmpty ? args[0] : "status";
  switch (command) {
    case "--version":
      stdout.writeln("sim-s1 $simVersion");
      return;
    case "check":
    case "apply":
      await runUpdate(command);
      return;
    default:
      stdout.writeln("sim-s1 hello world ($simVersion)");
      stdout.writeln("commands: --version | check | apply");
  }
}

Future<void> runUpdate(String command) async {
  final opened = await upv1.Updater.open(loadProfile(), buildRuntime());
  if (opened.kind != "opened" || opened.updater == null) {
    stderr.writeln("open sidecar failed: ${opened.error?.message}");
    exit(1);
  }
  final updater = opened.updater!;
  final cr = await updater.check(force: true);
  if (cr.hasUpdateAvailable()) {
    final ua = cr.updateAvailable;
    stdout.writeln("check-result: update-available ${ua.version} code=${ua.code} plan=${ua.planId}");
    if (command == "check") return;
    final dr = await updater.download(planId: ua.planId);
    if (dr.hasFailed()) {
      stderr.writeln("download failed: ${dr.failed.error.message}");
      exit(1);
    }
    final ar = await updater.apply(planId: ua.planId);
    if (ar.hasFailed()) {
      stderr.writeln("apply failed: ${ar.failed.error.message}");
      exit(1);
    }
    stdout.writeln("apply-result: ok");
  } else if (cr.hasUpToDate()) {
    stdout.writeln("check-result: up-to-date sequence=${cr.upToDate.sequence}");
  } else if (cr.hasThrottled()) {
    stdout.writeln("check-result: throttled");
  } else if (cr.hasFailed()) {
    stderr.writeln("check failed: ${cr.failed.error.message}");
    exit(1);
  } else {
    stdout.writeln("check-result: ${cr.whichKind()}");
  }
}

upv1.ClientProfile loadProfile() {
  if (bootstrapB64.isEmpty) {
    stderr.writeln("Runtime bootstrap was not injected at compile time");
    exit(1);
  }
  final bootstrap = runtimeBootstrapFromJson(utf8.decode(base64Decode(bootstrapB64)));
  return bootstrap.clientProfile;
}

upv1.Runtime buildRuntime() {
  final isWindows = Platform.isWindows;
  final installRoot = Platform.environment["SIM_INSTALL_ROOT"] ??
      File(Platform.resolvedExecutable).parent.path;
  final channel = Platform.environment["SIM_CHANNEL"] ?? "dev";
  // dart 客户端 selectors 沿用 x64 词汇（与 SvnMergeTool 一致；manifest 侧逐字匹配）。
  final selectors = <String, String>{
    "os": isWindows ? "windows" : "macos",
    if (isWindows) "arch": "x64",
  };
  final spec = upv1.InstallSpec(
    placement: isWindows
        ? upv1.Placement.PLACEMENT_LIBRARY
        : upv1.Placement.PLACEMENT_IN_PLACE,
    installRoot: installRoot,
    executableRelpath: isWindows ? "sim-s1.exe" : "sim-s1",
    sidecarRelpath: isWindows ? "relkit-updater.exe" : "relkit-updater",
    library: isWindows ? upv1.LibraryPolicy(retain: 2) : null,
    relaunch: true,
  );
  return upv1.Runtime(
    channel: channel,
    currentCode: Int64(_buildNumber(simVersion)),
    clientSelectors: selectors.entries,
    dataDir: Directory.systemTemp.createTempSync("sim-s1-updater").path,
    install: spec,
    sidecarPath: _sidecarPath(installRoot),
  );
}

String _sidecarPath(String installRoot) {
  final name = Platform.isWindows ? "relkit-updater.exe" : "relkit-updater";
  return "$installRoot${Platform.pathSeparator}$name";
}

int _buildNumber(String version) {
  final match = RegExp(r"\+(\d+)$").firstMatch(version);
  return match != null ? int.parse(match.group(1)!) : 1;
}
