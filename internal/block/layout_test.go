package block_test

import (
	"errors"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
)

// A proposed layout identical to the current one is always valid.
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

// Moving a block into an empty gap and an exact fit at the day's end are valid.
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

// A run before the day's start or past its end is out of bounds.
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

// Two blocks' runs may not share a slot, partially or fully.
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

// A zero or negative span is invalid before any bounds/overlap reasoning.
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

// A layout that drops, invents, or repeats an id is not the same block set.
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
