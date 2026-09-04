package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeAPI struct {
	calls []Target
	errs  []error
}

func (f *fakeAPI) ApplyAndDeploy(t Target) error {
	f.calls = append(f.calls, t)
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	if len(f.errs) > 1 {
		f.errs = f.errs[1:]
	} else {
		f.errs = nil
	}
	return err
}

func TestNoRenewWhenPlentyOfDays(t *testing.T) {
	api := &fakeAPI{}
	code := processTargets(fileConfig{
		RenewBeforeDays: 30,
		Targets:         []Target{{Domain: "raw.example", Region: "ap-guangzhou", Bucket: "b"}},
	}, time.Now(), false, func(string, time.Time) (int, error) { return 80, nil }, api, 3, 0)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(api.calls) != 0 {
		t.Fatalf("unexpected renew calls: %+v", api.calls)
	}
}

func TestRenewSuccess(t *testing.T) {
	api := &fakeAPI{}
	code := processTargets(fileConfig{
		RenewBeforeDays: 30,
		Targets:         []Target{{Domain: "raw.example", Region: "ap-guangzhou", Bucket: "b"}},
	}, time.Now(), false, func(string, time.Time) (int, error) { return 10, nil }, api, 3, 0)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(api.calls) != 1 {
		t.Fatalf("calls=%d", len(api.calls))
	}
}

func TestRetryOnTimeoutThenSuccess(t *testing.T) {
	api := &fakeAPI{errs: []error{errors.New("timeout from API"), nil}}
	code := processTargets(fileConfig{
		RenewBeforeDays: 30,
		Targets:         []Target{{Domain: "raw.example", Region: "ap-guangzhou", Bucket: "b"}},
	}, time.Now(), false, func(string, time.Time) (int, error) { return 5, nil }, api, 3, 0)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(api.calls) != 2 {
		t.Fatalf("calls=%d", len(api.calls))
	}
}

func TestPartialTargetFailure(t *testing.T) {
	api := &fakeAPI{errs: []error{errors.New("deploy failed")}}
	code := processTargets(fileConfig{
		RenewBeforeDays: 30,
		Targets: []Target{
			{Domain: "raw.example", Region: "ap-guangzhou", Bucket: "b1"},
			{Domain: "raw2.example", Region: "ap-chengdu", Bucket: "b2"},
		},
	}, time.Now(), false, func(domain string, _ time.Time) (int, error) {
		return 1, nil
	}, api, 1, 0)
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestProbeFailureCounts(t *testing.T) {
	code := processTargets(fileConfig{
		Targets: []Target{{Domain: "raw.example", Region: "ap-guangzhou", Bucket: "b"}},
	}, time.Now(), false, func(string, time.Time) (int, error) {
		return 0, errors.New("dial timeout")
	}, &fakeAPI{}, 1, 0)
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRenewConfigJSON(t *testing.T) {
	raw := []byte(`{"renewBeforeDays":30,"targets":[{"domain":"raw.example","region":"ap-guangzhou","bucket":"b"}]}`)
	path := filepath.Join(t.TempDir(), "renew.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-config", path, "-dry-run"}); code != 1 && code != 0 {
		t.Fatalf("unexpected code %d", code)
	}
}
