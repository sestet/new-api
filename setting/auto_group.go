package setting

import (
	"errors"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var autoGroups = []string{"default"}
var autoGroupsMutex sync.RWMutex

var DefaultUseAutoGroup = false

func ContainsAutoGroup(group string) bool {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	if err := CheckAutoGroups(jsonString); err != nil {
		return err
	}
	groups := make([]string, 0)
	if err := common.UnmarshalJsonStr(jsonString, &groups); err != nil {
		return err
	}
	autoGroupsMutex.Lock()
	autoGroups = groups
	autoGroupsMutex.Unlock()
	return nil
}

func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(GetAutoGroups())
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	return append([]string(nil), autoGroups...)
}

func CheckAutoGroups(jsonString string) error {
	groups := make([]string, 0)
	if err := common.UnmarshalJsonStr(jsonString, &groups); err != nil {
		return err
	}
	if len(groups) == 0 {
		return errors.New("at least one auto group is required")
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return errors.New("auto group name cannot be empty")
		}
		if _, exists := seen[group]; exists {
			return errors.New("auto groups must be unique: " + group)
		}
		seen[group] = struct{}{}
	}
	return nil
}
