package pulls

// CheckState is where a CI check stands.
type CheckState int

const (
	Failed CheckState = iota
	Pending
	Passed
	Skipped
)

// Check is one CI check on the pull request's latest commit.
type Check struct {
	Name string
	// Workflow is the GitHub Actions workflow the check belongs to, if any.
	Workflow string
	// URL is the check's page; for GitHub Actions, the job with its log.
	URL   string
	State CheckState
}

// CheckCounts counts the checks by state.
type CheckCounts struct {
	Failed  int
	Pending int
	Passed  int
	Skipped int
}

// Total is the number of checks.
func (c CheckCounts) Total() int {
	return c.Failed + c.Pending + c.Passed + c.Skipped
}

// CheckCounts counts p's checks by state.
func (p PR) CheckCounts() CheckCounts {
	var c CheckCounts
	for _, check := range p.Checks {
		switch check.State {
		case Failed:
			c.Failed++
		case Pending:
			c.Pending++
		case Passed:
			c.Passed++
		case Skipped:
			c.Skipped++
		}
	}
	return c
}

// HasRunningChecks reports whether any check is still queued or running.
func (p PR) HasRunningChecks() bool {
	return p.CheckCounts().Pending > 0
}

// FailingChecks are the checks that failed.
func (p PR) FailingChecks() []Check {
	var failing []Check
	for _, check := range p.Checks {
		if check.State == Failed {
			failing = append(failing, check)
		}
	}
	return failing
}

// checkRunState reads a GitHub Actions or app check: still running unless completed, and failed
// for any conclusion other than success, neutral or skipped.
func checkRunState(status, conclusion string) CheckState {
	if status != "COMPLETED" {
		return Pending
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL":
		return Passed
	case "SKIPPED":
		return Skipped
	default:
		return Failed
	}
}

// statusContextState reads a commit status set by an outside service.
func statusContextState(state string) CheckState {
	switch state {
	case "SUCCESS":
		return Passed
	case "PENDING", "EXPECTED":
		return Pending
	default:
		return Failed
	}
}
