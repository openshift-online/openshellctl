package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/api/v1alpha1"
	"github.com/openshift-online/openshellctl/pkg/paramtemplate"
)

// templateFlags groups sandbox create's manifest-templating flags
// (ROSAENG-74244): -p/--param, --param-file, --parameters,
// --ignore-unknown-parameters.
type templateFlags struct {
	params                  []string
	paramFiles              []string
	printParameters         bool
	ignoreUnknownParameters bool
}

// osFS is the fs.FS used to read --param-file paths in production. It opens
// via os.Open directly rather than os.DirFS, so it accepts the same absolute
// or relative paths --param-file users pass today, with none of
// os.DirFS/fs.ValidPath's rooted-filesystem restrictions. Tests inject
// fstest.MapFS instead (pkg/paramtemplate's own tests, and
// sandbox_create_params_test.go's CLI-layer tests).
type osFS struct{}

func (osFS) Open(name string) (fs.File, error) { return os.Open(name) }

// needsTemplating is the pure decision behind loadManifestWithParams's
// round-trip skip: a manifest with no "${" substring at all and no template
// flags needs no templating, full stop — decodeManifestBytes(data) runs on
// the original bytes and nothing else in this file is ever called. This
// substring check is deliberately a cheap pre-filter, not the real
// reference scan (CollectReferences, which needs a parsed document); it
// only has to be conservative (never say "no" when a real ${KEY} reference
// is present), not precise (it may say "yes" for an escaped $${KEY} or a
// manifest where --param was given but is unreferenced — applyManifestTemplate
// itself falls back to returning raw unchanged in exactly those cases).
func needsTemplating(data []byte, tf templateFlags) bool {
	return tf.printParameters || len(tf.params) > 0 || len(tf.paramFiles) > 0 || bytes.Contains(data, []byte("${"))
}

// loadManifestWithParams reads the -f manifest (path, or "-" for stdin) and,
// when parameter templating is requested or the manifest text could contain
// a ${KEY} reference, substitutes parameters before decoding — otherwise it
// decodes the original bytes unchanged, byte-for-byte identical to today's
// behavior (no YAML round-trip, so no reformatting drift for manifests that
// never use the feature).
//
// Only `sandbox create`'s own RunE calls this — every other -f consumer
// (doctor, namesFromManifest for get/connect/upload/download/ssh-config/
// logs/stop/start/delete/provider) goes through the plain loadManifest
// below, by design: ROSAENG-74244 is scoped to `sandbox create` only, and a
// manifest with a non-template "${...}"-shaped string (e.g. a shell command
// embedding "${HOME}") being treated as a required, now-unresolved
// parameter is an intentional consequence of adopting template syntax (the
// same tradeoff oc process makes) that must stay contained to the one
// command that opts into it.
//
// On --parameters, it writes the manifest's parameter names to cmd's stdout
// and returns (nil, true, nil): the caller must create nothing and exit 0.
func loadManifestWithParams(cmd *cobra.Command, fsys fs.FS, path string, tf templateFlags) (*v1alpha1.Sandbox, bool, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = readAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, false, err
	}

	if !needsTemplating(data, tf) {
		m, derr := decodeManifestBytes(data)
		return m, false, derr
	}

	substituted, printedOnly, err := applyManifestTemplate(fsys, cmd.OutOrStdout(), cmd.ErrOrStderr(), data, tf)
	if err != nil {
		return nil, false, err
	}
	if printedOnly {
		return nil, true, nil
	}
	m, err := decodeManifestBytes(substituted)
	return m, false, err
}

// applyManifestTemplate runs the parameter-substitution pipeline: parse the
// manifest as YAML, collect its ${KEY} references, resolve --param/
// --param-file values (warning on stderr about any --param that overrides a
// --param-file value), reject unknown parameters unless
// --ignore-unknown-parameters is set, substitute, and render. A --parameters
// request short-circuits after collecting references: it writes them to out
// and returns (nil, true, nil).
//
// When refs is empty, raw is returned unchanged instead of going through
// Substitute/Render — there is nothing to substitute, and skipping the
// render avoids reformatting a manifest whose only "${"-looking text turned
// out to be an escaped "$${KEY}" or a bare "$KEY" (loadManifestWithParams's
// substring pre-filter can't tell those apart from a real reference without
// actually parsing).
func applyManifestTemplate(fsys fs.FS, out, errw io.Writer, raw []byte, tf templateFlags) ([]byte, bool, error) {
	doc, err := paramtemplate.ParseDocument(raw)
	if err != nil {
		return nil, false, err
	}
	refs := paramtemplate.CollectReferences(doc)

	if tf.printParameters {
		for _, k := range refs {
			_, _ = fmt.Fprintln(out, k)
		}
		return nil, true, nil
	}

	flagParams := make([]paramtemplate.Param, 0, len(tf.params))
	for _, raw := range tf.params {
		p, perr := paramtemplate.ParseParam(raw)
		if perr != nil {
			return nil, false, &UsageError{Err: perr}
		}
		flagParams = append(flagParams, p)
	}

	fileSources := make([]map[string]string, 0, len(tf.paramFiles))
	for _, path := range tf.paramFiles {
		m, ferr := paramtemplate.LoadParamFile(fsys, path)
		if ferr != nil {
			return nil, false, &UsageError{Err: ferr}
		}
		fileSources = append(fileSources, m)
	}

	values, overridden := paramtemplate.MergeParams(fileSources, flagParams)
	for _, k := range overridden {
		_, _ = fmt.Fprintf(errw, "Warning: --param %s overrides value from --param-file\n", k)
	}

	if !tf.ignoreUnknownParameters {
		if unknown := paramtemplate.UnknownKeys(values, refs); len(unknown) > 0 {
			return nil, false, &UsageError{Err: &paramtemplate.UnknownParamsError{Keys: unknown}}
		}
	}

	if len(refs) == 0 {
		return raw, false, nil
	}

	if missing := paramtemplate.Substitute(doc, values); len(missing) > 0 {
		return nil, false, &UsageError{Err: &paramtemplate.UnresolvedParamsError{Keys: missing}}
	}

	substituted, err := paramtemplate.Render(doc)
	if err != nil {
		return nil, false, err
	}
	return substituted, false, nil
}

// decodeManifestBytes is loadManifest's original tail end: decode+validate,
// shared by the templating and non-templating paths so both produce
// identical errors for the same (possibly substituted) manifest bytes.
func decodeManifestBytes(data []byte) (*v1alpha1.Sandbox, error) {
	m, err := v1alpha1.Decode(bytesReader(data))
	if err != nil {
		return nil, err
	}
	if errs := m.Validate(); len(errs) > 0 {
		return nil, &UsageError{Err: fmt.Errorf("manifest invalid: %v", errs[0])}
	}
	return m, nil
}
