/// Generated host facade (relkit.updater.v1). Do not hand-edit the method set.
library;

import 'dart:core' hide Error;
import 'dart:io';
import 'dart:typed_data';

import 'package:fixnum/fixnum.dart';
import 'package:protobuf/protobuf.dart';

import 'gen/updater/v1/updater.pb.dart';

const int ipcMin = 1;
const int ipcMax = 1;
const int ipcCurrent = 1;

abstract class Glue {
  Future<String> locate(Runtime runtime);
  Stream<UpdaterEvent> run(
    String bin,
    List<String> args,
    Uint8List stdin,
  );
}

class DefaultGlue implements Glue {
  @override
  Future<String> locate(Runtime runtime) async {
    final name =
        Platform.isWindows ? 'relkit-updater.exe' : 'relkit-updater';
    final candidates = <String>[];
    if (runtime.sidecarPath.isNotEmpty) {
      candidates.add(runtime.sidecarPath);
    }
    final env = Platform.environment['RELKIT_UPDATER'];
    if (env != null && env.isNotEmpty) candidates.add(env);
    if (runtime.hasInstall() && runtime.install.installRoot.isNotEmpty) {
      candidates.add('${runtime.install.installRoot}${Platform.pathSeparator}$name');
    }
    candidates.add('${File(Platform.resolvedExecutable).parent.path}${Platform.pathSeparator}$name');
    for (final c in candidates) {
      if (File(c).existsSync()) return c;
    }
    throw StateError('sidecar not found');
  }

  @override
  Stream<UpdaterEvent> run(
    String bin,
    List<String> args,
    Uint8List stdinBytes,
  ) async* {
    final proc = await Process.start(bin, args);
    proc.stdin.add(stdinBytes);
    await proc.stdin.close();
    proc.stderr.listen((_) {});
    yield* _readFrames(proc.stdout);
    await proc.exitCode;
  }
}

Stream<UpdaterEvent> _readFrames(Stream<List<int>> stdout) async* {
  final buf = BytesBuilder(copy: false);
  await for (final chunk in stdout) {
    buf.add(chunk);
    while (true) {
      final data = buf.toBytes();
      if (data.length < 4) break;
      final n = ByteData.sublistView(data).getUint32(0, Endian.big);
      if (data.length < 4 + n) break;
      final payload = data.sublist(4, 4 + n);
      final rest = data.sublist(4 + n);
      buf.clear();
      buf.add(rest);
      yield UpdaterEvent.fromBuffer(payload);
    }
  }
}

Uint8List _frame(GeneratedMessage msg) {
  final payload = msg.writeToBuffer();
  final out = BytesBuilder(copy: false);
  final hdr = ByteData(4)..setUint32(0, payload.length, Endian.big);
  out.add(hdr.buffer.asUint8List());
  out.add(payload);
  return out.takeBytes();
}

class OpenResult {
  OpenResult.opened(this.updater, this.capabilities)
      : error = null,
        kind = 'opened';
  OpenResult.failed(this.error)
      : updater = null,
        capabilities = null,
        kind = 'failed';

  final String kind;
  final Updater? updater;
  final Capabilities? capabilities;
  final Error? error;
}

class Updater {
  Updater._(this._profile, this._runtime, this._glue, this._bin, this.caps);

  final ClientProfile _profile;
  final Runtime _runtime;
  final Glue _glue;
  final String _bin;
  final Capabilities caps;

  static Future<OpenResult> open(
    ClientProfile profile,
    Runtime runtime, {
    Glue? glue,
  }) async {
    glue ??= DefaultGlue();
    try {
      final bin = await glue.locate(runtime);
      Capabilities? caps;
      Error? err;
      await for (final ev in glue.run(bin, ['-capabilities'], Uint8List(0))) {
        if (ev.hasCapabilities()) caps = ev.capabilities;
        if (ev.hasFailed()) err = ev.failed.error;
      }
      if (err != null) {
        return OpenResult.failed(err);
      }
      if (caps == null) {
        return OpenResult.failed(
            Error(code: ErrorCode.ERROR_CODE_PROTOCOL_MISMATCH, message: 'no capabilities'));
      }
      if (caps.ipc < ipcMin) {
        return OpenResult.failed(
            Error(code: ErrorCode.ERROR_CODE_UPDATER_TOO_OLD, message: 'sidecar IPC too old'));
      }
      if (caps.ipc > ipcMax) {
        return OpenResult.failed(
            Error(code: ErrorCode.ERROR_CODE_UPDATER_TOO_NEW, message: 'sidecar IPC too new'));
      }
      return OpenResult.opened(
          Updater._(profile, runtime, glue, bin, caps), caps);
    } on Object catch (e) {
      return OpenResult.failed(Error(
          code: ErrorCode.ERROR_CODE_SIDECAR_NOT_FOUND, message: e.toString()));
    }
  }

  Stream<UpdaterEvent> _call(UpdaterRequest req) {
    req.hello = ClientHello(ipcMin: ipcMin, ipcMax: ipcMax);
    req.profile = _profile;
    req.runtime = _runtime;
    return _glue.run(_bin, const <String>[], _frame(req));
  }

  /// protobuf.dart reserves `check` on GeneratedMessage, so the field is `check_10`.
  Future<CheckResult> check({bool force = false, int exactCode = 0, CheckPolicy? policy}) async {
    final req = UpdaterRequest()
      ..check_10 = CheckOp(force: force, exactCode: Int64(exactCode), policy: policy);
    await for (final ev in _call(req)) {
      if (ev.hasCheck_10()) return ev.check_10;
      if (ev.hasFailed()) {
        return CheckResult()..failed = ev.failed;
      }
    }
    return CheckResult()
      ..failed = Failed(
          error: Error(
              code: ErrorCode.ERROR_CODE_NETWORK, message: 'no check result'));
  }

  Future<Result> skip({required int code}) async {
    final req = UpdaterRequest()..skip = SkipOp(code: Int64(code));
    await for (final ev in _call(req)) {
      if (ev.hasResult()) return ev.result;
    }
    return Result()..ok = Ok();
  }

  Future<DownloadResult> download({
    required String planId,
    void Function(UpdaterEvent event)? onEvent,
  }) async {
    final req = UpdaterRequest()..download = DownloadOp(planId: planId);
    await for (final ev in _call(req)) {
      onEvent?.call(ev);
      if (ev.hasDownload()) return ev.download;
    }
    return DownloadResult()
      ..failed = Failed(
          error: Error(
              code: ErrorCode.ERROR_CODE_NETWORK, message: 'no download result'));
  }

  Future<ApplyResult> apply({
    required String planId,
    void Function(UpdaterEvent event)? onEvent,
  }) async {
    final req = UpdaterRequest()..apply = ApplyOp(planId: planId);
    await for (final ev in _call(req)) {
      onEvent?.call(ev);
      if (ev.hasApply()) return ev.apply;
    }
    return ApplyResult()
      ..failed = Failed(
          error: Error(
              code: ErrorCode.ERROR_CODE_NETWORK, message: 'no apply result'));
  }

  Future<StatusSnapshot> status() async {
    final req = UpdaterRequest()..status = StatusOp();
    await for (final ev in _call(req)) {
      if (ev.hasStatus()) return ev.status;
    }
    return StatusSnapshot();
  }

  Future<Result> cleanup() async {
    final req = UpdaterRequest()..cleanup = CleanupOp();
    await for (final ev in _call(req)) {
      if (ev.hasResult()) return ev.result;
    }
    return Result()..ok = Ok();
  }

  Future<Result> cancel() async {
    return Result()..ok = Ok();
  }
}
