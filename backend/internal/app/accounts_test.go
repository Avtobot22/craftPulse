package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	pg "github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type sourceAccountItem struct {
	ID              uuid.UUID `json:"id"`
	InstanceID      uuid.UUID `json:"instance_id"`
	Provider        string    `json:"provider"`
	NativeAccountID string    `json:"native_account_id"`
	DisplayName     *string   `json:"display_name"`
	ProposedKind    string    `json:"proposed_kind"`
	Assignment      *struct {
		ID        uuid.UUID  `json:"id"`
		PersonID  *uuid.UUID `json:"person_id"`
		ActorKind string     `json:"actor_kind"`
	} `json:"assignment"`
	Proposals []struct {
		PersonID uuid.UUID `json:"person_id"`
		Basis    string    `json:"basis"`
	} `json:"proposals"`
}

type assignmentResult struct {
	Items []struct {
		SourceAccountID uuid.UUID  `json:"source_account_id"`
		Outcome         string     `json:"outcome"`
		AssignmentID    *uuid.UUID `json:"assignment_id"`
		ErrorCode       *string    `json:"error_code"`
	} `json:"items"`
}

// accountsFixture writes the accounts of a Jira instance and of a universal import as normalization does.
func (e *connectionsEnv) accountsFixture(t *testing.T) (jira, imported entity.ID, ids map[string]uuid.UUID) {
	t.Helper()
	orgs, _ := newOrgAccess(e.app, e.clock, nil, nil)
	caller := newConnectForTest(t, e)
	jiraInst, err := caller.EnsureInstance(t.Context(), e.orgID, entity.ProviderJiraCloud, _jiraInstance)
	require.NoError(t, err)
	importInst, err := caller.EnsureInstance(t.Context(), e.orgID, entity.ProviderUniversal, "import.example.test")
	require.NoError(t, err)
	ids = map[string]uuid.UUID{}
	write := func(inst entity.ID, accounts ...entity.SourceAccount) {
		require.NoError(t, pg.InOrgTx(t.Context(), e.app, uuid.UUID(e.orgID), func(ctx context.Context) error {
			got, err := orgs.EnsureSourceAccounts(ctx, e.orgID, inst, accounts)
			for native, id := range got {
				ids[native] = uuid.UUID(id)
			}
			return err
		}))
	}
	human := func(native, login, name, email string) entity.SourceAccount {
		return entity.SourceAccount{NativeID: native, Login: login, DisplayName: name, Email: email, Kind: entity.PersonHuman, Described: true}
	}
	write(jiraInst.ID,
		human("JIRAUSER1", "asmirnova", "Анна Смирнова", "anna@example.org"),
		human("JIRAUSER2", "ipetrov", "Иван Петров", ""),
		entity.SourceAccount{NativeID: "JIRAUSER3", Login: "ci-bot", DisplayName: "CI", Kind: entity.PersonBot, Described: true})
	write(importInst.ID,
		human("imp-1", "ASmirnova", "А. Смирнова", ""),
		human("imp-2", "petrov.i", "Иван Петров", ""),
		human("imp-3", "", "Anna", "Anna@Example.org"))
	return jiraInst.ID, importInst.ID, ids
}

func newConnectForTest(t *testing.T, e *connectionsEnv) interface {
	EnsureInstance(context.Context, entity.ID, entity.Provider, string) (entity.SourceInstance, error)
} {
	t.Helper()
	uc, err := newConnect(e.app, e.keyring, nil, e.clock)
	require.NoError(t, err)
	return uc
}

func (e *connectionsEnv) sourceAccounts(t *testing.T, b *browser, query string) (code int, byNative map[string]sourceAccountItem) {
	t.Helper()
	rec := e.do(t, b, http.MethodGet, "/api/v1/source-accounts?limit=50"+query, nil)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	page := decode[struct {
		Data []sourceAccountItem `json:"data"`
	}](t, rec)
	out := map[string]sourceAccountItem{}
	for _, a := range page.Data {
		out[a.NativeAccountID] = a
	}
	return rec.Code, out
}

func (e *connectionsEnv) assign(t *testing.T, key uuid.UUID, items ...map[string]any) (int, assignmentResult) {
	t.Helper()
	h := http.Header{"Idempotency-Key": {key.String()}}
	rec := e.json(t, e.admin, http.MethodPost, "/api/v1/account-assignments", h, map[string]any{"items": items})
	if rec.Code != http.StatusOK {
		return rec.Code, assignmentResult{}
	}
	return rec.Code, decode[assignmentResult](t, rec)
}

