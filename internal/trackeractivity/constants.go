package trackeractivity

// The keys in this map represent all valid story states.
//
// See https://www.pivotaltracker.com/help/api/rest/v5#story_resource
//
// See also https://www.pivotaltracker.com/help/articles/story_states/
//
// The values in this map represent all of the labels that should be
// automatically managed by this app per state. This code assumes that
// these labels already exist in your GitHub repository.
//
// When a story transitions into the state defined by the key, this app
// will update the linked issue to remove all of the labels mentioned
// by any value in the map, and then add the labels at that specific
// key's value.
var issueLabelsToApplyPerStoryState = map[string][]string{
	"unscheduled": {"priority/undecided"},
	"unstarted":   {"priority/backlog"},
	"started":     {"priority/backlog", "state/started"},
	"finished":    {"priority/backlog", "state/finished"},
	"delivered":   {"priority/backlog", "state/delivered"},
	"rejected":    {"priority/backlog", "state/rejected"},
	"accepted":    {"state/accepted"},

	// The feature of Tracker that causes a story to be "planned" is not commonly used.
	// It should be similar to the "unstarted" state for our purposes here.
	// See https://www.pivotaltracker.com/help/articles/automatic_vs_manual_planning/
	"planned": {"priority/backlog"},
}

// The keys in this map represent all valid story types.
//
// See https://www.pivotaltracker.com/help/api/rest/v5#story_resource
//
// See also https://www.pivotaltracker.com/help/articles/adding_stories/
//
// The values in this map represent all of the labels that should be
// automatically managed by this app per state. This code assumes that
// these labels already exist in your GitHub repository.
//
// When a story transitions into the story type defined by the key, this app
// will update the linked issue to remove all of the labels mentioned
// by any value in the map, and then add the labels at that specific
// key's value.
var issueLabelsToApplyPerStoryType = map[string][]string{
	"feature": {"enhancement"},
	"bug":     {"bug"},
	"chore":   {"chore"},
	"release": {}, // empty means just remove the other labels
}

// The keys in this map represent story estimates.
//
// See https://www.pivotaltracker.com/help/api/rest/v5#story_resource
//
// See also https://www.pivotaltracker.com/help/articles/estimating_stories/
//
// The values in this map represent all of the labels that should be
// automatically managed by this app per state. This code assumes that
// these labels already exist in your GitHub repository.
//
// When a story's estimate is edited, this app
// will update the linked issue to remove all of the labels mentioned
// by any value in the map, and then add the labels at that specific
// key's value.
//
// Tracker's estimation scales:
// - Fibonacci scale:   0, 1, 2, 3, 5, 8 -> XS, S, M, L, XL, XXL
// - Powers of 2 scale: 0, 1, 2, 4, 8    -> XS, S, M, L, XXL
// - Linear scale:      0, 1, 2, 3       -> XS, S, M, L
// - Custom scale: Not supported unless you edit the code below to match
//   allowed values of your custom scale.
var issueLabelsToApplyPerStoryEstimate = map[string][]string{
	"0": {"estimate/XS"},
	"1": {"estimate/S"},
	"2": {"estimate/M"},
	"3": {"estimate/L"},   // 3 is used in fibonacci and linear
	"4": {"estimate/L"},   // 4 is only used in powers of 2
	"5": {"estimate/XL"},  // 5 is only used in fibonacci
	"8": {"estimate/XXL"}, // 8 is only used in fibonacci and powers of 2
}
