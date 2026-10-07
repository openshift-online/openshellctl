package output

import (
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

// RenderGatewayList writes `gateway list`'s table/json/yaml view. Empty ->
// "No gateways found." (parallels RenderSandboxList's "No sandboxes found.").
func RenderGatewayList(w io.Writer, gateways []gatewayconfig.DetailedInfo, format Format) error {
	switch format {
	case FormatJSON:
		return renderGatewayListJSON(w, gateways)
	case FormatYAML:
		return renderGatewayListYAML(w, gateways)
	default:
		return renderGatewayListTable(w, gateways)
	}
}

// renderGatewayListTable writes the table view. The active gateway's NAME
// column is prefixed "* ", others "  ", so the marker lines up without a
// separate column.
func renderGatewayListTable(w io.Writer, gateways []gatewayconfig.DetailedInfo) error {
	if len(gateways) == 0 {
		_, err := fmt.Fprintln(w, "No gateways found.")
		return err
	}

	markedNames := make([]string, len(gateways))
	endpoints := make([]string, len(gateways))
	types := make([]string, len(gateways))
	sources := make([]string, len(gateways))
	for i, g := range gateways {
		markedNames[i] = activeMarker(g.Active) + g.Name
		endpoints[i] = g.Endpoint
		types[i] = g.Type
		sources[i] = string(g.Source)
	}
	nameWidth := maxLen(markedNames, 6) // "NAME" has no marker prefix; floor covers short names
	endpointWidth := maxLen(endpoints, 8)
	typeWidth := maxLen(types, 4)
	sourceWidth := maxLen(sources, 6)

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n",
		pad("NAME", nameWidth), pad("ENDPOINT", endpointWidth), pad("TYPE", typeWidth), pad("AUTH", 4), "SOURCE")
	for i, g := range gateways {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n",
			pad(markedNames[i], nameWidth), pad(g.Endpoint, endpointWidth), pad(g.Type, typeWidth), pad(g.Auth, 4), pad(sources[i], sourceWidth))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func activeMarker(active bool) string {
	if active {
		return "* "
	}
	return "  "
}

// gatewayMap is the sorted-key map used for the JSON/YAML views.
func gatewayMap(g gatewayconfig.DetailedInfo) map[string]any {
	return map[string]any{
		"active":   g.Active,
		"auth":     g.Auth,
		"endpoint": g.Endpoint,
		"name":     g.Name,
		"source":   string(g.Source),
		"type":     g.Type,
	}
}

func renderGatewayListJSON(w io.Writer, gateways []gatewayconfig.DetailedInfo) error {
	maps := make([]map[string]any, len(gateways))
	for i, g := range gateways {
		maps[i] = gatewayMap(g)
	}
	data, err := marshalSortedJSON(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func renderGatewayListYAML(w io.Writer, gateways []gatewayconfig.DetailedInfo) error {
	maps := make([]map[string]any, len(gateways))
	for i, g := range gateways {
		maps[i] = gatewayMap(g)
	}
	data, err := yaml.Marshal(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
