package block

import "errors"

// Bounds is a Day Plan's extent in 30-minute slot indexes counted from 00:00;
// the day covers slots [Start, End).
type Bounds struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Day Plan slot constants: 30-minute slot indexes counted from 00:00.
// Hard limits 4:00–18:00 for now; default day is 9:00–17:00.
const (
	MinDayStart     = 8  // 4:00
	MaxDayEnd       = 36 // 18:00
	DefaultDayStart = 18 // 9:00
	DefaultDayEnd   = 34 // 17:00
)

// ErrNotSameBlocks signals a proposed layout whose ids are not exactly the
// owner's current block set (missing, unknown, or duplicate id).
var ErrNotSameBlocks = errors.New("layout is not the owner's current block set")

// ErrOutOfBounds signals a run [slot, slot+span) outside the day's bounds.
var ErrOutOfBounds = errors.New("block placed outside the day's bounds")

// ErrOverlap signals two blocks' runs sharing at least one slot.
var ErrOverlap = errors.New("blocks overlap")

// ErrInvalidBounds signals bounds outside the hard limits or inverted.
var ErrInvalidBounds = errors.New("bounds outside 4:00–18:00 or end not after start")

// ErrBoundsOccupied signals a shrink that would strand a block outside the day.
var ErrBoundsOccupied = errors.New("bounds change strands a block outside the day")

// Placement is one block's proposed run of slots: [Slot, Slot+Span).
type Placement struct {
	ID   string `json:"id"`
	Slot int    `json:"slot"`
	Span int    `json:"span"`
}

// OccupiedSlots returns the set of slot indices covered by any block, each
// block claiming [Position, Position+span) with span floored at 1.
func OccupiedSlots(cs []Block) map[int]bool {
	occupied := make(map[int]bool)
	for _, c := range cs {
		span := c.Span
		if span < 1 {
			span = 1
		}
		for s := c.Position; s < c.Position+span; s++ {
			occupied[s] = true
		}
	}
	return occupied
}

// Envelope is the occupied extent of a day's blocks in slot indexes: FirstSlot
// is the earliest slot any block occupies, LastEnd the slot just past the
// latest. A working-hours window is fittable iff start ≤ FirstSlot and
// end ≥ LastEnd. With no blocks there is no footprint, so the envelope collapses
// to (MaxDayEnd, MinDayStart) — sentinels that leave the whole legal range
// pickable. JSON tags match the Datastar signal names the modal binds to.
type Envelope struct {
	FirstSlot int `json:"firstOccupiedSlot"`
	LastEnd   int `json:"lastOccupiedEnd"`
}

// OccupiedEnvelope computes the occupied extent of the owner's blocks (span
// floored at 1, matching OccupiedSlots). Pure — no DB or transport dependency.
func OccupiedEnvelope(cs []Block) Envelope {
	if len(cs) == 0 {
		return Envelope{FirstSlot: MaxDayEnd, LastEnd: MinDayStart}
	}
	first, last := MaxDayEnd, MinDayStart
	for _, c := range cs {
		span := c.Span
		if span < 1 {
			span = 1
		}
		if c.Position < first {
			first = c.Position
		}
		if c.Position+span > last {
			last = c.Position + span
		}
	}
	return Envelope{FirstSlot: first, LastEnd: last}
}

// ValidateLayout checks a proposed full layout against the Day Plan invariants
// (ADR 0005): same block set as current, every run within bounds, no overlaps.
// Pure — no DB or transport dependencies.
func ValidateLayout(bounds Bounds, current []Block, proposed []Placement) error {
	if len(proposed) != len(current) {
		return ErrNotSameBlocks
	}
	ids := make(map[string]struct{}, len(current))
	for _, c := range current {
		ids[c.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(proposed))
	for _, p := range proposed {
		if _, ok := ids[p.ID]; !ok {
			return ErrNotSameBlocks
		}
		if _, dup := seen[p.ID]; dup {
			return ErrNotSameBlocks
		}
		seen[p.ID] = struct{}{}
		if p.Span < 1 {
			return ErrInvalidSpan
		}
		if p.Slot < bounds.Start || p.Slot+p.Span > bounds.End {
			return ErrOutOfBounds
		}
	}
	// Mark each occupied slot; a second claim on any slot is an overlap.
	occupied := make(map[int]struct{})
	for _, p := range proposed {
		for s := p.Slot; s < p.Slot+p.Span; s++ {
			if _, taken := occupied[s]; taken {
				return ErrOverlap
			}
			occupied[s] = struct{}{}
		}
	}
	return nil
}
