package habit_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/habit"
)

func TestSetOrder(t *testing.T) {
	t.Run("reorders the complete owned habit set", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"Read", "Walk", "Write"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		before, err := svc.List(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		order := []habit.HabitOrder{
			{ID: before[2].ID, SortOrder: 0},
			{ID: before[0].ID, SortOrder: 1},
			{ID: before[1].ID, SortOrder: 2},
		}
		snap, err := svc.SetOrder(ctx, "alice", order)
		if err != nil {
			t.Fatal(err)
		}
		wantIDs := []int64{before[2].ID, before[0].ID, before[1].ID}
		if got := idsOf(snap.Habits); !slices.Equal(got, wantIDs) {
			t.Fatalf("snapshot order = %v, want %v", got, wantIDs)
		}
		for i, h := range snap.Habits {
			if h.SortOrder != int64(i) {
				t.Fatalf("snapshot sort order[%d] = %d", i, h.SortOrder)
			}
		}
		listed, err := svc.List(ctx, "alice")
		if err != nil || !slices.Equal(idsOf(listed), wantIDs) {
			t.Fatalf("persisted order = %v, err %v", idsOf(listed), err)
		}
	})

	t.Run("new habits append after a persisted reorder", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"Read", "Walk"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := svc.List(ctx, "alice")
		if _, err := svc.SetOrder(ctx, "alice", []habit.HabitOrder{
			{ID: before[1].ID, SortOrder: 0},
			{ID: before[0].ID, SortOrder: 1},
		}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Create(ctx, "alice", "Write", "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
		after, err := svc.List(ctx, "alice")
		if err != nil || len(after) != 3 || after[2].Name != "Write" || after[2].SortOrder != 2 {
			t.Fatalf("new habit order = %+v, err %v", after, err)
		}
	})

	t.Run("rejects a foreign id without changing either owner", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		if err := svc.Create(ctx, "alice", "Read", "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
		if err := svc.Create(ctx, "bob", "Private", "2020-01-01", "UTC"); err != nil {
			t.Fatal(err)
		}
		alice, _ := svc.List(ctx, "alice")
		bob, _ := svc.List(ctx, "bob")
		_, err := svc.SetOrder(ctx, "alice", []habit.HabitOrder{{ID: bob[0].ID, SortOrder: 0}})
		if !errors.Is(err, habit.ErrForeignHabit) || !habit.IsRejection(err) {
			t.Fatalf("foreign order error = %v", err)
		}
		assertHabitState(t, svc, "alice", alice)
		assertHabitState(t, svc, "bob", bob)
	})

	t.Run("rejects duplicate ids as a different set", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"Read", "Walk"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := svc.List(ctx, "alice")
		_, err := svc.SetOrder(ctx, "alice", []habit.HabitOrder{
			{ID: before[0].ID, SortOrder: 0},
			{ID: before[0].ID, SortOrder: 1},
		})
		if !errors.Is(err, habit.ErrNotSameHabits) || !habit.IsRejection(err) {
			t.Fatalf("duplicate order error = %v", err)
		}
		assertHabitState(t, svc, "alice", before)
	})

	t.Run("accepts the existing order without writing or publishing", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		published := 0
		svc := habit.NewService(db, publisherFunc(func(habit.Event) { published++ }))
		for _, name := range []string{"Read", "Walk"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		published = 0
		before, _ := svc.List(ctx, "alice")
		order := make([]habit.HabitOrder, len(before))
		for i, h := range before {
			order[i] = habit.HabitOrder{ID: h.ID, SortOrder: h.SortOrder}
		}
		snap, err := svc.SetOrder(ctx, "alice", order)
		if err != nil || !slices.Equal(idsOf(snap.Habits), idsOf(before)) {
			t.Fatalf("same order = %+v, err %v", snap, err)
		}
		if published != 0 {
			t.Fatalf("same order published %d events", published)
		}
	})

	t.Run("rejects non-dense ranks", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"Read", "Walk"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := svc.List(ctx, "alice")
		_, err := svc.SetOrder(ctx, "alice", []habit.HabitOrder{
			{ID: before[1].ID, SortOrder: -1},
			{ID: before[0].ID, SortOrder: 8},
		})
		if !errors.Is(err, habit.ErrNotSameHabits) || !habit.IsRejection(err) {
			t.Fatalf("non-dense order error = %v", err)
		}
		assertHabitState(t, svc, "alice", before)
	})

	t.Run("deletion closes the dense rank gap", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"Read", "Walk", "Write"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := svc.List(ctx, "alice")
		if err := svc.Delete(ctx, "alice", before[1].ID); err != nil {
			t.Fatal(err)
		}
		after, err := svc.List(ctx, "alice")
		if err != nil || len(after) != 2 || after[0].SortOrder != 0 || after[1].SortOrder != 1 {
			t.Fatalf("order after delete = %+v, err %v", after, err)
		}
	})

	t.Run("serializes concurrent complete orders", func(t *testing.T) {
		db, _ := database(t)
		ctx := context.Background()
		svc := habit.NewService(db, nil)
		for _, name := range []string{"One", "Two", "Three"} {
			if err := svc.Create(ctx, "alice", name, "2020-01-01", "UTC"); err != nil {
				t.Fatal(err)
			}
		}
		hs, _ := svc.List(ctx, "alice")
		ids := idsOf(hs)
		permutations := [][]int64{
			{ids[0], ids[1], ids[2]},
			{ids[2], ids[0], ids[1]},
			{ids[1], ids[2], ids[0]},
			{ids[2], ids[1], ids[0]},
		}
		var wg sync.WaitGroup
		errs := make([]error, len(permutations))
		for i, permutation := range permutations {
			wg.Go(func() {
				order := make([]habit.HabitOrder, len(permutation))
				for j, id := range permutation {
					order[j] = habit.HabitOrder{ID: id, SortOrder: int64(j)}
				}
				_, errs[i] = habit.NewService(db, nil).SetOrder(ctx, "alice", order)
			})
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("concurrent order %d: %v", i, err)
			}
		}
		after, err := svc.List(ctx, "alice")
		if err != nil {
			t.Fatal(err)
		}
		got := idsOf(after)
		if !slices.ContainsFunc(permutations, func(p []int64) bool { return slices.Equal(got, p) }) {
			t.Fatalf("final order %v matches no submitted order", got)
		}
	})
}

func idsOf(habits []habit.Habit) []int64 {
	ids := make([]int64, len(habits))
	for i, h := range habits {
		ids[i] = h.ID
	}
	return ids
}

func assertHabitState(t *testing.T, svc *habit.Service, owner string, want []habit.Habit) {
	t.Helper()
	got, err := svc.List(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(idsOf(got), idsOf(want)) {
		t.Fatalf("%s state = %+v, want %+v", owner, got, want)
	}
}
