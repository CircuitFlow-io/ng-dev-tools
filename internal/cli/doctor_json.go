package cli

import (
	"github.com/CircuitFlow-io/ng-dev-tools/internal/doctor"
)

var doctorStatusNames = map[doctor.Status]string{
	doctor.StatusPass: "pass",
	doctor.StatusWarn: "warn",
	doctor.StatusFail: "fail",
	doctor.StatusSkip: "skip",
}

type doctorJSON struct {
	Counts doctorCountsJSON  `json:"counts"`
	Checks []doctorCheckJSON `json:"checks"`
}

// doctorCountsJSON counts every check that ran, including the ones --problems leaves out.
type doctorCountsJSON struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

type doctorCheckJSON struct {
	Group   string   `json:"group"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	Details []string `json:"details,omitempty"`
	// Fix is the command or setting that resolves a warning or failure.
	Fix        string `json:"fix,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

func toDoctorJSON(outcomes []doctor.Outcome, problemsOnly bool) doctorJSON {
	checks := make([]doctorCheckJSON, 0, len(outcomes))
	for _, o := range outcomes {
		if problemsOnly && !o.Result.IsProblem() {
			continue
		}
		checks = append(checks, toDoctorCheckJSON(o))
	}
	return doctorJSON{
		Counts: doctorCountsJSON{
			Pass: doctor.Count(outcomes, doctor.StatusPass),
			Warn: doctor.Count(outcomes, doctor.StatusWarn),
			Fail: doctor.Count(outcomes, doctor.StatusFail),
			Skip: doctor.Count(outcomes, doctor.StatusSkip),
		},
		Checks: checks,
	}
}

func toDoctorCheckJSON(o doctor.Outcome) doctorCheckJSON {
	return doctorCheckJSON{
		Group:      o.Check.Group.Key(),
		Name:       o.Check.Name,
		Status:     doctorStatusNames[o.Result.Status],
		Summary:    o.Result.Summary,
		Details:    o.Result.Details,
		Fix:        o.Result.Fix,
		DurationMs: o.Duration.Milliseconds(),
	}
}
