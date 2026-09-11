//! Rust host facade for `relkit.updater.v1`.
//!
//! The DTOs in [`proto`] are generated from `proto/updater/v1/updater.proto`
//! by `build.rs`; this crate deliberately contains no handwritten wire DTOs.

use prost::Message;
use std::fs;
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::{Duration, SystemTime};

pub mod proto {
    include!(concat!(env!("OUT_DIR"), "/relkit.updater.v1.rs"));
}

use proto::updater_event;
use proto::updater_request;
use proto::{
    ApplyOp, ApplyResult, Capabilities, CheckOp, CheckPolicy, CheckResult, CleanupOp, ClientHello,
    ClientProfile, DownloadOp, DownloadResult, Error, ErrorCode, Failed, Ok,
    Result as OperationResult, Runtime, SchedulerConfig, SkipOp, StatusOp, StatusSnapshot,
    UpdaterEvent, UpdaterRequest,
};

pub const IPC_MIN: u32 = 1;
pub const IPC_MAX: u32 = 1;
pub const IPC_CURRENT: u32 = 1;
const MAX_FRAME_SIZE: usize = 32 * 1024 * 1024;

/// Prefixes one protobuf message with its 4-byte big-endian length.
pub fn encode_frame<M: Message>(message: &M) -> Vec<u8> {
    let payload = message.encode_to_vec();
    let mut framed = Vec::with_capacity(4 + payload.len());
    framed.extend_from_slice(&(payload.len() as u32).to_be_bytes());
    framed.extend_from_slice(&payload);
    framed
}

/// Incremental decoder used for fragmented and multi-frame stdout streams.
#[derive(Default)]
pub struct FrameDecoder {
    buffered: Vec<u8>,
}

impl FrameDecoder {
    pub fn push(&mut self, chunk: &[u8]) -> io::Result<Vec<UpdaterEvent>> {
        self.buffered.extend_from_slice(chunk);
        let mut events = Vec::new();
        loop {
            if self.buffered.len() < 4 {
                break;
            }
            let size = u32::from_be_bytes(self.buffered[..4].try_into().unwrap()) as usize;
            if size > MAX_FRAME_SIZE {
                return Err(io::Error::new(
                    io::ErrorKind::InvalidData,
                    format!("protobuf frame exceeds {MAX_FRAME_SIZE} bytes"),
                ));
            }
            if self.buffered.len() < 4 + size {
                break;
            }
            let event = UpdaterEvent::decode(&self.buffered[4..4 + size])
                .map_err(|error| io::Error::new(io::ErrorKind::InvalidData, error))?;
            self.buffered.drain(..4 + size);
            events.push(event);
        }
        Ok(events)
    }
}

/// Locates and runs the updater sidecar. Tauri hosts can inject a fake Glue.
pub trait Glue: Send + Sync {
    fn locate(&self, runtime: &Runtime) -> io::Result<PathBuf>;
    fn run(
        &self,
        bin: &Path,
        args: &[&str],
        stdin: &[u8],
        on_event: &mut dyn FnMut(&UpdaterEvent),
    ) -> io::Result<()>;
    fn cancel(&self) -> io::Result<()> {
        Ok(())
    }
}

#[derive(Default)]
pub struct DefaultGlue {
    active: Mutex<Option<Arc<Mutex<Child>>>>,
}

impl DefaultGlue {
    fn executable_name() -> &'static str {
        if cfg!(windows) {
            "relkit-updater.exe"
        } else {
            "relkit-updater"
        }
    }
}

fn locate_candidates(
    runtime: &Runtime,
    env_sidecar: Option<PathBuf>,
    executable: Option<PathBuf>,
) -> Vec<PathBuf> {
    let mut candidates = Vec::new();
    if !runtime.sidecar_path.is_empty() {
        candidates.push(PathBuf::from(&runtime.sidecar_path));
    }
    if let Some(path) = env_sidecar {
        candidates.push(path);
    }
    if let Some(install) = runtime.install.as_ref() {
        if !install.install_root.is_empty() {
            candidates.push(Path::new(&install.install_root).join(DefaultGlue::executable_name()));
        }
    }
    if let Some(exe) = executable {
        if let Some(parent) = exe.parent() {
            candidates.push(parent.join(DefaultGlue::executable_name()));
        }
    }
    if let Some(install) = runtime.install.as_ref() {
        if !install.install_root.is_empty() && !install.sidecar_relpath.is_empty() {
            candidates.push(
                Path::new(&install.install_root).join(
                    install
                        .sidecar_relpath
                        .replace('/', std::path::MAIN_SEPARATOR_STR),
                ),
            );
        }
    }
    candidates
}

