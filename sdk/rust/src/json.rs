//! Canonical ProtoJSON projection generated from `updater.proto`.
//!
//! `pbjson-build` owns names, oneofs, bytes, int64 and WKT/Timestamp mapping.
//! Its `emit_fields` option intentionally includes scalar defaults.

use crate::proto::CheckResult;

#[derive(Debug)]
pub struct JsonError(pub String);

impl std::fmt::Display for JsonError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl std::error::Error for JsonError {}

pub fn check_result_to_json(result: &CheckResult) -> Result<String, JsonError> {
    let mut value =
        serde_json::to_value(result).map_err(|error| JsonError(error.to_string()))?;
    normalize_utc_timestamps(&mut value);
    serde_json::to_string(&value).map_err(|error| JsonError(error.to_string()))
}

pub fn check_result_from_json(json: &str) -> Result<CheckResult, JsonError> {
    let input: serde_json::Value =
        serde_json::from_str(json).map_err(|error| JsonError(error.to_string()))?;
    let result: CheckResult =
        serde_json::from_value(input.clone()).map_err(|error| JsonError(error.to_string()))?;
    let canonical: serde_json::Value = serde_json::from_str(&check_result_to_json(&result)?)
        .map_err(|error| JsonError(error.to_string()))?;
    if input != canonical {
        let path = first_difference(&input, &canonical, "$");
        return Err(JsonError(format!(
            "non-canonical or missing protobuf JSON field at {path}"
        )));
    }
    Ok(result)
}

fn normalize_utc_timestamps(value: &mut serde_json::Value) {
    match value {
        serde_json::Value::String(text)
            if text.len() >= 25
                && text.ends_with("+00:00")
                && text.as_bytes().get(4) == Some(&b'-')
                && text.as_bytes().get(10) == Some(&b'T') =>
        {
            text.truncate(text.len() - 6);
            text.push('Z');
        }
        serde_json::Value::Array(values) => {
            values.iter_mut().for_each(normalize_utc_timestamps);
        }
        serde_json::Value::Object(values) => {
            values.values_mut().for_each(normalize_utc_timestamps);
        }
        _ => {}
    }
}

fn first_difference(input: &serde_json::Value, canonical: &serde_json::Value, path: &str) -> String {
    match (input, canonical) {
        (serde_json::Value::Object(left), serde_json::Value::Object(right)) => {
            for (key, value) in right {
                let child = format!("{path}.{key}");
                match left.get(key) {
                    Some(actual) if actual == value => {}
                    Some(actual) => return first_difference(actual, value, &child),
                    None => return child,
                }
            }
            left.keys()
                .find(|key| !right.contains_key(*key))
                .map(|key| format!("{path}.{key}"))
                .unwrap_or_else(|| path.to_owned())
        }
        (serde_json::Value::Array(left), serde_json::Value::Array(right)) => {
            for (index, value) in right.iter().enumerate() {
                let child = format!("{path}[{index}]");
                match left.get(index) {
                    Some(actual) if actual == value => {}
                    Some(actual) => return first_difference(actual, value, &child),
                    None => return child,
                }
            }
            path.to_owned()
        }
        _ => path.to_owned(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{
        check_result, Error, ErrorCode, Failed, FallbackRequired, Throttled, UpToDate,
        UpdateAvailable,
    };

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
                apply_disposition: 0,
            })),
        }
    }

    #[test]
    fn empty_notes_and_false_mandatory_emit_keys() {
        let json = check_result_to_json(&empty_notes_available()).unwrap();
        assert!(json.contains("\"releaseNotesMarkdown\":\"\""), "{json}");
        assert!(json.contains("\"releaseNotesUrl\":\"\""), "{json}");
        assert!(json.contains("\"mandatory\":false"), "{json}");
        assert_eq!(
            json,
            include_str!("../../../conformance/updater/check-result-empty-notes.json").trim_end()
        );
    }

    #[test]
    fn timestamp_is_rfc3339() {
        let result = CheckResult {
            kind: Some(check_result::Kind::Throttled(Throttled {
                next_allowed_at: Some(pbjson_types::Timestamp {
                    seconds: 1_786_943_706,
                    nanos: 123_000_000,
                }),
            })),
        };
        let json = check_result_to_json(&result).unwrap();
        assert_eq!(
            json,
            r#"{"throttled":{"nextAllowedAt":"2026-08-17T05:15:06.123Z"}}"#
        );
        assert_eq!(check_result_from_json(&json).unwrap(), result);
    }

    #[test]
    fn five_variants_round_trip() {
        let variants = [
            (CheckResult {
                kind: Some(check_result::Kind::UpToDate(UpToDate::default())),
            }, include_str!("../../../conformance/updater/check-result-up-to-date.json")),
            (empty_notes_available(), include_str!("../../../conformance/updater/check-result-empty-notes.json")),
            (CheckResult {
                kind: Some(check_result::Kind::FallbackRequired(FallbackRequired::default())),
            }, include_str!("../../../conformance/updater/check-result-fallback-required.json")),
            (CheckResult {
                kind: Some(check_result::Kind::Throttled(Throttled {
                    next_allowed_at: Some(pbjson_types::Timestamp {
                        seconds: 1_786_943_706,
                        nanos: 123_000_000,
                    }),
                })),
            }, include_str!("../../../conformance/updater/check-result-throttled.json")),
            (CheckResult {
                kind: Some(check_result::Kind::Failed(Failed {
                    error: Some(Error {
                        code: ErrorCode::Network as i32,
                        retryable: true,
                        message: "offline".into(),
                        attempts: Vec::new(),
                        recovery: None,
                    }),
                })),
            }, include_str!("../../../conformance/updater/check-result-failed.json")),
        ];
        for (result, fixture) in variants {
            let json = check_result_to_json(&result).unwrap();
            assert_eq!(json, fixture.trim_end());
            assert_eq!(check_result_from_json(&json).unwrap(), result, "{json}");
        }
    }

    #[test]
    fn missing_release_notes_markdown_is_rejected() {
        let mut value: serde_json::Value =
            serde_json::from_str(&check_result_to_json(&empty_notes_available()).unwrap()).unwrap();
        value["updateAvailable"].as_object_mut().unwrap().remove("releaseNotesMarkdown");
        let error = check_result_from_json(&value.to_string()).unwrap_err();
        assert!(error.0.contains("releaseNotesMarkdown"), "{}", error.0);
    }

    #[test]
    fn missing_mandatory_is_rejected() {
        let mut value: serde_json::Value =
            serde_json::from_str(&check_result_to_json(&empty_notes_available()).unwrap()).unwrap();
        value["updateAvailable"].as_object_mut().unwrap().remove("mandatory");
        let error = check_result_from_json(&value.to_string()).unwrap_err();
        assert!(error.0.contains("mandatory"), "{}", error.0);
    }
}
