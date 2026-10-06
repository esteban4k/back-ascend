package progression

import (
	"context"
	"strings"

	"ascend/internal/application/ports"
	"ascend/internal/domain/activity"
	"ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
)

// Profile is the user and where they sit on the level curve.
type Profile struct {
	User  progression.User       `json:"user"`
	Level progression.LevelState `json:"level"`
}

// ProfileService exposes the profile use cases.
type ProfileService struct{ p ports.Ports }

// NewProfileService binds the profile use cases to ports.
func NewProfileService(p ports.Ports) *ProfileService { return &ProfileService{p: p} }

// Get returns the current user and level.
func (s *ProfileService) Get(ctx context.Context) (Profile, error) {
	user, err := s.p.Users.GetCurrent(ctx)
	if err != nil {
		return Profile{}, err
	}
	return Profile{User: user, Level: user.Level()}, nil
}

// Rename changes the user's display name.
func (s *ProfileService) Rename(ctx context.Context, name string) (Profile, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Profile{}, shared.Invalid("Name cannot be empty")
	}
	if len([]rune(trimmed)) > 40 {
		return Profile{}, shared.Invalid("Keep the name under 40 characters")
	}
	return ports.InTx(ctx, s.p.Tx, func(ctx context.Context) (Profile, error) {
		user, err := s.p.Users.GetCurrent(ctx)
		if err != nil {
			return Profile{}, err
		}
		user.Name = trimmed
		if err := s.p.Users.Save(ctx, user); err != nil {
			return Profile{}, err
		}
		return Profile{User: user, Level: user.Level()}, nil
	})
}

// DefaultActivityLimit is used when the caller does not ask for a size.
const DefaultActivityLimit = 12

// Activity lists the most recent events, newest first.
func (s *ProfileService) Activity(ctx context.Context, limit int) ([]activity.Event, error) {
	if limit <= 0 {
		limit = DefaultActivityLimit
	}
	return s.p.Activity.List(ctx, ports.ActivityQuery{Limit: min(limit, 500)})
}
