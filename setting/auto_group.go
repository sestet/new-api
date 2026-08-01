package setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const DefaultMaxTokenAutoGroups = 5

var autoGroups = []string{"default"}
var autoGroupsMutex sync.RWMutex

var DefaultUseAutoGroup = false

var maxTokenAutoGroups atomic.Int64

func init() {
	maxTokenAutoGroups.Store(DefaultMaxTokenAutoGroups)
}

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
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return fmt.Errorf("auto group name cannot be empty")
		}
		if _, exists := seen[group]; exists {
			return fmt.Errorf("auto groups must be unique: %s", group)
		}
		seen[group] = struct{}{}
	}
	return nil
}

func GetMaxTokenAutoGroups() int {
	return int(maxTokenAutoGroups.Load())
}

func ValidateMaxTokenAutoGroups(value string) error {
	maxCount, err := strconv.Atoi(value)
	if err != nil || maxCount <= 0 {
		return fmt.Errorf("MaxTokenAutoGroups must be a positive integer")
	}
	return nil
}

func UpdateMaxTokenAutoGroups(value string) error {
	if err := ValidateMaxTokenAutoGroups(value); err != nil {
		return err
	}
	maxCount, _ := strconv.Atoi(value)
	maxTokenAutoGroups.Store(int64(maxCount))
	return nil
}
