package process

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCheckTargetDraft(t *testing.T) {
	revision := entity.ID{1}
	from := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	period := &entity.Window{From: from, To: from.AddDate(0, 0, 35)}
	group := entity.TargetGroup{TaskType: "Story", SizeLabel: "S"}
	manual := entity.TargetDraft{TargetGroup: group, DurationSeconds: 172800, Basis: entity.TargetManual}
	p85 := entity.TargetDraft{TargetGroup: group, DurationSeconds: 259200, Basis: entity.TargetHistoricalP85, BasisRevisionID: &revision, BasisPeriod: period}
	with := func(d entity.TargetDraft, change func(d *entity.TargetDraft)) entity.TargetDraft {
		change(&d)
		return d
	}

	tests := []struct {
		name      string
		give      entity.TargetDraft
		wantField string
		wantCode  string
	}{
		{"manual", manual, "", ""},
		{"historical p85", p85, "", ""},
		{"zero duration", with(manual, func(d *entity.TargetDraft) { d.DurationSeconds = 0 }), "/duration_seconds", entity.FieldOutOfRange},
		{"no size", with(manual, func(d *entity.TargetDraft) { d.SizeLabel = " " }), "/size_label", entity.FieldRequired},
		{"no type", with(manual, func(d *entity.TargetDraft) { d.TaskType = "" }), "/task_type", entity.FieldRequired},
		{"long size", with(manual, func(d *entity.TargetDraft) { d.SizeLabel = strings.Repeat("я", 201) }), "/size_label", entity.FieldTooLong},
		{"unknown basis", with(manual, func(d *entity.TargetDraft) { d.Basis = "median" }), "/basis", entity.FieldNotAllowedValue},
		{"p85 without revision", with(p85, func(d *entity.TargetDraft) { d.BasisRevisionID = nil }), "/basis_revision_id", entity.FieldRequired},
		{"p85 without period", with(p85, func(d *entity.TargetDraft) { d.BasisPeriod = nil }), "/basis_period", entity.FieldRequired},
		{"empty period", with(p85, func(d *entity.TargetDraft) { d.BasisPeriod = &entity.Window{From: from, To: from} }), "/basis_period", entity.FieldOutOfRange},
		{"manual with revision", with(manual, func(d *entity.TargetDraft) { d.BasisRevisionID = &revision }), "/basis_revision_id", entity.FieldNotAllowedValue},
		{"manual with period", with(manual, func(d *entity.TargetDraft) { d.BasisPeriod = period }), "/basis_period", entity.FieldNotAllowedValue},
	}
	u := New(Deps{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := u.CheckTargetDraft(tt.give)
			if tt.wantField == "" {
				assert.NoError(t, err)
				return
			}
			var verr *entity.ValidationError
			require.True(t, errors.As(err, &verr), "%v", err)
			require.Len(t, verr.Fields, 1)
			assert.Equal(t, entity.FieldError{Field: tt.wantField, Code: tt.wantCode, Detail: verr.Fields[0].Detail}, verr.Fields[0])
		})
	}
}
