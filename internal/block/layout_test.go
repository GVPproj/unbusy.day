package block_test

import (
	"errors"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
)

func TestValidateLayout_IdenticalLayoutIsValid(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34} // 9:00–17:00
	current := []block.Block{
		{ID: "a", Position: 18, Span: 1},
		{ID: "b", Position: 19, Span: 2},
		{ID: "c", Position: 21, Span: 1},
	}
	proposed := []block.Placement{
		{ID: "a", Slot: 18, Span: 1},
		{ID: "b", Slot: 19, Span: 2},
		{ID: "c", Slot: 21, Span: 1},
	}
	if err := block.ValidateLayout(bounds, current, proposed); err != nil {
		t.Fatalf("identical layout: want nil, got %v", err)
	}
}

func TestValidateLayout_AcceptsMoveIntoGapAndExactFitAtEnd(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34}
	current := []block.Block{
		{ID: "a", Position: 18, Span: 1},
		{ID: "b", Position: 19, Span: 1},
	}
	proposed := []block.Placement{
		{ID: "a", Slot: 25, Span: 1}, // into a gap
		{ID: "b", Slot: 32, Span: 2}, // [32,34) — exact fit at day end
	}
	if err := block.ValidateLayout(bounds, current, proposed); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestValidateLayout_RejectsOutOfBounds(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34}
	current := []block.Block{
		{ID: "a", Position: 18, Span: 1},
		{ID: "b", Position: 19, Span: 1},
	}
	cases := map[string][]block.Placement{
		"before day start": {
			{ID: "a", Slot: 17, Span: 1},
			{ID: "b", Slot: 19, Span: 1},
		},
		"span past day end": {
			{ID: "a", Slot: 18, Span: 1},
			{ID: "b", Slot: 33, Span: 2}, // [33,35) leaks past 34
		},
		"starts at day end": {
			{ID: "a", Slot: 18, Span: 1},
			{ID: "b", Slot: 34, Span: 1},
		},
	}
	for name, proposed := range cases {
		t.Run(name, func(t *testing.T) {
			err := block.ValidateLayout(bounds, current, proposed)
			if !errors.Is(err, block.ErrOutOfBounds) {
				t.Fatalf("want ErrOutOfBounds, got %v", err)
			}
		})
	}
}

func TestValidateLayout_RejectsOverlap(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34}
	current := []block.Block{
		{ID: "a", Position: 18, Span: 1},
		{ID: "b", Position: 19, Span: 1},
	}
	cases := map[string][]block.Placement{
		"partial overlap": {
			{ID: "a", Slot: 20, Span: 2}, // [20,22)
			{ID: "b", Slot: 21, Span: 2}, // [21,23)
		},
		"full overlap": {
			{ID: "a", Slot: 20, Span: 1},
			{ID: "b", Slot: 20, Span: 1},
		},
		"contained": {
			{ID: "a", Slot: 20, Span: 4}, // [20,24)
			{ID: "b", Slot: 21, Span: 1}, // [21,22)
		},
	}
	for name, proposed := range cases {
		t.Run(name, func(t *testing.T) {
			err := block.ValidateLayout(bounds, current, proposed)
			if !errors.Is(err, block.ErrOverlap) {
				t.Fatalf("want ErrOverlap, got %v", err)
			}
		})
	}
}

func TestValidateLayout_RejectsNonPositiveSpan(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34}
	current := []block.Block{{ID: "a", Position: 18, Span: 1}}
	for _, span := range []int{0, -1} {
		proposed := []block.Placement{{ID: "a", Slot: 18, Span: span}}
		if err := block.ValidateLayout(bounds, current, proposed); !errors.Is(err, block.ErrInvalidSpan) {
			t.Fatalf("span %d: want ErrInvalidSpan, got %v", span, err)
		}
	}
}

