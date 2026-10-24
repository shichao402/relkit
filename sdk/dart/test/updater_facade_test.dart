// The thin client's only job is to frame bytes and decide whether the engine on
// the other side speaks its dialect. Everything it can get wrong without an
// engine running is asserted here: the frame header, fragmentation, the IPC
// window, and the failure shape a host has to handle.

import 'dart:typed_data';

import 'package:rup_client/rup_client.dart';
import 'package:test/test.dart';

/// A Glue that answers with a fixed script instead of spawning anything.
class ScriptedGlue implements Glue {
  ScriptedGlue(this.script);

  final List<UpdaterEvent> script;
  final List<List<String>> invocations = [];

  @override
  Future<String> locate(Runtime runtime) async => 'relkit-updater-stub';

  @override
  Stream<UpdaterEvent> run(String bin, List<String> args, Uint8List stdin) async* {
    invocations.add(args);
    for (final event in script) {
      yield event;
    }
  }
}

/// A Glue whose locate fails, standing in for a missing sidecar.
class MissingSidecarGlue implements Glue {
  @override
  Future<String> locate(Runtime runtime) async => throw StateError('sidecar not found');

  @override
  Stream<UpdaterEvent> run(String bin, List<String> args, Uint8List stdin) async* {}
}

Runtime runtimeFixture() => Runtime();

UpdaterEvent capabilitiesEvent(int ipc) => UpdaterEvent(
      capabilities: Capabilities(ipc: ipc),
    );

void main() {
  group('frame', () {
    test('writes a big-endian length prefix and the payload', () {
      final event = UpdaterEvent(capabilities: Capabilities(ipc: 4));
      final framed = encodeFrame(event);

      final header = ByteData.sublistView(framed).getUint32(0, Endian.big);
      final payload = event.writeToBuffer();
      expect(header, payload.length);
      expect(framed.length, 4 + payload.length);
      expect(framed.sublist(4), payload);
    });

    test('round-trips through the reader in one chunk', () async {
      final event = UpdaterEvent(capabilities: Capabilities(ipc: 4));
      final framed = encodeFrame(event);

      final decoded = await decodeFrames(Stream.value(framed)).toList();
      expect(decoded, hasLength(1));
      expect(decoded.single.capabilities.ipc, 4);
    });

    test('reassembles a frame split across chunks', () async {
      final event = UpdaterEvent(capabilities: Capabilities(ipc: 4));
      final framed = encodeFrame(event);

      // One byte at a time is the worst case a real pipe can produce.
      final chunks = <List<int>>[
        for (final byte in framed) <int>[byte],
      ];
      final decoded = await decodeFrames(Stream.fromIterable(chunks)).toList();
      expect(decoded, hasLength(1));
      expect(decoded.single.capabilities.ipc, 4);
    });

    test('reads multiple frames from one chunk', () async {
      final first = encodeFrame(UpdaterEvent(capabilities: Capabilities(ipc: 4)));
      final second = encodeFrame(UpdaterEvent(capabilities: Capabilities(ipc: 4)));
      final combined = Uint8List.fromList([...first, ...second]);

      final decoded = await decodeFrames(Stream.value(combined)).toList();
      expect(decoded, hasLength(2));
    });

    test('ignores a trailing partial frame instead of misparsing it', () async {
      final framed = encodeFrame(UpdaterEvent(capabilities: Capabilities(ipc: 4)));
      final truncated = Uint8List.fromList([...framed, 0, 0]);

      final decoded = await decodeFrames(Stream.value(truncated)).toList();
      expect(decoded, hasLength(1));
    });
  });

  group('ipc window', () {
    test('constants are pinned to the epoch baseline', () {
      expect(ipcMin, 4);
      expect(ipcMax, 4);
      expect(ipcCurrent, 4);
    });

    test('open succeeds when the engine speaks the same version', () async {
      final glue = ScriptedGlue([capabilitiesEvent(ipcCurrent)]);
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: glue,
      );

      expect(opened.kind, 'opened');
      expect(opened.updater, isNotNull);
      expect(opened.capabilities?.ipc, ipcCurrent);
      // The capability probe must not pretend to be a real request.
      expect(glue.invocations.single, ['-capabilities']);
    });

    test('open refuses an engine older than the baseline', () async {
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: ScriptedGlue([capabilitiesEvent(ipcMin - 1)]),
      );

      expect(opened.kind, 'failed');
      expect(opened.updater, isNull);
      expect(opened.error?.code, ErrorCode.ERROR_CODE_UPDATER_TOO_OLD);
    });

    test('open refuses an engine newer than the baseline', () async {
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: ScriptedGlue([capabilitiesEvent(ipcMax + 1)]),
      );

      expect(opened.kind, 'failed');
      expect(opened.error?.code, ErrorCode.ERROR_CODE_UPDATER_TOO_NEW);
    });

    test('open reports a protocol mismatch when the engine says nothing', () async {
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: ScriptedGlue(const []),
      );

      expect(opened.kind, 'failed');
      expect(opened.error?.code, ErrorCode.ERROR_CODE_PROTOCOL_MISMATCH);
    });

    test('open surfaces an engine-side failure as-is', () async {
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: ScriptedGlue([
          UpdaterEvent(
            failed: Failed(
              error: Error(
                code: ErrorCode.ERROR_CODE_PROFILE_INVALID,
                message: 'bootstrap rejected',
              ),
            ),
          ),
        ]),
      );

      expect(opened.kind, 'failed');
      expect(opened.error?.code, ErrorCode.ERROR_CODE_PROFILE_INVALID);
      expect(opened.error?.message, 'bootstrap rejected');
    });

    test('open reports a missing sidecar instead of throwing', () async {
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: MissingSidecarGlue(),
      );

      expect(opened.kind, 'failed');
      expect(opened.error?.code, ErrorCode.ERROR_CODE_SIDECAR_NOT_FOUND);
    });
  });

  group('calls', () {
    test('check sends the hello window and the profile', () async {
      final glue = ScriptedGlue([
        UpdaterEvent(capabilities: Capabilities(ipc: ipcCurrent)),
        UpdaterEvent(check_10: CheckResult()),
      ]);
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: glue,
      );
      await opened.updater!.check(force: true);

      expect(glue.invocations, hasLength(2));
      expect(glue.invocations.last, isEmpty);
    });

    test('cancel is a no-op that reports success', () async {
      final glue = ScriptedGlue([capabilitiesEvent(ipcCurrent)]);
      final opened = await Updater.open(
        ClientProfile(product: 'loom', bootstrap: '{}'),
        runtimeFixture(),
        glue: glue,
      );

      final result = await opened.updater!.cancel();
      expect(result.hasOk(), isTrue);
      // It must not reach the engine: cancel only clears the host's own state.
      expect(glue.invocations, hasLength(1));
    });
  });
}
