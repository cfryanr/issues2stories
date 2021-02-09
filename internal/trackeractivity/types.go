package trackeractivity

import "encoding/json"

type TrackerEvent struct {
	Kind    string   `json:"kind"`
	Changes []Change `json:"changes"`
	Project Project  `json:"project"`
}

type Change struct {
	Kind           string        `json:"kind"`
	ID             int64         `json:"id"`
	ChangeType     string        `json:"change_type"`
	StoryType      string        `json:"story_type"`
	OriginalValues ChangedValues `json:"original_values"`
	NewValues      ChangedValues `json:"new_values"`
}

type ChangedValues struct {
	Title        string            `json:"name"`
	Description  string            `json:"description"`
	StoryType    string            `json:"story_type"`
	CurrentState string            `json:"current_state"`
	Estimate     OptionalInt64     `json:"estimate"`
	OwnerIDs     OptionalInt64List `json:"owner_ids"`
}

type Project struct {
	ID int64 `json:"id"`
}

type OptionalInt64 struct {
	Present bool
	Value   *int64
}

type OptionalInt64List struct {
	Present bool
	Value   *[]int64
}

func (o *OptionalInt64) UnmarshalJSON(data []byte) error {
	o.Present = true
	return json.Unmarshal(data, &o.Value)
}

func (o *OptionalInt64List) UnmarshalJSON(data []byte) error {
	o.Present = true
	return json.Unmarshal(data, &o.Value)
}
