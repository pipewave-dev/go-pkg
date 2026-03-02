package timeprovider

import (
	"fmt"
	"time"
)

// GetDateOnly converts time.Time to DateOnlyT using the provider's timezone
func (t *timeProvider) GetDateOnly(tm time.Time) DateOnlyT {
	return DateOnlyT(tm.In(t.timeLocation).Format(time.DateOnly))
}

// GetToday returns current date in DateOnlyT format
func (t *timeProvider) GetToday() DateOnlyT {
	return t.GetDateOnly(time.Now())
}

// ParseDateOnly parses a date string in YYYY-MM-DD format to DateOnlyT
func (t *timeProvider) ParseDateOnly(value string) (DateOnlyT, error) {
	_, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return "", fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}
	return DateOnlyT(value), nil
}

// DateOnlyToTime converts DateOnlyT to time.Time at start of day (00:00:00)
// in the provider's timezone
func (t *timeProvider) DateOnlyToTime(date DateOnlyT) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, string(date), t.timeLocation)
}

// AddDays adds specified number of days to the date
func (t *timeProvider) AddDays(date DateOnlyT, days int) (DateOnlyT, error) {
	tm, err := t.DateOnlyToTime(date)
	if err != nil {
		return "", err
	}
	newTime := tm.AddDate(0, 0, days)
	return t.GetDateOnly(newTime), nil
}

// SubDays subtracts specified number of days from the date
func (t *timeProvider) SubDays(date DateOnlyT, days int) (DateOnlyT, error) {
	return t.AddDays(date, -days)
}

// DaysBetween calculates the number of days between two dates
// Returns positive if 'to' is after 'from', negative if 'to' is before 'from'
func (t *timeProvider) DaysBetween(from, to DateOnlyT) (int, error) {
	fromTime, err := t.DateOnlyToTime(from)
	if err != nil {
		return 0, fmt.Errorf("invalid 'from' date: %w", err)
	}

	toTime, err := t.DateOnlyToTime(to)
	if err != nil {
		return 0, fmt.Errorf("invalid 'to' date: %w", err)
	}

	duration := toTime.Sub(fromTime)
	days := int(duration.Hours() / 24)

	return days, nil
}

// IsToday checks if the given date is today
func (t *timeProvider) IsToday(date DateOnlyT) bool {
	today := t.GetToday()
	return date.Equal(today)
}
