package player

import "testing"

func TestValidateCountry(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"FR", false},
		{"US", false},
		{"fr", true},
		{"FRA", true},
		{"", true},
		{"F1", true},
	}
	for _, tt := range tests {
		err := validateCountry(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateCountry(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
		}
	}
}

func TestValidateLen(t *testing.T) {
	if err := validateLen("f", "", 5); err == nil {
		t.Error("empty value should fail")
	}
	if err := validateLen("f", "abcdef", 5); err == nil {
		t.Error("too long value should fail")
	}
	if err := validateLen("f", "abcde", 5); err != nil {
		t.Errorf("value at limit should pass: %v", err)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		in   int32
		want int
	}{
		{0, 20},
		{-5, 20},
		{7, 7},
		{100, 100},
		{101, 100},
	}
	for _, tt := range tests {
		if got := clamp(tt.in, 20, 100); got != tt.want {
			t.Errorf("clamp(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
