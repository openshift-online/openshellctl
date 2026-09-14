package sandbox

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ValidateCPU mirrors run.rs:280-308. Accepts <digits>m (millicores) or a finite
// positive float (cores). Messages are verbatim (Appendix A.6).
func ValidateCPU(v string) error {
	if v == "" {
		return errors.New("--cpu must not be empty")
	}
	invalid := fmt.Errorf("invalid --cpu value '%s': expected positive cores or millicores, for example 2, 0.5, or 500m", v)

	if strings.HasSuffix(v, "m") {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "m"))
		if err != nil {
			return invalid
		}
		if n <= 0 {
			return errors.New("--cpu must be greater than zero")
		}
		return nil
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil || isNaNOrInf(f) {
		return invalid
	}
	if f <= 0 {
		return errors.New("--cpu must be greater than zero")
	}
	return nil
}

// memorySuffixes are the accepted --memory suffixes (Appendix A.6).
var memorySuffixes = []string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "K", "M", "G", "T", "P", "E"}

// ValidateMemory mirrors run.rs:310-336. Accepts positive bytes or a quantity
// with a known suffix. Messages verbatim (Appendix A.6).
func ValidateMemory(v string) error {
	if v == "" {
		return errors.New("--memory must not be empty")
	}
	invalid := fmt.Errorf("invalid --memory value '%s': expected positive bytes or a quantity such as 512Mi, 4Gi, or 8G", v)

	num := v
	for _, suf := range memorySuffixes {
		if strings.HasSuffix(v, suf) {
			num = strings.TrimSuffix(v, suf)
			break
		}
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		return invalid
	}
	if n <= 0 {
		return errors.New("--memory must be greater than zero")
	}
	return nil
}

// ResourcesStruct builds the template resources map: {"limits": {"cpu", "memory"}}
// with only non-empty keys; nil when both are empty (run.rs:227-262).
func ResourcesStruct(cpu, memory string) map[string]any {
	limits := map[string]any{}
	if cpu != "" {
		limits["cpu"] = cpu
	}
	if memory != "" {
		limits["memory"] = memory
	}
	if len(limits) == 0 {
		return nil
	}
	return map[string]any{"limits": limits}
}

func isNaNOrInf(f float64) bool {
	return f != f || f > 1e308 || f < -1e308
}
