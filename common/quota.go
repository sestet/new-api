package common

func GetTrustQuota() int64 {
	return QuotaFromFloat(10 * QuotaPerUnit)
}
