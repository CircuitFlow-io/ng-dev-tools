package claudesessions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
)

const (
	claudeProgram = "claude"
	pwdPrefix     = "PWD="
)

var errNoClaude = errors.New("the claude command is not on your PATH")

// ResumeArgs is the command that resumes the session, to run in its folder.
func (s Session) ResumeArgs() []string {
	return []string{claudeProgram, "--resume", s.ID}
}

// Resume replaces ngt with claude resuming the session in its folder, so it only returns on
// failure.
func Resume(s Session) error {
	program, err := exec.LookPath(claudeProgram)
	if err != nil {
		return errNoClaude
	}
	if !s.DirExists() {
		return fmt.Errorf("%s no longer exists, so the session cannot be resumed there", s.Dir)
	}
	if err := os.Chdir(s.Dir); err != nil {
		return err
	}
	return syscall.Exec(program, s.ResumeArgs(), withPWD(os.Environ(), s.Dir))
}

// withPWD points $PWD at dir, which the shell would have done on cd.
func withPWD(env []string, dir string) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, pwdPrefix) })
	return append(env, pwdPrefix+dir)
}
