package sdk

import "time"

// Policy holds the intervals from SPEC §12.2. Hosts may override them.
type Policy struct {
	AfterSuccess      time.Duration
	AfterFailure      time.Duration
	DocumentTimeout   time.Duration
	DownloadIdleTimeout time.Duration
	DownloadRetries   int
	DownloadWorkers   int
	DownloadChunkSize int64
}

// DefaultPolicy returns SPEC-aligned defaults.
func DefaultPolicy() Policy {
	return Policy{
		AfterSuccess:        24 * time.Hour,
		AfterFailure:        time.Hour,
		DocumentTimeout:     10 * time.Second,
		DownloadIdleTimeout: 60 * time.Second,
		DownloadRetries:     3,
		DownloadWorkers:     8,
		DownloadChunkSize:   4 << 20,
	}
}

// ShouldCheck reports whether a check is due given last check time and result.
// lastResult is "success", "available", "fallback", "failure", or empty.
func (p Policy) ShouldCheck(lastCheckAt *time.Time, lastResult string) bool {
	if lastCheckAt == nil {
		return true
	}
	elapsed := time.Since(*lastCheckAt)
	switch lastResult {
	case "failure":
		return elapsed >= p.normalizedFailure()
	default:
		return elapsed >= p.normalizedSuccess()
	}
}

// NextAllowedAt returns when the next check is allowed.
func (p Policy) NextAllowedAt(lastCheckAt *time.Time, lastResult string) time.Time {
	if lastCheckAt == nil {
		return time.Time{}
	}
	switch lastResult {
	case "failure":
		return lastCheckAt.Add(p.normalizedFailure())
	default:
		return lastCheckAt.Add(p.normalizedSuccess())
	}
}

func (p Policy) normalizedSuccess() time.Duration {
	if p.AfterSuccess <= 0 {
		return 24 * time.Hour
	}
	return p.AfterSuccess
}

func (p Policy) normalizedFailure() time.Duration {
	if p.AfterFailure <= 0 {
		return time.Hour
	}
	return p.AfterFailure
}

// RuntimeConfig is host-provided background-check configuration.
type RuntimeConfig struct {
	CheckOnStart bool
	ForceOnStart bool
	Policy       Policy
}
