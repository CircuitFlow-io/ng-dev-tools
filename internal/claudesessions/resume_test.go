package claudesessions

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	resumeHelperVariable = "NGT_TEST_RESUME_DIR"
	startHelperVariable  = "NGT_TEST_START_DIR"
)

// TestResumeHelper stands in for ngt: run by TestResumeExecsClaudeInTheSessionFolder, it resumes a
// session and is replaced by the fake claude.
func TestResumeHelper(t *testing.T) {
	dir := os.Getenv(resumeHelperVariable)
	if dir == "" {
		t.Skip("only run as a helper process")
	}
	err := Resume(Session{ID: "abc", Dir: dir})
	t.Fatalf("Resume returned: %v", err)
}

func TestResumeExecsClaudeInTheSessionFolder(t *testing.T) {
	bin, sessionDir := t.TempDir(), t.TempDir()
	record := filepath.Join(t.TempDir(), "record")
	fake := "#!/bin/sh\necho \"$(pwd -P) $PWD $*\" > " + record + "\n"
	if err := os.WriteFile(filepath.Join(bin, claudeProgram), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestResumeHelper$")
	cmd.Env = append(os.Environ(), "PATH="+bin+":/bin:/usr/bin", resumeHelperVariable+"="+sessionDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}

	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(sessionDir)
	if want := real + " " + sessionDir + " --resume abc\n"; string(got) != want {
		t.Errorf("claude ran as %q, want %q", got, want)
	}
}

func TestResumeRefusesAMissingFolder(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, claudeProgram), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := Resume(Session{ID: "abc", Dir: filepath.Join(bin, "gone")}); err == nil {
		t.Error("resumed in a folder that does not exist")
	}
}

// TestStartHelper stands in for ngt: run by TestStartExecsClaudeInTheFolder, it starts a session
// and is replaced by the fake claude.
func TestStartHelper(t *testing.T) {
	dir := os.Getenv(startHelperVariable)
	if dir == "" {
		t.Skip("only run as a helper process")
	}
	err := Start(dir)
	t.Fatalf("Start returned: %v", err)
}

func TestStartExecsClaudeInTheFolder(t *testing.T) {
	bin, projectDir := t.TempDir(), t.TempDir()
	record := filepath.Join(t.TempDir(), "record")
	fake := "#!/bin/sh\necho \"$(pwd -P) $PWD $#\" > " + record + "\n"
	if err := os.WriteFile(filepath.Join(bin, claudeProgram), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStartHelper$")
	cmd.Env = append(os.Environ(), "PATH="+bin+":/bin:/usr/bin", startHelperVariable+"="+projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}

	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(projectDir)
	if want := real + " " + projectDir + " 0\n"; string(got) != want {
		t.Errorf("claude ran as %q, want %q (folder, $PWD, argument count)", got, want)
	}
}
