package common

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// Quota conversions are centralized here so every billing path shares one
// saturation + logging policy. Quota columns are BIGINT, while the API and
// frontend represent quota as exact JavaScript numbers. Clamp to
// Number.MAX_SAFE_INTEGER so values cannot silently lose precision after
// leaving the backend.
const (
	MaxQuota int64 = 9_007_199_254_740_991
	MinQuota int64 = -MaxQuota
)

// QuotaClampKind identifies why a quota conversion had to be saturated.
type QuotaClampKind string

// Clamp kinds reported by QuotaClamp.Kind.
const (
	QuotaClampOverflow  QuotaClampKind = "overflow"
	QuotaClampUnderflow QuotaClampKind = "underflow"
	QuotaClampNaN       QuotaClampKind = "nan"
)

// QuotaClamp describes a single saturation event: a quota conversion whose
// input fell outside the exactly representable quota range (or was NaN) and was
// therefore clamped. It is surfaced to billing callers so the event can be
// recorded on the related consume/task log for admin auditing.
type QuotaClamp struct {
	Op       string         `json:"op"`       // "QuotaFromFloat" | "QuotaRound" | "QuotaFromDecimal"
	Kind     QuotaClampKind `json:"kind"`     // "overflow" | "underflow" | "nan"
	Original float64        `json:"original"` // best-effort pre-clamp value (decimal -> float64 approx)
	Clamped  int64          `json:"clamped"`  // the saturated result actually used
}

// Error lets the same typed value serve both as the settlement audit marker
// and as the fail-fast error returned by strict pre-consume conversions.
func (c *QuotaClamp) Error() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("quota conversion (%s) %s: original=%g, clamped=%d", c.Op, c.Kind, c.Original, c.Clamped)
}

// AuditMap renders the clamp as the marker stored under a log's
// admin_info.quota_saturation. Centralized here so every billing path (consume
// logs, task billing logs, task compensation logs) records the same shape.
func (c *QuotaClamp) AuditMap() map[string]interface{} {
	if c == nil {
		return nil
	}
	return map[string]interface{}{
		"op":       c.Op,
		"kind":     c.Kind,
		"original": c.Original,
		"clamped":  c.Clamped,
	}
}

// saturateQuota converts an already-rounded quota value to int64, clamping to
// the API-safe range. Whenever clamping (what would otherwise be an integer
// wraparound) or a NaN fallback is triggered it logs a warning, because in
// normal operation a single request never approaches these bounds — hitting
// them signals a bug or an abusive request. `op` names the caller. When a
// clamp occurs it returns a non-nil *QuotaClamp so callers can additionally
// record the event (e.g. on the consume log); the returned pointer is nil for
// in-range values.
func saturateQuota(value float64, op string) (int64, *QuotaClamp) {
	var clamp *QuotaClamp
	switch {
	case math.IsNaN(value):
		clamp = &QuotaClamp{Op: op, Kind: QuotaClampNaN, Original: value, Clamped: 0}
	case value > float64(MaxQuota):
		clamp = &QuotaClamp{Op: op, Kind: QuotaClampOverflow, Original: value, Clamped: MaxQuota}
	case value < float64(MinQuota):
		clamp = &QuotaClamp{Op: op, Kind: QuotaClampUnderflow, Original: value, Clamped: MinQuota}
	default:
		return int64(value), nil
	}
	SysError(clamp.Error())
	return clamp.Clamped, clamp
}

func strictQuota(quota int64, clamp *QuotaClamp) (int64, error) {
	if clamp != nil {
		return 0, clamp
	}
	return quota, nil
}

// QuotaFromFloat converts a computed quota value to int64, truncating toward
// zero, with saturation. Use for float products of prices, ratios, and
// user-controlled multipliers (image n, video seconds, resolution ratios).
func QuotaFromFloat(value float64) int64 {
	quota, _ := QuotaFromFloatChecked(value)
	return quota
}

// QuotaFromFloatChecked is QuotaFromFloat but also returns a non-nil
// *QuotaClamp when the value was clamped, so billing callers can audit it.
func QuotaFromFloatChecked(value float64) (int64, *QuotaClamp) {
	return saturateQuota(value, "QuotaFromFloat")
}

// QuotaFromFloatStrict converts an in-range value and returns a typed
// *QuotaClamp error instead of allowing a saturated result to reach billing.
func QuotaFromFloatStrict(value float64) (int64, error) {
	return strictQuota(QuotaFromFloatChecked(value))
}

// QuotaRound converts a float64 quota value to int64 using half-away-from-zero
// rounding, with saturation. Every tiered billing path (pre-consume,
// settlement, breakdown validation, log fields) MUST use this to avoid +-1
// discrepancies.
func QuotaRound(value float64) int64 {
	quota, _ := QuotaRoundChecked(value)
	return quota
}

// QuotaRoundChecked is QuotaRound but also returns a non-nil *QuotaClamp when
// the value was clamped, so billing callers can audit it.
func QuotaRoundChecked(value float64) (int64, *QuotaClamp) {
	return saturateQuota(math.Round(value), "QuotaRound")
}

// QuotaRoundStrict rounds an in-range value and returns a typed *QuotaClamp
// error instead of allowing a saturated result to reach billing.
func QuotaRoundStrict(value float64) (int64, error) {
	return strictQuota(QuotaRoundChecked(value))
}

// QuotaFromDecimal converts a computed quota decimal to int64 with saturation.
// The decimal is rounded (half away from zero) before conversion.
func QuotaFromDecimal(d decimal.Decimal) int64 {
	quota, _ := QuotaFromDecimalChecked(d)
	return quota
}

// QuotaFromDecimalChecked is QuotaFromDecimal but also returns a non-nil
// *QuotaClamp when the value was clamped, so billing callers can audit it.
func QuotaFromDecimalChecked(d decimal.Decimal) (int64, *QuotaClamp) {
	rounded := d.Round(0)
	max := decimal.NewFromInt(MaxQuota)
	min := decimal.NewFromInt(MinQuota)
	if rounded.GreaterThan(max) {
		original, _ := rounded.Float64()
		clamp := &QuotaClamp{Op: "QuotaFromDecimal", Kind: QuotaClampOverflow, Original: original, Clamped: MaxQuota}
		SysError(clamp.Error())
		return MaxQuota, clamp
	}
	if rounded.LessThan(min) {
		original, _ := rounded.Float64()
		clamp := &QuotaClamp{Op: "QuotaFromDecimal", Kind: QuotaClampUnderflow, Original: original, Clamped: MinQuota}
		SysError(clamp.Error())
		return MinQuota, clamp
	}
	return rounded.IntPart(), nil
}

// QuotaFromDecimalStrict rounds an in-range decimal and rejects saturation.
func QuotaFromDecimalStrict(d decimal.Decimal) (int64, error) {
	return strictQuota(QuotaFromDecimalChecked(d))
}

// TokenCountFromFloat converts a derived token count without allowing hostile
// media metadata to wrap an int. Token counts remain 32-bit compatible because
// their persisted columns and request DTOs are still int-based.
func TokenCountFromFloat(value float64) int {
	switch {
	case math.IsNaN(value), value <= 0:
		return 0
	case value >= math.MaxInt32:
		SysError(fmt.Sprintf("token count conversion overflow: original=%g, clamped=%d", value, math.MaxInt32))
		return math.MaxInt32
	default:
		return int(value)
	}
}
