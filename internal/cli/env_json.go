package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
)

var envLevelNames = map[envfiles.Level]string{
	envfiles.Exposed:      "exposed",
	envfiles.Missing:      "missing",
	envfiles.Empty:        "empty",
	envfiles.Undocumented: "undocumented",
	envfiles.NotSetUp:     "not-set-up",
	envfiles.NoExample:    "no-example",
	envfiles.Unchecked:    "unchecked",
	envfiles.Complete:     "complete",
}

type envJSON struct {
	Sets []envSetJSON `json:"sets"`
}

// envSetJSON holds key names only: the domain never reads values, so none can leak here.
type envSetJSON struct {
	Name        string           `json:"name"`
	Dir         string           `json:"dir"`
	Attention   string           `json:"attention"`
	Example     string           `json:"example,omitempty"`
	ExampleKeys int              `json:"exampleKeys"`
	Locals      []string         `json:"locals,omitempty"`
	Defaults    []string         `json:"defaults,omitempty"`
	Unread      []string         `json:"unread,omitempty"`
	Missing     []string         `json:"missing,omitempty"`
	Extra       []envKeyJSON     `json:"extra,omitempty"`
	Empty       []envKeyJSON     `json:"empty,omitempty"`
	InCode      []envCodeRefJSON `json:"inCode,omitempty"`
	Tracked     []string         `json:"tracked,omitempty"`
	Committed   []envCommitJSON  `json:"committed,omitempty"`
	Error       string           `json:"error,omitempty"`
}

type envKeyJSON struct {
	Key  string `json:"key"`
	File string `json:"file"`
}

type envCodeRefJSON struct {
	Key  string `json:"key"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type envCommitJSON struct {
	File string    `json:"file"`
	Hash string    `json:"hash"`
	At   time.Time `json:"at,omitzero"`
}

func toEnvJSON(sets []envfiles.Set) envJSON {
	views := make([]envSetJSON, 0, len(sets))
	for _, s := range sets {
		views = append(views, toEnvSetJSON(s))
	}
	return envJSON{Sets: views}
}

func toEnvSetJSON(s envfiles.Set) envSetJSON {
	return envSetJSON{
		Name:        s.Name,
		Dir:         s.Dir,
		Attention:   envLevelNames[s.Attention()],
		Example:     s.Example,
		ExampleKeys: s.ExampleKeys,
		Locals:      s.Locals,
		Defaults:    s.Defaults,
		Unread:      s.Unread,
		Missing:     s.Missing,
		Extra:       mapSlice(s.Extra, toEnvKeyJSON),
		Empty:       mapSlice(s.Empty, toEnvKeyJSON),
		InCode:      mapSlice(s.InCode, toEnvCodeRefJSON),
		Tracked:     s.Tracked,
		Committed:   mapSlice(s.Committed, toEnvCommitJSON),
		Error:       errText(s.Err),
	}
}

func toEnvKeyJSON(k envfiles.KeyInFile) envKeyJSON {
	return envKeyJSON{Key: k.Key, File: k.File}
}

func toEnvCodeRefJSON(r envfiles.CodeRef) envCodeRefJSON {
	return envCodeRefJSON{Key: r.Key, File: r.File, Line: r.Line}
}

func toEnvCommitJSON(c envfiles.Commit) envCommitJSON {
	return envCommitJSON{File: c.File, Hash: c.Hash, At: c.At}
}