impl Glue for DefaultGlue {
    fn locate(&self, runtime: &Runtime) -> io::Result<PathBuf> {
        let env_sidecar = std::env::var_os("RELKIT_UPDATER").map(PathBuf::from);
        let executable = std::env::current_exe().ok();
        locate_candidates(runtime, env_sidecar, executable)
            .into_iter()
            .find(|candidate| {
                fs::metadata(candidate)
                    .map(|m| m.is_file())
                    .unwrap_or(false)
            })
            .ok_or_else(|| io::Error::new(io::ErrorKind::NotFound, "sidecar not found"))
    }

    fn run(
        &self,
        bin: &Path,
        args: &[&str],
        stdin: &[u8],
        on_event: &mut dyn FnMut(&UpdaterEvent),
    ) -> io::Result<()> {
        let mut command = Command::new(bin);
        command
            .args(args)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit());
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            const CREATE_NO_WINDOW: u32 = 0x0800_0000;
            command.creation_flags(CREATE_NO_WINDOW);
        }
        let mut child = command.spawn()?;
        if let Some(mut input) = child.stdin.take() {
            input.write_all(stdin)?;
        }
        let mut stdout = child.stdout.take().ok_or_else(|| {
            io::Error::new(io::ErrorKind::BrokenPipe, "sidecar stdout unavailable")
        })?;
        let child = Arc::new(Mutex::new(child));
        *self.active.lock().unwrap() = Some(Arc::clone(&child));

        let read_result = (|| {
            let mut decoder = FrameDecoder::default();
            let mut chunk = [0_u8; 8192];
            loop {
                let count = stdout.read(&mut chunk)?;
                if count == 0 {
                    break;
                }
                for event in decoder.push(&chunk[..count])? {
                    on_event(&event);
                }
            }
            let status = child.lock().unwrap().wait()?;
            if !status.success() {
                return Err(io::Error::other(format!("sidecar exited with {status}")));
            }
            Ok(())
        })();
        *self.active.lock().unwrap() = None;
        read_result
    }

    fn cancel(&self) -> io::Result<()> {
        if let Some(child) = self.active.lock().unwrap().as_ref() {
            child.lock().unwrap().kill()?;
        }
        Ok(())
    }
}

pub enum OpenResult {
    Opened {
        updater: Box<Updater>,
        capabilities: Capabilities,
    },
    Failed(Error),
}

#[derive(Clone)]
pub struct Updater {
    profile: ClientProfile,
    runtime: Runtime,
    glue: Arc<dyn Glue>,
    bin: PathBuf,
    capabilities: Capabilities,
}

impl Updater {
    pub fn open(profile: ClientProfile, runtime: Runtime) -> OpenResult {
        Self::open_with_glue(profile, runtime, Arc::new(DefaultGlue::default()))
    }

    pub fn open_with_glue(
        profile: ClientProfile,
        runtime: Runtime,
        glue: Arc<dyn Glue>,
    ) -> OpenResult {
        let bin = match glue.locate(&runtime) {
            Ok(path) => path,
            Err(error) => {
                return OpenResult::Failed(facade_error(ErrorCode::SidecarNotFound, error))
            }
        };
        let mut events = Vec::new();
        let run_result = glue.run(&bin, &["-capabilities"], &[], &mut |event| {
            events.push(event.clone())
        });
        if let Some(error) = events.iter().find_map(event_failure) {
            return OpenResult::Failed(error);
        }
        if let Err(error) = run_result {
            return OpenResult::Failed(facade_error(ErrorCode::SidecarNotFound, error));
        }
        let Some(capabilities) = events.into_iter().find_map(|event| match event.kind {
            Some(updater_event::Kind::Capabilities(value)) => Some(value),
            _ => None,
        }) else {
            return OpenResult::Failed(error_message(
                ErrorCode::ProtocolMismatch,
                "no capabilities",
                false,
            ));
        };
        if capabilities.ipc < IPC_MIN {
            return OpenResult::Failed(error_message(
                ErrorCode::UpdaterTooOld,
                "sidecar IPC too old",
                false,
            ));
        }
        if capabilities.ipc > IPC_MAX {
            return OpenResult::Failed(error_message(
                ErrorCode::UpdaterTooNew,
                "sidecar IPC too new",
                false,
            ));
        }
        OpenResult::Opened {
            updater: Box::new(Self {
                profile,
                runtime,
                glue,
                bin,
                capabilities: capabilities.clone(),
            }),
            capabilities,
        }
    }

