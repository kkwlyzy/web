package main

import "testing"

func TestNormalizePlatform(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "x86", want: "linux/amd64"},
		{in: "amd64", want: "linux/amd64"},
		{in: "linux/amd64", want: "linux/amd64"},
		{in: "amr", want: "linux/arm64"},
		{in: "arm64", want: "linux/arm64"},
		{in: "linux/arm64", want: "linux/arm64"},
		{in: "windows/amd64", wantErr: true},
		{in: "mips", wantErr: true},
	}

	for _, tc := range tests {
		got, err := normalizePlatform(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("normalizePlatform(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizePlatform(%q) got err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizePlatform(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeContainerDir(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "/app", want: "/app"},
		{in: "app", want: "/app"},
		{in: "/a/b/../c", want: "/a/c"},
		{in: "", wantErr: true},
		{in: `C:\app`, wantErr: true},
	}

	for _, tc := range tests {
		got, err := normalizeContainerDir(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("normalizeContainerDir(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeContainerDir(%q) got err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeContainerDir(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}
