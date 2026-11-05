package mainthread

import (
	"errors"
	"testing"
)

func TestPostWithoutRunReportsFalse(t *testing.T) {
	if Post(func() {}) {
		t.Fatal("Post succeeded with no Run serving")
	}
}

func TestRunExecutesPostedFunctionsOnItsGoroutine(t *testing.T) {
	want := errors.New("worker result")
	ran := 0
	release := make(chan struct{})
	err := Run(func() error {
		for i := 0; i < 3; i++ {
			if !Post(func() { ran++; release <- struct{}{} }) {
				t.Error("Post failed while Run serves")
			}
			<-release
		}
		return want
	})
	if err != want {
		t.Fatalf("Run returned %v, want %v", err, want)
	}
	// ran is only touched by the serving goroutine, which is this one.
	if ran != 3 {
		t.Fatalf("ran %d posted functions, want 3", ran)
	}
	if Post(func() {}) {
		t.Fatal("Post succeeded after Run returned")
	}
}

func TestRunDrainsFunctionsPostedBeforeReturn(t *testing.T) {
	ran := false
	_ = Run(func() error {
		Post(func() { ran = true })
		return nil
	})
	if !ran {
		t.Fatal("a function posted before f returned was dropped")
	}
}
