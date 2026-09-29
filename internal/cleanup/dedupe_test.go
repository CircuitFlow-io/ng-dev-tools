package cleanup

import (
	"reflect"
	"testing"
)

func pathItem(title string, paths ...string) Item {
	return PathItem(CategoryCaches, SafetySafe, title, paths...)
}

func titlesAndPaths(items []Item) map[string][]string {
	got := make(map[string][]string, len(items))
	for _, item := range items {
		got[item.Title] = item.Paths
	}
	return got
}

func TestDedupe(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		want  map[string][]string
	}{
		{
			name:  "exact duplicate keeps the earlier item",
			items: []Item{pathItem("specific", "/h/a"), pathItem("generic", "/h/a")},
			want:  map[string][]string{"specific": {"/h/a"}},
		},
		{
			name:  "broader later path absorbs an earlier nested one",
			items: []Item{pathItem("chrome", "/h/c/Google/Chrome"), pathItem("google", "/h/c/Google")},
			want:  map[string][]string{"google": {"/h/c/Google"}},
		},
		{
			name:  "nested later path is dropped",
			items: []Item{pathItem("google", "/h/c/Google"), pathItem("chrome", "/h/c/Google/Chrome")},
			want:  map[string][]string{"google": {"/h/c/Google"}},
		},
		{
			name:  "multi-path item keeps the paths nobody else owns",
			items: []Item{pathItem("cache", "/h/c/app"), pathItem("app", "/Applications/App.app", "/h/c/app", "/h/prefs/app.plist")},
			want: map[string][]string{
				"cache": {"/h/c/app"},
				"app":   {"/Applications/App.app", "/h/prefs/app.plist"},
			},
		},
		{
			name:  "sibling paths with a shared prefix are independent",
			items: []Item{pathItem("a", "/h/cache"), pathItem("b", "/h/cache2")},
			want:  map[string][]string{"a": {"/h/cache"}, "b": {"/h/cache2"}},
		},
		{
			name: "command items survive without paths",
			items: []Item{
				pathItem("dir", "/h/brew"),
				{Title: "brew", Paths: []string{"/h/brew/cache"}, RemoveCommand: []string{"brew", "cleanup"}},
			},
			want: map[string][]string{"dir": {"/h/brew"}, "brew": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titlesAndPaths(dedupe(tt.items)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dedupe() = %v, want %v", got, tt.want)
			}
		})
	}
}
