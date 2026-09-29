package doctor

import (
	"context"
	"slices"
	"testing"
	"time"
)

func constant(result Result) func(context.Context, Env) Result {
	return func(context.Context, Env) Result { return result }
}

func TestRunKeepsCatalogOrder(t *testing.T) {
	slow := func(context.Context, Env) Result {
		time.Sleep(20 * time.Millisecond)
		return pass("slow")
	}
	d := Doctor{Checks: []Check{
		{Name: "first", Run: slow},
		{Name: "second", Run: constant(pass("fast"))},
		{Name: "third", Run: constant(fail("broken", "fix it"))},
	}, Env: newMachine(t).env()}

	var progress []Progress
	outcomes := d.Run(context.Background(), func(p Progress) { progress = append(progress, p) })

	names := make([]string, len(outcomes))
	for i, o := range outcomes {
		names[i] = o.Check.Name
	}
	if !slices.Equal(names, []string{"first", "second", "third"}) {
		t.Errorf("order = %v", names)
	}
	if last := progress[len(progress)-1]; last.Done != 3 || last.Total != 3 {
		t.Errorf("last progress = %+v", last)
	}
	if Count(outcomes, StatusFail) != 1 || Count(outcomes, StatusPass) != 2 {
		t.Errorf("counts wrong: %+v", outcomes)
	}
}

func TestRunTurnsPanicIntoFailure(t *testing.T) {
	d := Doctor{Checks: []Check{{Name: "boom", Run: func(context.Context, Env) Result { panic("kaboom") }}}}

	outcomes := d.Run(context.Background(), nil)

	assertStatus(t, outcomes[0].Result, StatusFail)
	assertContains(t, outcomes[0].Result.Details[0], "kaboom")
}

func TestRunTimesOutHungCheck(t *testing.T) {
	hung := func(ctx context.Context, _ Env) Result {
		<-ctx.Done()
		time.Sleep(10 * time.Millisecond)
		return pass("too late")
	}
	d := Doctor{Checks: []Check{{Name: "hung", Run: hung}}, Timeout: 20 * time.Millisecond}

	result := d.Run(context.Background(), nil)[0].Result

	assertStatus(t, result, StatusWarn)
	assertContains(t, result.Summary, "did not finish")
}

func TestRunSkipsNetworkChecksOffline(t *testing.T) {
	m := newMachine(t)
	m.install("node", "node --version", "v24.1.0")
	d := Doctor{Checks: []Check{
		{Name: "online", NeedsNetwork: true, Run: constant(pass("ran"))},
		{Name: "Node.js", Run: checkNode},
	}, Env: m.env(), Offline: true}

	outcomes := d.Run(context.Background(), nil)

	assertStatus(t, outcomes[0].Result, StatusSkip)
	assertStatus(t, outcomes[1].Result, StatusPass)
	assertContains(t, outcomes[1].Result.Summary, "offline")
}

func TestRunSkipsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := Doctor{Checks: []Check{{Name: "any", Run: constant(pass("ran"))}}}

	assertStatus(t, d.Run(ctx, nil)[0].Result, StatusSkip)
}

func TestSelectAndParseGroup(t *testing.T) {
	android, err := ParseGroup("Android")
	if err != nil || android != GroupAndroid {
		t.Fatalf("ParseGroup = %v, %v", android, err)
	}
	if _, err := ParseGroup("windows"); err == nil {
		t.Error("unknown group accepted")
	}

	for _, check := range Select(Default(), []Group{GroupAndroid}) {
		if check.Group != GroupAndroid {
			t.Errorf("%s is in group %s", check.Name, check.Group.Key())
		}
	}
	if len(Select(Default(), nil)) != len(Default()) {
		t.Error("no groups should select everything")
	}
}

func TestCheckNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, check := range Default() {
		if seen[check.Name] {
			t.Errorf("duplicate check name %q", check.Name)
		}
		seen[check.Name] = true
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		output string
		want   Version
	}{
		{"v22.13.1", Version{22, 13, 1}},
		{"go version go1.27.1 darwin/arm64", Version{1, 27, 1}},
		{"Xcode 27.0\nBuild version 27A266a", Version{27, 0, 0}},
		{"openjdk 17.0.17 2025-10-21", Version{17, 0, 17}},
		{"eas-cli/16.28.0 darwin-arm64 node-v22.13.1", Version{16, 28, 0}},
		{"2.1.267 (Claude Code)", Version{2, 1, 267}},
	}
	for _, tt := range tests {
		got, ok := ParseVersion(tt.output)
		if !ok || got != tt.want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", tt.output, got, ok, tt.want)
		}
	}
	if _, ok := ParseVersion("no digits here"); ok {
		t.Error("parsed a version from text without numbers")
	}
}
