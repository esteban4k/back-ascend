package seed

import "ascend/internal/domain/journal"

// journalPool holds hand-written entries, placed on the most recent journaling days.
var journalPool = []journal.Draft{
	{
		Title:      "Auth flow finally clicks",
		Content:    "Spent the morning block untangling refresh tokens in Monea. Moved the token rotation into its own service and the tests went green on the first run after that. Gym felt heavy but bench moved well.",
		Mood:       4,
		Energy:     4,
		Wins:       []string{"Refresh token rotation shipped", "Bench 70 kg × 8"},
		Problems:   []string{"Lost 40 minutes to Slack after lunch"},
		Reflection: "The 90-minute block works when I decide the exact task the night before. When I start without a target I drift.",
		Tomorrow:   []string{"Write integration tests for logout", "Weigh-in", "English: 1 podcast episode"},
	},
	{
		Title:      "Low energy, still showed up",
		Content:    "Slept 5.5 hours. Did the minimum version of everything instead of skipping. Short code session, short read.",
		Mood:       3,
		Energy:     2,
		Wins:       []string{"Kept every streak alive"},
		Problems:   []string{"Late screen time again", "Skipped the second meal"},
		Reflection: "Minimum versions protect the streak. Sleep is the lever for everything else this week.",
		Tomorrow:   []string{"Phone out of the room by 22:30", "Protein target"},
	},
	{
		Title:      "Budget review",
		Content:    "Sunday review. Spending on food delivery is still the leak. Moved the difference to the emergency fund manually.",
		Mood:       4,
		Energy:     3,
		Wins:       []string{"Emergency fund crossed $1,800"},
		Problems:   []string{"Delivery spending 30% over plan"},
		Reflection: "Meal prep on Sunday removes most of the delivery temptation during the week.",
		Tomorrow:   []string{"Meal prep for Mon–Wed", "Plan Monea sprint"},
	},
	{
		Title:      "English conversation session",
		Content:    "Forty minutes of conversation practice. Much less translating in my head than a month ago. Still slow with phrasal verbs.",
		Mood:       5,
		Energy:     4,
		Wins:       []string{"Held a 40 min conversation without switching to Spanish"},
		Problems:   []string{"Phrasal verbs"},
		Reflection: "Speaking daily, even 10 minutes, compounds faster than one long weekly session.",
		Tomorrow:   []string{"Anki: phrasal verbs deck", "Code 60 min"},
	},
	{
		Title:      "Deadlift PR",
		Content:    "Lower B day. Deadlift moved better than expected. Wrote the budget engine spec in the afternoon.",
		Mood:       5,
		Energy:     5,
		Wins:       []string{"Deadlift top set PR", "Budget engine spec drafted"},
		Problems:   []string{},
		Reflection: "Training mornings make the afternoon focus blocks noticeably sharper.",
		Tomorrow:   []string{"Review spec with fresh eyes", "Read 20 min"},
	},
	{
		Title:      "Scattered day",
		Content:    "Too many context switches. Three small tasks done, the important one untouched.",
		Mood:       2,
		Energy:     3,
		Wins:       []string{"Cleared the PR review backlog"},
		Problems:   []string{"No deep work block", "Reactive all afternoon"},
		Reflection: "If the first block of the day is not protected, the day belongs to other people.",
		Tomorrow:   []string{"Deep work 09:00–10:30 before opening Slack"},
	},
}
