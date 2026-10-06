// Package application binds every use case to a concrete set of ports.
// Delivery adapters (HTTP) only ever talk to the UseCases returned here.
package application

import (
	"ascend/internal/application/calendar"
	"ascend/internal/application/domains"
	"ascend/internal/application/focus"
	"ascend/internal/application/habits"
	"ascend/internal/application/journal"
	"ascend/internal/application/journey"
	"ascend/internal/application/objectives"
	"ascend/internal/application/performance"
	"ascend/internal/application/ports"
	"ascend/internal/application/progression"
	"ascend/internal/application/today"
	"ascend/internal/application/training"
)

// UseCases groups the use cases by feature, mirroring the web client.
type UseCases struct {
	Profile     *progression.ProfileService
	Today       *today.Service
	Habits      *habits.Service
	Objectives  *objectives.Service
	Domains     *domains.Service
	Journal     *journal.Service
	Focus       *focus.Service
	Training    *training.Service
	Calendar    *calendar.Service
	Performance *performance.Service
	Journey     *journey.Service
}

// New binds every use case to `p`.
func New(p ports.Ports) *UseCases {
	return &UseCases{
		Profile:     progression.NewProfileService(p),
		Today:       today.NewService(p),
		Habits:      habits.NewService(p),
		Objectives:  objectives.NewService(p),
		Domains:     domains.NewService(p),
		Journal:     journal.NewService(p),
		Focus:       focus.NewService(p),
		Training:    training.NewService(p),
		Calendar:    calendar.NewService(p),
		Performance: performance.NewService(p),
		Journey:     journey.NewService(p),
	}
}
