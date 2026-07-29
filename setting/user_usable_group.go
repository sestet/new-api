package setting

import (
	"errors"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var userUsableGroups = map[string]string{"default": "默认分组"}
var userUsableGroupsMutex sync.RWMutex

func GetUserUsableGroupsCopy() map[string]string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	groups := make(map[string]string, len(userUsableGroups))
	for name, description := range userUsableGroups {
		groups[name] = description
	}
	return groups
}

func UserUsableGroups2JSONString() string {
	bytes, err := common.Marshal(GetUserUsableGroupsCopy())
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func UpdateUserUsableGroupsByJSONString(jsonStr string) error {
	if err := CheckUserUsableGroups(jsonStr); err != nil {
		return err
	}
	groups := make(map[string]string)
	if err := common.UnmarshalJsonStr(jsonStr, &groups); err != nil {
		return err
	}
	userUsableGroupsMutex.Lock()
	userUsableGroups = groups
	userUsableGroupsMutex.Unlock()
	return nil
}

func CheckUserUsableGroups(jsonStr string) error {
	groups := make(map[string]string)
	if err := common.UnmarshalJsonStr(jsonStr, &groups); err != nil {
		return err
	}
	if _, ok := groups["default"]; !ok {
		return errors.New("default group is required")
	}
	for name, description := range groups {
		if strings.TrimSpace(name) == "" {
			return errors.New("group name cannot be empty")
		}
		if strings.TrimSpace(description) == "" {
			return errors.New("group description cannot be empty: " + name)
		}
	}
	return nil
}

func GetUsableGroupDescription(groupName string) string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()
	if description, ok := userUsableGroups[groupName]; ok {
		return description
	}
	return groupName
}
