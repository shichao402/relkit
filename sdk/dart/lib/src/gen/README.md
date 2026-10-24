Generated protobuf bindings live in this directory.

Regenerate them with `scripts/gen-proto.ps1` from the relkit repo root. That
script writes Go bindings from `proto/buf.gen.yaml` and the Dart `updater/v1`
bindings from `proto/buf.gen.dart.yaml`; RUP wire types are Go-only because a
Dart host never parses index/manifest.

Do not hand-edit any file under `lib/src/gen/`.
