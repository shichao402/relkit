/// IPC framing for `relkit.updater.v1`: a 4-byte big-endian length followed by
/// the protobuf payload, repeated until the stream ends.
///
/// This lives outside the facade because it is the only part of a thin client
/// that can be wrong in a way tests can catch without an engine running: an
/// off-by-one in the header, a frame split across reads, or a trailing partial
/// frame that must be ignored rather than misparsed.
library;

import 'dart:typed_data';

import 'package:protobuf/protobuf.dart';

import 'gen/updater/v1/updater.pb.dart';

/// Encodes one message into a length-prefixed frame.
Uint8List encodeFrame(GeneratedMessage message) {
  final payload = message.writeToBuffer();
  final out = BytesBuilder(copy: false);
  final header = ByteData(4)..setUint32(0, payload.length, Endian.big);
  out.add(header.buffer.asUint8List());
  out.add(payload);
  return out.takeBytes();
}

/// Decodes a byte stream into messages, buffering until each frame is complete.
///
/// A trailing partial frame is dropped: the stream ended mid-message, so there
/// is nothing valid to yield.
Stream<UpdaterEvent> decodeFrames(Stream<List<int>> stdout) async* {
  final buffer = BytesBuilder(copy: false);
  await for (final chunk in stdout) {
    buffer.add(chunk);
    while (true) {
      final data = buffer.toBytes();
      if (data.length < 4) break;
      final length = ByteData.sublistView(data).getUint32(0, Endian.big);
      if (data.length < 4 + length) break;
      final payload = data.sublist(4, 4 + length);
      final rest = data.sublist(4 + length);
      buffer.clear();
      buffer.add(rest);
      yield UpdaterEvent.fromBuffer(payload);
    }
  }
}
