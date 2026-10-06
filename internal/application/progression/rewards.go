// Package progression holds the reward path and the profile use cases.
package progression

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ascend/internal/application/ports"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/progression"
)

// Reward is what the client needs to celebrate an action.
type Reward struct {
	XP        int                    `json:"xp"`
	Level     progression.LevelState `json:"level"`
	LeveledUp bool                   `json:"leveledUp"`
	Streak    *int                   `json:"streak,omitempty"`
}

// RewardInput describes the action being rewarded.
type RewardInput struct {
	XP       int
	DomainID string
	RefID    string
	Kind     activity.Kind
	Title    string
	Detail   string
}

// GrantReward is the single path through which progress is registered:
// grants XP, invests in the life domain, logs the activity and records
// level-ups in the journey. Call it inside a unit of work.
func GrantReward(ctx context.Context, p ports.Ports, in RewardInput) (Reward, error) {
	now := p.Clock.Now().UTC()
	current, err := p.Users.GetCurrent(ctx)
	if err != nil {
		return Reward{}, err
	}
	user, award := progression.GrantXP(current, in.XP)
	if err := p.Users.Save(ctx, user); err != nil {
		return Reward{}, err
	}

	if in.DomainID != "" && in.XP > 0 {
		if err := invest(ctx, p, in.DomainID, in.XP); err != nil {
			return Reward{}, err
		}
	}

	err = p.Activity.Append(ctx, activity.Event{
		ID:       p.IDs.Next(),
		Kind:     in.Kind,
		Title:    in.Title,
		Detail:   in.Detail,
		XP:       in.XP,
		DomainID: in.DomainID,
		RefID:    in.RefID,
		At:       now,
	})
	if err != nil {
		return Reward{}, err
	}

	for level := award.Before.Level + 1; level <= award.After.Level; level++ {
		err := p.Activity.Append(ctx, activity.Event{
			ID:    p.IDs.Next(),
			Kind:  activity.KindLevel,
			Title: fmt.Sprintf("Reached Level %d", level),
			At:    now.Add(time.Millisecond),
		})
		if err != nil {
			return Reward{}, err
		}
		milestoneID := fmt.Sprintf("level-%d", level)
		existing, err := p.Journey.GetByID(ctx, milestoneID)
		if err != nil {
			return Reward{}, err
		}
		if existing == nil {
			err := p.Journey.Save(ctx, journey.Milestone{
				ID:          milestoneID,
				Level:       level,
				Kind:        journey.KindLevel,
				Title:       fmt.Sprintf("Level %d", level),
				Description: "Unlocked by " + strings.ToLower(in.Title),
				Date:        p.Clock.Today(),
			})
			if err != nil {
				return Reward{}, err
			}
		}
	}

	return Reward{XP: in.XP, Level: award.After, LeveledUp: award.LeveledUp}, nil
}

// RevokeReward undoes a reward previously granted for `refID` (e.g.
// un-checking a habit). Call it inside a unit of work.
func RevokeReward(ctx context.Context, p ports.Ports, refID string) (progression.LevelState, error) {
	event, err := p.Activity.FindByRef(ctx, refID)
	if err != nil {
		return progression.LevelState{}, err
	}
	current, err := p.Users.GetCurrent(ctx)
	if err != nil {
		return progression.LevelState{}, err
	}
	if event == nil {
		return current.Level(), nil
	}

	if err := p.Activity.Delete(ctx, event.ID); err != nil {
		return progression.LevelState{}, err
	}
	user, _ := progression.GrantXP(current, -event.XP)
	if err := p.Users.Save(ctx, user); err != nil {
		return progression.LevelState{}, err
	}
	if event.DomainID != "" && event.XP > 0 {
		if err := invest(ctx, p, event.DomainID, -event.XP); err != nil {
			return progression.LevelState{}, err
		}
	}
	return user.Level(), nil
}

func invest(ctx context.Context, p ports.Ports, domainID string, xp int) error {
	domain, err := p.Domains.GetByID(ctx, domainID)
	if err != nil || domain == nil {
		return err
	}
	return p.Domains.Save(ctx, lifedomain.Invest(*domain, xp))
}
