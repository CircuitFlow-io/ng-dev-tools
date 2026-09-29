package cli

import (
	"errors"
	"testing"

	"github.com/nasserghiasi/ng-dev-tools/internal/doctor"
)

func TestParseGroups(t *testing.T) {
	groups, err := parseGroups([]string{"android", "go"})
	if err != nil || len(groups) != 2 || groups[0] != doctor.GroupAndroid || groups[1] != doctor.GroupGo {
		t.Errorf("parseGroups = %v, %v", groups, err)
	}
	if _, err := parseGroups([]string{"windows"}); err == nil {
		t.Error("unknown group accepted")
	}
}

func TestFailureOnlyWhenAChecksFails(t *testing.T) {
	warned := []doctor.Outcome{{Result: doctor.Result{Status: doctor.StatusWarn}}}
	if err := failure(warned); err != nil {
		t.Errorf("warnings alone returned %v", err)
	}

	failed := []doctor.Outcome{warned[0], {Result: doctor.Result{Status: doctor.StatusFail}}}
	if err := failure(failed); !errors.Is(err, errChecksFailed) {
		t.Errorf("failure = %v, want errChecksFailed", err)
	}
}