func decision(account uuid.UUID, kind string, person, base *uuid.UUID, create bool) map[string]any {
	return map[string]any{"source_account_id": account, "actor_kind": kind, "person_id": person, "base_assignment_id": base, "create_person": create}
}

func TestAccounts_ProposalsAndConfirmedLinks(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})
	jira, _, ids := e.accountsFixture(t)

	code, first := e.assign(t, uuid.New(),
		decision(ids["JIRAUSER1"], "human", nil, nil, true), decision(ids["JIRAUSER2"], "human", nil, nil, true))
	require.Equal(t, http.StatusOK, code)
	require.Len(t, first.Items, 2)
	for _, it := range first.Items {
		assert.Equal(t, "saved", it.Outcome, "a new person is created with the link")
	}
	_, accounts := e.sourceAccounts(t, e.admin, "")
	anna := *accounts["JIRAUSER1"].Assignment.PersonID
	ivan := *accounts["JIRAUSER2"].Assignment.PersonID
	assert.Equal(t, "jira_cloud", accounts["JIRAUSER1"].Provider)
	assert.Equal(t, "universal", accounts["imp-1"].Provider)

	require.Len(t, accounts["imp-1"].Proposals, 1)
	assert.Equal(t, anna, accounts["imp-1"].Proposals[0].PersonID)
	assert.Equal(t, "login_match", accounts["imp-1"].Proposals[0].Basis, "the login of a linked account, in any case")
	require.Len(t, accounts["imp-3"].Proposals, 1)
	assert.Equal(t, "email_match", accounts["imp-3"].Proposals[0].Basis)
	assert.Empty(t, accounts["imp-2"].Proposals, "the same display name proposes nothing (V-15)")
	assert.Nil(t, accounts["imp-2"].Assignment, "and the accounts stay apart")
	assert.Empty(t, accounts["JIRAUSER1"].Proposals, "no proposal of the person the account is linked to")

	persons := decode[struct {
		Data []struct {
			ID   uuid.UUID `json:"id"`
			Kind string    `json:"kind"`
		} `json:"data"`
	}](t, e.do(t, e.admin, http.MethodGet, "/api/v1/persons?kind=human", nil))
	assert.ElementsMatch(t, []uuid.UUID{anna, ivan}, []uuid.UUID{persons.Data[0].ID, persons.Data[1].ID})

	pair := uuid.New()
	pairBody := []map[string]any{decision(ids["imp-1"], "human", &anna, nil, false), decision(ids["imp-2"], "bot", nil, nil, false)}
	code, saved := e.assign(t, pair, pairBody...)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "saved", saved.Items[0].Outcome)
	assert.Equal(t, "saved", saved.Items[1].Outcome, "both links of the batch are saved")

	stale := uuid.New()
	code, mixed := e.assign(t, uuid.New(),
		decision(ids["JIRAUSER1"], "human", &anna, &stale, false), decision(ids["JIRAUSER3"], "bot", nil, nil, false))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "version_conflict", mixed.Items[0].Outcome, "a link the form did not see")
	assert.Equal(t, "saved", mixed.Items[1].Outcome, "the other decision is saved")

	_, accounts = e.sourceAccounts(t, e.admin, "")
	humanLink := accounts["imp-1"].Assignment.ID
	var humanSeq int64
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT known_seq FROM org.account_assignment WHERE id = $1", humanLink).Scan(&humanSeq))
	code, toBot := e.assign(t, uuid.New(), decision(ids["imp-1"], "bot", nil, &humanLink, false))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "saved", toBot.Items[0].Outcome)
	orgs, _ := newOrgAccess(e.app, e.clock, nil, nil)
	old, found, err := orgs.AssignmentAt(t.Context(), e.orgID, entity.ID(ids["imp-1"]), humanSeq)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, entity.PersonHuman, old.ActorKind, "the former link holds for the former slice")
	require.NotNil(t, old.SupersededSeq)
	now, found, err := orgs.AssignmentAt(t.Context(), e.orgID, entity.ID(ids["imp-1"]), *old.SupersededSeq)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, entity.PersonBot, now.ActorKind, "the change is a new version")

	jiraBot := accounts["JIRAUSER3"].Assignment.ID
	code, rejected := e.assign(t, uuid.New(), decision(ids["JIRAUSER3"], "human", nil, &jiraBot, false))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "rejected", rejected.Items[0].Outcome)
	assert.Equal(t, "person_required", *rejected.Items[0].ErrorCode)

	versions := count(t, t.Context(), e.owner, "SELECT count(*) FROM org.account_assignment")
	code, again := e.assign(t, pair, pairBody...)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, saved.Items[0].AssignmentID, again.Items[0].AssignmentID, "a repeat returns the saved result")
	assert.Equal(t, saved.Items[1].AssignmentID, again.Items[1].AssignmentID)
	assert.Equal(t, versions, count(t, t.Context(), e.owner, "SELECT count(*) FROM org.account_assignment"), "and adds no version")
	code, _ = e.assign(t, pair, decision(ids["imp-1"], "system", nil, nil, false), decision(ids["imp-2"], "bot", nil, nil, false))
	assert.Equal(t, http.StatusUnprocessableEntity, code, "the same key with another decision")

	assert.Equal(t, versions, count(t, t.Context(), e.owner,
		"SELECT count(*) FROM audit.event WHERE operation = 'createAccountAssignments' AND outcome = 'succeeded'"),
		"every saved confirmation is in the audit")

	_, err = e.owner.Exec(t.Context(), "UPDATE org.app_user SET person_id = $1 WHERE subject = $2", anna, _adminSub)
	require.NoError(t, err)
	adminID := entity.ID(e.userID(t, e.orgID, _adminSub))
	account, found, err := orgs.UserSourceAccount(t.Context(), e.orgID, adminID, jira)
	require.NoError(t, err)
	require.True(t, found, "the user's account in Jira by the confirmed link")
	assert.Equal(t, "JIRAUSER1", account.NativeID)
	other := newBrowser()
	e.login(t, other, _otherSub)
	_, found, err = orgs.UserSourceAccount(t.Context(), e.orgID, entity.ID(e.userID(t, e.orgID, _otherSub)), jira)
	require.NoError(t, err)
	assert.False(t, found, "a user without a link has none")

	code, _ = e.sourceAccounts(t, other, "")
	assert.Equal(t, http.StatusForbidden, code, "without accounts.manage")
	assert.Equal(t, http.StatusForbidden, e.do(t, other, http.MethodGet, "/api/v1/persons", nil).Code)
}

