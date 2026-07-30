package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useIsolatedSchemaDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousLogDB := LOG_DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		initCol()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestInitializeDBSchemaCreatesCurrentBaseline(t *testing.T) {
	db := useIsolatedSchemaDB(t)

	require.NoError(t, initializeDBSchema())
	require.NoError(t, initializeDBSchema())
	for _, table := range []any{
		&User{},
		&Token{},
		&SubscriptionPlan{},
		&ExternalIdentityClaim{},
		&UpstreamBillingRecord{},
		&UpstreamBillingAccount{},
	} {
		assert.True(t, db.Migrator().HasTable(table))
	}
	assert.True(t, db.Migrator().HasColumn(&User{}, "auth_version"))
	assert.True(t, db.Migrator().HasColumn(&Token{}, "model_limits"))
	assert.True(t, db.Migrator().HasColumn(&Token{}, "rate_limit_5h"))
	assert.True(t, db.Migrator().HasColumn(&Token{}, "usage_1d"))
	assert.True(t, db.Migrator().HasColumn(&SubscriptionPlan{}, "price_amount"))
	assert.True(t, db.Migrator().HasColumn(&UpstreamBillingRecord{}, "request_finished_at_ms"))

	columnTypes, err := db.Migrator().ColumnTypes(&Log{})
	require.NoError(t, err)
	var quotaType string
	for _, columnType := range columnTypes {
		if strings.EqualFold(columnType.Name(), "quota") {
			quotaType = strings.ToLower(columnType.DatabaseTypeName())
			break
		}
	}
	assert.Contains(t, quotaType, "bigint")
}

func TestInitializeDBSchemaRejectsPartialBaseline(t *testing.T) {
	db := useIsolatedSchemaDB(t)
	require.NoError(t, db.Migrator().CreateTable(&User{}))

	err := initializeDBSchema()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database schema is incomplete")
	assert.False(t, db.Migrator().HasTable(&Token{}))
}
