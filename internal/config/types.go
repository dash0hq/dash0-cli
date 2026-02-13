package config

// Configuration represents a Dash0 configuration
type Configuration struct {
	ApiUrl    string `json:"apiUrl" yaml:"apiUrl"`
	AuthToken string `json:"authToken" yaml:"authToken"`
	OtlpUrl   string `json:"otlpUrl,omitempty" yaml:"otlpUrl,omitempty"`
	Dataset   string `json:"dataset,omitempty" yaml:"dataset,omitempty"`
}

// Profile represents a configuration profile
type Profile struct {
	Name          string        `json:"name" yaml:"name"`
	Configuration Configuration `json:"configuration" yaml:"configuration"`
}

// ProfilesFile represents the file storing multiple profiles
type ProfilesFile struct {
	Profiles []Profile `json:"profiles" yaml:"profiles"`
}