func TestAccounts_FiltersAndPages(t *testing.T) {
	e := newConnectionsEnv(t, testadapter.Config{})
	_, imported, ids := e.accountsFixture(t)
	code, _ := e.assign(t, uuid.New(), decision(ids["JIRAUSER3"], "bot", nil, nil, false))
	require.Equal(t, http.StatusOK, code)

	_, assigned := e.sourceAccounts(t, e.admin, "&assignment=assigned")
	assert.Len(t, assigned, 1)
	_, unassigned := e.sourceAccounts(t, e.admin, "&assignment=unassigned")
	assert.Len(t, unassigned, 5)
	_, fromImport := e.sourceAccounts(t, e.admin, "&instance_id="+imported.String())
	assert.Len(t, fromImport, 3)

	seen := map[uuid.UUID]bool{}
	target := "/api/v1/source-accounts?limit=4"
	for range 3 {
		page := decode[struct {
			Data       []sourceAccountItem `json:"data"`
			NextCursor *string             `json:"next_cursor"`
		}](t, e.do(t, e.admin, http.MethodGet, target, nil))
		for _, a := range page.Data {
			assert.False(t, seen[a.ID], "a page does not repeat an account")
			seen[a.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		target = "/api/v1/source-accounts?limit=4&cursor=" + *page.NextCursor
	}
	assert.Len(t, seen, 6)
	assert.Equal(t, http.StatusBadRequest,
		e.do(t, e.admin, http.MethodGet, "/api/v1/source-accounts?limit=4&assignment=assigned&cursor="+encodeTestCursor(t, e), nil).Code,
		"a cursor of another filter")
}

// encodeTestCursor is the first cursor of the unfiltered list.
func encodeTestCursor(t *testing.T, e *connectionsEnv) string {
	t.Helper()
	page := decode[struct {
		NextCursor *string `json:"next_cursor"`
	}](t, e.do(t, e.admin, http.MethodGet, "/api/v1/source-accounts?limit=1", nil))
	require.NotNil(t, page.NextCursor)
	return *page.NextCursor
}
