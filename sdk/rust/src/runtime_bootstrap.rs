use std::collections::HashSet;
use std::fmt;

use base64::engine::general_purpose::STANDARD;
use base64::Engine;
use serde::Deserialize;

use crate::proto::{ClientProfile, RecoveryHelp, RecoveryLink, TrustedKey};

const SCHEMA: &str = "relkit.runtime-bootstrap/1";

#[derive(Clone, Debug)]
pub struct RuntimeBootstrap {
    pub resolution_digest: String,
    pub profile_digest: String,
    pub root_fingerprint: String,
    pub client_profile: ClientProfile,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct RuntimeBootstrapError(pub String);

impl fmt::Display for RuntimeBootstrapError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl std::error::Error for RuntimeBootstrapError {}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Document {
    schema: String,
    resolution_digest: String,
    profile_digest: String,
    root_fingerprint: String,
    profile: Profile,
    trusted_keys: Vec<Key>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Profile {
    product: String,
    allowed_channels: Vec<String>,
    directory_bootstrap: DirectoryBootstrap,
    digest: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct DirectoryBootstrap {
    entry_urls: Vec<String>,
    recovery: Option<Recovery>,
}

#[derive(Default, Deserialize)]
struct Recovery {
    #[serde(default)]
    message: String,
    #[serde(default)]
    links: Vec<Link>,
}

#[derive(Deserialize)]
struct Link {
    label: String,
    url: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Key {
    key_id: String,
    key: String,
}

pub fn runtime_bootstrap_from_json(json: &str) -> Result<RuntimeBootstrap, RuntimeBootstrapError> {
    let document: Document = serde_json::from_str(json).map_err(|error| {
        RuntimeBootstrapError(format!("invalid runtime bootstrap JSON: {error}"))
    })?;
    if document.schema != SCHEMA {
        return Err(RuntimeBootstrapError(format!(
            "runtime bootstrap schema is {:?}, expected {:?}",
            document.schema, SCHEMA
        )));
    }
    require_digest("resolutionDigest", &document.resolution_digest)?;
    require_digest("profileDigest", &document.profile_digest)?;
    require_digest("rootFingerprint", &document.root_fingerprint)?;
    if document.profile.digest != document.profile_digest {
        return Err(RuntimeBootstrapError(
            "embedded profile digest does not match profileDigest".into(),
        ));
    }
    if document.profile.product.is_empty() {
        return Err(RuntimeBootstrapError(
            "profile product must not be empty".into(),
        ));
    }
    if document.profile.allowed_channels.is_empty() {
        return Err(RuntimeBootstrapError(
            "profile allowedChannels must not be empty".into(),
        ));
    }
    if document.profile.directory_bootstrap.entry_urls.is_empty() {
        return Err(RuntimeBootstrapError(
            "profile directoryBootstrap.entryUrls must not be empty".into(),
        ));
    }
    if document.trusted_keys.is_empty() {
        return Err(RuntimeBootstrapError(
            "trustedKeys must not be empty".into(),
        ));
    }
    let mut seen = HashSet::with_capacity(document.trusted_keys.len());
    let mut trusted_keys = Vec::with_capacity(document.trusted_keys.len());
    for key in document.trusted_keys {
        if key.key_id.is_empty() || !seen.insert(key.key_id.clone()) {
            return Err(RuntimeBootstrapError(
                "trusted key ids must be non-empty and unique".into(),
            ));
        }
        let public_key = STANDARD.decode(key.key).map_err(|error| {
            RuntimeBootstrapError(format!("trusted key is not base64: {error}"))
        })?;
        if public_key.len() != 32 {
            return Err(RuntimeBootstrapError(format!(
                "trusted key {:?} must contain a 32-byte ed25519 key",
                key.key_id
            )));
        }
        trusted_keys.push(TrustedKey {
            key_id: key.key_id,
            public_key,
        });
    }
    let recovery = document
        .profile
        .directory_bootstrap
        .recovery
        .map(|value| RecoveryHelp {
            message: value.message,
            links: value
                .links
                .into_iter()
                .map(|link| RecoveryLink {
                    label: link.label,
                    url: link.url,
                })
                .collect(),
        });
    Ok(RuntimeBootstrap {
        resolution_digest: document.resolution_digest,
        profile_digest: document.profile_digest,
        root_fingerprint: document.root_fingerprint,
        client_profile: ClientProfile {
            product: document.profile.product,
            allowed_channels: document.profile.allowed_channels,
            entry_urls: document.profile.directory_bootstrap.entry_urls,
            index_urls: Vec::new(),
            fallback_urls: Vec::new(),
            trusted_keys,
            recovery,
        },
    })
}

fn require_digest(name: &str, value: &str) -> Result<(), RuntimeBootstrapError> {
    if value.len() != 71
        || !value.starts_with("sha256:")
        || !value[7..].bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        return Err(RuntimeBootstrapError(format!(
            "{name} must be a sha256 digest"
        )));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn projects_verified_bootstrap_to_client_profile() {
        let digest = format!("sha256:{}", "a".repeat(64));
        let root = format!("sha256:{}", "b".repeat(64));
        let key = STANDARD.encode([7u8; 32]);
        let json = format!(
            r#"{{
                "schema":"relkit.runtime-bootstrap/1",
                "resolutionDigest":"{digest}",
                "profileDigest":"{digest}",
                "rootFingerprint":"{root}",
                "profile":{{
                    "schema":"relkit.runtime-profile/1",
                    "product":"demo",
                    "profileVersion":1,
                    "allowedChannels":["stable"],
                    "directoryBootstrap":{{
                        "entryUrls":["https://updates.example/directory/demo.pb"],
                        "recovery":{{"message":"offline","links":[{{"label":"download","url":"https://updates.example/"}}]}}
                    }},
                    "rupSchemaSet":"rup/v2",
                    "digest":"{digest}",
                    "signatures":[{{"keyId":"root","algorithm":"ed25519","value":"ignored"}}]
                }},
                "trustedKeys":[{{"keyId":"root","key":"{key}"}}]
            }}"#
        );
        let value = runtime_bootstrap_from_json(&json).unwrap();
        assert_eq!(value.client_profile.product, "demo");
        assert_eq!(value.client_profile.index_urls, Vec::<String>::new());
        assert_eq!(
            value.client_profile.trusted_keys[0].public_key,
            vec![7u8; 32]
        );
        assert_eq!(value.client_profile.recovery.unwrap().message, "offline");
    }

    #[test]
    fn rejects_digest_or_key_drift() {
        let error = runtime_bootstrap_from_json("{}").unwrap_err();
        assert!(error.0.contains("invalid runtime bootstrap JSON"));
    }
}
