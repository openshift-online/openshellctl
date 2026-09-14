package output

import (
	"fmt"
	"sort"
	"strings"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// FormatLogLine renders a log line: "[<secs>.<millis:03>] [<source:<7>]
// [<level:<5>] [<target>] <message>" plus " k=v k=v" (sorted) when fields are
// non-empty. An empty source renders as "gateway" (run.rs:6911-7045, A.9).
func FormatLogLine(l types.LogLine) string {
	source := l.Source
	if source == "" {
		source = "gateway"
	}

	ms := l.Timestamp.UnixMilli()
	secs := ms / 1000
	millis := ms % 1000
	if millis < 0 {
		millis += 1000
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[%d.%03d] [%-7s] [%-5s] [%s] %s",
		secs, millis, source, l.Level, l.Target, l.Message)

	if len(l.Fields) > 0 {
		keys := make([]string, 0, len(l.Fields))
		for k := range l.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, " %s=%s", k, l.Fields[k])
		}
	}
	return b.String()
}
