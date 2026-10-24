/// The `relkit.updater.v1` protobuf projection, nothing else.
///
/// Hosts that only need the wire types (for example to decode events a facade
/// already produced) import this barrel instead of the whole client.
library;

export 'src/gen/updater/v1/updater.pb.dart';
export 'src/gen/updater/v1/updater.pbenum.dart';
