package doctor

import (
	"context"
	"fmt"
)

// toolSpec describes a command-line tool that must be installed, optionally at a minimum version.
type toolSpec struct {
	command string
	args    []string
	minimum Version
	fix     string
	// optional tools only warn when missing.
	optional bool
}

func (s toolSpec) check(ctx context.Context, env Env) Result {
	v, err := env.version(ctx, s.command, s.args...)
	if err != nil {
		return s.missing(err)
	}
	if v.Less(s.minimum) {
		return fail(fmt.Sprintf("%s, need %s or newer", v, s.minimum), s.fix)
	}
	return pass(v.String())
}

func (s toolSpec) missing(err error) Result {
	result := missing(err, s.fix)
	if s.optional {
		result.Status = StatusWarn
	}
	return result
}
