package block

import (
	"errors"
	"strconv"
)

// Slot is a 30-minute interval index counted from 00:00.
type Slot int

// Label renders the slot as clock text ("9:00", "9:30").
func (s Slot) Label() string {
	if s%2 == 1 {
		return strconv.Itoa(int(s)/2) + ":30"
	}
	return strconv.Itoa(int(s)/2) + ":00"
}

// Range renders the clock span a placement covers ("9:00 to 10:30").
func (s Slot) Range(span int) string {
	return s.Label() + " to " + s.Add(span).Label()
}

func (s Slot) Add(span int) Slot { return s + Slot(span) }

// Bounds is a Day Plan's extent in 30-minute slot indexes counted from 00:00;
// the day covers slots [Start, End).
type Bounds struct {
	Start Slot `json:"start"`
	End   Slot `json:"end"`
}

// Contains reports whether the run [s, s+span) lies inside the day.
func (b Bounds) Contains(s Slot, span int) bool {
	return s >= b.Start && s.Add(span) <= b.End
}

// Valid reports whether the bounds sit inside the hard limits with End > Start.
func (b Bounds) Valid() bool {
	return b.Start >= MinDayStart && b.End <= MaxDayEnd && b.End > b.Start
}

// Slots is the day's slot run in render order.
func (b Bounds) Slots() []Slot {
	out := make([]Slot, 0, b.Len())
	for s := b.Start; s < b.End; s++ {
		out = append(out, s)
	}
	return out
}

// Row is the 1-based CSS grid row a slot paints on.
func (b Bounds) Row(s Slot) int { return int(s-b.Start) + 1 }

func (b Bounds) Len() int { return int(b.End - b.Start) }

// Day Plan slot constants: 30-minute slot indexes counted from 00:00.
// Hard limits 4:00–18:00 for now; default day is 9:00–17:00. The DB column
// defaults in the baseline migration restate DefaultDayStart/End; the
// bounds-round-trip test in block_test.go pins the two in agreement.
const (
	MinDayStart     = 8  // 4:00
	MaxDayEnd       = 36 // 18:00
	DefaultDayStart = 18 // 9:00
	DefaultDayEnd   = 34 // 17:00
)

var ErrNotSameBlocks = errors.New("layout is not the owner's current block set")
var ErrOutOfBounds = errors.New("block placed outside the day's bounds")
var ErrOverlap = errors.New("blocks overlap")
var ErrInvalidBounds = errors.New("bounds outside 4:00–18:00 or end not after start")
var ErrBoundsOccupied = errors.New("bounds change strands a block outside the day")

// Placement is one block's proposed run of slots: [Slot, Slot+Span).
type Placement struct {
	ID   string `json:"id"`
	Slot Slot   `json:"slot"`
	Span int    `json:"span"`
}

// SpanOr1 is the one span floor: a stored span below 1 renders and occupies as 1.
func SpanOr1(span int) int {
	if span < 1 {
		return 1
	}
	return span
}

// OccupiedSlots returns the set of slot indices covered by any block.
func OccupiedSlots(cs []Block) map[Slot]bool {
	occupied := make(map[Slot]bool)
	for _, c := range cs {
		for s := c.Position; s < c.Position.Add(SpanOr1(c.Span)); s++ {
			occupied[s] = true
		}
	}
	return occupied
}

// Envelope is the occupied extent of a day's blocks. With no blocks it
// collapses to (MaxDayEnd, MinDayStart) — sentinels that leave the whole legal
// range pickable. JSON tags match the Datastar signal names the modal binds to.
type Envelope struct {
	FirstSlot Slot `json:"firstOccupiedSlot"`
	LastEnd   Slot `json:"lastOccupiedEnd"`
}

func OccupiedEnvelope(cs []Block) Envelope {
	if len(cs) == 0 {
		return Envelope{FirstSlot: MaxDayEnd, LastEnd: MinDayStart}
	}
	first, last := Slot(MaxDayEnd), Slot(MinDayStart)
	for _, c := range cs {
		if c.Position < first {
			first = c.Position
		}
		if end := c.Position.Add(SpanOr1(c.Span)); end > last {
			last = end
		}
	}
	return Envelope{FirstSlot: first, LastEnd: last}
}

// ValidateLayout checks a proposed layout against the invariants (ADR 0005):
// same block set as current, span ≥ 1, in bounds, no overlaps.
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
		if !bounds.Contains(p.Slot, p.Span) {
			return ErrOutOfBounds
		}
	}
	occupied := make(map[Slot]struct{})
	for _, p := range proposed {
		for s := p.Slot; s < p.Slot.Add(p.Span); s++ {
			if _, taken := occupied[s]; taken {
				return ErrOverlap
			}
			occupied[s] = struct{}{}
		}
	}
	return nil
}
