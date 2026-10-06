// Package journal holds the journal use cases.
package journal

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"ascend/internal/application/habits"
	"ascend/internal/application/ports"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
)

// Created is a saved entry plus the reward of the habit it completed, if any.
type Created struct {
	Entry  journal.Entry           `json:"entry"`
	Reward *habits.TriggeredReward `json:"reward"`
}

// Service exposes the journal use cases.
type Service struct{ p ports.Ports }

// NewService binds the journal use cases to ports.
func NewService(p ports.Ports) *Service { return &Service{p: p} }

// List returns entries, newest first.
func (s *Service) List(ctx context.Context) ([]journal.Entry, error) {
	entries, err := s.p.Journal.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(entries, func(a, b journal.Entry) int {
		return cmp.Or(strings.Compare(string(b.Date), string(a.Date)), b.CreatedAt.Compare(a.CreatedAt))
	})
	return entries, nil
}

// Get returns one entry.
func (s *Service) Get(ctx context.Context, id string) (journal.Entry, error) {
	return ports.Require(ctx, s.p.Journal, "Journal entry", id)
}

// Create saves the entry and completes the habit wired to journaling, if any.
func (s *Service) Create(ctx context.Context, d journal.Draft) (Created, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (Created, error) {
		now := s.p.Clock.Now().UTC()
		entry, err := journal.New(s.p.IDs.Next(), d, now)
		if err != nil {
			return Created{}, err
		}
		if err := s.p.Journal.Save(ctx, entry); err != nil {
			return Created{}, err
		}
		err = s.p.Activity.Append(ctx, activity.Event{
			ID:     s.p.IDs.Next(),
			Kind:   activity.KindJournal,
			Title:  "Journal entry created",
			Detail: entry.Title,
			At:     now,
		})
		if err != nil {
			return Created{}, err
		}
		reward, err := habits.CompleteTriggered(ctx, s.p, habit.TriggerJournal)
		if err != nil {
			return Created{}, err
		}
		return Created{Entry: entry, Reward: reward}, nil
	})
}

// Update edits an entry.
func (s *Service) Update(ctx context.Context, id string, d journal.Draft) (journal.Entry, error) {
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (journal.Entry, error) {
		current, err := ports.Require(ctx, s.p.Journal, "Journal entry", id)
		if err != nil {
			return journal.Entry{}, err
		}
		updated, err := journal.Update(current, d)
		if err != nil {
			return journal.Entry{}, err
		}
		return updated, s.p.Journal.Save(ctx, updated)
	})
}

// Delete removes an entry.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.p.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := ports.Require(ctx, s.p.Journal, "Journal entry", id); err != nil {
			return err
		}
		return s.p.Journal.Delete(ctx, id)
	})
}
