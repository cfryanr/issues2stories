package trackeractivity

type TrackerEvent struct {
	Kind    string   `json:"kind"`
	Changes []Change `json:"changes"`
	Project Project  `json:"project"`
}

type Change struct {
	Kind       string    `json:"kind"`
	ID         int64     `json:"id"`
	ChangeType string    `json:"change_type"`
	StoryType  string    `json:"story_type"`
	NewValues  NewValues `json:"new_values"`
}

type NewValues struct {
	StoryType    string `json:"story_type"`
	CurrentState string `json:"current_state"`
}

type Project struct {
	ID int64 `json:"id"`
}
