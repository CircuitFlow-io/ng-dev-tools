package macos

import (
	"context"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

func TestLastUsedDatesParsesNULSeparatedValues(t *testing.T) {
	runner := &macostest.Runner{Outputs: map[string]string{
		"mdls -name kMDItemLastUsedDate -raw /A.app /B.app /C.app": "2025-01-02 10:00:00 +0000\x00(null)\x002026-09-01 08:30:00 +0000",
	}}

	dates, err := LastUsedDates(context.Background(), runner, []string{"/A.app", "/B.app", "/C.app"})
	if err != nil {
		t.Fatal(err)
	}

	if want := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC); !dates["/A.app"].Equal(want) {
		t.Errorf("A = %v, want %v", dates["/A.app"], want)
	}
	if _, ok := dates["/B.app"]; ok {
		t.Error("B has a date, want none for (null)")
	}
	if dates["/C.app"].IsZero() {
		t.Error("C has no date")
	}
}

func TestParseSimulators(t *testing.T) {
	data := []byte(`{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-18-6":[
		{"udid":"U1","name":"iPhone 16","dataPath":"/d/U1/data","isAvailable":false,"lastUsedAt":"2025-01-01T00:00:00Z"}]}}`)

	sims, err := ParseSimulators(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(sims) != 1 || sims[0].UDID != "U1" || sims[0].IsAvailable || sims[0].RuntimeID != "com.apple.CoreSimulator.SimRuntime.iOS-18-6" {
		t.Errorf("unexpected simulators: %+v", sims)
	}
}

func TestParseSimRuntimes(t *testing.T) {
	data := []byte(`{"X":{"identifier":"X","runtimeIdentifier":"com.apple.CoreSimulator.SimRuntime.iOS-18-6","version":"18.6","sizeBytes":8838255185,"deletable":true,"lastUsedAt":"2025-09-10T08:08:38Z"}}`)

	runtimes, err := ParseSimRuntimes(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(runtimes) != 1 || runtimes[0].SizeBytes != 8838255185 || !runtimes[0].Deletable {
		t.Errorf("unexpected runtimes: %+v", runtimes)
	}
}
