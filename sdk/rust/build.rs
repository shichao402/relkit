use std::path::PathBuf;

fn main() {
    let manifest_dir = PathBuf::from(
        std::env::var_os("CARGO_MANIFEST_DIR").expect("CARGO_MANIFEST_DIR is unavailable"),
    );
    let repo_proto = manifest_dir.join("../../proto");
    let packaged_proto = manifest_dir.join("proto");
    let proto_root = if repo_proto.join("updater/v1/updater.proto").is_file() {
        repo_proto.as_path()
    } else {
        packaged_proto.as_path()
    };
    let input = proto_root.join("updater/v1/updater.proto");
    if !input.is_file() {
        panic!(
            "canonical proto/updater/v1/updater.proto not found (checked {} and {})",
            repo_proto.display(),
            packaged_proto.display()
        );
    }

    let protoc = protoc_bin_vendored::protoc_bin_path().expect("vendored protoc is unavailable");
    let well_known =
        protoc_bin_vendored::include_path().expect("vendored protobuf includes are unavailable");
    std::env::set_var("PROTOC", protoc);

    let includes: Vec<PathBuf> = vec![proto_root.to_path_buf(), well_known];
    prost_build::Config::new()
        .compile_protos(std::slice::from_ref(&input), &includes)
        .expect("generate relkit.updater.v1 DTOs from canonical IDL");

    println!("cargo:rerun-if-changed={}", input.display());
}
