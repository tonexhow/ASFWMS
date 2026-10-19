package domain

import "testing"

func TestMeetingDayNormalization(t *testing.T) {
	for in, want := range map[string]string{"7,1,6": "1,6,7", "weekdays": "1,2,3,4,5", "sunday": "7", "saturday": "6"} {
		got, err := NormalizeDays(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "8", "1,1", "1,", "foo", "1;DROP TABLE subjects"} {
		if _, err := NormalizeDays(in); err == nil {
			t.Fatalf("accepted invalid day mask %q", in)
		}
	}
}
func TestSubjectDaysAndFallbackValidation(t *testing.T) {
	base := map[string]any{"code": "X", "title": "X", "allowed_days": "7", "pattern": "sun"}
	r, err := Normalize("subjects", base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r["allowed_days"] != "7" {
		t.Fatal(r)
	}
	base["allowed_days"] = "6"
	base["saturday_fallback"] = 1
	if _, err = Normalize("subjects", base, nil); err == nil {
		t.Fatal("regular Saturday and fallback both accepted")
	}
}
