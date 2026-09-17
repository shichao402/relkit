/// Protocol helpers and the generated `relkit-updater` facade.
///
/// Check/download/apply for products goes through [Updater] in
/// `updater_facade.dart`. The in-process engine stays in this repo for
/// tests and is not part of the host barrel.
library;

export 'package:fixnum/fixnum.dart' show Int64;

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
        DirectoryService,
        Fallback,
        FallbackRule,
        Index,
        Manifest,
        MetaEntry,
        Selector,
        Staged,
        StagedArtifact,
        UpdateDirectory,
        VersionNode;
export 'src/gen/rup/v2/objects.pbenum.dart' show ArtifactKind;
export 'src/models.dart';
export 'src/preference.dart';
export 'src/release_notes.dart';
export 'src/runtime_config.dart';
export 'src/selectors.dart';
export 'src/state.dart';
export 'src/updater_facade.dart'
    show Glue, DefaultGlue, OpenResult, Updater, ipcMin, ipcMax, ipcCurrent;
