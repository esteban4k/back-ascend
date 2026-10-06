// Package system provides the real clock and id generator.
package system

import (
	"crypto/rand"
	"fmt"
	"time"

	"ascend/internal/domain/shared"
)

// Clock reads the wall clock in a fixed location (the user's timezone).
type Clock struct {
	loc *time.Location
}

// NewClock returns a clock for `loc` (time.Local when nil).
func NewClock(loc *time.Location) *Clock {
	if loc == nil {
		loc = time.Local
	}
	return &Clock{loc: loc}
}

// Now returns the current instant, truncated to milliseconds like the web client.
func (c *Clock) Now() time.Time { return time.Now().In(c.loc).Truncate(time.Millisecond) }

// Today returns the current date in the clock's location.
func (c *Clock) Today() shared.LocalDate { return shared.DateOf(c.Now()) }

// Location returns the clock's location.
func (c *Clock) Location() *time.Location { return c.loc }

// FixedClock always returns the same instant. Useful for tests and demos.
type FixedClock struct {
	At time.Time
}

// Now returns the fixed instant.
func (c FixedClock) Now() time.Time { return c.At }

// Today returns the fixed instant's date.
func (c FixedClock) Today() shared.LocalDate { return shared.DateOf(c.At) }

// Location returns the fixed instant's location.
func (c FixedClock) Location() *time.Location { return c.At.Location() }

// UUIDGenerator creates random RFC 4122 version 4 UUIDs.
type UUIDGenerator struct{}

// Next returns a new UUID.
func (UUIDGenerator) Next() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
