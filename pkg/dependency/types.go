package dependency

type Dependency struct {
	Name string

	// either branch or tag should be set, not both
	Branch string
	Tag    string
}
