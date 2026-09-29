package cleanup

import "testing"

func TestGuardCheck(t *testing.T) {
	guard := NewGuard("/Users/me")
	tests := []struct {
		path    string
		allowed bool
	}{
		{"/Users/me/Library/Caches/com.example", true},
		{"/Users/me/.npm/_cacache", true},
		{"/Applications/Foo.app", true},
		{"/Library/Caches/com.example", true},
		{"/Users/me", false},
		{"/Users/me/Library", false},
		{"/Users/me/Library/Caches", false},
		{"/Users/me/Documents", false},
		{"/Users/me/.Trash", false},
		{"/Applications", false},
		{"/", false},
		{"/System/Library/Caches/x", false},
		{"/Library/Preferences/x.plist", false},
		{"/Users/other/Library/Caches/x", false},
		{"/Users/me/.ssh/id_ed25519", false},
		{"/Users/me/Library/Keychains/login.keychain-db", false},
		{"/Users/me/Library/Caches/../Documents", false},
		{"relative/path", false},
		{"/Users/meow/Library/Caches/x", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := guard.Check(tt.path)
			if allowed := err == nil; allowed != tt.allowed {
				t.Errorf("Check(%q) allowed = %v, want %v (err: %v)", tt.path, allowed, tt.allowed, err)
			}
		})
	}
}