    pub fn capabilities(&self) -> &Capabilities {
        &self.capabilities
    }

    fn call(
        &self,
        op: updater_request::Op,
        on_event: &mut dyn FnMut(&UpdaterEvent),
    ) -> Vec<UpdaterEvent> {
        let request = UpdaterRequest {
            hello: Some(ClientHello {
                ipc_min: IPC_MIN,
                ipc_max: IPC_MAX,
            }),
            profile: Some(self.profile.clone()),
            runtime: Some(self.runtime.clone()),
            op: Some(op),
        };
        let mut events = Vec::new();
        let run_result = self
            .glue
            .run(&self.bin, &[], &encode_frame(&request), &mut |event| {
                on_event(event);
                events.push(event.clone());
            });
        if let Err(error) = run_result {
            if events.is_empty() {
                events.push(failed_event(ErrorCode::Network, error.to_string(), true));
            }
        }
        events
    }

    pub fn check(&self, force: bool, exact_code: i64, policy: Option<CheckPolicy>) -> CheckResult {
        let events = self.call(
            updater_request::Op::Check(CheckOp {
                force,
                exact_code,
                policy,
            }),
            &mut |_| {},
        );
        for event in events {
            match event.kind {
                Some(updater_event::Kind::Check(result)) => return result,
                Some(updater_event::Kind::Failed(failed)) => {
                    return CheckResult {
                        kind: Some(proto::check_result::Kind::Failed(failed)),
                    }
                }
                _ => {}
            }
        }
        CheckResult {
            kind: Some(proto::check_result::Kind::Failed(Failed {
                error: Some(error_message(ErrorCode::Network, "no check result", true)),
            })),
        }
    }

    pub fn skip(&self, code: i64) -> OperationResult {
        operation_result(self.call(updater_request::Op::Skip(SkipOp { code }), &mut |_| {}))
    }

    pub fn download<F>(&self, plan_id: impl Into<String>, mut on_event: F) -> DownloadResult
    where
        F: FnMut(&UpdaterEvent),
    {
        let events = self.call(
            updater_request::Op::Download(DownloadOp {
                plan_id: plan_id.into(),
            }),
            &mut on_event,
        );
        for event in &events {
            if let Some(updater_event::Kind::Download(result)) = &event.kind {
                return result.clone();
            }
            if let Some(updater_event::Kind::Failed(failed)) = &event.kind {
                return DownloadResult {
                    kind: Some(proto::download_result::Kind::Failed(failed.clone())),
                };
            }
        }
        DownloadResult {
            kind: Some(proto::download_result::Kind::Failed(Failed {
                error: Some(error_message(
                    ErrorCode::Network,
                    "no download result",
                    true,
                )),
            })),
        }
    }

    pub fn apply<F>(&self, plan_id: impl Into<String>, mut on_event: F) -> ApplyResult
    where
        F: FnMut(&UpdaterEvent),
    {
        let events = self.call(
            updater_request::Op::Apply(ApplyOp {
                plan_id: plan_id.into(),
            }),
            &mut on_event,
        );
        for event in &events {
            if let Some(updater_event::Kind::Apply(result)) = &event.kind {
                return result.clone();
            }
            if let Some(updater_event::Kind::Failed(failed)) = &event.kind {
                return ApplyResult {
                    kind: Some(proto::apply_result::Kind::Failed(failed.clone())),
                };
            }
        }
        ApplyResult {
            kind: Some(proto::apply_result::Kind::Failed(Failed {
                error: Some(error_message(ErrorCode::Network, "no apply result", true)),
            })),
        }
    }

