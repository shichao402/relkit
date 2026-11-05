package model

import "testing"

func TestNextSequenceJumpsExistingDocumentsPastFloor(t *testing.T) {
	cases := []struct{ base, want int }{
		{0, 1},
		{1, SequenceFloor},
		{6, SequenceFloor},
		{SequenceFloor - 1, SequenceFloor},
		{SequenceFloor, SequenceFloor + 1},
		{5000, 5001},
	}
	for _, c := range cases {
		if got := NextSequence(c.base); got != c.want {
			t.Fatalf("NextSequence(%d)=%d want %d", c.base, got, c.want)
		}
	}
}
