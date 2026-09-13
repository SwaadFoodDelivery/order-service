package repository

import "testing"

func TestNumericToMinorExact(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int64
	}{
		{"0.00", 0}, {"0.05", 5}, {"1.05", 105}, {"10.50", 1050}, {"0.5", 50},
		{"12", 1200}, {"99999999.99", 9999999999}, {"92233720368547758.07", 9223372036854775807},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, err := numericToMinor(tc.text)
			if err != nil || got != tc.want {
				t.Fatalf("numericToMinor(%q) = %d, %v; want %d", tc.text, got, err, tc.want)
			}
		})
	}
}

func TestNumericToMinorRejectsInvalidData(t *testing.T) {
	for _, text := range []string{"", "NaN", "Infinity", "-0.05", "1.001", "1e2", "+1", "1.", " 1.05", "1.05 ", "1..2", "92233720368547758.08"} {
		if _, err := numericToMinor(text); err == nil {
			t.Errorf("accepted invalid money %q", text)
		}
	}
}
