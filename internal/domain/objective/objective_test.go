package objective_test

import (
	"fmt"
	"testing"

	"ascend/internal/domain/objective"
	"ascend/internal/domain/shared"
)

const today shared.LocalDate = "2026-09-24"

func ids() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("m%d", n) }
}

func TestMetricObjectiveCompletesAndReopens(t *testing.T) {
	o, err := objective.New("o", objective.Draft{
		Name: "Reach 65 kg", DomainID: "physical", Deadline: "2026-12-31",
		Metric: &objective.Metric{Unit: " kg ", Start: 56, Current: 60, Target: 65},
	}, today, ids())
	if err != nil {
		t.Fatal(err)
	}
	if o.Metric.Unit != "kg" {
		t.Fatalf("unit = %q", o.Metric.Unit)
	}
	done, err := objective.RecordMetricValue(o, 65.123, today)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != objective.Completed || done.CompletedAt != today || done.Metric.Current != 65.12 {
		t.Fatalf("after target: %+v %+v", done, done.Metric)
	}
	if o.Metric.Current != 60 {
		t.Fatal("RecordMetricValue mutated the original")
	}
	back, _ := objective.RecordMetricValue(done, 62, today)
	if back.Status != objective.Active || back.CompletedAt != "" {
		t.Fatalf("after dropping below target: %+v", back)
	}
}

func TestMilestoneObjective(t *testing.T) {
	o, err := objective.New("o", objective.Draft{
		Name: "Ship", DomainID: "career", Deadline: "2026-12-31", Milestones: []string{"A", " ", "B"},
	}, today, ids())
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Milestones) != 2 {
		t.Fatalf("milestones = %+v", o.Milestones)
	}
	o, _ = objective.ToggleMilestone(o, "m1", today)
	if objective.Progress(o) != 0.5 || o.Status != objective.Active {
		t.Fatalf("progress = %v", objective.Progress(o))
	}
	o, _ = objective.ToggleMilestone(o, "m2", today)
	if o.Status != objective.Completed {
		t.Fatalf("status = %s", o.Status)
	}
	if _, err := objective.ToggleMilestone(o, "missing", today); !shared.IsNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]objective.Draft{
		"no name":         {DomainID: "d", Deadline: "2026-12-31", Milestones: []string{"a"}},
		"bad deadline":    {Name: "x", DomainID: "d", Deadline: "2026-13-40", Milestones: []string{"a"}},
		"nothing to meas": {Name: "x", DomainID: "d", Deadline: "2026-12-31"},
		"same start":      {Name: "x", DomainID: "d", Deadline: "2026-12-31", Metric: &objective.Metric{Unit: "kg", Start: 1, Target: 1}},
	}
	for name, d := range cases {
		if err := objective.Validate(d); !shared.IsValidation(err) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestOnTrack(t *testing.T) {
	o := objective.Objective{
		Status: objective.Active, CreatedAt: "2026-09-01", Deadline: "2026-10-01",
		Milestones: []objective.Milestone{{Done: true}, {Done: false}},
	}
	if !objective.IsOnTrack(o, "2026-09-10") {
		t.Fatal("50% done a third of the way in should be on track")
	}
	if objective.IsOnTrack(o, "2026-09-28") {
		t.Fatal("50% done near the deadline should be behind")
	}
}
