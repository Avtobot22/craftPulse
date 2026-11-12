package dataset

import (
	"fmt"
	"math/rand/v2"
)

type Account struct {
	InstanceID       string `json:"instance_id"`
	Login            string `json:"login"`
	ExpectedPersonID string `json:"expected_person_id"`
}

type Person struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	TeamID         string    `json:"team_id"`
	DisplayName    string    `json:"display_name"`
	Roles          []string  `json:"roles"`
	Bot            bool      `json:"bot"`
	DeniedAreas    []string  `json:"denied_areas"`
	Accounts       []Account `json:"accounts"`
}

func generatePeople(rng *rand.Rand) []Person {
	people := make([]Person, 0, 23)
	roles := [][]string{{"admin", "mapping_owner"}, {"leader", "pm"}, {"tech_lead", "developer"}, {"developer", "restricted_user"}}
	for i := 1; i <= 20; i++ {
		person := Person{ID: fmt.Sprintf("org-1/person-%02d", i), OrganizationID: "org-1", TeamID: "team-1", DisplayName: fmt.Sprintf("Demo person %08x", rng.Uint32()), Roles: []string{"developer"}, DeniedAreas: []string{}, Accounts: []Account{}}
		if i <= len(roles) {
			person.Roles = roles[i-1]
		}
		if i == 4 {
			person.DeniedAreas = []string{"org-1/github-1/repo-2", "org-1/confluence-dc-1/wiki-space"}
		}
		for _, instance := range []string{"jira-dc-1", "confluence-dc-1", "gitlab-1", "github-1"} {
			person.Accounts = append(person.Accounts, Account{InstanceID: instance, Login: fmt.Sprintf("demo-person-%02d", i), ExpectedPersonID: person.ID})
		}
		people = append(people, person)
	}
	// Equal names must never imply equal identities.
	people[19].DisplayName = people[18].DisplayName
	people = append(people, Person{ID: "org-2/person-01", OrganizationID: "org-2", TeamID: "", DisplayName: "Other organization", Roles: []string{"developer"}, DeniedAreas: []string{"org-1/*"}, Accounts: []Account{}})
	for _, instance := range []string{"jira-dc-1", "confluence-dc-1", "gitlab-1", "github-1"} {
		people[20].Accounts = append(people[20].Accounts, Account{InstanceID: instance, Login: "demo-other-person", ExpectedPersonID: people[20].ID})
	}
	for _, instance := range []string{"gitlab-1", "github-1"} {
		id := "org-1/bot-" + instance
		people = append(people, Person{ID: id, OrganizationID: "org-1", TeamID: "", DisplayName: "Demo review bot", Roles: []string{"mr_comment_bot"}, Bot: true, DeniedAreas: []string{"tracker-write"}, Accounts: []Account{{InstanceID: instance, Login: "demo-review-bot", ExpectedPersonID: id}}})
	}
	return people
}
