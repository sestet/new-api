package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserUsableGroupsAppliesSpecialVisibilityRules(t *testing.T) {
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialRules := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialRules))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(
		`{"default":"Default","standard":"Standard"}`,
	))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(
		`{"default":{"+:premium":"Premium","-:standard":""}}`,
	))

	assert.Equal(t, map[string]string{
		"default": "Default",
		"premium": "Premium",
	}, GetUserUsableGroups("default"))
}

func TestGetUserUsableGroupsKeepsUsersOwnGroupAvailable(t *testing.T) {
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialRules := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialRules))
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	assert.Equal(t, "用户分组", GetUserUsableGroups("vip")["vip"])
}

func TestGetUserAutoGroupPreservesConfiguredUsableOrder(t *testing.T) {
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["premium","hidden","default"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","premium":"Premium"}`))

	assert.Equal(t, []string{"premium", "default"}, GetUserAutoGroup("default"))
}
