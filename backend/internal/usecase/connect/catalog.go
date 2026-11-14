package connect

import (
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _catalog describes the built-in adapters for the connection form; listAdapters shows the ones the
// installation runs. A built-in adapter adds its entry together with its code.
var _catalog = []entity.AdapterInfo{
	{
		Provider: entity.ProviderJiraCloud, DisplayName: "Jira Cloud", Editions: []string{"cloud"},
		DataKinds:  []string{"tasks", "accounts", "permissions"},
		ScopeKinds: []entity.ScopeKind{entity.ScopeTrackerProject},
		CredentialFields: []entity.CredentialField{
			{Name: "email", Label: "Email служебной учётной записи Atlassian", Required: true},
			{Name: "token", Label: "API token служебной учётной записи Atlassian", Required: true, Secret: true},
		},
	},
	{
		Provider: entity.ProviderGitLab, DisplayName: "GitLab self-managed", Editions: []string{"ce", "ee"},
		DataKinds:  []string{"merge_requests", "ci", "reports", "accounts", "permissions"},
		ScopeKinds: []entity.ScopeKind{entity.ScopeRepository},
		CredentialFields: []entity.CredentialField{
			{Name: "read_token", Label: "Токен чтения API и репозитория", Required: true, Secret: true},
			{Name: "write_token", Label: "Токен бота для комментария MR", Secret: true},
			{Name: "webhook_secret", Label: "Секрет подписи webhook", Secret: true},
		},
	},
	{
		// Areas are repositories by their normalized remote; https takes a token, ssh a key with host keys.
		Provider: entity.ProviderGit, DisplayName: "Git по SSH или HTTPS",
		DataKinds:  []string{"documents"},
		ScopeKinds: []entity.ScopeKind{entity.ScopeRepository},
		CredentialFields: []entity.CredentialField{
			{Name: "token", Label: "Токен чтения репозитория для HTTPS", Secret: true},
			{Name: "username", Label: "Имя пользователя для токена HTTPS"},
			{Name: "private_key", Label: "Закрытый ключ SSH только на чтение", Secret: true},
			{Name: "known_hosts", Label: "Ключи хоста SSH в формате known_hosts"},
			{Name: "ssh_user", Label: "Пользователь SSH, по умолчанию git"},
		},
	},
}

// _scopeDataKinds are the data kinds of an adapter of a client, which has no catalog entry.
var _scopeDataKinds = map[entity.ScopeKind][]string{
	entity.ScopeTrackerProject:  {"tasks", "accounts"},
	entity.ScopeRepository:      {"merge_requests", "ci", "reports", "accounts"},
	entity.ScopeWikiSpace:       {"documents"},
	entity.ScopeCIProject:       {"ci"},
	entity.ScopeAnalysisProject: {"reports"},
}

// Catalog lists the catalog entries without a built-in adapter in the installation as well.
func Catalog() []entity.AdapterInfo { return _catalog }

func catalogEntry(p entity.Provider) (entity.AdapterInfo, bool) {
	i := slices.IndexFunc(_catalog, func(a entity.AdapterInfo) bool { return a.Provider == p })
	if i < 0 {
		return entity.AdapterInfo{}, false
	}
	return _catalog[i], true
}

// ListAdapters returns the built-in adapters the installation runs (FR-014, UI-10).
func (u *UseCase) ListAdapters() []entity.AdapterInfo {
	out := make([]entity.AdapterInfo, 0, len(_catalog))
	for i := range _catalog {
		a := _catalog[i]
		if u.d.Builtin(a.Provider) {
			a.ContractVersions = ContractVersions
			out = append(out, a)
		}
	}
	return out
}

// dataKinds are what the first load collects: the kinds of the built-in adapter, or for an adapter of a
// client the kinds its scopes hold.
func dataKinds(c entity.Connection, scopes []entity.ConnectionScope) []string {
	if a, ok := catalogEntry(c.Provider); ok && c.AdapterKind == entity.AdapterBuiltin {
		return a.DataKinds
	}
	var out []string
	for i := range scopes {
		s := &scopes[i]
		if !s.Active {
			continue
		}
		for _, k := range _scopeDataKinds[s.Kind] {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}
