package claudesessions

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	transcriptExt = ".jsonl"
	interrupted   = "[Request interrupted"
	humanOrigin   = "human"
	// detachedHead is what Claude Code records outside a branch, including outside a repository.
	detachedHead = "HEAD"
	// maxRecordBytes skips transcript lines bigger than any prompt or reply, such as a tool's
	// output with screenshots, instead of holding them whole.
	maxRecordBytes = 16 << 20
)

// record is the part of a transcript line that describes the session. Lines of other types, and
// fields not listed, are skipped.
type record struct {
	Type             string    `json:"type"`
	IsMeta           bool      `json:"isMeta"`
	IsSidechain      bool      `json:"isSidechain"`
	IsCompactSummary bool      `json:"isCompactSummary"`
	ToolUseResult    presence  `json:"toolUseResult"`
	Origin           origin    `json:"origin"`
	Cwd              string    `json:"cwd"`
	GitBranch        string    `json:"gitBranch"`
	Timestamp        time.Time `json:"timestamp"`
	Message          message   `json:"message"`
	CustomTitle      string    `json:"customTitle"`
	AITitle          string    `json:"aiTitle"`
	RelocatedCwd     string    `json:"relocatedCwd"`
	PRURL            string    `json:"prUrl"`
}

type origin struct {
	Kind string `json:"kind"`
}

type message struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// presence records whether a field is set without keeping its value, which can be large.
type presence bool

func (p *presence) UnmarshalJSON(data []byte) error {
	*p = string(data) != "null"
	return nil
}

// Read reads one transcript. Lines that are not valid JSON, such as one still being written, and
// lines over maxRecordBytes are skipped.
func Read(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Session{}, err
	}

	b := builder{session: Session{
		ID:   strings.TrimSuffix(filepath.Base(path), transcriptExt),
		File: path,
		Size: info.Size(),
	}}
	r := bufio.NewReader(f)
	var line []byte
	for {
		var tooLong bool
		line, tooLong, err = readLine(r, line[:0])
		var rec record
		if !tooLong && json.Unmarshal(line, &rec) == nil {
			b.add(rec)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Session{}, err
		}
	}
	return b.finish(filepath.Base(filepath.Dir(path)), info.ModTime()), nil
}

// readLine appends the next line to buf, unless it is over maxRecordBytes, which it reads past.
func readLine(r *bufio.Reader, buf []byte) (line []byte, tooLong bool, err error) {
	for {
		chunk, err := r.ReadSlice('\n')
		tooLong = tooLong || len(buf)+len(chunk) > maxRecordBytes
		if !tooLong {
			buf = append(buf, chunk...)
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return buf, tooLong, err
		}
	}
}

// builder gathers a session from its records, in file order.
type builder struct {
	session       Session
	cwds          []string
	relocatedCwd  string
	customTitle   string
	aiTitle       string
	models        map[string]int
	lastMessageID string
}

func (b *builder) add(rec record) {
	switch rec.Type {
	case "custom-title":
		b.customTitle = rec.CustomTitle
	case "ai-title":
		b.aiTitle = rec.AITitle
	case "relocated":
		b.relocatedCwd = rec.RelocatedCwd
	case "pr-link":
		if rec.PRURL != "" && !slices.Contains(b.session.PRs, rec.PRURL) {
			b.session.PRs = append(b.session.PRs, rec.PRURL)
		}
	case "user":
		b.addPrompt(rec)
	case "assistant":
		b.addReply(rec)
	}
}

func (b *builder) addPrompt(rec record) {
	if rec.IsSidechain || rec.IsMeta || rec.IsCompactSummary || bool(rec.ToolUseResult) {
		return
	}
	if rec.Origin.Kind != "" && rec.Origin.Kind != humanOrigin {
		return
	}
	text := oneLine(textOf(rec.Message.Content))
	if text == "" || isMachineText(text) {
		return
	}
	b.noteContext(rec)
	s := &b.session
	if s.Prompts == 0 {
		s.FirstPrompt = text
	}
	s.LastPrompt = text
	s.Prompts++
	s.Turns = append(s.Turns, Turn{Yours: true, Text: text})
}

