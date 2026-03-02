package timeprovider

import (
	"testing"
	"time"
)

func TestDateOnlyT_Comparisons(t *testing.T) {
	date1 := DateOnlyT("2024-01-15")
	date2 := DateOnlyT("2024-01-20")
	date3 := DateOnlyT("2024-01-15")

	t.Run("Equal", func(t *testing.T) {
		if !date1.Equal(date3) {
			t.Error("Expected date1 to equal date3")
		}
		if date1.Equal(date2) {
			t.Error("Expected date1 not to equal date2")
		}
	})

	t.Run("NotEqual", func(t *testing.T) {
		if !date1.NotEqual(date2) {
			t.Error("Expected date1 not to equal date2")
		}
		if date1.NotEqual(date3) {
			t.Error("Expected date1 to equal date3")
		}
	})

	t.Run("LT", func(t *testing.T) {
		if !date1.LT(date2) {
			t.Error("Expected date1 to be less than date2")
		}
		if date2.LT(date1) {
			t.Error("Expected date2 not to be less than date1")
		}
	})

	t.Run("LTE", func(t *testing.T) {
		if !date1.LTE(date2) {
			t.Error("Expected date1 to be less than or equal to date2")
		}
		if !date1.LTE(date3) {
			t.Error("Expected date1 to be less than or equal to date3")
		}
	})

	t.Run("GT", func(t *testing.T) {
		if !date2.GT(date1) {
			t.Error("Expected date2 to be greater than date1")
		}
		if date1.GT(date2) {
			t.Error("Expected date1 not to be greater than date2")
		}
	})

	t.Run("GTE", func(t *testing.T) {
		if !date2.GTE(date1) {
			t.Error("Expected date2 to be greater than or equal to date1")
		}
		if !date1.GTE(date3) {
			t.Error("Expected date1 to be greater than or equal to date3")
		}
	})
}

