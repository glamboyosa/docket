package provider

import "testing"

func TestRequiresRemoteExtraction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{path: "notes.txt", want: false},
		{path: "README.MD", want: false},
		{path: "scan.pdf", want: true},
		{path: "receipt.jpg", want: true},
	}
	for _, test := range tests {
		if got := RequiresRemoteExtraction(test.path); got != test.want {
			t.Errorf("RequiresRemoteExtraction(%q) = %t, want %t", test.path, got, test.want)
		}
	}
}
