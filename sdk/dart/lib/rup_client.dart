/// A RUP client: check for an update, verify it, download it, apply it.
///
/// The protocol contract lives in `SPEC.md`; this package implements the client
/// half of it. Everything up to and including "here is a file whose sha256
/// matches a signed manifest" is protocol, and is what `src/updater.dart`
/// provides.
///
/// Applying that file is not protocol. `src/apply/` offers one strategy —
/// replacing a directory of files, which is what a portable desktop
/// application is — and a host whose shape is different (an installer, a
/// package manager, a service) should ignore it and use the verified file
/// directly. It is exported rather than hidden because that one strategy
/// covers the common case, and the Windows part of it is subtle enough that
/// every host reinventing it would get it wrong in the same way.
library;

export 'package:fixnum/fixnum.dart' show Int64;

export 'src/apply/apply.dart';
export 'src/apply/swap.dart';
export 'src/apply/unpack.dart'
    show
        expandInnerInstallerIfPresent,
        selectInstallRootContainingExecutable,
        unpackUpdatePackage;
export 'src/chain.dart';
export 'src/download.dart';
export 'src/envelope.dart';
export 'src/fetch.dart';
export 'src/gen/rup/v2/envelope.pb.dart' show Envelope, Signature;
export 'src/gen/rup/v2/keys.pb.dart' show PrivateKeyDocument, PublicKeyDocument;
export 'src/gen/rup/v2/objects.pb.dart'
    show
        Artifact,
        DigestRef,
        Fallback,
        FallbackRule,
        Index,
        Manifest,
        MetaEntry,
        Selector,
        Staged,
        StagedArtifact,
        VersionNode;
export 'src/models.dart';
export 'src/release_notes.dart';
export 'src/runtime_config.dart';
export 'src/scheduler.dart';
export 'src/selectors.dart';
export 'src/state.dart';
export 'src/updater.dart';