    pub fn status(&self) -> StatusSnapshot {
        self.call(updater_request::Op::Status(StatusOp {}), &mut |_| {})
            .into_iter()
            .find_map(|event| match event.kind {
                Some(updater_event::Kind::Status(status)) => Some(status),
                _ => None,
            })
            .unwrap_or_default()
    }

    pub fn cleanup(&self) -> OperationResult {
        operation_result(self.call(updater_request::Op::Cleanup(CleanupOp {}), &mut |_| {}))
    }

    pub fn cancel(&self) -> OperationResult {
        match self.glue.cancel() {
            Ok(()) => ok_result(),
            Err(error) => failed_result(ErrorCode::Canceled, error.to_string(), false),
        }
    }

    pub fn scheduler<F>(&self, config: SchedulerConfig, on_event: F) -> Scheduler
    where
        F: Fn(SchedulerEvent) + Send + Sync + 'static,
    {
        Scheduler::new(self.clone(), config, Arc::new(on_event))
    }
}

#[derive(Clone)]
pub enum SchedulerEvent {
    Tick {
        at: SystemTime,
        force: bool,
    },
    Result {
        at: SystemTime,
        force: bool,
        result: CheckResult,
    },
}

pub struct Scheduler {
    updater: Updater,
    config: SchedulerConfig,
    on_event: Arc<dyn Fn(SchedulerEvent) + Send + Sync>,
    running: Arc<AtomicBool>,
    worker: Mutex<Option<JoinHandle<()>>>,
}

impl Scheduler {
    fn new(
        updater: Updater,
        config: SchedulerConfig,
        on_event: Arc<dyn Fn(SchedulerEvent) + Send + Sync>,
    ) -> Self {
        Self {
            updater,
            config,
            on_event,
            running: Arc::new(AtomicBool::new(false)),
            worker: Mutex::new(None),
        }
    }

    pub fn start(&self) -> OperationResult {
        if self.running.swap(true, Ordering::SeqCst) {
            return ok_result();
        }
        let updater = self.updater.clone();
        let config = self.config;
        let on_event = Arc::clone(&self.on_event);
        let running = Arc::clone(&self.running);
        *self.worker.lock().unwrap() = Some(thread::spawn(move || {
            let mut first = true;
            while running.load(Ordering::SeqCst) {
                if !first || config.check_on_start {
                    let force = first && config.force_on_start;
                    on_event(SchedulerEvent::Tick {
                        at: SystemTime::now(),
                        force,
                    });
                    let result = updater.check(force, 0, config.policy);
                    on_event(SchedulerEvent::Result {
                        at: SystemTime::now(),
                        force,
                        result,
                    });
                }
                first = false;
                let wait = scheduler_wait(config.policy.as_ref());
                let slice = Duration::from_millis(100);
                let mut elapsed = Duration::ZERO;
                while elapsed < wait && running.load(Ordering::SeqCst) {
                    let step = std::cmp::min(slice, wait - elapsed);
                    thread::sleep(step);
                    elapsed += step;
                }
            }
        }));
        ok_result()
    }

    pub fn stop(&self) -> OperationResult {
        self.running.store(false, Ordering::SeqCst);
        if let Some(worker) = self.worker.lock().unwrap().take() {
            let _ = worker.join();
        }
        ok_result()
    }

    pub fn is_running(&self) -> bool {
        self.running.load(Ordering::SeqCst)
    }
}

impl Drop for Scheduler {
    fn drop(&mut self) {
        self.running.store(false, Ordering::SeqCst);
        if let Some(worker) = self.worker.get_mut().unwrap().take() {
            let _ = worker.join();
        }
    }
}

fn scheduler_wait(policy: Option<&CheckPolicy>) -> Duration {
    let minimum = Duration::from_secs(5 * 60);
    let success = policy
        .and_then(|p| p.after_success.as_ref())
        .map(prost_duration)
        .filter(|value| !value.is_zero())
        .unwrap_or(Duration::from_secs(24 * 60 * 60));
    let failure = policy
        .and_then(|p| p.after_failure.as_ref())
        .map(prost_duration)
        .filter(|value| !value.is_zero())
        .unwrap_or(Duration::from_secs(60 * 60));
    std::cmp::max(minimum, std::cmp::min(success, failure))
}

