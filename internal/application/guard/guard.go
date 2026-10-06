// Package guard checks references between entities before they are stored,
// so a draft cannot point at a domain, habit or objective that does not exist.
package guard

import (
	"context"

	"ascend/internal/application/ports"
	"ascend/internal/domain/shared"
)

// Domain fails when the life domain does not exist.
func Domain(ctx context.Context, p ports.Ports, id string) error {
	found, err := p.Domains.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if found == nil {
		return shared.Invalidf("Unknown domain %q", id)
	}
	return nil
}

// Habits fails when any of the habits does not exist.
func Habits(ctx context.Context, p ports.Ports, ids []string) error {
	for _, id := range ids {
		found, err := p.Habits.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if found == nil {
			return shared.Invalidf("Unknown habit %q", id)
		}
	}
	return nil
}

// Objective fails when the objective does not exist. An empty id is allowed.
func Objective(ctx context.Context, p ports.Ports, id string) error {
	if id == "" {
		return nil
	}
	found, err := p.Objectives.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if found == nil {
		return shared.Invalidf("Unknown objective %q", id)
	}
	return nil
}
