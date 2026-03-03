package timeprovider

import (
	"time"
)

// DateOnlyT represents a date in YYYY-MM-DD format (time.DateOnly)
type DateOnlyT string

// Comparison methods

// Equal checks if two dates are equal
func (d DateOnlyT) Equal(other DateOnlyT) bool {
	return d == other
}

// NotEqual checks if two dates are not equal
func (d DateOnlyT) NotEqual(other DateOnlyT) bool {
	return d != other
}

// LT checks if date is less than (before) other date
func (d DateOnlyT) LT(other DateOnlyT) bool {
	return string(d) < string(other)
}

// LTE checks if date is less than or equal to other date
func (d DateOnlyT) LTE(other DateOnlyT) bool {
	return string(d) <= string(other)
}

// GT checks if date is greater than (after) other date
func (d DateOnlyT) GT(other DateOnlyT) bool {
	return string(d) > string(other)
}

// GTE checks if date is greater than or equal to other date
func (d DateOnlyT) GTE(other DateOnlyT) bool {
	return string(d) >= string(other)
}

// ToTime converts DateOnlyT to time.Time at start of day in given location
func (d DateOnlyT) ToTime(loc *time.Location) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, string(d), loc)
}

// String returns the string representation
func (d DateOnlyT) String() string {
	return string(d)
}

// IsValid checks if the date string is valid
func (d DateOnlyT) IsValid() bool {
	_, err := time.Parse(time.DateOnly, string(d))
	return err == nil
}
