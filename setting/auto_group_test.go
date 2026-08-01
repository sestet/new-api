package setting

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateAutoGroupsPreservesConfiguredOrder(t *testing.T) {
	original := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(original))
	})

	require.NoError(t, UpdateAutoGroupsByJsonString(`["default","premium"]`))
	groups := GetAutoGroups()
	assert.Equal(t, []string{"default", "premium"}, groups)
	assert.True(t, ContainsAutoGroup("premium"))

	groups[0] = "changed"
	assert.Equal(t, []string{"default", "premium"}, GetAutoGroups())
}

func TestCheckAutoGroupsRejectsInvalidLists(t *testing.T) {
	tests := []string{
		`[""]`,
		`["default","default"]`,
		`{"default":true}`,
	}
	for _, input := range tests {
		assert.Error(t, CheckAutoGroups(input), input)
	}
}

func TestUpdateMaxTokenAutoGroupsAcceptsAnyPositiveInteger(t *testing.T) {
	original := GetMaxTokenAutoGroups()
	t.Cleanup(func() {
		require.NoError(t, UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", original)))
	})

	require.NoError(t, UpdateMaxTokenAutoGroups("123456"))
	assert.Equal(t, 123456, GetMaxTokenAutoGroups())
}

func TestUpdateMaxTokenAutoGroupsRejectsInvalidValuesWithoutChangingState(t *testing.T) {
	original := GetMaxTokenAutoGroups()
	for _, value := range []string{"", "0", "-1", "1.5", "not-a-number"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, UpdateMaxTokenAutoGroups(value))
			assert.Equal(t, original, GetMaxTokenAutoGroups())
		})
	}
}
