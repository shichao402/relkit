/// Protocol helpers and the generated `relkit-updater` facade.
///
/// Check/download/apply for products goes through [Updater] in
/// `updater_facade.dart`. Every decision (parsing, signature checks, selectors,
/// version ordering, artifact download, file replacement) belongs to the
/// `relkit-updater` sidecar; this package only locates it, frames IPC messages
/// and maps events back to the host.
library;

export 'package:fixnum/fixnum.dart' show Int64;

export 'src/gen/updater/v1/updater.pb.dart';
export 'src/gen/updater/v1/updater.pbenum.dart';
export 'src/frame.dart' show encodeFrame, decodeFrames;
export 'src/updater_facade.dart'
    show Glue, DefaultGlue, OpenResult, Updater, ipcMin, ipcMax, ipcCurrent;