func TestDateOnlyT_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		date  DateOnlyT
		valid bool
	}{
		{"Valid date", DateOnlyT("2024-01-15"), true},
		{"Invalid format", DateOnlyT("2024/01/15"), false},
		{"Invalid date", DateOnlyT("2024-13-45"), false},
		{"Empty string", DateOnlyT(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.IsValid(); got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestDateOnlyT_ToTime(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")

	t.Run("valid date", func(t *testing.T) {
		d := DateOnlyT("2024-06-15")
		got, err := d.ToTime(loc)
		if err != nil {
			t.Fatalf("ToTime() error = %v", err)
		}
		want := time.Date(2024, 6, 15, 0, 0, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("ToTime() = %v, want %v", got, want)
		}
	})

	t.Run("invalid date", func(t *testing.T) {
		d := DateOnlyT("not-a-date")
		_, err := d.ToTime(loc)
		if err == nil {
			t.Error("Expected error for invalid date string")
		}
	})

	t.Run("UTC location", func(t *testing.T) {
		d := DateOnlyT("2024-06-15")
		got, err := d.ToTime(time.UTC)
		if err != nil {
			t.Fatalf("ToTime() error = %v", err)
		}
		want := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ToTime() = %v, want %v", got, want)
		}
	})
}

func TestDateOnlyT_String(t *testing.T) {
	tests := []struct {
		name string
		date DateOnlyT
		want string
	}{
		{"normal date", DateOnlyT("2024-01-15"), "2024-01-15"},
		{"empty string", DateOnlyT(""), ""},
		{"arbitrary string", DateOnlyT("abc"), "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTimeProvider_DateOnlyMethods(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	tp := &timeProvider{timeLocation: loc}

	t.Run("GetDateOnly", func(t *testing.T) {
		testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, loc)
		got := tp.GetDateOnly(testTime)
		want := DateOnlyT("2024-01-15")
		if got != want {
			t.Errorf("GetDateOnly() = %v, want %v", got, want)
		}
	})

	t.Run("ParseDateOnly", func(t *testing.T) {
		got, err := tp.ParseDateOnly("2024-01-15")
		if err != nil {
			t.Fatalf("ParseDateOnly() error = %v", err)
		}
		want := DateOnlyT("2024-01-15")
		if got != want {
			t.Errorf("ParseDateOnly() = %v, want %v", got, want)
		}

		_, err = tp.ParseDateOnly("invalid-date")
		if err == nil {
			t.Error("Expected error for invalid date format")
		}
	})

	t.Run("DateOnlyToTime", func(t *testing.T) {
		date := DateOnlyT("2024-01-15")
		got, err := tp.DateOnlyToTime(date)
		if err != nil {
			t.Fatalf("DateOnlyToTime() error = %v", err)
		}

		want := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
		if !got.Equal(want) {
			t.Errorf("DateOnlyToTime() = %v, want %v", got, want)
		}
	})

	t.Run("AddDays", func(t *testing.T) {
		date := DateOnlyT("2024-01-15")
		got, err := tp.AddDays(date, 5)
		if err != nil {
			t.Fatalf("AddDays() error = %v", err)
		}

		want := DateOnlyT("2024-01-20")
		if got != want {
			t.Errorf("AddDays() = %v, want %v", got, want)
		}
	})

	t.Run("SubDays", func(t *testing.T) {
		date := DateOnlyT("2024-01-15")
		got, err := tp.SubDays(date, 5)
		if err != nil {
			t.Fatalf("SubDays() error = %v", err)
		}

		want := DateOnlyT("2024-01-10")
		if got != want {
			t.Errorf("SubDays() = %v, want %v", got, want)
		}
	})

	t.Run("DaysBetween", func(t *testing.T) {
		from := DateOnlyT("2024-01-15")
		to := DateOnlyT("2024-01-20")
		got, err := tp.DaysBetween(from, to)
		if err != nil {
			t.Fatalf("DaysBetween() error = %v", err)
		}

		want := 5
		if got != want {
			t.Errorf("DaysBetween() = %v, want %v", got, want)
		}

		// Test reverse
		got, err = tp.DaysBetween(to, from)
		if err != nil {
			t.Fatalf("DaysBetween() error = %v", err)
		}

		want = -5
		if got != want {
			t.Errorf("DaysBetween() = %v, want %v", got, want)
		}
	})

	t.Run("DaysBetween same date", func(t *testing.T) {
		date := DateOnlyT("2024-06-01")
		got, err := tp.DaysBetween(date, date)
		if err != nil {
			t.Fatalf("DaysBetween() error = %v", err)
		}
		if got != 0 {
			t.Errorf("DaysBetween() = %v, want 0", got)
		}
	})

	t.Run("DaysBetween invalid from", func(t *testing.T) {
		_, err := tp.DaysBetween(DateOnlyT("bad"), DateOnlyT("2024-01-20"))
		if err == nil {
			t.Error("Expected error for invalid 'from' date")
		}
	})

	t.Run("DaysBetween invalid to", func(t *testing.T) {
		_, err := tp.DaysBetween(DateOnlyT("2024-01-15"), DateOnlyT("bad"))
		if err == nil {
			t.Error("Expected error for invalid 'to' date")
		}
	})

	t.Run("AddDays cross month", func(t *testing.T) {
		date := DateOnlyT("2024-01-30")
		got, err := tp.AddDays(date, 5)
		if err != nil {
			t.Fatalf("AddDays() error = %v", err)
		}
		want := DateOnlyT("2024-02-04")
		if got != want {
			t.Errorf("AddDays() = %v, want %v", got, want)
		}
	})

	t.Run("AddDays negative", func(t *testing.T) {
		date := DateOnlyT("2024-01-15")
		got, err := tp.AddDays(date, -5)
		if err != nil {
			t.Fatalf("AddDays() error = %v", err)
		}
		want := DateOnlyT("2024-01-10")
		if got != want {
			t.Errorf("AddDays() = %v, want %v", got, want)
		}
	})

	t.Run("AddDays invalid date", func(t *testing.T) {
		_, err := tp.AddDays(DateOnlyT("not-a-date"), 1)
		if err == nil {
			t.Error("Expected error for invalid date")
		}
	})

	t.Run("SubDays invalid date", func(t *testing.T) {
		_, err := tp.SubDays(DateOnlyT("not-a-date"), 1)
		if err == nil {
			t.Error("Expected error for invalid date")
		}
	})

	t.Run("DateOnlyToTime invalid date", func(t *testing.T) {
		_, err := tp.DateOnlyToTime(DateOnlyT("not-a-date"))
		if err == nil {
			t.Error("Expected error for invalid date")
		}
	})

	t.Run("ParseDateOnly invalid formats", func(t *testing.T) {
		invalidDates := []string{
			"2024/01/15",
			"15-01-2024",
			"01-15-2024",
			"20240115",
			"",
		}
		for _, v := range invalidDates {
			_, err := tp.ParseDateOnly(v)
			if err == nil {
				t.Errorf("ParseDateOnly(%q) expected error, got nil", v)
			}
		}
	})
}
