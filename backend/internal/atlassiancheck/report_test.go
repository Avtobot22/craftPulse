package atlassiancheck

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func inventory(s *Server, role, id string) {
	s.state.Tokens[role] = token{
		AccountID: id, Access: "owner-bearer", Refresh: "refresh", Expires: time.Now().Add(time.Hour),
		Retrieved: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestReportingErasesClosedAndUpdatedAndPersistsSchedule(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	inventory(s, "owner", "id-owner")
	inventory(s, "a_allowed", "id-allowed")
	inventory(s, "a_denied", "id-denied")
	calls := 0
	s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https://api.atlassian.com/app/report-accounts/", r.URL.String())
		require.Equal(t, "Bearer owner-bearer", r.Header.Get("Authorization"))
		var input struct {
			Accounts []reportAccount `json:"accounts"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		require.Len(t, input.Accounts, 3)
		for _, account := range input.Accounts {
			require.Equal(t, 2026, account.UpdatedAt.Year())
			require.Equal(t, time.October, account.UpdatedAt.Month())
		}
		response := reply(200, `{"accounts":[{"accountId":"id-allowed","status":"closed"},{"accountId":"id-denied","status":"updated"}]}`)
		response.Header.Set("Cycle-Period", "1209600")
		return response, nil
	})
	result, err := s.report(context.Background())
	require.NoError(t, err)
	require.Equal(t, reportResult{Reported: 3, Erased: 2}, result)
	require.Len(t, s.state.Tokens, 1)
	require.Contains(t, s.state.Schedules, "a_allowed")
	require.Contains(t, s.state.Schedules, "a_denied")
	require.WithinDuration(t, time.Now().Add(14*24*time.Hour), s.state.Tokens["owner"].Next, time.Second)
	restarted, err := New(s.config)
	require.NoError(t, err)
	restarted.provider = s.provider
	result, err = restarted.report(context.Background())
	require.NoError(t, err)
	require.Zero(t, result.Reported)
	require.Equal(t, 1, calls)
	require.Equal(t, time.October, restarted.state.Tokens["owner"].Retrieved.Month())
}

func TestReportingBatchesAtMost90And204(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	inventory(s, "owner", "id-owner")
	for i := range 180 {
		inventory(s, "role-"+strconv.Itoa(i), "id-"+strconv.Itoa(i))
	}
	sizes := make([]int, 0, 3)
	s.provider.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var input struct {
			Accounts []reportAccount `json:"accounts"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		require.LessOrEqual(t, len(input.Accounts), 90)
		sizes = append(sizes, len(input.Accounts))
		return reply(204, ""), nil
	})
	result, err := s.report(context.Background())
	require.NoError(t, err)
	require.Equal(t, 181, result.Reported)
	require.Equal(t, []int{90, 90, 1}, sizes)
}

func TestReportingFailureDoesNotAdvanceAccountsAndHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	for _, status := range []int{400, 403, 429, 500} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			s := testServer(t)
			inventory(s, "owner", "id-owner")
			calls := 0
			s.provider.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				response := reply(status, `{"error":"private-details"}`)
				response.Header.Set("Retry-After", "7200")
				return response, nil
			})
			_, err := s.report(context.Background())
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-details")
			require.True(t, s.state.Tokens["owner"].Next.IsZero())
			require.True(t, s.state.LastReport.IsZero())
			if status == 429 {
				require.WithinDuration(t, time.Now().Add(2*time.Hour), s.state.RetryAfter, time.Second)
			}
			restarted, err := New(s.config)
			require.NoError(t, err)
			restarted.provider = s.provider
			_, err = restarted.report(context.Background())
			require.NoError(t, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestReportingRejectsUnknownActionsWithoutErasingUnreportedAccount(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	inventory(s, "owner", "id-owner")
	s.provider.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return reply(200, `{"accounts":[{"accountId":"not-in-request","status":"closed"}]}`), nil
	})
	_, err := s.report(context.Background())
	require.Error(t, err)
	require.Len(t, s.state.Tokens, 1)
	require.True(t, s.state.LastReport.IsZero())
}
