package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGroupRatioUsesBaseRatioWhenNoOverrideExists(t *testing.T) {
	originalBase := GroupRatio2JSONString()
	originalOverrides := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(originalBase))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalOverrides))
	})

	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"premium":1.5}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{}`))

	ratio, source := ResolveGroupRatio("default", "premium")

	assert.Equal(t, 1.5, ratio)
	assert.Equal(t, "group_ratio", source)
}

func TestResolveGroupRatioUsesOverrideInsteadOfMultiplyingBaseRatio(t *testing.T) {
	originalBase := GroupRatio2JSONString()
	originalOverrides := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(originalBase))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalOverrides))
	})

	require.NoError(t, UpdateGroupRatioByJSONString(`{"default":1,"premium":1.5}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"default":{"premium":2}}`))

	ratio, source := ResolveGroupRatio("default", "premium")

	assert.Equal(t, 2.0, ratio)
	assert.Equal(t, "group_group_ratio", source)
}

func TestGroupRatioValidationAllowsFreeGroupAndRejectsUnsafeValues(t *testing.T) {
	assert.NoError(t, CheckGroupRatio(`{"default":0}`))
	assert.Error(t, CheckGroupRatio(`{"default":-1}`))
	assert.Error(t, CheckGroupRatio(`{"paid":1}`))
	assert.Error(t, CheckGroupRatio(`{"default":1e999}`))
	assert.Error(t, CheckGroupGroupRatio(`{"default":{"paid":-0.1}}`))
}

func TestGroupSpecialUsableGroupValidationAndUpdate(t *testing.T) {
	original := GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(original))
	})

	rules := `{"default":{"+:premium":"Premium","-:internal":""}}`
	require.NoError(t, UpdateGroupSpecialUsableGroupByJSONString(rules))

	assert.Equal(t, map[string]string{
		"+:premium":  "Premium",
		"-:internal": "",
	}, GetGroupSpecialUsableGroups("default"))
	assert.NoError(t, CheckGroupSpecialUsableGroup(`{}`))
	assert.Error(t, CheckGroupSpecialUsableGroup(`{"":{"+:premium":"Premium"}}`))
	assert.Error(t, CheckGroupSpecialUsableGroup(`{"default":{"+:":"Premium"}}`))
	assert.Error(t, CheckGroupSpecialUsableGroup(`{"default":{"+:premium":""}}`))
}
