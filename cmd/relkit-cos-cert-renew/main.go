package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type fileConfig struct {
	RenewBeforeDays int      `json:"renewBeforeDays"`
	Targets         []Target `json:"targets"`
}

type Target struct {
	Domain string `json:"domain"`
	Region string `json:"region"`
	Bucket string `json:"bucket"`
}

type API interface {
	ApplyAndDeploy(t Target) error
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	fs := flag.NewFlagSet("relkit-cos-cert-renew", flag.ContinueOnError)
	configPath := fs.String("config", "/etc/relkit-cos-cert/renew.json", "config path")
	dryRun := fs.Bool("dry-run", false, "report only")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	raw, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	var cfg fileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return processTargets(cfg, time.Now(), *dryRun, probeCertDays, envAPI(), 3, time.Second)
}

func processTargets(cfg fileConfig, now time.Time, dry bool, probe func(string, time.Time) (int, error), api API, attempts int, backoff time.Duration) int {
	if cfg.RenewBeforeDays <= 0 {
		cfg.RenewBeforeDays = 30
	}
	if attempts <= 0 {
		attempts = 1
	}
	failed := 0
	for _, target := range cfg.Targets {
		days, err := probe(target.Domain, now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: probe: %v\n", target.Domain, err)
			if !dry {
				failed++
			}
			continue
		}
		fmt.Printf("%s: certificate valid for %d more days\n", target.Domain, days)
		if days > cfg.RenewBeforeDays {
			continue
		}
		fmt.Printf("%s: renewal due (threshold %d days)\n", target.Domain, cfg.RenewBeforeDays)
		if dry {
			continue
		}
		if api == nil {
			fmt.Fprintf(os.Stderr, "%s: no renew API configured (set RELKIT_COS_CERT_RENEW_HOOK or TENCENTCLOUD_SECRET_ID/KEY)\n", target.Domain)
			failed++
			continue
		}
		if err := applyWithRetry(api, target, attempts, backoff); err != nil {
			fmt.Fprintf(os.Stderr, "%s: renew: %v\n", target.Domain, err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func applyWithRetry(api API, t Target, attempts int, backoff time.Duration) error {
	var last error
	for i := 0; i < attempts; i++ {
		last = api.ApplyAndDeploy(t)
		if last == nil {
			return nil
		}
		if !retryable(last) || i == attempts-1 {
			return last
		}
		time.Sleep(backoff)
	}
	return last
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "temporar") || strings.Contains(msg, "429")
}

func probeCertDays(domain string, now time.Time) (int, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(domain, "443"), &tls.Config{ServerName: domain})
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return 0, fmt.Errorf("no peer certificates")
	}
	left := certs[0].NotAfter.Sub(now)
	return int(left.Hours() / 24), nil
}

func envAPI() API {
	if hook := os.Getenv("RELKIT_COS_CERT_RENEW_HOOK"); hook != "" {
		return hookAPI{cmd: hook}
	}
	client := newSSLClientFromEnv()
	if client == nil {
		return nil
	}
	return liveAPI{client: client}
}

type hookAPI struct {
	cmd string
}

func (h hookAPI) ApplyAndDeploy(t Target) error {
	c := exec.Command(h.cmd, t.Domain, t.Region, t.Bucket)
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
