package seed

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"ascend/internal/domain/activity"
	"ascend/internal/domain/focus"
	"ascend/internal/domain/habit"
	"ascend/internal/domain/journal"
	"ascend/internal/domain/journey"
	"ascend/internal/domain/lifedomain"
	"ascend/internal/domain/objective"
	"ascend/internal/domain/progression"
	"ascend/internal/domain/shared"
	"ascend/internal/domain/task"
	"ascend/internal/domain/training"
)

func minutesOf(clock string) int {
	h, m, _ := strings.Cut(clock, ":")
	hours, _ := strconv.Atoi(h)
	minutes, _ := strconv.Atoi(m)
	return hours*60 + minutes
}

func intp(v int) *int { return &v }

// Demo builds a realistic account for "Esteban" relative to `now` (in the
// user's location): ~6 months of habit history, focus sessions, workouts with
// progressive overload, objectives, journal entries and journey milestones.
// It is deterministic for a given `now`.
func Demo(now time.Time) Dataset {
	rng := newRandom(17)
	loc := now.Location()
	today := shared.DateOf(now)
	nowMinutes := now.Hour()*60 + now.Minute()
	joinedAt := today.AddDays(-196)

	seq := 0
	id := func(prefix string) string {
		seq++
		return prefix + "-" + strconv.FormatInt(int64(seq), 36)
	}

	// at places an event on `date` at `clock`, plus up to `jitter` random
	// minutes. Nothing in the history may happen after "now".
	latest := now.Add(-time.Minute)
	at := func(date shared.LocalDate, clock string, jitter int) time.Time {
		minutes := minutesOf(clock)
		if jitter > 0 {
			minutes += rng.intn(0, jitter)
		}
		d := date.In(loc)
		t := time.Date(d.Year(), d.Month(), d.Day(), 0, minutes, 0, 0, loc)
		if t.After(latest) {
			t = latest
		}
		return t.UTC().Truncate(time.Millisecond)
	}

	events := []activity.Event{}

	// ─── Habits ─────────────────────────────────────────────────────────────
	habits := []habit.Habit{}
	for _, spec := range catalogHabits() {
		base := spec.habit
		base.Completions = []shared.LocalDate{}
		base.CreatedAt = today.AddDays(-spec.age)
		clock := base.Time
		if clock == "" {
			clock = "12:00"
		}

		dueToday := habit.IsDueOn(base, today)
		doneToday := dueToday && !spec.eveningHabit && nowMinutes >= minutesOf(clock)+45
		pastDue := []shared.LocalDate{}
		for _, d := range shared.DateRange(base.CreatedAt, today.AddDays(-1)) {
			if habit.IsDueOn(base, d) {
				pastDue = append(pastDue, d)
			}
		}
		slices.Reverse(pastDue)

		streakDays := spec.streak
		if doneToday {
			streakDays--
		}
		completions := []shared.LocalDate{}
		for i, date := range pastDue {
			switch {
			case i < streakDays:
				completions = append(completions, date)
			case i == streakDays:
				// The miss that bounds the current streak.
			case rng.chance(spec.reliability):
				completions = append(completions, date)
			}
		}
		if doneToday {
			completions = append(completions, today)
		}
		slices.Sort(completions)
		base.Completions = completions
		habits = append(habits, base)

		for _, date := range completions {
			events = append(events, activity.Event{
				ID:       id("evt"),
				Kind:     activity.KindHabit,
				Title:    "Completed " + base.Name,
				XP:       base.XP,
				DomainID: base.DomainID,
				RefID:    "habit:" + base.ID + ":" + string(date),
				At:       at(date, clock, 40),
			})
		}
	}
	find := func(habitID string) habit.Habit {
		i := slices.IndexFunc(habits, func(h habit.Habit) bool { return h.ID == habitID })
		return habits[i]
	}

	// ─── Workouts (one per gym completion in the last 10 weeks) ─────────────
	workouts := []training.Workout{}
	gymDates := []shared.LocalDate{}
	for _, d := range find("gym").Completions {
		if d >= today.AddDays(-70) {
			gymDates = append(gymDates, d)
		}
	}
	for i, date := range gymDates {
		tpl := workoutTemplates[i%len(workoutTemplates)]
		cycle := i / len(workoutTemplates)
		w := training.Workout{ID: id("wo"), Name: tpl.name, Date: date, Status: training.Completed}
		w.DurationMinutes = intp(rng.intn(55, 80))
		w.Exercises = []training.Exercise{}
		for _, l := range tpl.lifts {
			e := training.Exercise{ID: id("ex"), Name: l.name, Sets: []training.Set{}}
			var top *float64
			if l.base != nil {
				v := *l.base + l.step*float64(cycle/2)
				top = &v
			}
			for s := range 3 {
				reps := max(l.reps[0], rng.intn(l.reps[0], l.reps[1])-s)
				var weight *float64
				if top != nil {
					v := *top
					if s == 2 && rng.chance(0.4) {
						v -= l.step
					}
					weight = &v
				}
				setID := id("set")
				rir := max(0, 3-s-rng.intn(0, 1))
				e.Sets = append(e.Sets, training.Set{ID: setID, Weight: weight, Reps: reps, RIR: intp(rir)})
			}
			w.Exercises = append(w.Exercises, e)
		}
		workouts = append(workouts, w)
	}
	for _, w := range workouts {
		events = append(events, activity.Event{
			ID:       id("evt"),
			Kind:     activity.KindWorkout,
			Title:    "Finished " + w.Name,
			Detail:   strconv.Itoa(len(w.Exercises)*3) + " sets · " + shared.FormatNumber(training.Volume(w)) + " kg",
			DomainID: training.DomainID,
			At:       at(w.Date, "08:15", 20),
		})
	}

	// ─── Focus sessions ─────────────────────────────────────────────────────
	sessions := []focus.Session{}
	focusedHours := 0.0
	hundredHoursDate := today.AddDays(-30)
	addSession := func(date shared.LocalDate, start string, planned int, label focusLabel, full bool) {
		focusedSeconds := planned * 60
		if !full {
			focusedSeconds = shared.RoundInt(float64(planned*60) * (0.6 + rng.next()*0.3))
		}
		startedAt := at(date, start, 15)
		status := focus.StatusStopped
		if full {
			status = focus.StatusCompleted
		}
		session := focus.Session{
			ID:             id("fs"),
			Label:          label.label,
			DomainID:       label.domainID,
			PlannedMinutes: planned,
			StartedAt:      startedAt,
			EndedAt:        startedAt.Add(time.Duration(focusedSeconds) * time.Second),
			FocusedSeconds: focusedSeconds,
			Status:         status,
			XP:             focus.XP(focusedSeconds),
		}
		sessions = append(sessions, session)
		before := focusedHours
		focusedHours += float64(focusedSeconds) / 3600
		if before < 100 && focusedHours >= 100 {
			hundredHoursDate = date
		}
		events = append(events, activity.Event{
			ID:       id("evt"),
			Kind:     activity.KindFocus,
			Title:    "Deep work · " + session.Label,
			Detail:   strconv.Itoa(shared.RoundInt(float64(focusedSeconds)/60)) + " min focused",
			XP:       session.XP,
			DomainID: session.DomainID,
			RefID:    "focus:" + session.ID,
			At:       session.EndedAt,
		})
	}

	for _, date := range shared.DateRange(today.AddDays(-150), today.AddDays(-1)) {
		workday := date.Weekday() < 5
		p := 0.3
		if workday {
			p = 0.82
		}
		if !rng.chance(p) {
			continue
		}
		planned := pick(rng, []int{50, 90, 90})
		label := pick(rng, focusLabels)
		full := rng.chance(0.85)
		addSession(date, "09:30", planned, label, full)
		if workday && rng.chance(0.45) {
			planned := pick(rng, []int{25, 50})
			label := pick(rng, focusLabels)
			full := rng.chance(0.8)
			addSession(date, "15:00", planned, label, full)
		}
	}
	if nowMinutes >= 11*60+15 {
		addSession(today, "09:35", 90, focusLabels[0], true)
	}

	// ─── Tasks ──────────────────────────────────────────────────────────────
	type taskSpec struct {
		title, domainID string
		xp              int
	}
	pastTasks := []taskSpec{
		{"Review PR #142", "career", 30},
		{"Write transactions API docs", "career", 60},
		{"Pay utilities", "finance", 20},
		{"Meal prep", "physical", 40},
		{"Call grandparents", "relationships", 40},
		{"Fix CI pipeline", "career", 60},
		{"Plan the week", "personal", 30},
		{"Update budget sheet", "finance", 30},
	}
	tasks := []task.Task{}
	completeTask := func(t task.Task, when time.Time) {
		events = append(events, activity.Event{
			ID:       id("evt"),
			Kind:     activity.KindTask,
			Title:    "Completed " + t.Title,
			XP:       t.XP,
			DomainID: t.DomainID,
			RefID:    "task:" + t.ID,
			At:       when,
		})
	}
	for _, date := range shared.DateRange(today.AddDays(-21), today.AddDays(-1)) {
		if date.Weekday() > 4 || !rng.chance(0.6) {
			continue
		}
		spec := pick(rng, pastTasks)
		done := rng.chance(0.85)
		t := task.Task{ID: id("task"), Title: spec.title, DomainID: spec.domainID, XP: spec.xp, Date: date, Done: done}
		t.Time = pick(rng, []string{"11:00", "16:30", ""})
		tasks = append(tasks, t)
		if done {
			clock := t.Time
			if clock == "" {
				clock = "17:00"
			}
			completeTask(t, at(date, clock, 30))
		}
	}

	todayTasks := []task.Task{
		{Title: "Ship auth refactor", DomainID: "career", XP: 60, Date: today, Time: "10:30", DurationMinutes: intp(90), ObjectiveID: "build-monea"},
		{Title: "Pay credit card", DomainID: "finance", XP: 20, Date: today},
	}
	for _, t := range todayTasks {
		duration := 45
		if t.DurationMinutes != nil {
			duration = *t.DurationMinutes
		}
		t.Done = t.Time != "" && nowMinutes >= minutesOf(t.Time)+duration
		t.ID = id("task")
		tasks = append(tasks, t)
		if t.Done {
			completeTask(t, at(today, t.Time, 10))
		}
	}

	upcoming := []task.Task{
		{Title: "Monea sprint planning", DomainID: "career", XP: 40, Date: today.AddDays(1), Time: "09:30", DurationMinutes: intp(60), ObjectiveID: "build-monea"},
		{Title: "Dinner with friends", DomainID: "relationships", XP: 30, Date: today.AddDays(2), Time: "20:00", DurationMinutes: intp(120)},
		{Title: "Transfer to emergency fund", DomainID: "finance", XP: 30, Date: today.AddDays(3)},
		{Title: "Mock B2 speaking test", DomainID: "intellect", XP: 80, Date: today.AddDays(4), Time: "17:00", DurationMinutes: intp(45), ObjectiveID: "english-b2"},
		{Title: "Dentist", DomainID: "personal", XP: 20, Date: today.AddDays(6), Time: "15:00", DurationMinutes: intp(60)},
		{Title: "Budget engine design review", DomainID: "career", XP: 60, Date: today.AddDays(8), Time: "11:00", DurationMinutes: intp(60), ObjectiveID: "build-monea"},
	}
	for _, t := range upcoming {
		t.ID = id("task")
		tasks = append(tasks, t)
	}

	// ─── Objectives ─────────────────────────────────────────────────────────
	run10kDone := hundredHoursDate.AddDays(-24)
	objectives := []objective.Objective{
		{
			ID:          "reach-65kg",
			Name:        "Reach 65 kg",
			Description: "Lean bulk: +0.3 kg per week without losing waist definition.",
			DomainID:    "physical",
			Deadline:    today.AddDays(23),
			Status:      objective.Active,
			Metric:      &objective.Metric{Unit: "kg", Start: 56.6, Current: 60.2, Target: 65},
			Milestones: []objective.Milestone{
				{ID: "m-60", Title: "Cross 60 kg", Done: true},
				{ID: "m-625", Title: "Cross 62.5 kg"},
				{ID: "m-65", Title: "Hold 65 kg for two weigh-ins"},
			},
			HabitIDs:  []string{"gym", "protein", "weigh-in"},
			CreatedAt: today.AddDays(-110),
		},
		{
			ID:          "build-monea",
			Name:        "Build Monea",
			Description: "Personal finance app: ship a private beta to 20 users.",
			DomainID:    "career",
			Deadline:    shared.LocalDate(string(today)[:4] + "-12-31"),
			Status:      objective.Active,
			Milestones: []objective.Milestone{
				{ID: "mo-1", Title: "Authentication", Done: true},
				{ID: "mo-2", Title: "Transactions API", Done: true},
				{ID: "mo-3", Title: "Budget engine"},
				{ID: "mo-4", Title: "Mobile client (Kotlin)"},
				{ID: "mo-5", Title: "Private beta · 20 users"},
			},
			HabitIDs:  []string{"code"},
			CreatedAt: today.AddDays(-150),
		},
		{
			ID:          "english-b2",
			Name:        "English B2",
			Description: "Pass the B2 certification with comfortable margins in speaking.",
			DomainID:    "intellect",
			Deadline:    today.AddDays(95),
			Status:      objective.Active,
			Milestones: []objective.Milestone{
				{ID: "en-1", Title: "Finish B1 course", Done: true},
				{ID: "en-2", Title: "30 conversation sessions", Done: true},
				{ID: "en-3", Title: "Read 3 books in English"},
				{ID: "en-4", Title: "Mock exam ≥ B2"},
				{ID: "en-5", Title: "Take the exam"},
			},
			HabitIDs:  []string{"english", "read"},
			CreatedAt: today.AddDays(-120),
		},
		{
			ID:          "emergency-fund",
			Name:        "Emergency fund",
			Description: "Four months of expenses set aside.",
			DomainID:    "finance",
			Deadline:    today.AddDays(180),
			Status:      objective.Active,
			Metric:      &objective.Metric{Unit: "USD", Start: 0, Current: 1850, Target: 4000},
			Milestones:  []objective.Milestone{},
			HabitIDs:    []string{"budget"},
			CreatedAt:   today.AddDays(-150),
		},
		{
			ID:          "run-10k",
			Name:        "Run 10K",
			Description: "Finish a 10 km run without walking breaks.",
			DomainID:    "physical",
			Deadline:    run10kDone.AddDays(10),
			Status:      objective.Completed,
			CompletedAt: run10kDone,
			Milestones: []objective.Milestone{
				{ID: "r-1", Title: "5K without stopping", Done: true},
				{ID: "r-2", Title: "8K long run", Done: true},
				{ID: "r-3", Title: "10K", Done: true},
			},
			HabitIDs:  []string{},
			CreatedAt: run10kDone.AddDays(-70),
		},
	}

	// ─── Journal ────────────────────────────────────────────────────────────
	journalDates := []shared.LocalDate{}
	for _, d := range find("journal").Completions {
		if d < today {
			journalDates = append(journalDates, d)
		}
	}
	journalDates = journalDates[max(0, len(journalDates)-len(journalPool)):]
	slices.Reverse(journalDates)
	entries := []journal.Entry{}
	for i, date := range journalDates {
		tpl := journalPool[i]
		entryID := id("jr")
		entries = append(entries, journal.Entry{
			ID:         entryID,
			Date:       date,
			Title:      tpl.Title,
			Content:    tpl.Content,
			Mood:       tpl.Mood,
			Energy:     tpl.Energy,
			Wins:       slices.Clone(tpl.Wins),
			Problems:   slices.Clone(tpl.Problems),
			Reflection: tpl.Reflection,
			Tomorrow:   slices.Clone(tpl.Tomorrow),
			CreatedAt:  at(date, "22:15", 20),
		})
	}
	for _, e := range entries {
		events = append(events, activity.Event{ID: id("evt"), Kind: activity.KindJournal, Title: "Journal entry created", Detail: e.Title, At: e.CreatedAt})
	}

	// ─── Journey ────────────────────────────────────────────────────────────
	milestones := []journey.Milestone{
		{ID: "origin", Level: 1, Kind: journey.KindOrigin, Title: "Started", Description: "System initialized with Gym, Code and Read.", Date: joinedAt},
		{ID: "streak-first-7", Level: 5, Kind: journey.KindStreak, Title: "First 7-day streak", Description: "Code 60 min, seven days in a row.", Date: joinedAt.AddDays(31)},
		{ID: "record-5k", Level: 8, Kind: journey.KindRecord, Title: "First 5K without stopping", Description: "28:40 · first milestone of Run 10K.", Date: joinedAt.AddDays(70)},
		{ID: "objective-run-10k", Level: 10, Kind: journey.KindObjective, Title: "First objective completed", Description: "Run 10K · finished without walking breaks.", Date: run10kDone},
		{ID: "streak-code-30", Level: 12, Kind: journey.KindStreak, Title: "30× Code streak", Description: "A month of daily coding for Monea.", Date: hundredHoursDate.AddDays(-12)},
		{ID: "focus-100", Level: 15, Kind: journey.KindFocus, Title: "100 hours focused", Description: "Cumulative deep work across all sessions.", Date: hundredHoursDate},
	}

	return Dataset{
		User: progression.User{
			ID:       "esteban",
			Name:     "Esteban",
			TotalXP:  progression.TotalXPForLevel(17) + 7420,
			JoinedAt: joinedAt.In(loc).UTC(),
		},
		Domains:    catalogDomains(),
		Habits:     habits,
		Objectives: objectives,
		Tasks:      tasks,
		Journal:    entries,
		Focus:      sessions,
		Workouts:   workouts,
		Journey:    milestones,
		Activity:   events,
	}
}

// Blank builds an empty account: the user, the default life domains at a
// neutral score and the origin milestone.
func Blank(now time.Time, userName string) Dataset {
	today := shared.DateOf(now)
	if strings.TrimSpace(userName) == "" {
		userName = "You"
	}
	domains := catalogDomains()
	for i := range domains {
		domains[i].Score = lifedomain.InitialScore
		domains[i].History = []float64{lifedomain.InitialScore}
	}
	return Dataset{
		User:       progression.User{ID: "me", Name: strings.TrimSpace(userName), JoinedAt: today.In(now.Location()).UTC()},
		Domains:    domains,
		Habits:     []habit.Habit{},
		Objectives: []objective.Objective{},
		Tasks:      []task.Task{},
		Journal:    []journal.Entry{},
		Focus:      []focus.Session{},
		Workouts:   []training.Workout{},
		Journey: []journey.Milestone{
			{ID: "origin", Level: 1, Kind: journey.KindOrigin, Title: "Started", Description: "System initialized.", Date: today},
		},
		Activity: []activity.Event{},
	}
}
