package severity

import "fmt"

// OtlpLogSeverityRange represents the OpenTelemetry log severity ranges.
// Each range covers a band of four severity numbers as defined by the
// OpenTelemetry specification.
type OtlpLogSeverityRange string

const (
	Unknown OtlpLogSeverityRange = "UNKNOWN"
	Trace       OtlpLogSeverityRange = "TRACE"
	Debug       OtlpLogSeverityRange = "DEBUG"
	Info        OtlpLogSeverityRange = "INFO"
	Warn        OtlpLogSeverityRange = "WARN"
	Error       OtlpLogSeverityRange = "ERROR"
	Fatal       OtlpLogSeverityRange = "FATAL"
)

// FromNumber maps an OTel severity number (0–24) to its severity range.
// Numbers outside the defined bands return a formatted string like "SEVERITY_25".
func FromNumber(n int32) string {
	switch {
	case n == 0:
		return string(Unknown)
	case n >= 1 && n <= 4:
		return string(Trace)
	case n >= 5 && n <= 8:
		return string(Debug)
	case n >= 9 && n <= 12:
		return string(Info)
	case n >= 13 && n <= 16:
		return string(Warn)
	case n >= 17 && n <= 20:
		return string(Error)
	case n >= 21 && n <= 24:
		return string(Fatal)
	default:
		return fmt.Sprintf("SEVERITY_%d", n)
	}
}