fn prost_duration(value: &prost_types::Duration) -> Duration {
    if value.seconds < 0 || value.nanos < 0 {
        return Duration::ZERO;
    }
    Duration::new(value.seconds as u64, value.nanos as u32)
}

fn event_failure(event: &UpdaterEvent) -> Option<Error> {
    match &event.kind {
        Some(updater_event::Kind::Failed(failed)) => failed.error.clone(),
        _ => None,
    }
}

fn facade_error(error_code: ErrorCode, error: impl std::fmt::Display) -> Error {
    error_message(error_code, error.to_string(), false)
}

fn error_message(code: ErrorCode, message: impl Into<String>, retryable: bool) -> Error {
    Error {
        code: code as i32,
        retryable,
        message: message.into(),
        attempts: Vec::new(),
        recovery: None,
    }
}

fn failed_event(code: ErrorCode, message: impl Into<String>, retryable: bool) -> UpdaterEvent {
    UpdaterEvent {
        kind: Some(updater_event::Kind::Failed(Failed {
            error: Some(error_message(code, message, retryable)),
        })),
    }
}

fn ok_result() -> OperationResult {
    OperationResult {
        kind: Some(proto::result::Kind::Ok(Ok {})),
    }
}

fn failed_result(code: ErrorCode, message: impl Into<String>, retryable: bool) -> OperationResult {
    OperationResult {
        kind: Some(proto::result::Kind::Failed(Failed {
            error: Some(error_message(code, message, retryable)),
        })),
    }
}