func TestValidateLayout_RejectsBlockSetMismatch(t *testing.T) {
	bounds := block.Bounds{Start: 18, End: 34}
	current := []block.Block{
		{ID: "a", Position: 18, Span: 1},
		{ID: "b", Position: 19, Span: 1},
	}
	cases := map[string][]block.Placement{
		"missing id": {{ID: "a", Slot: 18, Span: 1}},
		"extra id": {
			{ID: "a", Slot: 18, Span: 1},
			{ID: "b", Slot: 19, Span: 1},
			{ID: "z", Slot: 20, Span: 1},
		},
		"unknown id": {
			{ID: "a", Slot: 18, Span: 1},
			{ID: "z", Slot: 19, Span: 1},
		},
		"duplicate id": {
			{ID: "a", Slot: 18, Span: 1},
			{ID: "a", Slot: 19, Span: 1},
		},
		"empty": {},
	}
	for name, proposed := range cases {
		t.Run(name, func(t *testing.T) {
			err := block.ValidateLayout(bounds, current, proposed)
			if !errors.Is(err, block.ErrNotSameBlocks) {
				t.Fatalf("want ErrNotSameBlocks, got %v", err)
			}
		})
	}
}

func TestOccupiedSlots_SingleBlock(t *testing.T) {
	got := block.OccupiedSlots([]block.Block{{ID: "a", Position: 20, Span: 1}})
	want := map[int]bool{20: true}
	if len(got) != len(want) || !got[20] {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestOccupiedSlots_MultiSpanBlock(t *testing.T) {
	got := block.OccupiedSlots([]block.Block{{ID: "a", Position: 20, Span: 3}})
	for _, s := range []int{20, 21, 22} {
		if !got[s] {
			t.Fatalf("slot %d: want occupied, got %v", s, got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("want 3 slots, got %v", got)
	}
}

// Non-positive spans floor to one occupied slot, matching spanOr1.
func TestOccupiedSlots_FloorsSpanAtOne(t *testing.T) {
	for _, span := range []int{0, -1} {
		got := block.OccupiedSlots([]block.Block{{ID: "a", Position: 20, Span: span}})
		if len(got) != 1 || !got[20] {
			t.Fatalf("span %d: want {20}, got %v", span, got)
		}
	}
}

func TestOccupiedSlots_UnionsBlocks(t *testing.T) {
	got := block.OccupiedSlots([]block.Block{
		{ID: "a", Position: 18, Span: 2}, // 18,19
		{ID: "b", Position: 22, Span: 1}, // 22
	})
	for _, s := range []int{18, 19, 22} {
		if !got[s] {
			t.Fatalf("slot %d: want occupied, got %v", s, got)
		}
	}
	if got[20] || got[21] || len(got) != 3 {
		t.Fatalf("gap leaked: got %v", got)
	}
}

func TestOccupiedSlots_Empty(t *testing.T) {
	if got := block.OccupiedSlots(nil); len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

// OccupiedEnvelope is the day's occupied extent: earliest occupied slot to the
// slot just past the latest.
func TestOccupiedEnvelope(t *testing.T) {
	cases := []struct {
		name               string
		blocks             []block.Block
		wantFirst, wantEnd int
	}{
		// No blocks: sentinels that leave the whole legal range pickable
		// (start ≤ MaxDayEnd and end ≥ MinDayStart are always true).
		{"empty", nil, block.MaxDayEnd, block.MinDayStart},
		{"single", []block.Block{{ID: "a", Position: 20, Span: 2}}, 20, 22},
		{"gap", []block.Block{
			{ID: "a", Position: 19, Span: 1},
			{ID: "b", Position: 24, Span: 2}, // ends at 26
		}, 19, 26},
		{"flush", []block.Block{
			{ID: "a", Position: block.MinDayStart, Span: 1},
			{ID: "b", Position: block.MaxDayEnd - 1, Span: 1},
		}, block.MinDayStart, block.MaxDayEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := block.OccupiedEnvelope(tc.blocks)
			if got.FirstSlot != tc.wantFirst || got.LastEnd != tc.wantEnd {
				t.Fatalf("envelope = {%d,%d}, want {%d,%d}",
					got.FirstSlot, got.LastEnd, tc.wantFirst, tc.wantEnd)
			}
		})
	}
}
