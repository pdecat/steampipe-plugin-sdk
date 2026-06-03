package telemetry

import (
	"os"
	"sync"
	"testing"
)

// resetRowSpanSampleRatio clears the lazily-resolved ratio so a test
// re-resolves it from the current environment.
func resetRowSpanSampleRatio() {
	rowSpanSampleRatioOnce = sync.Once{}
	rowSpanSampleRatioVal = 0
}

// TestRowSpanSampleRatioResolvedLazily guards against resolving the ratio in a
// package-level var initialiser: an env var set after package load (init has
// long since run) must be honoured by the first call. This is the situation in
// a dlopen'd host process, where the Go env is only populated from the host's
// C environ after dependency packages have initialised.
func TestRowSpanSampleRatioResolvedLazily(t *testing.T) {
	resetRowSpanSampleRatio()
	t.Setenv(EnvOtelRowSpanSampleRatio, "0.25")
	if got := RowSpanSampleRatio(); got != 0.25 {
		t.Fatalf("RowSpanSampleRatio() = %v, want 0.25", got)
	}
}

// TestRowSpanSampleRatioResolvedOnce documents the resolve-once semantics: the
// first resolved value is cached and later env changes are ignored.
func TestRowSpanSampleRatioResolvedOnce(t *testing.T) {
	resetRowSpanSampleRatio()
	t.Setenv(EnvOtelRowSpanSampleRatio, "0.25")
	if got := RowSpanSampleRatio(); got != 0.25 {
		t.Fatalf("RowSpanSampleRatio() = %v, want 0.25", got)
	}
	t.Setenv(EnvOtelRowSpanSampleRatio, "0.75")
	if got := RowSpanSampleRatio(); got != 0.25 {
		t.Fatalf("RowSpanSampleRatio() after env change = %v, want cached 0.25", got)
	}
}

func TestRowSpanSampleRatioResolution(t *testing.T) {
	tests := []struct {
		name  string
		value string
		unset bool
		want  float64
	}{
		{name: "unset defaults to 1", unset: true, want: 1.0},
		{name: "empty defaults to 1", value: "", want: 1.0},
		{name: "unparseable defaults to 1", value: "not-a-float", want: 1.0},
		{name: "zero disables", value: "0", want: 0},
		{name: "fraction", value: "0.1", want: 0.1},
		{name: "whitespace trimmed", value: " 0.5 ", want: 0.5},
		{name: "clamped above", value: "2", want: 1.0},
		{name: "clamped below", value: "-0.5", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRowSpanSampleRatio()
			// register restoration of the original value, then apply the case
			t.Setenv(EnvOtelRowSpanSampleRatio, tt.value)
			if tt.unset {
				os.Unsetenv(EnvOtelRowSpanSampleRatio)
			}
			if got := RowSpanSampleRatio(); got != tt.want {
				t.Fatalf("RowSpanSampleRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSampleRowSpanBoundaries(t *testing.T) {
	resetRowSpanSampleRatio()
	t.Setenv(EnvOtelRowSpanSampleRatio, "0")
	if sampleRowSpan() {
		t.Fatal("sampleRowSpan() = true with ratio 0, want false")
	}

	resetRowSpanSampleRatio()
	t.Setenv(EnvOtelRowSpanSampleRatio, "1")
	if !sampleRowSpan() {
		t.Fatal("sampleRowSpan() = false with ratio 1, want true")
	}
}
