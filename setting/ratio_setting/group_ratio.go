package ratio_setting

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const defaultGroupName = "default"

var groupRatioMap = types.NewRWMap[string, float64]()
var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()
var groupSpecialUsableGroupMap = types.NewRWMap[string, map[string]string]()
var groupRatioSettingsMutex sync.RWMutex

func init() {
	groupRatioMap.Set(defaultGroupName, 1)
}

func GetGroupRatioCopy() map[string]float64 {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	return groupRatioMap.ReadAll()
}

func GroupRatio2JSONString() string {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	if err := CheckGroupRatio(jsonStr); err != nil {
		return err
	}
	groupRatioSettingsMutex.Lock()
	defer groupRatioSettingsMutex.Unlock()
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatioCopy() map[string]map[string]float64 {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	return groupGroupRatioMap.ReadAll()
}

func GroupSpecialUsableGroup2JSONString() string {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	return groupSpecialUsableGroupMap.MarshalJSONString()
}

func UpdateGroupSpecialUsableGroupByJSONString(jsonStr string) error {
	if err := CheckGroupSpecialUsableGroup(jsonStr); err != nil {
		return err
	}
	groupRatioSettingsMutex.Lock()
	defer groupRatioSettingsMutex.Unlock()
	return types.LoadFromJsonString(groupSpecialUsableGroupMap, jsonStr)
}

func GetGroupSpecialUsableGroups(userGroup string) map[string]string {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	groups, ok := groupSpecialUsableGroupMap.Get(userGroup)
	if !ok {
		return nil
	}
	copy := make(map[string]string, len(groups))
	for group, description := range groups {
		copy[group] = description
	}
	return copy
}

func GroupGroupRatio2JSONString() string {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	if err := CheckGroupGroupRatio(jsonStr); err != nil {
		return err
	}
	groupRatioSettingsMutex.Lock()
	defer groupRatioSettingsMutex.Unlock()
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()
	groupRatios, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return 0, false
	}
	ratio, ok := groupRatios[usingGroup]
	return ratio, ok
}

func ResolveGroupRatio(userGroup, usingGroup string) (float64, string) {
	groupRatioSettingsMutex.RLock()
	defer groupRatioSettingsMutex.RUnlock()

	if groupRatios, ok := groupGroupRatioMap.Get(userGroup); ok {
		if ratio, exists := groupRatios[usingGroup]; exists {
			return ratio, "group_group_ratio"
		}
	}
	ratio, ok := groupRatioMap.Get(usingGroup)
	if ok {
		return ratio, "group_ratio"
	}
	common.SysLog("group ratio not found: " + usingGroup)
	return 1, "group_ratio"
}

func UpdateGroupRatioOptionsByJSONString(groupRatioJSON, groupGroupRatioJSON, groupSpecialUsableGroupJSON string) error {
	if err := CheckGroupRatio(groupRatioJSON); err != nil {
		return err
	}
	if err := CheckGroupGroupRatio(groupGroupRatioJSON); err != nil {
		return err
	}
	if err := CheckGroupSpecialUsableGroup(groupSpecialUsableGroupJSON); err != nil {
		return err
	}

	groupRatioSettingsMutex.Lock()
	defer groupRatioSettingsMutex.Unlock()
	if err := types.LoadFromJsonString(groupRatioMap, groupRatioJSON); err != nil {
		return err
	}
	if err := types.LoadFromJsonString(groupGroupRatioMap, groupGroupRatioJSON); err != nil {
		return err
	}
	return types.LoadFromJsonString(groupSpecialUsableGroupMap, groupSpecialUsableGroupJSON)
}

func CheckGroupRatio(jsonStr string) error {
	groupRatios := make(map[string]float64)
	if err := common.UnmarshalJsonStr(jsonStr, &groupRatios); err != nil {
		return err
	}
	if len(groupRatios) == 0 {
		return errors.New("at least one group is required")
	}
	if _, ok := groupRatios[defaultGroupName]; !ok {
		return errors.New("default group is required")
	}
	for group, ratio := range groupRatios {
		if strings.TrimSpace(group) == "" {
			return errors.New("group name cannot be empty")
		}
		if err := validateBillingRatio(group, ratio); err != nil {
			return err
		}
	}
	return nil
}

func CheckGroupGroupRatio(jsonStr string) error {
	overrides := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(jsonStr, &overrides); err != nil {
		return err
	}
	for userGroup, groupRatios := range overrides {
		if strings.TrimSpace(userGroup) == "" {
			return errors.New("user group name cannot be empty")
		}
		for usingGroup, ratio := range groupRatios {
			if strings.TrimSpace(usingGroup) == "" {
				return errors.New("using group name cannot be empty")
			}
			if err := validateBillingRatio(userGroup+" -> "+usingGroup, ratio); err != nil {
				return err
			}
		}
	}
	return nil
}

func CheckGroupSpecialUsableGroup(jsonStr string) error {
	rules := make(map[string]map[string]string)
	if err := common.UnmarshalJsonStr(jsonStr, &rules); err != nil {
		return err
	}
	for userGroup, groupRules := range rules {
		if strings.TrimSpace(userGroup) == "" {
			return errors.New("user group name cannot be empty")
		}
		for rawGroup, description := range groupRules {
			targetGroup := rawGroup
			hidden := strings.HasPrefix(rawGroup, "-:")
			if hidden || strings.HasPrefix(rawGroup, "+:") {
				targetGroup = rawGroup[2:]
			}
			if strings.TrimSpace(targetGroup) == "" {
				return errors.New("special usable group name cannot be empty")
			}
			if !hidden && strings.TrimSpace(description) == "" {
				return errors.New("special usable group description cannot be empty: " + targetGroup)
			}
		}
	}
	return nil
}

func validateBillingRatio(name string, ratio float64) error {
	if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return fmt.Errorf("group ratio must be a finite non-negative number: %s", name)
	}
	return nil
}
