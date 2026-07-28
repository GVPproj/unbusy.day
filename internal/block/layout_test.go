package block_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/block"
)

func TestSlotLabel(t *testing.T) {
	cases := []struct {
		slot block.Slot
		want string
	}{
		{0, "0:00"},
		{1, "0:30"},
		{block.MinDayStart, "4:00"},
		{block.DefaultDayStart, "9:00"},
		{19, "9:30"},
		{block.MaxDayEnd, "18:00"},
	}
	for _, tc := range cases {
		if got := tc.slot.Label(); got != tc.want {
			t.Errorf("Slot(%d).Label() = %q, want %q", tc.slot, got, tc.want)
		}
	}
}

func TestSlotRange(t *testing.T) {
	if got := block.Slot(18).Range(3); got != "9:00 to 10:30" {
		t.Errorf("Range = %q, want %q", got, "9:00 to 10:30")
	}
}

func TestSlotAdd(t *testing.T) {
	if got := block.Slot(18).Add(3); got != 21 {
		t.Errorf("Add = %d, want 21", got)
	}
}

// Contains must be span-aware: the span-1 form and the span-N form agree at
// every edge (the check Create's old start-only variant got wrong).
func TestBoundsContains(t *testing.T) {
	b := block.Bounds{Start: 18, End: 34}
	cases := []struct {
		name string
		slot block.Slot
		span int
		want bool
	}{
		{"span-1 at start", 18, 1, true},
		{"span-1 at last slot", 33, 1, true},
		{"span-1 at end", 34, 1, false},
		{"span-1 before start", 17, 1, false},
		{"span-N exact fit at end", 32, 2, true},
		{"span-N leaks past end", 33, 2, false},
		{"span-N fills day", 18, 16, true},
		{"span-N overfills day", 18, 17, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.Contains(tc.slot, tc.span); got != tc.want {
				t.Errorf("Contains(%d, %d) = %v, want %v", tc.slot, tc.span, got, tc.want)
			}
		})
	}
}

func TestBoundsValid(t *testing.T) {
	cases := []struct {
		name string
		b    block.Bounds
		want bool
	}{
		{"default day", block.Bounds{Start: block.DefaultDayStart, End: block.DefaultDayEnd}, true},
		{"hard limits", block.Bounds{Start: block.MinDayStart, End: block.MaxDayEnd}, true},
		{"before 4:00", block.Bounds{Start: block.MinDayStart - 1, End: 34}, false},
		{"after 18:00", block.Bounds{Start: 18, End: block.MaxDayEnd + 1}, false},
		{"end not beyond start", block.Bounds{Start: 18, End: 18}, false},
		{"inverted", block.Bounds{Start: 20, End: 18}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.Valid(); got != tc.want {
				t.Errorf("%+v.Valid() = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}

func TestBoundsSlotsRowLen(t *testing.T) {
	b := block.Bounds{Start: 18, End: 21}
	slots := b.Slots()
	if len(slots) != 3 || slots[0] != 18 || slots[2] != 20 {
		t.Fatalf("Slots() = %v, want [18 19 20]", slots)
	}
	if got := b.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
	if got := b.Row(18); got != 1 {
		t.Errorf("Row(18) = %d, want 1 (grid rows are 1-based)", got)
	}
	if got := b.Row(20); got != 3 {
		t.Errorf("Row(20) = %d, want 3", got)
	}
}

func TestSpanOr1(t *testing.T) {
	for span, want := range map[int]int{-1: 1, 0: 1, 1: 1, 3: 3} {
		if got := block.SpanOr1(span); got != want {
			t.Errorf("SpanOr1(%d) = %d, want %d", span, got, want)
		}
	}
}

// Pin Slot.Label for every legal slot to the committed golden file. The JS
// mirror (keyboard-reducer.js timeLabel — kept separate by ADR 0005) asserts
// against the same file in jstest/keyboard-reducer.test.js, so neither
// formatter can drift silently. On mismatch the file is regenerated and the
// test fails — re-run and commit the new file.
func TestSlotLabelGoldenFile(t *testing.T) {
	labels := make(map[string]string)
	for s := block.Slot(block.MinDayStart); s <= block.MaxDayEnd; s++ {
		labels[strconv.Itoa(int(s))] = s.Label()
	}
	want, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want = append(want, '\n')
	path := filepath.Join("testdata", "slot_labels.json")
	if got, err := os.ReadFile(path); err == nil && string(got) == string(want) {
		return
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	t.Fatalf("%s drifted from Slot.Label; regenerated — verify and commit it", path)
}

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
	want := map[block.Slot]bool{20: true}
	if len(got) != len(want) || !got[20] {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestOccupiedSlots_MultiSpanBlock(t *testing.T) {
	got := block.OccupiedSlots([]block.Block{{ID: "a", Position: 20, Span: 3}})
	for _, s := range []block.Slot{20, 21, 22} {
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
	for _, s := range []block.Slot{18, 19, 22} {
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
		wantFirst, wantEnd block.Slot
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
