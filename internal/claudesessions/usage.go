package claudesessions

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
)

// Usage is the tokens a session's replies used, as the API reported them, subagents' included.
type Usage struct {
	Input      int64
	CacheWrite int64
	CacheRead  int64
	Output     int64
	// Context is the size of the conversation at the latest reply: all it sent, cached or not.
	Context int64
}

// IsZero reports whether no reply recorded its usage.
func (u Usage) IsZero() bool {
	return u == Usage{}
}

func (u *Usage) add(api apiUsage, sign int64) {
	u.Input += sign * api.InputTokens
	u.CacheWrite += sign * api.CacheCreationInputTokens
	u.CacheRead += sign * api.CacheReadInputTokens
	u.Output += sign * api.OutputTokens
}

// apiUsage is the usage the API reports with a reply.
type apiUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
}

func (u apiUsage) context() int64 {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// usageCounter counts each reply once, by its message id: Claude Code repeats a reply's usage on
// the line of every block of its content, and the last line's is the final one.
type usageCounter struct {
	replies map[string]apiUsage
	total   Usage
}

func (c *usageCounter) add(rec record) {
	api := rec.Message.Usage
	if rec.Type != "assistant" || api == nil || rec.Message.ID == "" || rec.Message.Model == syntheticModel {
		return
	}
	if c.replies == nil {
		c.replies = map[string]apiUsage{}
	}
	c.total.add(c.replies[rec.Message.ID], -1)
	c.total.add(*api, 1)
	c.replies[rec.Message.ID] = *api
	if !rec.IsSidechain {
		c.total.Context = api.context()
	}
}

// UsageTail follows the usage of a transcript that is still being written, reading only the
// lines added since it last read.
type UsageTail struct {
	path string
	// offset is the end of the last whole line read.
	offset  int64
	counter usageCounter
}

// FollowUsage follows the session's usage from where Read left off.
func (s Session) FollowUsage() *UsageTail {
	tail := s.usageTail
	tail.counter.replies = maps.Clone(tail.counter.replies)
	return &tail
}

// Read reads the lines added since the last read and returns the usage so far. A line still
// being written is left for the next read, and a transcript that shrank is read again from the
// start.
func (t *UsageTail) Read() (Usage, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return t.counter.total, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return t.counter.total, err
	}
	if info.Size() < t.offset {
		t.offset, t.counter = 0, usageCounter{}
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return t.counter.total, err
	}
	r := bufio.NewReaderSize(f, readBufferBytes)
	var line []byte
	for {
		var size int64
		var tooLong bool
		line, size, tooLong, err = readLine(r, line[:0])
		if errors.Is(err, io.EOF) {
			return t.counter.total, nil
		}
		if err != nil {
			return t.counter.total, err
		}
		t.offset += size
		var rec record
		if !tooLong && json.Unmarshal(line, &rec) == nil {
			t.counter.add(rec)
		}
	}
}
