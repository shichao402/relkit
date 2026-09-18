use std::path::PathBuf;

fn main() {
    let manifest_dir = PathBuf::from(
        std::env::var_os("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR is unavailable"),
    );
    // Prefer the IDL packaged next to this crate (release zip / product tree).
    // Only fall back to the relkit repo root proto/ when building in-tree, where
    // packaged proto/ is absent. A leftover product-tree third_party/relkit/proto
    // must not win over sdk/rust/proto after consume upgrades the SDK.
    let packaged_proto = manifest_dir.join("proto");
    let repo_proto = manifest_dir.join("../../proto");
    let proto_root = if packaged_proto.join("updater/v1/updater.proto").is_file() {
        packaged_proto.as_path()
    } else {
        repo_proto.as_path()
    };
    let input = proto_root.join("updater/v1/updater.proto");
    if !input.is_file() {
        panic!(
            "canonical proto/updater/v1/updater.proto not found (checked {} and {})",
            packaged_proto.display(),
            repo_proto.display()
        );
    }

    let protoc = protoc_bin_vendored::protoc_bin_path().expect("vendored protoc is unavailable");
    let well_known =
        protoc_bin_vendored::include_path().expect("vendored protobuf includes are unavailable");
    std::env::set_var("PROTOC", protoc);

    let includes: Vec<PathBuf> = vec![proto_root.to_path_buf(), well_known];
    let descriptor_path = PathBuf::from(std::env::var_os("OUT_DIR").unwrap())
        .join("updater_descriptor.bin");
    prost_build::Config::new()
        .file_descriptor_set_path(&descriptor_path)
        .compile_well_known_types()
        .extern_path(".google.protobuf", "::pbjson_types")
        .compile_protos(std::slice::from_ref(&input), &includes)
        .expect("generate relkit.updater.v1 DTOs from canonical IDL");
    let descriptors = std::fs::read(&descriptor_path).expect("read updater descriptor set");
    pbjson_build::Builder::new()
        .register_descriptors(&descriptors)
        .expect("register updater descriptors")
        .emit_fields()
        .build(&[".relkit.updater.v1"])
        .expect("generate canonical protobuf JSON implementations");

    println!("cargo:rerun-if-changed={}", input.display());
}
