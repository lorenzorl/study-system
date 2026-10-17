package domain

import "time"

// InstantPrecision is the resolution at which instants are persisted.
//
// Stored timestamps are encoded as RFC3339, which carries whole seconds. An
// instant created with sub-second precision therefore does not survive a round
// trip: the caller is told 16:14:44.163922 while the database holds 16:14:44.
// Creating instants at this precision makes the reported value and the stored
// value the same one, which is a far stronger property than "close enough".
//
// The fixed width matters for a second reason, and this one is load-bearing:
// the due-cards query filters with `next_review <= ?` against a bound string, so
// it relies on lexicographic comparison. That comparison is only correct because
// every stored value shares one layout. Storing RFC3339Nano would break it:
// "16:14:44.1Z" sorts *before* "16:14:44Z", inverting the order of two instants
// a tenth of a second apart.
const InstantPrecision = time.Second

// Now returns the current instant at the persisted precision, in UTC.
//
// Every stored timestamp goes through this function, so there is one place that
// decides what precision an instant has rather than one per call site.
func Now() time.Time {
	return time.Now().UTC().Truncate(InstantPrecision)
}
