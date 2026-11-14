package process

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_maxTimezone = 64
	_maxNative   = 255
	_maxMeaning  = 200
	_maxRank     = 1<<15 - 1
)

var (
	_statusMeanings = []string{
		entity.MeaningIntake, entity.MeaningActive, entity.MeaningWait, entity.MeaningBlocked, entity.MeaningDone,
		entity.MeaningCancelled, entity.MeaningDuplicate, entity.MeaningIgnored,
	}
	_resolutionMeanings = []string{entity.MeaningAccepted, entity.MeaningCancelled, entity.MeaningDuplicate, entity.MeaningIgnored}
)

// ruleSpec is what a kind of rule allows on M0: the meanings it takes (any when nil), whether the meaning
// must be filled and whether the rule names a value of a field.
type ruleSpec struct {
	meanings []string
	label    bool
	team     bool
	valued   bool
}

var _ruleSpecs = map[entity.RuleKind]ruleSpec{
	entity.RuleStatus:           {meanings: _statusMeanings},
	entity.RuleResolution:       {meanings: _resolutionMeanings},
	entity.RuleIssueType:        {label: true},
	entity.RuleCodeRequiredType: {meanings: []string{entity.MeaningCodeRequired}},
	entity.RuleSizeField:        {},
	entity.RuleSizeValue:        {label: true, valued: true},
	entity.RuleTeamField:        {},
	entity.RuleTeamValue:        {team: true, valued: true},
	entity.RuleBlockedFlag:      {meanings: []string{entity.MeaningBlocked}, valued: true},
}

// draftCheck is a draft that passed the checks, with its rules in the order of their key.
type draftCheck struct {
	draft      entity.MappingDraft
	calendarID *entity.ID
	hash       []byte
}

// CheckDraft checks a draft of the mapping of a team without saving it; the preview of calc uses it before it
// applies the rules (MVP-04-T04). Errors of fields are *entity.ValidationError.
func (u *UseCase) CheckDraft(ctx context.Context, orgID, teamID entity.ID, d entity.MappingDraft) error {
	_, err := u.validateDraft(ctx, orgID, teamID, d)
	return err
}

func (u *UseCase) validateDraft(ctx context.Context, orgID, teamID entity.ID, d entity.MappingDraft) (draftCheck, error) {
	instances, err := u.instances(ctx, orgID, teamID)
	if err != nil {
		return draftCheck{}, err
	}
	verr := &entity.ValidationError{}
	checkTimezone(verr, d.Timezone)
	if d.CalendarVersionNo != nil && *d.CalendarVersionNo < 1 {
		verr.Add("/calendar_version_no", entity.FieldOutOfRange, "version numbers start at 1")
	}
	checkRules(verr, d.Rules, instances)
	out := draftCheck{draft: d}
	var zoneKnown, calendarKnown bool
	err = u.d.Tx.InOrgTx(ctx, orgID, func(ctx context.Context) error {
		var err error
		if zoneKnown, err = u.timezoneKnown(ctx, d.Timezone); err != nil {
			return err
		}
		out.calendarID, calendarKnown = nil, true
		if d.CalendarVersionNo == nil || *d.CalendarVersionNo < 1 {
			return nil
		}
		cal, err := u.d.Repo.CalendarVersionByNo(ctx, orgID, teamID, *d.CalendarVersionNo)
		if isNotFound(err) {
			calendarKnown = false
			return nil
		}
		if err != nil {
			return err
		}
		out.calendarID = &cal.ID
		return nil
	})
	if err != nil {
		return draftCheck{}, err
	}
	if !zoneKnown {
		verr.Add("/timezone", entity.FieldNotAllowedValue, "not an IANA time zone")
	}
	if !calendarKnown {
		verr.Add("/calendar_version_no", entity.FieldUnknownReference, "the team has no calendar version with this number")
	}
	if len(verr.Fields) > 0 {
		return draftCheck{}, verr
	}
	out.draft.Rules = slices.SortedFunc(slices.Values(d.Rules), compareRules)
	out.hash, err = contentHash(out.draft)
	return out, err
}

// instances are the source instances of the areas of the projects of the team now.
func (u *UseCase) instances(ctx context.Context, orgID, teamID entity.ID) (map[entity.ID]bool, error) {
	areas, err := u.d.Teams.TeamAreas(ctx, orgID, teamID, u.d.Clock.Now())
	if err != nil {
		return nil, err
	}
	dicts, err := u.d.Dictionaries.Dictionaries(ctx, orgID, areas)
	if err != nil {
		return nil, err
	}
	out := make(map[entity.ID]bool, len(dicts))
	for _, d := range dicts {
		out[d.InstanceID] = true
	}
	return out, nil
}

// timezoneKnown checks a well-formed name against the zones of the database (TZ 12.5.6); a malformed one is
// reported by checkTimezone and counts as known here.
func (u *UseCase) timezoneKnown(ctx context.Context, name string) (bool, error) {
	if name == "" || len(name) > _maxTimezone {
		return true, nil
	}
	return u.d.Repo.TimezoneExists(ctx, name)
}

func checkTimezone(verr *entity.ValidationError, name string) {
	switch {
	case name == "":
		verr.Add("/timezone", entity.FieldRequired, "")
	case len(name) > _maxTimezone:
		verr.Add("/timezone", entity.FieldTooLong, "")
	}
}

