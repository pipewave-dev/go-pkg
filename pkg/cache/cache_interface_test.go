package cache

import (
	"testing"
)

func TestConvertNumberToStr(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"int", 123, "123"},
		{"int64", int64(1234567890), "1234567890"},
		{"int8", int8(120), "120"},
		{"int16", int16(32000), "32000"},
		{"int32", int32(2147483647), "2147483647"},
		{"uint", uint(123), "123"},
		{"uint8", uint8(250), "250"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(123.456), "123.456"},
		{"float64", 789.1234, "789.1234"},
		{"nil input", nil, ""},
		{"unsupported type", "string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertNumberToStr(tt.input)
			if result != tt.expected {
				t.Errorf("convertNumberToStr(%v) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}
func TestConvertStrToNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
		typ      any
		wantErr  bool
	}{
		{"valid int", "123", int(123), int(0), false},
		{"valid int64", "1234567890", int64(1234567890), int64(0), false},
		{"valid int8", "120", int8(120), int8(0), false},
		{"valid int16", "32000", int16(32000), int16(0), false},
		{"valid int32", "2147483647", int32(2147483647), int32(0), false},
		{"valid uint", "123", uint(123), uint(0), false},
		{"valid uint8", "250", uint8(250), uint8(0), false},
		{"valid uint16", "65535", uint16(65535), uint16(0), false},
		{"valid uint32", "4294967295", uint32(4294967295), uint32(0), false},
		{"valid uint64", "18446744073709551615", uint64(18446744073709551615), uint64(0), false},
		{"valid float32", "123.456", float32(123.456), float32(0), false},
		{"valid float64", "789.1234", float64(789.1234), float64(0), false},
		{"invalid int", "abc", int(0), int(0), true},
		{"invalid uint", "-123", uint(0), uint(0), true},
		{"invalid float32", "abc", float32(0), float32(0), true},
		{"empty input string", "", int(0), int(0), true},
		{"unsupported type", "123", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.typ.(type) {
			case int:
				result, err := convertStrToNumber[int](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[int](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[int](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case int8:
				result, err := convertStrToNumber[int8](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[int8](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[int8](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case int16:
				result, err := convertStrToNumber[int16](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[int16](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[int16](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case int32:
				result, err := convertStrToNumber[int32](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[int32](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[int32](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case int64:
				result, err := convertStrToNumber[int64](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[int64](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[int64](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case uint:
				result, err := convertStrToNumber[uint](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[uint](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[uint](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case uint8:
				result, err := convertStrToNumber[uint8](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[uint8](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[uint8](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case uint16:
				result, err := convertStrToNumber[uint16](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[uint16](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[uint16](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case uint32:
				result, err := convertStrToNumber[uint32](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[uint32](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[uint32](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case uint64:
				result, err := convertStrToNumber[uint64](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[uint64](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[uint64](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case float32:
				result, err := convertStrToNumber[float32](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[float32](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[float32](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			case float64:
				result, err := convertStrToNumber[float64](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[float64](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
					return
				}
				if err == nil && result != tt.expected {
					t.Errorf("convertStrToNumber[float64](%v) = %v, want %v", tt.input, result, tt.expected)
				}
			default:
				_, err := convertStrToNumber[any](tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertStrToNumber[any](%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
			}
		})
	}
}