func (b *builder) addReply(rec record) {
	if rec.IsSidechain {
		return
	}
	b.noteContext(rec)
	if rec.Message.Model != "" && rec.Message.Model != syntheticModel && rec.Message.ID != b.lastMessageID {
		if b.models == nil {
			b.models = map[string]int{}
		}
		b.models[rec.Message.Model]++
	}
	b.lastMessageID = rec.Message.ID
	if text := oneLine(textOf(rec.Message.Content)); text != "" {
		b.session.Turns = append(b.session.Turns, Turn{Text: text})
	}
}

// noteContext records the folder, branch and time of a prompt or reply.
func (b *builder) noteContext(rec record) {
	s := &b.session
	if rec.Cwd != "" && !slices.Contains(b.cwds, rec.Cwd) {
		b.cwds = append(b.cwds, rec.Cwd)
	}
	if rec.GitBranch != "" && rec.GitBranch != detachedHead {
		s.Branch = rec.GitBranch
		if !slices.Contains(s.Branches, rec.GitBranch) {
			s.Branches = append(s.Branches, rec.GitBranch)
		}
	}
	if rec.Timestamp.IsZero() {
		return
	}
	if s.Started.IsZero() || rec.Timestamp.Before(s.Started) {
		s.Started = rec.Timestamp
	}
	if rec.Timestamp.After(s.LastActive) {
		s.LastActive = rec.Timestamp
	}
}

func (b *builder) finish(storageDir string, modified time.Time) Session {
	s := b.session
	s.Dir = resumeDir(storageDir, b.relocatedCwd, b.cwds)
	s.Title = b.customTitle
	if s.Title == "" {
		s.Title = b.aiTitle
	}
	if s.LastActive.IsZero() {
		s.LastActive = modified
	}
	for id, replies := range b.models {
		s.Models = append(s.Models, ModelUse{ID: id, Replies: replies})
	}
	slices.SortFunc(s.Models, func(a, b ModelUse) int {
		if a.Replies != b.Replies {
			return b.Replies - a.Replies
		}
		return strings.Compare(a.ID, b.ID)
	})
	return s
}

// resumeDir is the folder claude --resume finds the session from: the one Claude Code named the
// session's storage folder after. The session may have moved on to other folders, and a
// relocated session is stored under its new one.
func resumeDir(storageDir, relocatedCwd string, cwds []string) string {
	candidates := slices.Concat([]string{relocatedCwd}, cwds)
	for _, dir := range candidates {
		if dir != "" && StorageName(dir) == storageDir {
			return dir
		}
	}
	for _, dir := range candidates {
		if dir != "" {
			return dir
		}
	}
	return ""
}

// StorageName is the name Claude Code gives the folder holding a directory's sessions: the path
// with every character other than a letter or digit replaced by a dash.
func StorageName(dir string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, dir)
}

// textOf joins the text of a message's content, which is either a string or a list of blocks.
func textOf(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var blocks []contentBlock
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// isMachineText reports whether a user message was written by Claude Code rather than typed: a
// slash command's echo, a shell command's output or a background task's notice, all of which come
// wrapped in a tag such as <command-name>...</command-name>, or the note left when a reply is
// interrupted.
func isMachineText(text string) bool {
	if strings.HasPrefix(text, interrupted) {
		return true
	}
	tag, ok := strings.CutPrefix(text, "<")
	if !ok {
		return false
	}
	name, _, opened := strings.Cut(tag, ">")
	return opened && isTagName(name) && strings.Contains(text, "</"+name+">")
}

// isTagName accepts the lowercase, dashed names of the tags Claude Code wraps its messages in.
func isTagName(name string) bool {
	return name != "" && !strings.ContainsFunc(name, func(r rune) bool { return (r < 'a' || r > 'z') && r != '-' })
}
