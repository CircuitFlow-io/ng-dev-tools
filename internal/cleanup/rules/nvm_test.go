package rules

import (
	"context"
	"os"
	"slices"
	"testing"
)

func TestNvmRuleKeepsNewestAndDefault(t *testing.T) {
	tests := []struct {
		name    string
		aliases map[string]string
		want    []string
	}{
		{"default by major", map[string]string{"default": "22"}, []string{"Node v18.20.0 (nvm)", "Node v9.11.2 (nvm)"}},
		{"default through an lts chain", map[string]string{"default": "lts/*", "lts/*": "lts/iron", "lts/iron": "v18.20.0"}, []string{"Node v22.1.0 (nvm)", "Node v9.11.2 (nvm)"}},
		{"no default alias", nil, []string{"Node v18.20.0 (nvm)", "Node v22.1.0 (nvm)", "Node v9.11.2 (nvm)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testEnv(t)
			for _, v := range []string{"v9.11.2", "v18.20.0", "v22.1.0", "v24.21.0"} {
				touch(t, env.InHome(nvmVersionsDir, v, "bin", "node"), 1, longAgo)
			}
			for name, target := range tt.aliases {
				writeFile(t, env.InHome(nvmAliasDir, name), target+"\n")
			}

			items, err := nvmRule{}.Scan(context.Background(), env)
			if err != nil {
				t.Fatal(err)
			}
			if got := titles(items); !slices.Equal(got, tt.want) {
				t.Errorf("items = %v, want %v (newest v24.21.0 always kept)", got, tt.want)
			}
		})
	}
}

func TestNvmRuleIgnoresASingleVersion(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome(nvmVersionsDir, "v22.1.0", "bin", "node"), 1, longAgo)
	if err := os.MkdirAll(env.InHome(nvmAliasDir), 0o755); err != nil {
		t.Fatal(err)
	}

	items, _ := nvmRule{}.Scan(context.Background(), env)
	if len(items) != 0 {
		t.Errorf("items = %v, want none", titles(items))
	}
}

func TestCompareNodeVersions(t *testing.T) {
	versions := []string{"v24.2.0", "v9.11.2", "v24.10.1", "v18.20.0"}
	slices.SortFunc(versions, compareNodeVersions)
	if want := []string{"v9.11.2", "v18.20.0", "v24.2.0", "v24.10.1"}; !slices.Equal(versions, want) {
		t.Errorf("sorted = %v, want %v", versions, want)
	}
}
