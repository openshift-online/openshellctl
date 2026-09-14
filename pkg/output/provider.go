package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	types "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// RenderProviderList writes the `sandbox provider list` table
// (run.rs:2152-2175, 2293-2347). Empty → "No providers attached to sandbox <name>.".
func RenderProviderList(w io.Writer, sandbox string, providers []*types.Provider) error {
	if len(providers) == 0 {
		_, err := fmt.Fprintf(w, "No providers attached to sandbox %s.\n", sandbox)
		return err
	}

	names := make([]string, len(providers))
	pTypes := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
		pTypes[i] = p.Type
	}
	nameWidth := maxLen(names, 4)
	typeWidth := maxLen(pTypes, 4)
	const credWidth = 16

	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s  %s\n",
		pad("NAME", nameWidth), pad("TYPE", typeWidth), pad("CREDENTIAL_KEYS", credWidth), "CONFIG_KEYS")
	for _, p := range providers {
		credCount := providerCredentialCount(p)
		cfgCount := len(p.Spec.Config)
		fmt.Fprintf(&b, "%s  %s  %s  %d\n",
			pad(p.Name, nameWidth), pad(p.Type, typeWidth), pad(fmt.Sprintf("%d", credCount), credWidth), cfgCount)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// providerCredentialCount is len(sorted(dedupe(keys(credentials) ∪ keys(handles)))).
func providerCredentialCount(p *types.Provider) int {
	set := map[string]struct{}{}
	for k := range p.Spec.Credentials {
		set[k] = struct{}{}
	}
	for k := range p.Spec.CredentialHandles {
		set[k] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return len(keys)
}
