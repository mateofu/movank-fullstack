package config

import "testing"

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"", "8080"}, {"9090", "9090"}, {"65535", "65535"},
		{"0", ""}, {"65536", ""}, {"-1", ""}, {"abc", ""}, {" 8080", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			cfg, err := Load(func(string) string { return tc.input })
			if tc.want == "" {
				if err == nil {
					t.Fatal("expected invalid port to fail")
				}
				return
			}
			if err != nil || cfg.HTTPPort != tc.want {
				t.Fatalf("got %v, %v; want port %s", cfg, err, tc.want)
			}
		})
	}
}
