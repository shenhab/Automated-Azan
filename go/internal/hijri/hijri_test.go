package hijri

import "testing"

// TestDateString covers formatting of known Hijri dates.
func TestDateString(t *testing.T) {
	cases := []struct {
		name string
		d    Date
		want string
	}{
		{
			name: "typical date",
			d:    Date{Day: 12, Month: 12, MonthEN: "Dhū al-Ḥijjah", Year: 1447},
			want: "12 Dhū al-Ḥijjah 1447 AH",
		},
		{
			name: "first day of first month",
			d:    Date{Day: 1, Month: 1, MonthEN: "Muḥarram", Year: 1447},
			want: "1 Muḥarram 1447 AH",
		},
		{
			name: "leap-year 30th day of Dhū al-Ḥijjah",
			d:    Date{Day: 30, Month: 12, MonthEN: "Dhū al-Ḥijjah", Year: 1445},
			want: "30 Dhū al-Ḥijjah 1445 AH",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestIsRamadan covers the month boundary between Sha'ban (8), Ramadan (9)
// and Shawwal (10).
func TestIsRamadan(t *testing.T) {
	cases := []struct {
		name  string
		month int
		want  bool
	}{
		{"shaban not ramadan", 8, false},
		{"ramadan", 9, true},
		{"shawwal not ramadan", 10, false},
		{"muharram not ramadan", 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Date{Month: tc.month, Day: 15}
			if got := d.IsRamadan(); got != tc.want {
				t.Errorf("IsRamadan() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRamadanDay covers day boundaries within and outside Ramadan, including
// the last day of a 30-day leap Ramadan.
func TestRamadanDay(t *testing.T) {
	cases := []struct {
		name  string
		month int
		day   int
		want  int
	}{
		{"first day of ramadan", 9, 1, 1},
		{"mid ramadan", 9, 15, 15},
		{"last day of 29-day ramadan", 9, 29, 29},
		{"last day of 30-day leap ramadan", 9, 30, 30},
		{"not ramadan returns zero", 10, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Date{Month: tc.month, Day: tc.day}
			if got := d.RamadanDay(); got != tc.want {
				t.Errorf("RamadanDay() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSpecialDay covers the named-occasion boundaries: Eid al-Fitr,
// Eid al-Adha, Day of Arafat, Islamic New Year, Mawlid, Laylat al-Mi'raj,
// Laylat al-Bara'at, the last-ten-nights window of Ramadan, and ordinary days.
func TestSpecialDay(t *testing.T) {
	cases := []struct {
		name  string
		month int
		day   int
		want  string
	}{
		{"eid al-fitr day 1", 10, 1, "Eid al-Fitr"},
		{"eid al-fitr day 3", 10, 3, "Eid al-Fitr"},
		{"day after eid al-fitr window", 10, 4, ""},
		{"eid al-adha day 10", 12, 10, "Eid al-Adha"},
		{"eid al-adha day 13", 12, 13, "Eid al-Adha"},
		{"day after eid al-adha window", 12, 14, ""},
		{"day of arafat", 12, 9, "Day of Arafat"},
		{"day before day of arafat", 12, 8, ""},
		{"islamic new year", 1, 1, "Islamic New Year"},
		{"mawlid al-nabi", 3, 12, "Mawlid al-Nabi"},
		{"laylat al-miraj", 7, 27, "Laylat al-Mi'raj"},
		{"laylat al-baraat", 8, 15, "Laylat al-Bara'at"},
		{"potential laylat al-qadr, night 19", 9, 19, "Potentially Laylat al-Qadr"},
		{"potential laylat al-qadr, night 30", 9, 30, "Potentially Laylat al-Qadr"},
		{"early ramadan not last-ten-nights", 9, 18, "Ramadan"},
		{"ordinary day", 2, 5, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Date{Month: tc.month, Day: tc.day}
			if got := d.SpecialDay(); got != tc.want {
				t.Errorf("SpecialDay() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSpecialDayHolidaysFallback covers the fallback to the Holidays list
// when no built-in occasion matches the date.
func TestSpecialDayHolidaysFallback(t *testing.T) {
	d := Date{Month: 5, Day: 20, Holidays: []string{"Custom Regional Holiday", "Second Holiday"}}
	if got, want := d.SpecialDay(), "Custom Regional Holiday"; got != want {
		t.Errorf("SpecialDay() = %q, want %q", got, want)
	}

	empty := Date{Month: 5, Day: 20}
	if got := empty.SpecialDay(); got != "" {
		t.Errorf("SpecialDay() = %q, want empty string", got)
	}
}
