package training_test

import (
	"testing"

	"ascend/internal/domain/shared"
	"ascend/internal/domain/training"
)

func kg(v float64) *float64 { return &v }

func workout(id string, date shared.LocalDate, sets ...training.Set) training.Workout {
	return training.Workout{ID: id, Name: "Upper", Date: date, Status: training.Completed, Exercises: []training.Exercise{{ID: "e", Name: "Bench", Sets: sets}}}
}

func TestPersonalRecordsRankLoadedByOneRepMax(t *testing.T) {
	history := []training.Workout{
		workout("a", "2026-09-01", training.Set{ID: "1", Weight: kg(60), Reps: 10}),
		workout("b", "2026-09-08", training.Set{ID: "2", Weight: kg(70), Reps: 2}),
	}
	records := training.PersonalRecords(history)
	if len(records) != 1 || *records[0].Weight != 60 || records[0].EstimatedOneRepMax != 80 {
		t.Fatalf("records = %+v", records)
	}
}

func TestIsRecordSet(t *testing.T) {
	history := []training.Workout{workout("a", "2026-09-01", training.Set{ID: "1", Weight: kg(60), Reps: 10})}
	current := workout("b", "2026-09-08")
	if !training.IsRecordSet(history, current, "Bench", training.Set{Weight: kg(65), Reps: 10}) {
		t.Fatal("65×10 should beat 60×10")
	}
	if training.IsRecordSet(history, current, "Bench", training.Set{Weight: kg(55), Reps: 10}) {
		t.Fatal("55×10 should not be a record")
	}
	if training.IsRecordSet(history, current, "Squat", training.Set{Weight: kg(100), Reps: 5}) {
		t.Fatal("a first-ever exercise is not a record")
	}
}

func TestLogSetAndComplete(t *testing.T) {
	w := training.Workout{ID: "w", Name: "Upper", Date: "2026-09-24", Status: training.InProgress, Exercises: []training.Exercise{{ID: "e", Name: "Bench", Sets: []training.Set{}}}}
	if _, err := training.Complete(w, 60); !shared.IsValidation(err) {
		t.Fatalf("empty workout err = %v", err)
	}
	if _, err := training.LogSet(w, "e", training.Set{ID: "s", Reps: 0}); !shared.IsValidation(err) {
		t.Fatalf("zero reps err = %v", err)
	}
	logged, err := training.LogSet(w, "e", training.Set{ID: "s", Weight: kg(60), Reps: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Exercises[0].Sets) != 0 {
		t.Fatal("LogSet mutated the original workout")
	}
	done, err := training.Complete(logged, 55)
	if err != nil || done.Status != training.Completed || *done.DurationMinutes != 55 {
		t.Fatalf("done = %+v, %v", done, err)
	}
	if training.Volume(done) != 480 {
		t.Fatalf("volume = %v", training.Volume(done))
	}
	if _, err := training.LogSet(done, "e", training.Set{ID: "t", Reps: 5}); !shared.IsValidation(err) {
		t.Fatalf("logging on a finished workout err = %v", err)
	}
}
