package seed

import (
	"slices"

	"ascend/internal/domain/habit"
	"ascend/internal/domain/lifedomain"
)

// Static reference data for the demo account.

func catalogDomains() []lifedomain.LifeDomain {
	domains := []lifedomain.LifeDomain{
		{ID: "physical", Name: "Physical", Description: "Strength, body composition, sleep and energy.", Score: 82, History: []float64{71, 73, 74, 76, 77, 79, 80, 81}},
		{ID: "intellect", Name: "Intellect", Description: "Reading, languages and deliberate learning.", Score: 67, History: []float64{64, 66, 65, 67, 68, 66, 68, 69}},
		{ID: "career", Name: "Career", Description: "Shipping Monea and growing as an engineer.", Score: 74, History: []float64{62, 64, 66, 67, 69, 70, 71, 72}},
		{ID: "finance", Name: "Finance", Description: "Budget discipline, savings and runway.", Score: 58, History: []float64{55, 55, 56, 57, 56, 57, 58, 58}},
		{ID: "relationships", Name: "Relationships", Description: "Family, friends and the people who matter.", Score: 61, History: []float64{66, 65, 64, 63, 63, 62, 62, 63}},
		{ID: "personal", Name: "Personal", Description: "Reflection, rest and the systems behind everything else.", Score: 70, History: []float64{60, 62, 63, 65, 66, 67, 68, 69}},
	}
	for i := range domains {
		domains[i].History = slices.Clone(domains[i].History)
	}
	return domains
}

type habitSpec struct {
	habit habit.Habit
	// reliability is the probability of completing a due occurrence, before the current streak.
	reliability float64
	// streak is the current streak to reproduce, in scheduled occurrences.
	streak int
	// age is days since the habit was created.
	age int
	// eveningHabit leaves today's occurrence open so there's something to do.
	eveningHabit bool
}

func weekly(days ...int) habit.Schedule { return habit.Schedule{Kind: habit.Weekly, Days: days} }

var daily = habit.Schedule{Kind: habit.Daily}

func catalogHabits() []habitSpec {
	return []habitSpec{
		{habit: habit.Habit{ID: "gym", Name: "Gym", DomainID: "physical", XP: 80, Schedule: weekly(0, 1, 3, 4), Time: "07:00", Trigger: habit.TriggerWorkout}, reliability: 0.86, streak: 14, age: 190},
		{habit: habit.Habit{ID: "code", Name: "Code 60 min", DomainID: "career", XP: 100, Schedule: daily, Time: "09:00"}, reliability: 0.82, streak: 21, age: 190},
		{habit: habit.Habit{ID: "english", Name: "English practice", DomainID: "intellect", XP: 40, Schedule: weekly(0, 2, 4), Time: "13:00"}, reliability: 0.72, streak: 5, age: 120},
		{habit: habit.Habit{ID: "protein", Name: "Protein target", DomainID: "physical", XP: 30, Schedule: daily, Time: "20:00"}, reliability: 0.75, streak: 6, age: 110, eveningHabit: true},
		{habit: habit.Habit{ID: "weigh-in", Name: "Weigh-in", DomainID: "physical", XP: 20, Schedule: weekly(0, 3), Time: "06:45"}, reliability: 0.9, streak: 9, age: 110},
		{habit: habit.Habit{ID: "budget", Name: "Review budget", DomainID: "finance", XP: 30, Schedule: weekly(6), Time: "18:00"}, reliability: 0.7, streak: 3, age: 150},
		{habit: habit.Habit{ID: "read", Name: "Read 20 min", DomainID: "intellect", XP: 40, Schedule: daily, Time: "21:30"}, reliability: 0.78, streak: 8, age: 190, eveningHabit: true},
		{habit: habit.Habit{ID: "journal", Name: "Journal", DomainID: "personal", XP: 30, Schedule: daily, Time: "22:15", Trigger: habit.TriggerJournal}, reliability: 0.7, streak: 4, age: 170, eveningHabit: true},
	}
}

type focusLabel struct {
	label    string
	domainID string
}

var focusLabels = []focusLabel{
	{"Build authentication", "career"},
	{"Monea · transactions API", "career"},
	{"Monea · budget engine", "career"},
	{"System design reading", "intellect"},
	{"English listening", "intellect"},
	{"Code review backlog", "career"},
}

type lift struct {
	name string
	// base is the starting top-set weight; nil means bodyweight.
	base *float64
	step float64
	reps [2]int
}

type workoutTemplate struct {
	name  string
	lifts []lift
}

func kg(v float64) *float64 { return &v }

// Four-day upper/lower split with gentle progressive overload.
var workoutTemplates = []workoutTemplate{
	{name: "Upper A", lifts: []lift{
		{"Bench Press", kg(60), 2.5, [2]int{7, 10}},
		{"Pull Ups", nil, 0, [2]int{5, 8}},
		{"Overhead Press", kg(35), 2.5, [2]int{6, 9}},
		{"Barbell Row", kg(50), 2.5, [2]int{8, 10}},
	}},
	{name: "Lower A", lifts: []lift{
		{"Back Squat", kg(75), 2.5, [2]int{5, 8}},
		{"Romanian Deadlift", kg(65), 2.5, [2]int{8, 10}},
		{"Leg Press", kg(130), 5, [2]int{10, 12}},
		{"Standing Calf Raise", kg(50), 5, [2]int{12, 15}},
	}},
	{name: "Upper B", lifts: []lift{
		{"Incline Dumbbell Press", kg(20), 2, [2]int{8, 10}},
		{"Chin Ups", nil, 0, [2]int{6, 9}},
		{"Seated Cable Row", kg(50), 2.5, [2]int{10, 12}},
		{"Dips", nil, 0, [2]int{8, 12}},
	}},
	{name: "Lower B", lifts: []lift{
		{"Deadlift", kg(95), 5, [2]int{4, 6}},
		{"Bulgarian Split Squat", kg(16), 2, [2]int{8, 10}},
		{"Lying Leg Curl", kg(35), 2.5, [2]int{10, 12}},
	}},
}
