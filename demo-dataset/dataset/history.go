package dataset

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// history writes the events of one task in order, keeping the current value of every field.
type history struct {
	g        *taskGen
	task     Task
	current  map[string]string
	reporter Person
	author   Person
	// late is the delivery delay of the next events; zero means they are observed when they happen.
	late time.Duration
}

func (g *taskGen) newHistory(p TaskProject, key, summary string) *history {
	people := g.people[p.OrganizationID]
	return &history{
		g: g,
		task: Task{
			ID: p.OrganizationID + "/" + p.InstanceID + "/" + key, OrganizationID: p.OrganizationID,
			InstanceID: p.InstanceID, ProjectID: p.ID, Key: key, Summary: summary, Events: []TaskEvent{},
		},
		current:  map[string]string{},
		reporter: people[g.rng.IntN(len(people))],
		author:   people[g.rng.IntN(len(people))],
	}
}

// withOffset returns a person of the organization who works in the given UTC offset.
func (g *taskGen) withOffset(org, offset string) Person {
	for _, person := range g.people[org] {
		if offsetOf(person) == offset {
			return person
		}
	}
	return g.people[org][0]
}

func (h *history) create(at time.Time, f TaskFields) {
	initial := f
	h.emit(at, h.reporter, EventCreated, "", "", "", &initial)
	h.current[FieldStatus], h.current[FieldType], h.current[FieldSize], h.current[FieldTeam] = f.Status, f.Type, f.Size, f.Team
	h.track(at, f.Status)
}

func (h *history) set(at time.Time, field, value string) {
	h.emit(at, h.author, EventFieldChange, field, h.current[field], value, nil)
	h.current[field] = value
}

func (h *history) move(at time.Time, status string) {
	h.set(at, FieldStatus, status)
	h.track(at, status)
}

// finish moves the task into a terminal status and sets its resolution in the same moment.
func (h *history) finish(at time.Time, status, resolution string) {
	h.move(at, status)
	h.set(at, FieldResolution, resolution)
}

func (h *history) reopen(at time.Time) {
	h.move(at, StatusInProgress)
	h.set(at, FieldResolution, "")
}

// loops adds n round trips In Review -> In Progress spread evenly inside (from, to).
func (h *history) loops(from, to time.Time, n int) {
	step := to.Sub(from) / time.Duration(2*n+1)
	for i := 1; i <= 2*n; i++ {
		status := StatusInReview
		if i%2 == 0 {
			status = StatusInProgress
		}
		h.move(from.Add(step*time.Duration(i)).Truncate(time.Second), status)
	}
}

func (h *history) done() Task {
	return h.task
}

func (h *history) track(at time.Time, status string) {
	id := h.task.ID
	if _, ok := h.g.started[id]; !ok && (status == StatusInProgress || status == StatusInReview) {
		h.g.started[id] = at
	}
	if _, ok := h.g.completed[id]; !ok && status == StatusDone {
		h.g.completed[id] = at
	}
}

func (h *history) emit(at time.Time, who Person, kind, field, from, to string, initial *TaskFields) {
	offset := offsetOf(who)
	event := TaskEvent{
		ID:        fmt.Sprintf("%s/e%02d", h.task.ID, len(h.task.Events)+1),
		Kind:      kind,
		EventTime: SourceTime{UTC: at.UTC().Format(time.RFC3339), OriginalOffset: offset},
		AuthorID:  who.ID,
		Field:     field, From: from, To: to,
		Initial: initial,
	}
	if h.late > 0 {
		event.ObservedTime = &SourceTime{UTC: at.Add(h.late).UTC().Format(time.RFC3339), OriginalOffset: offset}
	}
	h.task.Events = append(h.task.Events, event)
}

// offsetOf gives every person a stable UTC offset: most of the team works in +03:00.
func offsetOf(p Person) string {
	if p.OrganizationID != "org-1" {
		return "+02:00"
	}
	n, err := strconv.Atoi(strings.TrimPrefix(p.ID, "org-1/person-"))
	switch {
	case err != nil:
		return "Z"
	case n%7 == 0:
		return "Z"
	case n%5 == 0:
		return "+05:00"
	}
	return "+03:00"
}