fn operation_result(events: Vec<UpdaterEvent>) -> OperationResult {
    for event in events {
        match event.kind {
            Some(updater_event::Kind::Result(result)) => return result,
            Some(updater_event::Kind::Failed(failed)) => {
                return OperationResult {
                    kind: Some(proto::result::Kind::Failed(failed)),
                }
            }
            _ => {}
        }
    }
    failed_result(ErrorCode::Network, "no operation result", true)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::VecDeque;
    use std::sync::atomic::AtomicUsize;

    struct FakeGlue {
        runs: Mutex<Vec<Vec<u8>>>,
        replies: Mutex<VecDeque<Vec<UpdaterEvent>>>,
    }

    impl FakeGlue {
        fn new(replies: Vec<Vec<UpdaterEvent>>) -> Self {
            Self {
                runs: Mutex::new(Vec::new()),
                replies: Mutex::new(replies.into()),
            }
        }
    }

    impl Glue for FakeGlue {
        fn locate(&self, _runtime: &Runtime) -> io::Result<PathBuf> {
            Ok(PathBuf::from("fake-sidecar"))
        }

        fn run(
            &self,
            _bin: &Path,
            _args: &[&str],
            stdin: &[u8],
            on_event: &mut dyn FnMut(&UpdaterEvent),
        ) -> io::Result<()> {
            self.runs.lock().unwrap().push(stdin.to_vec());
            for event in self.replies.lock().unwrap().pop_front().unwrap_or_default() {
                on_event(&event);
            }
            Ok(())
        }
    }

    struct ExitGlue {
        events: Vec<UpdaterEvent>,
    }

    impl Glue for ExitGlue {
        fn locate(&self, _runtime: &Runtime) -> io::Result<PathBuf> {
            Ok(PathBuf::from("fake-sidecar"))
        }

        fn run(
            &self,
            _bin: &Path,
            _args: &[&str],
            _stdin: &[u8],
            on_event: &mut dyn FnMut(&UpdaterEvent),
        ) -> io::Result<()> {
            for event in &self.events {
                on_event(event);
            }
            Err(io::Error::other("sidecar exited with 7"))
        }
    }

    struct FailingCallGlue {
        calls: AtomicUsize,
    }

    impl Glue for FailingCallGlue {
        fn locate(&self, _runtime: &Runtime) -> io::Result<PathBuf> {
            Ok(PathBuf::from("fake-sidecar"))
        }

        fn run(
            &self,
            _bin: &Path,
            _args: &[&str],
            _stdin: &[u8],
            on_event: &mut dyn FnMut(&UpdaterEvent),
        ) -> io::Result<()> {
            match self.calls.fetch_add(1, Ordering::SeqCst) {
                0 => {
                    on_event(&capabilities(1));
                    Ok(())
                }
                1 => {
                    on_event(&UpdaterEvent {
                        kind: Some(updater_event::Kind::Result(ok_result())),
                    });
                    Err(io::Error::other("late exit"))
                }
                _ => Err(io::Error::other("empty exit")),
            }
        }
    }

    struct StreamingGlue {
        host_observed: Arc<AtomicBool>,
        observed_before_return: Arc<AtomicBool>,
    }

    impl Glue for StreamingGlue {
        fn locate(&self, _runtime: &Runtime) -> io::Result<PathBuf> {
            Ok(PathBuf::from("fake-sidecar"))
        }

        fn run(
            &self,
            _bin: &Path,
            args: &[&str],
            _stdin: &[u8],
            on_event: &mut dyn FnMut(&UpdaterEvent),
        ) -> io::Result<()> {
            if args == ["-capabilities"] {
                on_event(&capabilities(1));
                return Ok(());
            }
            on_event(&UpdaterEvent {
                kind: Some(updater_event::Kind::Progress(proto::Progress {
                    bytes_received: 1,
                    bytes_total: 2,
                    bytes_per_second: 1,
                })),
            });
            self.observed_before_return
                .store(self.host_observed.load(Ordering::SeqCst), Ordering::SeqCst);
            on_event(&UpdaterEvent {
                kind: Some(updater_event::Kind::Download(DownloadResult {
                    kind: Some(proto::download_result::Kind::Downloaded(
                        proto::Downloaded {
                            plan_id: "plan".into(),
                            bytes: 2,
                        },
                    )),
                })),
            });
            Ok(())
        }
    }

    fn capabilities(ipc: u32) -> UpdaterEvent {
        UpdaterEvent {
            kind: Some(updater_event::Kind::Capabilities(Capabilities {
                ipc,
                ..Default::default()
            })),
        }
    }

    #[test]
    fn locate_uses_locked_order() {
        let runtime = Runtime {
            sidecar_path: "explicit".into(),
            install: Some(proto::InstallSpec {
                install_root: "root".into(),
                sidecar_relpath: "tools/custom-updater".into(),
                ..Default::default()
            }),
            ..Default::default()
        };
        let candidates = locate_candidates(
            &runtime,
            Some(PathBuf::from("env")),
            Some(PathBuf::from("app/loom")),
        );
        assert_eq!(candidates[0], PathBuf::from("explicit"));
        assert_eq!(candidates[1], PathBuf::from("env"));
        assert_eq!(
            candidates[2],
            Path::new("root").join(DefaultGlue::executable_name())
        );
        assert_eq!(
            candidates[3],
            Path::new("app").join(DefaultGlue::executable_name())
        );
        assert_eq!(
            candidates[4],
            Path::new("root").join("tools").join("custom-updater")
        );
    }

    #[test]
    fn frame_is_big_endian_and_round_trips() {
        let event = capabilities(1);
        let framed = encode_frame(&event);
        assert_eq!(
            u32::from_be_bytes(framed[..4].try_into().unwrap()) as usize,
            framed.len() - 4
        );
        let decoded = FrameDecoder::default().push(&framed).unwrap();
        assert_eq!(decoded.len(), 1);
        assert!(matches!(
            decoded[0].kind,
            Some(updater_event::Kind::Capabilities(_))
        ));
    }

    #[test]
    fn decoder_handles_fragmented_multiple_frames() {
        let mut bytes = encode_frame(&capabilities(1));
        bytes.extend(encode_frame(&failed_event(
            ErrorCode::Network,
            "offline",
            true,
        )));
        let mut decoder = FrameDecoder::default();
        assert!(decoder.push(&bytes[..3]).unwrap().is_empty());
        let decoded = decoder.push(&bytes[3..]).unwrap();
        assert_eq!(decoded.len(), 2);
    }

    #[test]
    fn decoder_rejects_frames_over_32_mib() {
        let mut decoder = FrameDecoder::default();
        let header = ((32_u32 << 20) + 1).to_be_bytes();
        let error = decoder.push(&header).unwrap_err();
        assert_eq!(error.kind(), io::ErrorKind::InvalidData);
        assert!(error.to_string().contains("33554432"));
    }

    #[test]
    fn handshake_enforces_window_and_call_writes_hello() {
        for (ipc, expected) in [(0, ErrorCode::UpdaterTooOld), (2, ErrorCode::UpdaterTooNew)] {
            let result = Updater::open_with_glue(
                ClientProfile::default(),
                Runtime::default(),
                Arc::new(FakeGlue::new(vec![vec![capabilities(ipc)]])),
            );
            match result {
                OpenResult::Failed(error) => assert_eq!(error.code, expected as i32),
                OpenResult::Opened { .. } => panic!("incompatible IPC opened"),
            }
        }

        let glue = Arc::new(FakeGlue::new(vec![
            vec![capabilities(1)],
            vec![UpdaterEvent {
                kind: Some(updater_event::Kind::Result(ok_result())),
            }],
        ]));
        let result =
            Updater::open_with_glue(ClientProfile::default(), Runtime::default(), glue.clone());
        let OpenResult::Opened { updater, .. } = result else {
            panic!("compatible sidecar did not open");
        };
        updater.skip(42);
        let calls = glue.runs.lock().unwrap();
        let payload = &calls[1];
        let size = u32::from_be_bytes(payload[..4].try_into().unwrap()) as usize;
        let request = UpdaterRequest::decode(&payload[4..4 + size]).unwrap();
        let hello = request.hello.unwrap();
        assert_eq!(hello.ipc_min, 1);
        assert_eq!(hello.ipc_max, 1);
    }

    #[test]
    fn handshake_nonzero_exit_fails_and_failed_event_wins() {
        let result = Updater::open_with_glue(
            ClientProfile::default(),
            Runtime::default(),
            Arc::new(ExitGlue {
                events: vec![capabilities(1)],
            }),
        );
        match result {
            OpenResult::Failed(error) => {
                assert_eq!(error.code, ErrorCode::SidecarNotFound as i32)
            }
            OpenResult::Opened { .. } => panic!("nonzero capabilities process opened"),
        }

        let result = Updater::open_with_glue(
            ClientProfile::default(),
            Runtime::default(),
            Arc::new(ExitGlue {
                events: vec![failed_event(ErrorCode::Signature, "bad signature", false)],
            }),
        );
        match result {
            OpenResult::Failed(error) => assert_eq!(error.code, ErrorCode::Signature as i32),
            OpenResult::Opened { .. } => panic!("failed event did not win"),
        }
    }

    #[test]
    fn ordinary_call_keeps_events_but_maps_empty_error() {
        let result = Updater::open_with_glue(
            ClientProfile::default(),
            Runtime::default(),
            Arc::new(FailingCallGlue {
                calls: AtomicUsize::new(0),
            }),
        );
        let OpenResult::Opened { updater, .. } = result else {
            panic!("fake sidecar did not open");
        };
        assert!(matches!(
            updater.skip(1).kind,
            Some(proto::result::Kind::Ok(_))
        ));
        assert!(matches!(
            updater.skip(2).kind,
            Some(proto::result::Kind::Failed(Failed {
                error: Some(Error {
                    code,
                    retryable: true,
                    ..
                })
            })) if code == ErrorCode::Network as i32
        ));
    }

    #[test]
    fn progress_callback_runs_before_glue_returns() {
        let host_observed = Arc::new(AtomicBool::new(false));
        let observed_before_return = Arc::new(AtomicBool::new(false));
        let result = Updater::open_with_glue(
            ClientProfile::default(),
            Runtime::default(),
            Arc::new(StreamingGlue {
                host_observed: Arc::clone(&host_observed),
                observed_before_return: Arc::clone(&observed_before_return),
            }),
        );
        let OpenResult::Opened { updater, .. } = result else {
            panic!("fake sidecar did not open");
        };
        let downloaded = updater.download("plan", |event| {
            if matches!(event.kind, Some(updater_event::Kind::Progress(_))) {
                host_observed.store(true, Ordering::SeqCst);
            }
        });
        assert!(matches!(
            downloaded.kind,
            Some(proto::download_result::Kind::Downloaded(_))
        ));
        assert!(observed_before_return.load(Ordering::SeqCst));
    }
}
