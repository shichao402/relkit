package updater

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	relkitproto "github.com/shichao402/relkit/proto"
)

// ADR 0026, decision 7: the wire is relkit.updater.v2 as ProtoJSON lines.
// These fixtures are what a host's generated types must decode strictly and
// produce equivalently. Comparison is semantic (proto.Equal), never byte for
// byte: Go's protojson deliberately randomizes whitespace.

var strictJSON = protojson.UnmarshalOptions{DiscardUnknown: false}

// conformanceRequest is the request a LIBRARY host sends for a check.
func conformanceRequest(t *testing.T) *updaterv2.UpdaterRequest {
	t.Helper()
	return &updaterv2.UpdaterRequest{
		Protocol: Protocol,
		Profile: &updaterv2.ClientProfile{
			Bootstrap:       string(conformanceBootstrap(t)),
			ExpectedProduct: "loom",
		},
		Runtime: &updaterv2.Runtime{
			Channel:         "dev",
			CurrentCode:     3,
			ClientSelectors: map[string]string{"edition": "pro"},
			DataDir:         "/var/lib/loom/updater",
			Install: &updaterv2.InstallSpec{
				Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
				InstallRoot:       "/opt/loom",
				ExecutableRelpath: "bin/loom",
			},
		},
		Op: &updaterv2.UpdaterRequest_Check{Check: &updaterv2.CheckOp{}},
	}
}

// conformanceEvents is one representative stdout stream: the handshake line,
// then one of each kind a host's UI has to render.
func conformanceEvents(t *testing.T) []*updaterv2.UpdaterEvent {
	t.Helper()
	_, report, err := openProfile(conformanceRequest(t).GetProfile(), "dev")
	if err != nil {
		t.Fatalf("conformance bootstrap must open: %v", err)
	}
	return []*updaterv2.UpdaterEvent{
		{Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: (&Engine{Version: "conformance"}).capabilities(report)}},
		{Kind: &updaterv2.UpdaterEvent_Check{Check: &updaterv2.CheckResult{
			Kind: &updaterv2.CheckResult_UpdateAvailable{UpdateAvailable: &updaterv2.UpdateAvailable{
				PlanId:               "plan-conformance",
				PromptKey:            "code:4",
				Version:              "1.0.4",
				Code:                 4,
				Sequence:             9,
				RemainingHops:        1,
				ReleaseNotesMarkdown: "- fixes",
				Artifacts:            []*updaterv2.ArtifactView{{Name: "loom-payload.zip", Size: 1024, Sha256: bytes.Repeat([]byte{0xab}, 32)}},
				ApplyDisposition:     updaterv2.ApplyDisposition_APPLY_DISPOSITION_INTERNAL,
			}},
		}}},
		{Kind: &updaterv2.UpdaterEvent_Progress{Progress: &updaterv2.Progress{BytesReceived: 512, BytesTotal: 1024, BytesPerSecond: 256}}},
		{Kind: &updaterv2.UpdaterEvent_Apply{Apply: &updaterv2.ApplyResult{
			Kind: &updaterv2.ApplyResult_Accepted{Accepted: &updaterv2.ApplyAccepted{
				SessionId: "session-conformance", PlanId: "plan-conformance", RequiresHostExit: true,
			}},
		}}},
		{Kind: &updaterv2.UpdaterEvent_ApplyProgress{ApplyProgress: &updaterv2.ApplyProgress{
			SessionId: "session-conformance", Phase: updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		}}},
		failedEvent(updaterv2.ErrorCode_ERROR_CODE_NETWORK, true, "offline", []string{"https://update.example.invalid/"}),
	}
}

func writeLines(t *testing.T, path string, msgs ...proto.Message) {
	t.Helper()
	var buf bytes.Buffer
	for _, m := range msgs {
		raw, err := protojson.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readLines(t *testing.T, path string) [][]byte {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(fixtureBytes(t, path)))
	sc.Buffer(make([]byte, 0, 64<<10), MaxRequestBytes)
	var out [][]byte
	for sc.Scan() {
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 {
			out = append(out, append([]byte(nil), line...))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProtocolRequestConformance(t *testing.T) {
	path := fixturePath(t, "protocol-request-check.json")
	want := conformanceRequest(t)
	if *updateFixtures {
		writeLines(t, path, want)
		return
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("request fixture must be one line, got %d", len(lines))
	}
	got, err := ReadRequest(bytes.NewReader(lines[0]))
	if err != nil {
		t.Fatalf("engine must accept the fixture request: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Fatal("protocol-request-check.json drifted from the schema; regenerate with -run ProtocolRequestConformance -args -update")
	}
}

func TestProtocolEventsConformance(t *testing.T) {
	path := fixturePath(t, "protocol-events.jsonl")
	want := conformanceEvents(t)
	if *updateFixtures {
		msgs := make([]proto.Message, len(want))
		for i, ev := range want {
			msgs[i] = ev
		}
		writeLines(t, path, msgs...)
		return
	}
	lines := readLines(t, path)
	if len(lines) != len(want) {
		t.Fatalf("events fixture has %d lines, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		got := &updaterv2.UpdaterEvent{}
		if err := strictJSON.Unmarshal(line, got); err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if !proto.Equal(got, want[i]) {
			t.Fatalf("line %d drifted from the engine; regenerate with -run ProtocolEventsConformance -args -update", i+1)
		}
	}
	if want[0].GetCapabilities().GetProtocol() != Protocol {
		t.Fatal("first event must carry the engine protocol")
	}
}

// The check-result-*.json samples predate ADR 0026 and were consumed by the
// deleted SDKs. They stay as strict-decode samples for host UIs.
func TestCheckResultSamplesDecodeStrictly(t *testing.T) {
	matches, err := filepath.Glob(fixturePath(t, "check-result-*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no check-result samples: %v", err)
	}
	for _, path := range matches {
		raw := fixtureBytes(t, path)
		first := &updaterv2.CheckResult{}
		if err := strictJSON.Unmarshal(raw, first); err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		again, err := protojson.Marshal(first)
		if err != nil {
			t.Fatal(err)
		}
		second := &updaterv2.CheckResult{}
		if err := strictJSON.Unmarshal(again, second); err != nil || !proto.Equal(first, second) {
			t.Fatalf("%s: semantic round trip failed (%v)", filepath.Base(path), err)
		}
	}
}

// `relkit-updater proto` prints relkitproto.UpdaterV2; it must be the very
// schema the engine was generated from.
func TestEmbeddedSchemaIsTheV2Contract(t *testing.T) {
	if !strings.Contains(relkitproto.UpdaterV2, "package relkit.updater.v2;") {
		t.Fatal("embedded schema is not relkit.updater.v2")
	}
	if got := string(updaterv2.File_updater_v2_updater_proto.Package()); got != "relkit.updater.v2" {
		t.Fatalf("generated package = %q", got)
	}
	for _, gone := range []string{"ClientHello", "CancelOp", "TrustedKey", "index_urls", "fallback_urls"} {
		if strings.Contains(relkitproto.UpdaterV2, gone) {
			t.Fatalf("host-visible schema still mentions %s", gone)
		}
	}
}
