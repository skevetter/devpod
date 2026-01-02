package config

type ImageDetails struct {
	ID     string
	Config ImageDetailsConfig
}

type ImageDetailsConfig struct {
	User       string
	Env        []string
	Labels     map[string]string
	Entrypoint []string
	Cmd        []string
}

type ContainerDetails struct {
	ID      string                 `json:"ID,omitempty"`
	Created string                 `json:"Created,omitempty"`
	State   ContainerDetailsState  `json:"State"`
	Config  ContainerDetailsConfig `json:"Config"`
}

type ContainerDetailsConfig struct {
	Labels map[string]string `json:"Labels,omitempty"`

	// WorkingDir specifies default working directory inside the container
	WorkingDir string `json:"WorkingDir,omitempty"`

	// User specifies the user that the container runs as
	User string `json:"User,omitempty"`

	// LegacyImage shouldn't get used anymore and is only there for testing
	LegacyImage string `json:"Image,omitempty"`
}

type ContainerDetailsState struct {
	Status    string `json:"Status,omitempty"`
	StartedAt string `json:"StartedAt,omitempty"`
}
