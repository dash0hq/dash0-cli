package config

// Configuration represents a Dash0 configuration
type Configuration struct {
	ApiUrl    string `json:"apiUrl" yaml:"apiUrl"`
	AuthToken string `json:"authToken" yaml:"authToken"`
}

// Context represents a configuration context
type Context struct {
	Name          string        `json:"name" yaml:"name"`
	Configuration Configuration `json:"configuration" yaml:"configuration"`
}

// ContextsFile represents the file storing multiple contexts
type ContextsFile struct {
	Contexts []Context `json:"contexts" yaml:"contexts"`
}