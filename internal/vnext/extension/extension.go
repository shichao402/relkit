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
	var stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if runContext.Err() == context.DeadlineExceeded {
			return fmt.Errorf("extension %s timed out after %s", spec.Kind, timeout)
		}
		return fmt.Errorf("extension %s failed: %w; stderr: %s", spec.Kind, err, stderr.String())
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
