package claudesessions

import (
	"os"
	"testing"
)

const usageTranscript = `{"type":"user","cwd":"/p","timestamp":"2026-10-01T10:00:00Z","message":{"role":"user","content":"count my tokens"}}
{"type":"assistant","cwd":"/p","timestamp":"2026-10-01T10:00:01Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"x"}],"usage":{"input_tokens":3,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000,"output_tokens":5}}}
{"type":"assistant","cwd":"/p","timestamp":"2026-10-01T10:00:02Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"text","text":"Counting."}],"usage":{"input_tokens":3,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000,"output_tokens":40}}}
{"type":"assistant","isSidechain":true,"cwd":"/p","timestamp":"2026-10-01T10:00:03Z","message":{"id":"m2","model":"claude-haiku-4-5-20251001","content":[{"type":"text","text":"subagent"}],"usage":{"input_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":50,"output_tokens":7}}}
{"type":"assistant","cwd":"/p","timestamp":"2026-10-01T10:00:04Z","message":{"id":"m3","model":"<synthetic>","content":[{"type":"text","text":"API Error"}],"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}
`

func TestReadAddsUpEachReplysUsageOnce(t *testing.T) {
	path := writeTranscript(t, t.TempDir(), StorageName("/p"), "u", usageTranscript)
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}

	want := Usage{Input: 13, CacheWrite: 100, CacheRead: 1050, Output: 47, Context: 1103}
	if s.Usage != want {
		t.Errorf("Usage = %+v, want %+v", s.Usage, want)
	}
}

func TestFollowUsageReadsOnlyWhatWasAdded(t *testing.T) {
	const halfWritten = `{"type":"assistant","cwd":"/p","message":{"id":"m4","model":"claude-opus-5-5","usage":{"input_tokens":1,`
	path := writeTranscript(t, t.TempDir(), StorageName("/p"), "u", usageTranscript+halfWritten)
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	tail := s.FollowUsage()

	appendTo(t, path, `"cache_creation_input_tokens":20,"cache_read_input_tokens":1103,"output_tokens":9}}}`+"\n")
	got, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := Usage{Input: 14, CacheWrite: 120, CacheRead: 2153, Output: 56, Context: 1124}
	if got != want {
		t.Errorf("after the reply was written, usage = %+v, want %+v", got, want)
	}
	if s.FollowUsage().counter.total != s.Usage {
		t.Error("following changed the session's own count")
	}

	if again, err := tail.Read(); err != nil || again != want {
		t.Errorf("reading with nothing added = %+v, %v", again, err)
	}
}

func TestFollowUsageStartsAgainWhenTheTranscriptShrinks(t *testing.T) {
	path := writeTranscript(t, t.TempDir(), StorageName("/p"), "u", usageTranscript)
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	tail := s.FollowUsage()

	short := `{"type":"assistant","cwd":"/p","message":{"id":"m9","model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":4}}}` + "\n"
	if err := os.WriteFile(path, []byte(short), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := tail.Read()
	if want := (Usage{Input: 2, Output: 4, Context: 2}); err != nil || got != want {
		t.Errorf("usage = %+v, %v, want %+v", got, err, want)
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
