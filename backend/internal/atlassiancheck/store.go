package atlassiancheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

type token struct {
	AccountID string    `json:"account_id"`
	Access    string    `json:"access"`
	Refresh   string    `json:"refresh"`
	Expires   time.Time `json:"expires"`
	Retrieved time.Time `json:"retrieved"`
	Next      time.Time `json:"next"`
}

type state struct {
	Tokens       map[string]token     `json:"tokens"`
	Schedules    map[string]time.Time `json:"schedules"`
	RetryAfter   time.Time            `json:"retry_after"`
	LastReport   time.Time            `json:"last_report"`
	ReportStatus int                  `json:"report_status"`
}

type store struct {
	path string
	keys *crypto.Keyring
}

func (s store) load() (state, error) {
	result := state{Tokens: make(map[string]token), Schedules: make(map[string]time.Time)}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read encrypted store: %w", err)
	}
	plain, err := s.keys.OpenEnvelope(data, 1, []byte("atlassian-check/v1"))
	if err != nil {
		return result, fmt.Errorf("open encrypted store: %w", err)
	}
	if err = json.Unmarshal(plain, &result); err != nil || result.Tokens == nil {
		return result, errors.New("invalid encrypted store contents")
	}
	if result.Schedules == nil {
		result.Schedules = make(map[string]time.Time)
	}
	return result, nil
}

func (s store) save(st state) error {
	plain, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode stand state: %w", err)
	}
	data, _, err := s.keys.SealEnvelope(plain, []byte("atlassian-check/v1"))
	if err != nil {
		return fmt.Errorf("encrypt stand state: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return fmt.Errorf("create state transaction: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write state transaction: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync state transaction: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close state transaction: %w", err)
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return fmt.Errorf("commit state transaction: %w", err)
	}
	return nil
}
