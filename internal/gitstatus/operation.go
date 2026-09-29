package gitstatus

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	fetchHeadFile  = "FETCH_HEAD"
	commonDirFile  = "commondir"
	resolveOrAbort = " --continue or --abort"
)

// Operation is a multi-step git command left unfinished, such as a rebase stopped on a conflict.
type Operation int

const (
	NoOperation Operation = iota
	Merge
	Rebase
	CherryPick
	Revert
	Bisect
)

type operationInfo struct {
	markers []string
	name    string
	verb    string
	finish  string
}

// operations is checked in order: a rebase replaying a merge also leaves MERGE_HEAD.
var operations = []struct {
	op   Operation
	info operationInfo
}{
	{Rebase, operationInfo{[]string{"rebase-merge", "rebase-apply"}, "Rebase", "rebasing", "git rebase" + resolveOrAbort}},
	{Merge, operationInfo{[]string{"MERGE_HEAD"}, "Merge", "merging", "git merge" + resolveOrAbort}},
	{CherryPick, operationInfo{[]string{"CHERRY_PICK_HEAD"}, "Cherry-pick", "cherry-picking", "git cherry-pick" + resolveOrAbort}},
	{Revert, operationInfo{[]string{"REVERT_HEAD"}, "Revert", "reverting", "git revert" + resolveOrAbort}},
	{Bisect, operationInfo{[]string{"BISECT_LOG"}, "Bisect", "bisecting", "git bisect reset"}},
}

func (o Operation) info() operationInfo {
	for _, entry := range operations {
		if entry.op == o {
			return entry.info
		}
	}
	return operationInfo{}
}

// Name is the operation's name, such as "Rebase".
func (o Operation) Name() string {
	return o.info().name
}

// Verb says what the repository is in the middle of, such as "rebasing".
func (o Operation) Verb() string {
	return o.info().verb
}

// Finish is the command that ends the operation.
func (o Operation) Finish() string {
	return o.info().finish
}

func detectOperation(gitDir string) Operation {
	if gitDir == "" {
		return NoOperation
	}
	for _, entry := range operations {
		for _, marker := range entry.info.markers {
			if exists(filepath.Join(gitDir, marker)) {
				return entry.op
			}
		}
	}
	return NoOperation
}

// fetchedAt is when the repository was last fetched, from FETCH_HEAD, which a worktree may keep
// in the main repository's folder.
func fetchedAt(gitDir string) time.Time {
	if gitDir == "" {
		return time.Time{}
	}
	for _, dir := range []string{gitDir, commonDir(gitDir)} {
		if info, err := os.Stat(filepath.Join(dir, fetchHeadFile)); err == nil {
			return info.ModTime()
		}
	}
	return time.Time{}
}

// commonDir is the main repository folder of a worktree's git dir, or gitDir itself.
func commonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, commonDirFile))
	if err != nil {
		return gitDir
	}
	dir := strings.TrimSpace(string(data))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(gitDir, dir)
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
