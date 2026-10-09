package output

import (
	"fmt"
	"io"
	"strings"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"sigs.k8s.io/yaml"
)

// RenderProviders writes `provider list`'s table/json/yaml view. Empty ->
// "No providers found." Never prints credential values — only key names and
// counts, matching RenderProviderList's (sandbox-scoped) discipline.
func RenderProviders(w io.Writer, providers []*types.Provider, format Format) error {
	switch format {
	case FormatJSON:
		return renderProvidersJSON(w, providers)
	case FormatYAML:
		return renderProvidersYAML(w, providers)
	default:
		return renderProvidersTable(w, providers)
	}
}

// RenderProvider writes `provider get`'s single-item table/json/yaml view.
func RenderProvider(w io.Writer, p *types.Provider, format Format) error {
	switch format {
	case FormatJSON:
		data, err := marshalSortedJSON(providerMap(p))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	case FormatYAML:
		data, err := yaml.Marshal(providerMap(p))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	default:
		return renderProviderTable(w, p)
	}
}

// RenderProviderNames writes one provider name per line, for `provider list
// --names`.
func RenderProviderNames(w io.Writer, providers []*types.Provider) error {
	var b strings.Builder
	for _, p := range providers {
		fmt.Fprintf(&b, "%s\n", p.Name)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func renderProvidersTable(w io.Writer, providers []*types.Provider) error {
	if len(providers) == 0 {
		_, err := fmt.Fprintln(w, "No providers found.")
		return err
	}

	names := make([]string, len(providers))
	ptypes := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
		ptypes[i] = p.Type
	}
	nameWidth := maxLen(names, 4)
	typeWidth := maxLen(ptypes, 4)
	const credWidth = 16

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s  %s\n",
		pad("NAME", nameWidth), pad("TYPE", typeWidth), pad("CREDENTIAL_KEYS", credWidth), "CONFIG_KEYS")
	for _, p := range providers {
		fmt.Fprintf(&b, "%s  %s  %s  %d\n",
			pad(p.Name, nameWidth), pad(p.Type, typeWidth), pad(fmt.Sprintf("%d", len(p.Spec.Credentials)), credWidth), len(p.Spec.Config))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func renderProviderTable(w io.Writer, p *types.Provider) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Name:      %s\n", p.Name)
	fmt.Fprintf(&b, "Type:      %s\n", p.Type)
	fmt.Fprintf(&b, "Workspace: %s\n", p.Workspace)
	fmt.Fprintf(&b, "Created:   %s\n", formatCreatedAt(p.CreatedAt))
	fmt.Fprintf(&b, "Credential keys: %s\n", strings.Join(sortedStringKeys(p.Spec.Credentials), ", "))
	fmt.Fprintf(&b, "Config:\n")
	for _, k := range sortedStringKeys(p.Spec.Config) {
		fmt.Fprintf(&b, "  %s: %s\n", k, p.Spec.Config[k])
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func renderProvidersJSON(w io.Writer, providers []*types.Provider) error {
	maps := make([]map[string]any, len(providers))
	for i, p := range providers {
		maps[i] = providerMap(p)
	}
	data, err := marshalSortedJSON(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func renderProvidersYAML(w io.Writer, providers []*types.Provider) error {
	maps := make([]map[string]any, len(providers))
	for i, p := range providers {
		maps[i] = providerMap(p)
	}
	data, err := yaml.Marshal(maps)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// providerMap is the sorted-key map used for list/get JSON+YAML. Credential
// *values* are never included — only their key names, so scripting against
// this output can tell which credentials are set without ever seeing a
// secret.
func providerMap(p *types.Provider) map[string]any {
	return map[string]any{
		"config":           emptyMapIfNil(p.Spec.Config),
		"created_at":       formatCreatedAt(p.CreatedAt),
		"credential_keys":  sortedStringKeys(p.Spec.Credentials),
		"id":               p.ID,
		"name":             p.Name,
		"resource_version": p.ResourceVersion,
		"type":             p.Type,
		"workspace":        p.Workspace,
	}
}
