package gitlab

import (
	"cmp"
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func (u account) nativeID() *string {
	if u.ID < 1 {
		return nil
	}
	return optional(id(u.ID))
}

func (u account) record(page *adaptersdk.Page, observed time.Time) error {
	if u.ID < 1 || u.Username == "" && u.Name == "" {
		return nil
	}
	kind := envelope.AccountV1AccountKindHuman
	switch {
	case u.ServiceAccount, u.UserType == "service_account":
		kind = envelope.AccountV1AccountKindSystem
	case u.Bot, u.UserType == "project_bot", tokenBot(u.Username):
		kind = envelope.AccountV1AccountKindBot
	case u.UserType != "" && u.UserType != "human":
		kind = envelope.AccountV1AccountKindUnknown
	}
	return addRecord(page, "account", "account/1", "object", "", id(u.ID), nil, nil, observed, envelope.AccountV1{
		Login: optional(u.Username), DisplayName: optional(u.Name), Email: optional(cmp.Or(u.Email, u.PublicEmail)),
		AccountKind: kind, Active: u.State == "active",
	})
}

func tokenBot(login string) bool {
	parts := strings.SplitN(login, "_", 4)
	if len(parts) != 4 || (parts[0] != "project" && parts[0] != "group") || parts[2] != "bot" {
		return false
	}
	_, ok := numericID(parts[1])
	return ok && parts[3] != ""
}

func (s source) fetchAccounts(ctx context.Context, projectID int64, in adaptersdk.FetchInput, m *envelope.Manifest, page *adaptersdk.Page) error {
	if len(in.ObjectKeys) != 0 {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotSupported, "GitLab accounts are read within the project")
	}
	current, size := 1, min(in.Limit, _sourcePageSize)
	if in.Cursor != nil && in.Cursor.NativeId != "" {
		rawPage, rawSize, ok := strings.Cut(in.Cursor.NativeId, "/")
		var err error
		current, err = strconv.Atoi(rawPage)
		if !ok || err != nil || current < 2 || rawSize != strconv.Itoa(size) {
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidRequest, "invalid GitLab accounts cursor")
		}
	}
	next, err := readPage(ctx, s, "members", projectPath(projectID)+"/members/all", url.Values{}, current, size, func(user account) error {
		if user.ID <= 0 {
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab project member has no ID")
		}
		var profile account
		err := s.get(ctx, "user", "users/"+id(user.ID), &profile)
		if hasCode(err, adaptersdk.AdapterErrorCodeNotFound) {
			return user.record(page, m.CreatedAt)
		}
		if err != nil {
			return err
		}
		if profile.ID != user.ID {
			return adaptersdk.NewError(adaptersdk.AdapterErrorCodeInvalidResponse, "GitLab returned a different user profile")
		}
		profile.Username = cmp.Or(profile.Username, user.Username)
		profile.Name = cmp.Or(profile.Name, user.Name)
		profile.State = cmp.Or(profile.State, user.State)
		profile.UserType = cmp.Or(profile.UserType, user.UserType)
		profile.Email = cmp.Or(profile.Email, user.Email)
		profile.PublicEmail = cmp.Or(profile.PublicEmail, user.PublicEmail)
		profile.Bot = profile.Bot || user.Bot
		profile.ServiceAccount = profile.ServiceAccount || user.ServiceAccount
		return profile.record(page, m.CreatedAt)
	})
	if err != nil {
		return err
	}
	if next != 0 {
		m.Selection.End = false
		m.Selection.NextCursor = &envelope.Cursor{ModifiedAt: m.CreatedAt, NativeId: strconv.Itoa(next) + "/" + strconv.Itoa(size)}
	}
	return nil
}
