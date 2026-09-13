//! Current-IDL JSON projection of [`CheckResult`] for WebView hosts.
//!
//! This is not a second wire protocol. Missing keys are bugs; do not default them.

use crate::proto::check_result;
use crate::proto::{
    ArtifactView, CheckResult, Error, Failed, FallbackRequired, PriorReleaseNotes, RecoveryHelp,
    RecoveryLink, Throttled, UpToDate, UpdateAvailable,
};
use serde::{Deserialize, Serialize};

#[derive(Debug)]
pub struct JsonError(pub String);

impl std::fmt::Display for JsonError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl std::error::Error for JsonError {}

/// Serialize a check result. Empty strings and `false` are always present.
pub fn check_result_to_json(result: &CheckResult) -> Result<String, JsonError> {
    let wire = CheckResultWire::from_proto(result)?;
    serde_json::to_string(&wire).map_err(|error| JsonError(error.to_string()))
}

/// Strict parse: absent `releaseNotesMarkdown` (and other IDL scalars) fails.
pub fn check_result_from_json(json: &str) -> Result<CheckResult, JsonError> {
    let wire: CheckResultWire =
        serde_json::from_str(json).map_err(|error| JsonError(error.to_string()))?;
    Ok(wire.into_proto())
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
enum CheckResultWire {
    UpToDate(UpToDateWire),
    UpdateAvailable(UpdateAvailableWire),
    FallbackRequired(FallbackRequiredWire),
    Throttled(ThrottledWire),
    Failed(FailedWire),
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct UpToDateWire {
    sequence: i64,
    current_is_yanked: bool,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct UpdateAvailableWire {
    plan_id: String,
    prompt_key: String,
    version: String,
    code: i64,
    mandatory: bool,
    remaining_hops: i32,
    sequence: i64,
    release_notes_markdown: String,
    release_notes_url: String,
    prior_release_notes: Vec<PriorReleaseNotesWire>,
    artifacts: Vec<ArtifactViewWire>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct PriorReleaseNotesWire {
    version: String,
    code: i64,
    notes: String,
    notes_url: String,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ArtifactViewWire {
    name: String,
    size: i64,
    sha256: String,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct FallbackRequiredWire {
    prompt_key: String,
    manual_url: String,
    message: String,
    mandatory: bool,
    sequence: i64,
    min_code: i64,
    max_code: i64,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ThrottledWire {
    next_allowed_at: Option<TimestampWire>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TimestampWire {
    seconds: i64,
    nanos: i32,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct FailedWire {
    error: Option<ErrorWire>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ErrorWire {
    code: i32,
    retryable: bool,
    message: String,
    attempts: Vec<String>,
    recovery: Option<RecoveryHelpWire>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RecoveryHelpWire {
    message: String,
    links: Vec<RecoveryLinkWire>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RecoveryLinkWire {
    label: String,
    url: String,
}

impl CheckResultWire {
    fn from_proto(result: &CheckResult) -> Result<Self, JsonError> {
        match &result.kind {
            Some(check_result::Kind::UpToDate(value)) => Ok(Self::UpToDate(UpToDateWire {
                sequence: value.sequence,
                current_is_yanked: value.current_is_yanked,
            })),
            Some(check_result::Kind::UpdateAvailable(value)) => {
                Ok(Self::UpdateAvailable(UpdateAvailableWire::from_proto(value)))
            }
            Some(check_result::Kind::FallbackRequired(value)) => {
                Ok(Self::FallbackRequired(FallbackRequiredWire {
                    prompt_key: value.prompt_key.clone(),
                    manual_url: value.manual_url.clone(),
                    message: value.message.clone(),
                    mandatory: value.mandatory,
                    sequence: value.sequence,
                    min_code: value.min_code,
                    max_code: value.max_code,
                }))
            }
            Some(check_result::Kind::Throttled(value)) => Ok(Self::Throttled(ThrottledWire {
                next_allowed_at: value.next_allowed_at.as_ref().map(|ts| TimestampWire {
                    seconds: ts.seconds,
                    nanos: ts.nanos,
                }),
            })),
            Some(check_result::Kind::Failed(value)) => Ok(Self::Failed(FailedWire {
                error: value.error.as_ref().map(ErrorWire::from_proto),
            })),
            None => Err(JsonError("check result has no kind".into())),
        }
    }

    fn into_proto(self) -> CheckResult {
        let kind = match self {
            Self::UpToDate(value) => check_result::Kind::UpToDate(UpToDate {
                sequence: value.sequence,
                current_is_yanked: value.current_is_yanked,
            }),
            Self::UpdateAvailable(value) => {
                check_result::Kind::UpdateAvailable(value.into_proto())
            }
            Self::FallbackRequired(value) => check_result::Kind::FallbackRequired(FallbackRequired {
                prompt_key: value.prompt_key,
                manual_url: value.manual_url,
                message: value.message,
                mandatory: value.mandatory,
                sequence: value.sequence,
                min_code: value.min_code,
                max_code: value.max_code,
            }),
            Self::Throttled(value) => check_result::Kind::Throttled(Throttled {
                next_allowed_at: value.next_allowed_at.map(|ts| prost_types::Timestamp {
                    seconds: ts.seconds,
                    nanos: ts.nanos,
                }),
            }),
            Self::Failed(value) => check_result::Kind::Failed(Failed {
                error: value.error.map(ErrorWire::into_proto),
            }),
        };
        CheckResult { kind: Some(kind) }
    }
}

impl UpdateAvailableWire {
    fn from_proto(value: &UpdateAvailable) -> Self {
        Self {
            plan_id: value.plan_id.clone(),
            prompt_key: value.prompt_key.clone(),
            version: value.version.clone(),
            code: value.code,
            mandatory: value.mandatory,
            remaining_hops: value.remaining_hops,
            sequence: value.sequence,
            release_notes_markdown: value.release_notes_markdown.clone(),
            release_notes_url: value.release_notes_url.clone(),
            prior_release_notes: value
                .prior_release_notes
                .iter()
                .map(|notes| PriorReleaseNotesWire {
                    version: notes.version.clone(),
                    code: notes.code,
                    notes: notes.notes.clone(),
                    notes_url: notes.notes_url.clone(),
                })
                .collect(),
            artifacts: value
                .artifacts
                .iter()
                .map(|artifact| ArtifactViewWire {
                    name: artifact.name.clone(),
                    size: artifact.size,
                    sha256: hex_encode(&artifact.sha256),
                })
                .collect(),
        }
    }

    fn into_proto(self) -> UpdateAvailable {
        UpdateAvailable {
            plan_id: self.plan_id,
            prompt_key: self.prompt_key,
            version: self.version,
            code: self.code,
            mandatory: self.mandatory,
            remaining_hops: self.remaining_hops,
            sequence: self.sequence,
            release_notes_markdown: self.release_notes_markdown,
            release_notes_url: self.release_notes_url,
            prior_release_notes: self
                .prior_release_notes
                .into_iter()
                .map(|notes| PriorReleaseNotes {
                    version: notes.version,
                    code: notes.code,
                    notes: notes.notes,
                    notes_url: notes.notes_url,
                })
                .collect(),
            artifacts: self
                .artifacts
                .into_iter()
                .map(|artifact| ArtifactView {
                    name: artifact.name,
                    size: artifact.size,
                    sha256: hex_decode(&artifact.sha256),
                })
                .collect(),
        }
    }
}

impl ErrorWire {
    fn from_proto(value: &Error) -> Self {
        Self {
            code: value.code,
            retryable: value.retryable,
            message: value.message.clone(),
            attempts: value.attempts.clone(),
            recovery: value.recovery.as_ref().map(|help| RecoveryHelpWire {
                message: help.message.clone(),
                links: help
                    .links
                    .iter()
                    .map(|link| RecoveryLinkWire {
                        label: link.label.clone(),
                        url: link.url.clone(),
                    })
                    .collect(),
            }),
        }
    }

    fn into_proto(self) -> Error {
        Error {
            code: self.code,
            retryable: self.retryable,
            message: self.message,
            attempts: self.attempts,
            recovery: self.recovery.map(|help| RecoveryHelp {
                message: help.message,
                links: help
                    .links
                    .into_iter()
                    .map(|link| RecoveryLink {
                        label: link.label,
                        url: link.url,
                    })
                    .collect(),
            }),
        }
    }
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

fn hex_decode(text: &str) -> Vec<u8> {
    let bytes = text.as_bytes();
    if bytes.len() % 2 != 0 {
        return Vec::new();
    }
    let mut out = Vec::with_capacity(bytes.len() / 2);
    let mut index = 0;
    while index < bytes.len() {
        let Some(high) = from_hex(bytes[index]) else {
            return Vec::new();
        };
        let Some(low) = from_hex(bytes[index + 1]) else {
            return Vec::new();
        };
        out.push((high << 4) | low);
        index += 2;
    }
    out
}

fn from_hex(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn empty_notes_available() -> CheckResult {
        CheckResult {
            kind: Some(check_result::Kind::UpdateAvailable(UpdateAvailable {
                plan_id: "plan".into(),
                prompt_key: "code:1".into(),
                version: "1.0.0".into(),
                code: 1,
                mandatory: false,
                remaining_hops: 1,
                sequence: 7,
                release_notes_markdown: String::new(),
                release_notes_url: String::new(),
                prior_release_notes: Vec::new(),
                artifacts: Vec::new(),
            })),
        }
    }

    #[test]
    fn empty_notes_and_false_mandatory_emit_keys() {
        let json = check_result_to_json(&empty_notes_available()).unwrap();
        assert!(
            json.contains("\"releaseNotesMarkdown\":\"\""),
            "{json}"
        );
        assert!(json.contains("\"releaseNotesUrl\":\"\""), "{json}");
        assert!(json.contains("\"mandatory\":false"), "{json}");
        assert_eq!(
            json,
            include_str!("../../../conformance/updater/check-result-empty-notes.json").trim_end()
        );
    }

    #[test]
    fn missing_release_notes_markdown_is_rejected() {
        let mut value: serde_json::Value =
            serde_json::from_str(&check_result_to_json(&empty_notes_available()).unwrap()).unwrap();
        value["updateAvailable"]
            .as_object_mut()
            .unwrap()
            .remove("releaseNotesMarkdown");
        let error = check_result_from_json(&value.to_string()).unwrap_err();
        assert!(
            error.0.contains("releaseNotesMarkdown"),
            "{}",
            error.0
        );
    }

    #[test]
    fn missing_mandatory_is_rejected() {
        let mut value: serde_json::Value =
            serde_json::from_str(&check_result_to_json(&empty_notes_available()).unwrap()).unwrap();
        value["updateAvailable"]
            .as_object_mut()
            .unwrap()
            .remove("mandatory");
        let error = check_result_from_json(&value.to_string()).unwrap_err();
        assert!(error.0.contains("mandatory"), "{}", error.0);
    }
}
