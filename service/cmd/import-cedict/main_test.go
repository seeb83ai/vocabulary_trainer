package main

import "testing"

func TestCheckReplaceSize(t *testing.T) {
	cases := []struct {
		existing, incoming int
		force, wantErr     bool
	}{
		{0, 10, false, false},          // first import
		{120000, 121000, false, false}, // new version, about the same size
		{120000, 1, false, true},       // a one-line file would wipe the dictionary
		{120000, 1, true, false},       // unless forced
	}
	for _, c := range cases {
		err := checkReplaceSize(c.existing, c.incoming, c.force)
		if (err != nil) != c.wantErr {
			t.Errorf("checkReplaceSize(%d, %d, %v) = %v, want error %v", c.existing, c.incoming, c.force, err, c.wantErr)
		}
	}
}
