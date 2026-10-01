package todos

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

const githubUserCall = `gh api user --jq .login, (.name // "")`

func TestWroteMatchesAnyOfYourEmailsAndNames(t *testing.T) {
	me := identity{emails: []string{"me@work.com", "me@home.com"}, names: []string{"Me Myself"}, login: "mine"}
	tests := []struct {
		author, email string
		want          bool
	}{
		{"Someone", "ME@home.com", true},
		{"me myself", "old@laptop.local", true},
		{"Someone", "123+Mine@users.noreply.github.com", true},
		{"Someone", "mine@users.noreply.github.com", true},
		{"Someone", "123+minerva@users.noreply.github.com", false},
		{"Kim", "kim@work.com", false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got := me.wrote(Item{Author: tt.author, Email: tt.email}); got != tt.want {
			t.Errorf("wrote(%q <%s>) = %v, want %v", tt.author, tt.email, got, tt.want)
		}
	}
}

func TestGitHubIdentityIsYourLoginAndProfileName(t *testing.T) {
	runner := &macostest.Runner{Outputs: map[string]string{githubUserCall: "mine\nMe Myself\n"}}
	me := githubIdentity(context.Background(), runner)
	if me.login != "mine" || len(me.names) != 1 || me.names[0] != "Me Myself" {
		t.Errorf("githubIdentity = %+v", me)
	}
	offline := &macostest.Runner{Errors: map[string]error{githubUserCall: errors.New("offline")}}
	if me := githubIdentity(context.Background(), offline); me.login != "" || me.names != nil {
		t.Errorf("githubIdentity without gh = %+v, want nobody", me)
	}
}

func TestLinesUnderYourGlobalEmailAreYoursWhereTheRepoSetsAnother(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	global := filepath.Join(root, "gitconfig")
	write(t, root, "gitconfig", "[user]\n\temail = me@home.com\n\tname = Me Myself\n")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := filepath.Join(root, "api")
	runGit(t, root, "init", "--quiet", "api")
	runGit(t, dir, "config", "user.email", "me@work.com")
	write(t, dir, "home.ts", "// TODO from home\n")
	runGit(t, dir, "add", ".")
	commitAs(t, dir, "Someone", "me@home.com", "2025-01-01T10:00:00Z", "from home")
	write(t, dir, "laptop.ts", "// TODO from an old laptop\n")
	runGit(t, dir, "add", ".")
	commitAs(t, dir, "Me Myself", "me@old-laptop.local", "2025-01-02T10:00:00Z", "from an old laptop")
	write(t, dir, "kim.ts", "// TODO from Kim\n")
	runGit(t, dir, "add", ".")
	commitAs(t, dir, "Kim", "kim@work.com", "2025-01-03T10:00:00Z", "from Kim")

	items, err := Find(context.Background(), macos.ExecRunner{}, "api", dir)
	if err != nil {
		t.Fatal(err)
	}
	mine := map[string]bool{}
	for _, i := range items {
		mine[i.File] = i.Mine
	}
	if !mine["home.ts"] || !mine["laptop.ts"] || mine["kim.ts"] {
		t.Errorf("mine = %v, want home.ts and laptop.ts only", mine)
	}
}

func TestClaimMarksWhatGitHubWroteForYouAndEverythingByItsEmail(t *testing.T) {
	items := []Item{
		{File: "squashed.ts", Author: "Me Myself", Email: "me@home.com"},
		{File: "home-laptop.ts", Author: "me", Email: "me@home.com"},
		{File: "work.ts", Author: "me.work", Email: "me@work.com", Mine: true},
		{File: "work-other-name.ts", Author: "Me At Work", Email: "me@work.com"},
		{File: "kim.ts", Author: "Kim", Email: "kim@work.com"},
	}
	claim(items, identity{names: []string{"Me Myself"}, login: "mine"})
	for _, item := range items {
		if want := item.File != "kim.ts"; item.Mine != want {
			t.Errorf("%s mine = %v, want %v", item.File, item.Mine, want)
		}
	}
}
