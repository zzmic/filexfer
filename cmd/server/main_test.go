package main

import (
	"path/filepath"
	"testing"
)

func TestPathSanitization(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name         string
		userPath     string
		expectedPath string
		errExpected  bool
	}{
		{
			name:         "basic file",
			userPath:     "file.txt",
			expectedPath: filepath.Join(base, "file.txt"),
		},
		{
			name:         "nested path",
			userPath:     "dir/sub/file.txt",
			expectedPath: filepath.Join(base, "dir", "sub", "file.txt"),
		},
		{
			name:         "dot segment",
			userPath:     "dir/./file.txt",
			expectedPath: filepath.Join(base, "dir", "file.txt"),
		},
		{
			name:        "empty path",
			userPath:    "",
			errExpected: true,
		},
		{
			name:        "absolute path",
			userPath:    filepath.Join(string(filepath.Separator), "etc", "passwd"),
			errExpected: true,
		},
		{
			name:        "parent traversal unix style",
			userPath:    "../etc/passwd",
			errExpected: true,
		},
		{
			name:        "parent traversal mixed separators",
			userPath:    "dir/../secret.txt",
			errExpected: true,
		},
		{
			name:        "parent traversal backslash",
			userPath:    "dir\\..\\secret.txt",
			errExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePath(base, tt.userPath)

			if tt.errExpected {
				if err == nil {
					t.Fatalf("`sanitizePath` error = nil, expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("`sanitizePath` unexpected error = %v", err)
			}

			if got != tt.expectedPath {
				t.Fatalf("`sanitizePath` = %q, expected %q", got, tt.expectedPath)
			}
		})
	}
}
