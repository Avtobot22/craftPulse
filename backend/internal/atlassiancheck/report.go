package atlassiancheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type reportAccount struct {
	AccountID string    `json:"accountId"` //nolint:tagliatelle // Atlassian wire format.
	UpdatedAt time.Time `json:"updatedAt"` //nolint:tagliatelle // Atlassian wire format.
}

type reportAction struct {
	AccountID string `json:"accountId"` //nolint:tagliatelle // Atlassian wire format.
	Status    string `json:"status"`
}

type reportResult struct {
	Reported int `json:"reported"`
	Erased   int `json:"erased"`
}

// RunReports resumes persisted per-account schedules. The stand stores no profile or source content.
func (s *Server) RunReports(ctx context.Context, onError func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, cancel := context.WithTimeout(ctx, 90*time.Second)
			s.mu.Lock()
			err := s.persist()
			if err == nil {
				for role := range s.state.Tokens {
					if _, refreshErr := s.access(job, role); refreshErr != nil {
						onError(fmt.Errorf("refresh role %s: %w", role, refreshErr))
					}
				}
				_, err = s.report(job)
			}
			s.mu.Unlock()
			cancel()
			if err != nil {
				onError(err)
			}
		}
	}
}

func (s *Server) report(ctx context.Context) (reportResult, error) {
	var result reportResult
	now := time.Now().UTC()
	if s.state.RetryAfter.After(now) || len(s.state.Tokens) == 0 {
		return result, nil
	}
	roles := make([]string, 0, len(s.state.Tokens))
	for role, t := range s.state.Tokens {
		if !t.Next.After(now) {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return result, nil
	}
	slices.Sort(roles)
	bearer, err := s.access(ctx, "owner")
	if err != nil {
		return result, err
	}
	for start := 0; start < len(roles); start += 90 {
		batch := roles[start:min(start+90, len(roles))]
		part, err := s.reportBatch(ctx, batch, bearer, now)
		result.Reported += part.Reported
		result.Erased += part.Erased
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) reportBatch(ctx context.Context, roles []string, bearer string, now time.Time) (reportResult, error) {
	var result reportResult
	accounts := make([]reportAccount, 0, len(roles))
	requested := make(map[string]string, len(roles))
	for _, role := range roles {
		t := s.state.Tokens[role]
		if !_accountID.MatchString(t.AccountID) || t.AccountID == "unknown" || t.Retrieved.IsZero() {
			return result, errors.New("invalid account in privacy inventory")
		}
		accounts = append(accounts, reportAccount{AccountID: t.AccountID, UpdatedAt: t.Retrieved})
		requested[t.AccountID] = role
	}
	body := struct {
		Accounts []reportAccount `json:"accounts"`
	}{accounts}
	b, headers, status, err := s.provider.request(ctx, http.MethodPost,
		"https://api.atlassian.com/app/report-accounts/", bearer, body)
	if err != nil {
		return result, s.reportFailure(now, 0, time.Minute, err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		delay := time.Hour
		if status == http.StatusTooManyRequests {
			delay = retryDelay(headers.Get("Retry-After"), now)
		}
		return result, s.reportFailure(now, status, delay, providerError{Status: status})
	}
	var response struct {
		Accounts []reportAction `json:"accounts"`
	}
	if status == http.StatusOK {
		if err = json.Unmarshal(b, &response); err != nil {
			return result, s.reportFailure(now, status, time.Hour, errors.New("invalid privacy response JSON"))
		}
	}
	for _, action := range response.Accounts {
		if requested[action.AccountID] == "" || (action.Status != "closed" && action.Status != "updated") {
			return result, s.reportFailure(now, status, time.Hour, errors.New("invalid privacy action"))
		}
	}
	cycle, cycleErr := cyclePeriod(headers.Get("Cycle-Period"))
	if cycleErr != nil {
		// Apply erasure even if scheduling metadata is unexpected; pause instead of over-reporting.
		cycle = 365 * 24 * time.Hour
	}
	for _, role := range roles {
		t := s.state.Tokens[role]
		t.Next = now.Add(cycle)
		s.state.Schedules[role] = t.Next
		s.state.Tokens[role] = t
	}
	for _, action := range response.Accounts {
		// For both closed and updated, erase the entire copy including OAuth tokens.
		// An updated account may explicitly authorize again to supply a fresh copy.
		delete(s.state.Tokens, requested[action.AccountID])
		result.Erased++
	}
	s.state.LastReport, s.state.ReportStatus = now, status
	s.state.RetryAfter = time.Time{}
	if err := s.persist(); err != nil {
		return result, err
	}
	result.Reported = len(roles)
	return result, cycleErr
}

func (s *Server) reportFailure(now time.Time, status int, delay time.Duration, cause error) error {
	s.state.ReportStatus, s.state.RetryAfter = status, now.Add(delay)
	if err := s.persist(); err != nil {
		return err
	}
	return cause
}

func cyclePeriod(value string) (time.Duration, error) {
	if value == "" {
		return 7 * 24 * time.Hour, nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	if days := regexp.MustCompile(`^P([1-9]\d{0,3})D$`).FindStringSubmatch(value); days != nil {
		n, _ := strconv.Atoi(days[1])
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if duration, err := time.ParseDuration(strings.TrimPrefix(value, "PT")); err == nil && duration > 0 {
		return duration, nil
	}
	return 0, errors.New("unrecognized privacy cycle period; reporting paused for operator review")
}

func retryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return time.Hour
}
