// SIM-S3: 可选产品套件（模拟 Dec 形态）
//
// 三个组件 sim-server / sim-exec / sim-host-setup 模拟 Dec 的 runtime
// components，共享同一套 suite 运行时（component × audience 选择器）。
// 与 Dec internal/update 同构：
//   - 编译期 ldflags 注入 runtimeBootstrapBase64
//   - sdk.RuntimeBootstrapFromJSON 解析 ClientProfile
//   - Runtime 带 component/audience selectors + InstallSpec(in-place)
//
// hello-world 业务: 打印版本与组件名。

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
	"github.com/shichao402/relkit/sdk"
	"github.com/shichao402/relkit/sdk/updaterfacade"
)

// 由 canonical producer 通过 -ldflags -X 注入。
var runtimeBootstrapBase64 string

var version = "0.1.0+1"

func clientProfile() (*updaterv1.ClientProfile, error) {
	if runtimeBootstrapBase64 == "" {
		return nil, errors.New("Relkit Runtime Profile was not injected at build time")
	}
	raw, err := base64.StdEncoding.DecodeString(runtimeBootstrapBase64)
	if err != nil {
		return nil, fmt.Errorf("decode Relkit runtime bootstrap: %w", err)
	}
	bootstrap, err := sdk.RuntimeBootstrapFromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse Relkit runtime bootstrap: %w", err)
	}
	return bootstrap.ClientProfile, nil
}

func suiteRuntime(component, goos, goarch, channel string, currentCode int64, installRoot string) *updaterv1.Runtime {
	name := component
	sidecar := "relkit-updater"
	if goos == "windows" {
		name += ".exe"
		sidecar += ".exe"
	}
	return &updaterv1.Runtime{
		Channel:     channel,
		CurrentCode: currentCode,
		ClientSelectors: map[string]string{
			"os":        goos,
			"arch":      goarch,
			"component": component,
			"audience":  "runtime",
		},
		DataDir: filepath.Join(installRoot, "updater", goos+"-"+goarch+"-"+component),
		Install: &updaterv1.InstallSpec{
			Placement:         updaterv1.Placement_PLACEMENT_IN_PLACE,
			InstallRoot:       installRoot,
			ExecutableRelpath: name,
			SidecarRelpath:    sidecar,
		},
		SidecarPath: filepath.Join(installRoot, sidecar),
	}
}

func openUpdater(ctx context.Context, rt *updaterv1.Runtime) (*updaterfacade.Updater, error) {
	profile, err := clientProfile()
	if err != nil {
		return nil, err
	}
	opened := updaterfacade.Open(ctx, profile, rt, nil)
	if opened.Kind != "opened" || opened.Updater == nil {
		msg := "open sidecar failed"
		if opened.Error != nil && opened.Error.Message != "" {
			msg = opened.Error.Message
		}
		return nil, errors.New(msg)
	}
	return opened.Updater, nil
}

// 发布 code = 版本串的 build 号（+N），与服务器索引节点 code 空间一致。
// 注意：不能用 sdk.SemverCode（那是 semver 段编码 major*1e6+minor*1e3+patch，
// 会剥掉 +N，导致 CurrentCode 恒大于索引节点而永远 up-to-date）。
func codeOf(v string) int64 {
	if i := strings.LastIndex(v, "+"); i >= 0 {
		if n, err := strconv.ParseInt(v[i+1:], 10, 64); err == nil {
			return n
		}
	}
	return 1
}

func run(component string) {
	command := "status"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	channel := os.Getenv("SIM_CHANNEL")
	if channel == "" {
		channel = "dev"
	}
	installRoot := os.Getenv("SIM_INSTALL_ROOT")
	if installRoot == "" {
		exe, _ := os.Executable()
		installRoot = filepath.Dir(exe)
	}

	switch command {
	case "--version":
		fmt.Printf("sim-s3 %s %s\n", component, version)
	case "check":
		ctx := context.Background()
		rt := suiteRuntime(component, runtime.GOOS, runtime.GOARCH, channel, codeOf(version), installRoot)
		u, err := openUpdater(ctx, rt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		cr := u.Check(ctx, true, 0, nil)
		switch k := cr.Kind.(type) {
		case *updaterv1.CheckResult_UpdateAvailable:
			fmt.Printf("check-result: update-available %s code=%d plan=%s\n", k.UpdateAvailable.GetVersion(), k.UpdateAvailable.GetCode(), k.UpdateAvailable.GetPlanId())
		case *updaterv1.CheckResult_UpToDate:
			fmt.Printf("check-result: up-to-date sequence=%d\n", k.UpToDate.GetSequence())
		case *updaterv1.CheckResult_Failed:
			fmt.Printf("check-result: failed %s\n", k.Failed.GetError().GetMessage())
			os.Exit(1)
		default:
			fmt.Printf("check-result: %v\n", cr.Kind)
		}
	case "apply":
		ctx := context.Background()
		rt := suiteRuntime(component, runtime.GOOS, runtime.GOARCH, channel, codeOf(version), installRoot)
		u, err := openUpdater(ctx, rt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		cr := u.Check(ctx, true, 0, nil)
		if ua, ok := cr.Kind.(*updaterv1.CheckResult_UpdateAvailable); ok {
			planID := ua.UpdateAvailable.GetPlanId()
			if dr := u.Download(ctx, planID, nil); dr.GetFailed() != nil {
				fmt.Fprintf(os.Stderr, "download failed: %s\n", dr.GetFailed().GetError().GetMessage())
				os.Exit(1)
			}
			if ar := u.Apply(ctx, planID, nil); ar.GetFailed() != nil {
				fmt.Fprintf(os.Stderr, "apply failed: %s\n", ar.GetFailed().GetError().GetMessage())
				os.Exit(1)
			}
			fmt.Println("apply-result: ok")
		} else {
			fmt.Println("apply-result: no-update")
		}
	default:
		fmt.Printf("sim-s3 %s hello world (%s)\n", component, version)
		fmt.Println("commands: --version | check | apply")
		fmt.Println("env: SIM_CHANNEL, SIM_INSTALL_ROOT")
	}
}

func main() {
	// 组件身份由二进制名决定（e2e driver 直接运行三个入口中的某一个）。
	exe, _ := os.Executable()
	component := filepath.Base(exe)
	component = component[:len(component)-len(filepath.Ext(component))]
	run(component)
}
