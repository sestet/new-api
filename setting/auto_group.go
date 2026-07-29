package setting

var DefaultUseAutoGroup = false

func ContainsAutoGroup(group string) bool {
	return group == "default"
}

// UpdateAutoGroupsByJsonString remains for compatibility with old option rows.
// Automatic selection always uses the default group.
func UpdateAutoGroupsByJsonString(_ string) error {
	return nil
}

func AutoGroups2JsonString() string {
	return `["default"]`
}

func GetAutoGroups() []string {
	return []string{"default"}
}
