package setting

import (
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
		`[]`,
		`[""]`,
		`["default","default"]`,
		`{"default":true}`,
	}
	for _, input := range tests {
		assert.Error(t, CheckAutoGroups(input), input)
	}
}
