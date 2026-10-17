package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// inNonUTCZone runs body with time.Local pinned to a fixed non-UTC zone, which
// is where a naive implementation of "now" goes wrong. Argentina is UTC-3, so an
// untagged local time is three hours off from the instant it claims to be.
func inNonUTCZone(t *testing.T, offsetHours int, body func()) {
	t.Helper()

	original := time.Local
	time.Local = time.FixedZone("TEST", offsetHours*60*60)
	t.Cleanup(func() { time.Local = original })

	body()
}

// A scheduled review date is an instant. It must carry its zone, so that
// serialising it cannot produce a string that claims the wrong one.
func TestDummyFSRS_ReturnsUTCInstants(t *testing.T) {
	inNonUTCZone(t, -3, func() {
		next := domain.DummyFSRS{}.CalculateNextState(domain.CardState{
			FlashcardID: "f1",
			Stability:   1.0,
			NextReview:  time.Now(),
			LastReview:  time.Now(),
		}, 3)

		assert.Equal(t, time.UTC, next.NextReview.Location(),
			"NextReview must be a UTC instant, got zone %v", next.NextReview.Location())
		assert.Equal(t, time.UTC, next.LastReview.Location(),
			"LastReview must be a UTC instant, got zone %v", next.LastReview.Location())
	})
}

// The scheduling decision must be the same instant regardless of the machine's
// timezone. A local-time implementation drifts by the offset.
func TestDummyFSRS_SchedulesTheSameInstantInAnyZone(t *testing.T) {
	var inUTC time.Time
	var inArgentina time.Time

	grade := 3
	base := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)

	func() {
		original := time.Local
		time.Local = time.UTC
		t.Cleanup(func() { time.Local = original })
		inUTC = domain.DummyFSRS{}.CalculateNextState(domain.CardState{NextReview: base, LastReview: base}, grade).NextReview
	}()

	inNonUTCZone(t, -3, func() {
		inArgentina = domain.DummyFSRS{}.CalculateNextState(domain.CardState{NextReview: base, LastReview: base}, grade).NextReview
	})

	// Both are "now + 3 days", evaluated seconds apart, so compare with tolerance.
	require.WithinDuration(t, inUTC, inArgentina, 2*time.Second,
		"the scheduled instant must not depend on the machine timezone")
}
