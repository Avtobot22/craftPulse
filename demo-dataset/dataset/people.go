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
	const jira, confluence, gitlab, github = "jira-cloud-1", "confluence-cloud-1", "gitlab-1", "github-1"
	team := []struct {
		roles []string
		git   []string
	}{
		{[]string{"admin", "mapping_owner"}, []string{gitlab, github}},
		{[]string{"leader", "pm"}, []string{gitlab, github}},
		{[]string{"tech_lead", "developer"}, []string{gitlab, github}},
		{[]string{"developer", "restricted_user"}, []string{gitlab}},
		{[]string{"developer"}, nil},
	}
	people := make([]Person, 0, len(team)+4)
	for i, member := range team {
		login := fmt.Sprintf("demo-person-%02d", i+1)
		person := Person{
			ID: fmt.Sprintf("org-1/person-%02d", i+1), OrganizationID: "org-1", TeamID: "team-1",
			DisplayName: fmt.Sprintf("Demo person %08x", rng.Uint32()), Roles: member.roles, DeniedAreas: []string{},
		}
		people = append(people, bindAccounts(person, login, append([]string{jira, confluence}, member.git...)...))
	}
	people[3].DeniedAreas = []string{"org-1/github-1/repo-2", "org-1/gitlab-1/restricted-repo", "org-1/confluence-cloud-1/wiki-space"}
	// Equal names must never imply equal identities.
	people[2].DisplayName = people[1].DisplayName
	people = append(people, bindAccounts(Person{
		ID: "org-2/person-01", OrganizationID: "org-2", DisplayName: "Other organization",
		Roles: []string{"developer"}, DeniedAreas: []string{"org-1/*"},
	}, "demo-other-person", jira, confluence, gitlab, github))
	people = append(people, bindAccounts(Person{
		ID: "org-1/collector", OrganizationID: "org-1", DisplayName: "Atlassian collector",
		Roles: []string{"collector"}, Bot: true, DeniedAreas: []string{"tracker-write"},
	}, "demo-collector", jira, confluence))
	for _, instance := range []string{gitlab, github} {
		people = append(people, bindAccounts(Person{
			ID: "org-1/bot-" + instance, OrganizationID: "org-1", DisplayName: "Demo review bot",
			Roles: []string{"mr_comment_bot"}, Bot: true, DeniedAreas: []string{"tracker-write"},
		}, "demo-review-bot", instance))
	}
	return people
}

func bindAccounts(person Person, login string, instances ...string) Person {
	person.Accounts = make([]Account, len(instances))
	for i, instance := range instances {
		person.Accounts[i] = Account{InstanceID: instance, Login: login, ExpectedPersonID: person.ID}
	}
	return person
}
