// Package testadapter is the test adapter on recorded Jira Cloud answers (TZ 14.12): faults are injected by
// operation, page and attempt, so the core is tested against failures a real source cannot repeat.
package testadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
)

const (
	OpProbe       = "probe"
	OpFetch       = "fetch"
	OpManifest    = "manifest"
	OpCheckAccess = "check_access"
)

// Fault kinds: answers of the source as the core sees them, plus broken streams of Fetch.
const (
	FaultAuth              = "auth"
	FaultPermissionMissing = "permission_missing"
	FaultRateLimited       = "rate_limited"
	FaultUnavailable       = "unavailable"
	FaultHang              = "hang"
	FaultDropConnection    = "drop_connection"
	FaultTruncated         = "truncated"
	FaultSchemaViolation   = "schema_violation"
	FaultPreparing         = "preparing"
)

const (
	EditionCloud       = "cloud"
	EditionUnsupported = "unsupported"
)

var (
	_operations = []string{OpProbe, OpFetch, OpManifest, OpCheckAccess}
	_kinds      = []string{
		FaultAuth, FaultPermissionMissing, FaultRateLimited, FaultUnavailable, FaultHang,
		FaultDropConnection, FaultTruncated, FaultSchemaViolation, FaultPreparing,
	}
	_streamKinds = []string{FaultDropConnection, FaultTruncated}
	_decisions   = []string{"allowed", "denied", "unknown"}
)

// Config is one scenario. The same scenario always gives the same sequence of answers: attempts are
// counted per operation, data kind and page from the start of the process or the last change of faults.
type Config struct {
	// Edition is cloud by default; unsupported injects the wrong-edition response (V-12).
	Edition string `json:"edition"`
	// Capabilities verified by Probe; empty is the M0 set of jiracloud.
	Capabilities []string `json:"capabilities"`
	// HistoryDenied answers 403 to history: its capabilities are missing and task_events is refused.
	HistoryDenied bool    `json:"history_denied"`
	Faults        []Fault `json:"faults"`
	Access        Access  `json:"access"`
}

// Fault applies to the attempts listed, or to every attempt, of an operation on a page; page 0 is any
// page. Pages of Fetch and Manifest are numbered from 1 by the position of the cursor and the limit.
type Fault struct {
	Operation string `json:"operation"`
	SchemaID  string `json:"schema_id"`
	Page      int    `json:"page"`
	Attempts  []int  `json:"attempts"`
	Kind      string `json:"kind"`
	// RetryAfterSeconds is the pause of rate_limited and preparing; zero sends no Retry-After.
	RetryAfterSeconds int `json:"retry_after_seconds"`
	// DelayMS is how long hang holds the answer; zero is 45 s, longer than the 40 s the core waits.
	DelayMS int `json:"delay_ms"`
	// AfterBytes is where drop_connection and truncated cut the package; zero is 64 bytes.
	AfterBytes int `json:"after_bytes"`
}

// Access answers CheckAccess: the first rule that matches decides, otherwise Default (unknown).
type Access struct {
	Default string       `json:"default"`
	DelayMS int          `json:"delay_ms"`
	Rules   []AccessRule `json:"rules"`
}

// AccessRule matches by user, item kind and native ID; an empty field matches anything.
type AccessRule struct {
	UserID     string `json:"user_id"`
	ItemKind   string `json:"item_kind"`
	NativeID   string `json:"native_id"`
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
}

func (f Fault) matches(op, schema string, page, attempt int) bool {
	return f.Operation == op &&
		(f.SchemaID == "" || f.SchemaID == schema) &&
		(f.Page == 0 || f.Page == page) &&
		(len(f.Attempts) == 0 || slices.Contains(f.Attempts, attempt))
}

func (c Config) validate() error {
	var errs []error
	if c.Edition != "" && c.Edition != EditionCloud && c.Edition != EditionUnsupported {
		errs = append(errs, fmt.Errorf("edition %q is not cloud or unsupported", c.Edition))
	}
	for _, capability := range c.Capabilities {
		if !slices.Contains(_jiraCapabilities, capability) {
			errs = append(errs, fmt.Errorf("capability %q is not one of jiracloud", capability))
		}
	}
	for i, f := range c.Faults {
		switch {
		case !slices.Contains(_operations, f.Operation):
			errs = append(errs, fmt.Errorf("fault %d: operation %q is unknown", i, f.Operation))
		case !slices.Contains(_kinds, f.Kind):
			errs = append(errs, fmt.Errorf("fault %d: kind %q is unknown", i, f.Kind))
		case slices.Contains(_streamKinds, f.Kind) && f.Operation != OpFetch:
			errs = append(errs, fmt.Errorf("fault %d: %s breaks only the Fetch stream", i, f.Kind))
		case f.Page < 0 || f.RetryAfterSeconds < 0 || f.DelayMS < 0 || f.AfterBytes < 0:
			errs = append(errs, fmt.Errorf("fault %d: numbers must not be negative", i))
		}
	}
	if c.Access.Default != "" && !slices.Contains(_decisions, c.Access.Default) {
		errs = append(errs, fmt.Errorf("access default %q is unknown", c.Access.Default))
	}
	for i, r := range c.Access.Rules {
		if !slices.Contains(_decisions, r.Decision) {
			errs = append(errs, fmt.Errorf("access rule %d: decision %q is unknown", i, r.Decision))
		}
	}
	return errors.Join(errs...)
}

// ParseConfig reads a scenario; an unknown field is an error, so a misspelt fault is not silently lost.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("testadapter: scenario: %w", err)
	}
	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("testadapter: scenario: %w", err)
	}
	return c, nil
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("testadapter: %w", err)
	}
	return ParseConfig(data)
}
