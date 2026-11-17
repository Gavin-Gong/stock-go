package markettime

import "testing"

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		local, zone string
		ms          int64
	}{
		{"2024-01-02 09:30:00", CN, 1704159000000},
		{"20240102093000", HK, 1704159000000},
		{"2024/01/02 09:30:00", HK, 1704159000000},
		{"2024-07-09 09:30", US, 1720531800000},
		{"2024-01-02T09:30", US, 1704205800000},
		{"2024-11-03 01:30", US, 1730611800000},
		{"2024-03-10 02:30", US, 1710055800000},
		{"2024-03-10 03:00", US, 1710054000000},
		{"2024-11-03 04:00", US, 1730624400000},
		{"2024-01-02", CN, 1704124800000},
	} {
		t.Run(tt.local+tt.zone, func(t *testing.T) {
			got := Parse(tt.local, tt.zone)
			if got == nil || *got != tt.ms {
				t.Fatalf("got=%v want=%d", got, tt.ms)
			}
		})
	}
	for _, local := range []string{"", "bad", "2024-02-30", "2024-01-02 25:00"} {
		if got := Parse(local, US); got != nil {
			t.Errorf("%s: %d", local, *got)
		}
	}
	if Parse("20240102", "unknown") != nil {
		t.Fatal("unknown time zone")
	}
	a, b := DateAndTime("20240102", "09:30", CN), Parse("2024-01-02 09:30", CN)
	if a == nil || b == nil || *a != *b {
		t.Fatal("date/time combination")
	}
}
