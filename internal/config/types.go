package config

type Config struct {
	// Note that UserIDMapping can be nil.
	UserIDMapping map[int64]string `yaml:"tracker_id_to_github_username_mapping"`
}