func checkRules(verr *entity.ValidationError, rules []entity.MappingRule, instances map[entity.ID]bool) {
	if len(rules) > entity.MaxMappingRules {
		verr.Add("/rules", entity.FieldOutOfRange, "at most "+strconv.Itoa(entity.MaxMappingRules)+" rules")
		return
	}
	type key struct {
		kind       entity.RuleKind
		instance   entity.ID
		ref, value string
	}
	seen := make(map[key]bool, len(rules))
	for i := range rules {
		r := &rules[i]
		at := "/rules/" + strconv.Itoa(i)
		k := key{r.Kind, r.InstanceID, r.NativeRef, r.NativeValue}
		if seen[k] {
			verr.Add(at, entity.FieldDuplicate, "a rule with this kind, instance, reference and value is already in the draft")
		}
		seen[k] = true
		if !instances[r.InstanceID] {
			verr.Add(at+"/instance_id", entity.FieldOutsideScope, "the instance is not a source of the projects of the team")
		}
		checkNative(verr, at, r)
		spec, ok := _ruleSpecs[r.Kind]
		if !ok {
			verr.Add(at+"/rule_kind", entity.FieldNotAllowedValue, "")
			continue
		}
		checkValue(verr, at, r, spec)
		checkMeaning(verr, at, r, spec)
		checkRank(verr, at, r)
	}
}

func checkNative(verr *entity.ValidationError, at string, r *entity.MappingRule) {
	switch {
	case r.NativeRef == "":
		verr.Add(at+"/native_ref", entity.FieldRequired, "")
	case len(r.NativeRef) > _maxNative:
		verr.Add(at+"/native_ref", entity.FieldTooLong, "")
	}
	if len(r.NativeValue) > _maxNative {
		verr.Add(at+"/native_value", entity.FieldTooLong, "")
	}
}

// checkValue requires the value of a field for valued rules and an empty string for the others.
func checkValue(verr *entity.ValidationError, at string, r *entity.MappingRule, spec ruleSpec) {
	switch {
	case spec.valued && r.NativeValue == "":
		verr.Add(at+"/native_value", entity.FieldRequired, "the rule names a value of the field")
	case !spec.valued && r.NativeValue != "":
		verr.Add(at+"/native_value", entity.FieldNotAllowedValue, "the rule has no value, send an empty string")
	}
}

func checkMeaning(verr *entity.ValidationError, at string, r *entity.MappingRule, spec ruleSpec) {
	field := at + "/meaning"
	switch {
	case utf8.RuneCountInString(r.Meaning) > _maxMeaning:
		verr.Add(field, entity.FieldTooLong, "")
	case spec.meanings != nil && !slices.Contains(spec.meanings, r.Meaning):
		verr.Add(field, entity.FieldNotAllowedValue, "allowed: "+strings.Join(spec.meanings, ", "))
	case (spec.label || spec.team) && strings.TrimSpace(r.Meaning) == "":
		verr.Add(field, entity.FieldRequired, "")
	case spec.team:
		var id entity.ID
		if id.UnmarshalText([]byte(r.Meaning)) != nil {
			verr.Add(field, entity.FieldInvalidFormat, "the meaning of team_value is the ID of a team")
		}
	}
}

func checkRank(verr *entity.ValidationError, at string, r *entity.MappingRule) {
	switch {
	case r.Rank == nil && r.Kind == entity.RuleSizeValue:
		verr.Add(at+"/rank", entity.FieldRequired, "a size value has its rank")
	case r.Rank != nil && (*r.Rank < 0 || *r.Rank > _maxRank):
		verr.Add(at+"/rank", entity.FieldOutOfRange, "")
	}
}

// compareRules orders rules by their key, as the primary key of process.mapping_rule does.
func compareRules(a, b entity.MappingRule) int {
	return cmp.Or(
		strings.Compare(string(a.Kind), string(b.Kind)), bytes.Compare(a.InstanceID[:], b.InstanceID[:]),
		strings.Compare(a.NativeRef, b.NativeRef), strings.Compare(a.NativeValue, b.NativeValue),
	)
}

// contentHash is SHA-256 of the ordered rules, the time zone and the calendar version (TZ 12.5.6).
func contentHash(d entity.MappingDraft) ([]byte, error) {
	type rule struct {
		Kind        entity.RuleKind `json:"rule_kind"`
		InstanceID  entity.ID       `json:"instance_id"`
		NativeRef   string          `json:"native_ref"`
		NativeValue string          `json:"native_value"`
		Meaning     string          `json:"meaning"`
		Rank        *int            `json:"rank"`
	}
	content := struct {
		Timezone          string `json:"timezone"`
		CalendarVersionNo *int   `json:"calendar_version_no"`
		Rules             []rule `json:"rules"`
	}{Timezone: d.Timezone, CalendarVersionNo: d.CalendarVersionNo, Rules: make([]rule, len(d.Rules))}
	for i, r := range d.Rules {
		content.Rules[i] = rule{r.Kind, r.InstanceID, r.NativeRef, r.NativeValue, r.Meaning, r.Rank}
	}
	b, err := json.Marshal(content)
	if err != nil {
		return nil, errorf("encode mapping content: %w", err)
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}
