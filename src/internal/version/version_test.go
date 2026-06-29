package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"1.1.0", "1.1.0", 0},
		{"1.1.1", "1.1.0", 1},
		{"1.2.0", "1.10.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"v1.2.3", "1.2.3", 0},
	}
	for _, tt := range tests {
		got, err := Compare(tt.a, tt.b)
		if err != nil {
			t.Fatalf("Compare(%q, %q): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("Compare(%q, %q)=%d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareRejectsInvalidVersion(t *testing.T) {
	if _, err := Compare("1.2", "1.2.0"); err == nil {
		t.Fatal("expected invalid version error")
	}
}
