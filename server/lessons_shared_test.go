package server

import "testing"

func TestRSILessonsSharedIsOptIn(t *testing.T) {
	t.Setenv("ARTEX_RSI_LESSONS_SHARED", "")
	if rsiLessonsShared() {
		t.Fatal("cross-engagement lesson sharing must be off by default")
	}
	for _, v := range []string{"1", "true", "ON", " yes "} {
		t.Setenv("ARTEX_RSI_LESSONS_SHARED", v)
		if !rsiLessonsShared() {
			t.Errorf("%q should enable sharing", v)
		}
	}
	t.Setenv("ARTEX_RSI_LESSONS_SHARED", "0")
	if rsiLessonsShared() {
		t.Fatal("0 must not enable sharing")
	}
}
