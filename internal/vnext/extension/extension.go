// Package extension executes the narrow, versioned process boundary allowed
// by vNext. Extensions receive JSON on stdin and return JSON on stdout; they
// never run in-process or inherit the full environment.
package extension

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.extension/1"

type Kind string

const (
	KindArtifactProducer Kind = "artifact-producer"
	KindVersionProvider  Kind = "version-provider"
	KindMirrorProducer   Kind = "mirror-producer"
	KindBundleProducer   Kind = "bundle-producer"
	KindCIAdapter        Kind = "ci-adapter"
	KindDeployAdapter    Kind = "deploy-adapter"
	KindRotateAdapter    Kind = "rotate-adapter"
)

type Spec struct {
	Schema           string   `json:"schema"`
	Kind             Kind     `json:"kind"`
	Command          []string `json:"command"`
	StdinSchema      string   `json:"stdinSchema"`
	StdoutSchema     string   `json:"stdoutSchema"`
	TimeoutSeconds   int      `json:"timeoutSeconds,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	EnvAllowlist     []string `json:"envAllowlist,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
}

func (s Spec) Validate() error {
	if s.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", s.Schema, Schema)
	}
	switch s.Kind {
	case KindArtifactProducer, KindVersionProvider, KindMirrorProducer, KindBundleProducer, KindCIAdapter, KindDeployAdapter, KindRotateAdapter:
	default:
		return fmt.Errorf("unknown extension kind %q", s.Kind)
	}
	if len(s.Command) == 0 || s.Command[0] == "" {
		return fmt.Errorf("command must not be empty")
	}
	if s.StdinSchema == "" || s.StdoutSchema == "" {
		return fmt.Errorf("stdinSchema and stdoutSchema must not be empty")
	}
	if s.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must not be negative")
	}
	seen := map[string]bool{}
	for _, name := range s.EnvAllowlist {
		if name == "" || seen[name] {
			return fmt.Errorf("envAllowlist contains an empty or duplicate name")
		}
		seen[name] = true
	}
	allowedPermissions := []string{"read-workspace", "write-out-dir", "network-declared", "exec-self"}
	for _, permission := range s.Permissions {
		if !slices.Contains(allowedPermissions, permission) {
			return fmt.Errorf("unknown extension permission %q", permission)
		}
	}
	return nil
}

// Run executes spec without a shell. Only allowlisted environment variables
// are inherited. stdout is capped and decoded strictly into output.
func Run(ctx context.Context, spec Spec, input any, output any) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	timeout := time.Duration(spec.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdin, err := canonical.Bytes(input)
	if err != nil {
		return err
	}
	stdin = append(stdin, '\n')
	command := exec.CommandContext(runContext, spec.Command[0], spec.Command[1:]...)
	command.Dir = spec.WorkingDirectory
	command.Env = allowedEnvironment(spec.EnvAllowlist)
	command.Stdin = bytes.NewReader(stdin)

	var stdout limitedBuffer
	// stderr 必须实时透传，不能只收进内存缓冲。
	//
	// 扩展（尤其是 artifact-producer）会把构建工具的输出写到 stderr，
	// 一次构建正常要跑十几分钟。此前 stderr 全部收进内存、等 command.Run()
	// 返回才可能被打印，导致超时或挂死时整段输出随缓冲区一起丢失，
	// CI 任务日志呈现"长时间零输出"，无法判断卡在哪一步（D17 #94）。
	//
	// 现在：每一行到达即写入本进程 stderr（蓝盾/终端可见），同时保留
	// 有界尾部副本，供失败信息里附带最后若干行上下文。
	stderrSink := newTeeTailWriter(os.Stderr, maxExtensionStderrTail)
	command.Stdout = &stdout
	command.Stderr = stderrSink
	if err := command.Run(); err != nil {
		if runContext.Err() == context.DeadlineExceeded {
			return fmt.Errorf("extension %s timed out after %s; stderr tail: %s",
				spec.Kind, timeout, stderrSink.Tail())
		}
		return fmt.Errorf("extension %s failed: %w; stderr tail: %s",
			spec.Kind, err, stderrSink.Tail())
	}
	if output == nil {
		if len(bytes.TrimSpace(stdout.Bytes())) != 0 {
			return fmt.Errorf("extension %s returned unexpected stdout", spec.Kind)
		}
		return nil
	}
	if err := document.Decode(stdout.Bytes(), output); err != nil {
		return fmt.Errorf("extension %s returned invalid %s: %w", spec.Kind, spec.StdoutSchema, err)
	}
	return nil
}

func allowedEnvironment(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}

const maxExtensionOutput = 16 << 20

// stderr 尾部预留：失败信息里附带的上下文长度，不影响实时透传。
const maxExtensionStderrTail = 8 << 10

// teeTailWriter 把写入同时转发给上游 writer（实时可见）并保留最后 N 字节，
// 供错误信息附带上下文。并发由单 goroutine 读取保证，无需加锁。
type teeTailWriter struct {
	dst  io.Writer
	keep int
	tail []byte
}

func newTeeTailWriter(dst io.Writer, keep int) *teeTailWriter {
	return &teeTailWriter{dst: dst, keep: keep}
}

func (w *teeTailWriter) Write(data []byte) (int, error) {
	// 落盘/透传失败不影响进程本身：构建工具的输出丢了也不该让构建失败。
	_, _ = w.dst.Write(data)
	if w.keep > 0 {
		w.tail = append(w.tail, data...)
		if len(w.tail) > w.keep {
			w.tail = w.tail[len(w.tail)-w.keep:]
		}
	}
	return len(data), nil
}

// Tail 返回已保留的尾部内容（可能被截断在行中间，仅用于诊断展示）。
func (w *teeTailWriter) Tail() string {
	return string(w.tail)
}

type limitedBuffer struct {
	bytes.Buffer
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := maxExtensionOutput - b.Len()
	if remaining <= 0 {
		return 0, fmt.Errorf("extension output exceeds %d bytes", maxExtensionOutput)
	}
	if len(data) > remaining {
		_, _ = b.Buffer.Write(data[:remaining])
		return remaining, fmt.Errorf("extension output exceeds %d bytes", maxExtensionOutput)
	}
	return b.Buffer.Write(data)
}

var _ io.Writer = (*limitedBuffer)(nil)
