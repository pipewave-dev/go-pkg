package timeprovider

import (
	"time"
)

type TimeProvider interface {
	ParseTime(layout, value string) (time.Time, error)

	GetNow() time.Time

	GetLocation() *time.Location

	// DateOnly methods
	GetDateOnly(t time.Time) DateOnlyT
	GetToday() DateOnlyT
	ParseDateOnly(value string) (DateOnlyT, error)
	DateOnlyToTime(date DateOnlyT) (time.Time, error)
	AddDays(date DateOnlyT, days int) (DateOnlyT, error)
	SubDays(date DateOnlyT, days int) (DateOnlyT, error)
	DaysBetween(from, to DateOnlyT) (int, error)
	IsToday(date DateOnlyT) bool

	IsBetween(target, from, to time.Time) bool
	IsExpired(t time.Time, buffer time.Duration) bool
	//
}

type timeProvider struct {
	timeLocation *time.Location
}
type Config struct {
	TimeLocation *time.Location
}

func New(c Config) TimeProvider {
	return &timeProvider{timeLocation: c.TimeLocation}
}

func (t *timeProvider) ParseTime(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, t.timeLocation)
}

func (t *timeProvider) GetNow() time.Time {
	return time.Now().In(t.timeLocation)
}

func (t *timeProvider) GetLocation() *time.Location {
	return t.timeLocation
}

// IsBetween checks if the current time is within the [from, to] range (inclusive)
func (t *timeProvider) IsBetween(target, from, to time.Time) bool {
	return !target.Before(from) && !target.After(to)
}

// IsExpired checks if the given time has passed beyond now plus the buffer duration
func (t *timeProvider) IsExpired(tm time.Time, buffer time.Duration) bool {
	return t.GetNow().After(tm.Add(buffer))
}
