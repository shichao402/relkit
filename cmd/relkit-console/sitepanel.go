package main

import (
	"log"
	"os"
	"sync"
	"time"

	"go.firoyang.com/relkit/internal/makers"
)

// makersPanel is the console's read-only Pages view for the Site card (ADR
// 0016 step 5): the latest deployment statuses of the makers project. It is
// the third adapter-shaped read path -- a query, never a write; publishing
// stays with relkit-agent.
//
// A short cache keeps a browser auto-refresh from hammering the Pages API on
// every render. Errors and a missing token degrade to "no live rows": the
// site/status.json snapshot remains the primary fact on the card.
type makersPanel struct {
	cfg    makers.Config
	client *makers.Client

	mu      sync.Mutex
	fetched time.Time
	rows    []makers.Deployment
}

// makersCacheTTL bounds how stale the Pages rows on the panel may be. The
// snapshot answers "what did the last rebuild do"; the API rows answer "what
// is Pages doing right now", and a minute of staleness there is invisible.
const makersCacheTTL = time.Minute

func newMakersPanel(cfg makers.Config) *makersPanel {
	return &makersPanel{
		cfg:    cfg,
		client: &makers.Client{BaseURL: makers.APIBaseURL(cfg.Region)},
	}
}

// latest returns the most recent deployments, from cache when fresh. A missing
// token or an API failure is not an error for the panel: it just shows the
// snapshot half of the card.
func (p *makersPanel) latest(limit int) []makers.Deployment {
	if p == nil || p.cfg.ProjectID == "" {
		return nil
	}
	p.mu.Lock()
	if time.Since(p.fetched) < makersCacheTTL && p.rows != nil {
		rows := p.rows
		p.mu.Unlock()
		return rows
	}
	p.mu.Unlock()

	tokenEnv := p.cfg.TokenEnv
	if tokenEnv == "" {
		tokenEnv = makers.DefaultTokenEnv
	}
	client := *p.client
	client.Token = os.Getenv(tokenEnv)
	if client.Token == "" {
		// Quiet degradation by design: the snapshot still renders, and the
		// operator sees the Pages token is simply not wired to the console.
		return nil
	}
	rows, err := client.ListDeployments(p.cfg.ProjectID, limit)
	if err != nil {
		log.Printf("WARNING: makers panel: %v", err)
		return nil
	}
	p.mu.Lock()
	p.rows = rows
	p.fetched = time.Now()
	p.mu.Unlock()
	return rows
}
