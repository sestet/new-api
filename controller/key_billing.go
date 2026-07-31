package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetKeyBillingDeclaration exposes the effective upstream rate to the holder
// of an API key without revealing the surrounding group configuration.
func GetKeyBillingDeclaration(c *gin.Context) {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "auto" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": gin.H{"message": "auto-group API keys do not have one stable billing rate"},
		})
		return
	}
	rate, _ := ratio_setting.ResolveGroupRatio(userGroup, usingGroup)
	if rate <= 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": gin.H{"message": "the API key resolves to a non-positive billing rate"},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"object":                    "new-api.key_billing",
		"schema_version":            1,
		"billing_scope":             "token",
		"effective_rate_multiplier": rate,
		"observed_at":               time.Now().UTC().Format(time.RFC3339Nano),
	})
}
