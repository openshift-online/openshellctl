package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/api/v1alpha1"
	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/sandbox"
)

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
func bytesReader(b []byte) io.Reader      { return bytes.NewReader(b) }

func newSandboxCreateCommand() *cobra.Command {
	var (
		name         string
		from         string
		file         string
		cpu          string
		memory       string
		providers    []string
		labels       []string
		env          []string
		approvalMode string
		format       string
		editor       string
		noKeep       bool
		noCredWarn   bool
	)
	c := &cobra.Command{
		Use:   "create [-- COMMAND...]",
		Short: "Create a sandbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			if editor != "" {
				return &UsageError{Err: fmt.Errorf("--editor is not supported by openshellctl")}
			}

			flags, err := buildCreateFlags(cmd, createFlagInput{
				name: name, from: from, cpu: cpu, memory: memory,
				providers: providers, labels: labels, env: env,
				approvalMode: approvalMode, format: format,
				noKeep: noKeep, noCredWarn: noCredWarn, command: commandArgs(cmd, args),
			})
			if err != nil {
				return err
			}

			var manifest *v1alpha1.Sandbox
			if file != "" {
				manifest, err = loadManifest(cmd, file)
				if err != nil {
					return err
				}
			}

			req, err := sandbox.MergeManifestAndFlags(manifest, flags)
			if err != nil {
				return err
			}
			if err := validateCreateRequest(req); err != nil {
				return err
			}

			return withGateway(cmd, func(gw gateway.Gateway) error {
				deps := sandbox.CreateDeps{
					GW:          gw,
					Env:         os.Getenv,
					Stderr:      cmd.ErrOrStderr(),
					Sink:        newPlainSink(cmd.ErrOrStderr()),
					Watch:       req.Output == "table",
					IdleTimeout: provisionTimeout(),
				}
				res, err := sandbox.Create(cmd.Context(), deps, req, resolveTTY(req))
				if err != nil {
					return err
				}
				return renderSandbox(cmd, res.Sandbox, req.Output)
			})
		},
	}
	f := c.Flags()
	f.StringVar(&name, "name", "", "sandbox name")
	f.StringVar(&from, "from", "", "image reference or community name")
	f.StringVarP(&file, "file", "f", "", "manifest file (- for stdin)")
	f.StringVar(&cpu, "cpu", "", "CPU request")
	f.StringVar(&memory, "memory", "", "memory request")
	f.StringSliceVar(&providers, "provider", nil, "provider to attach (repeatable)")
	f.StringSliceVar(&labels, "label", nil, "label key=value (repeatable)")
	f.StringSliceVar(&env, "env", nil, "env KEY=VALUE (repeatable)")
	f.StringVar(&approvalMode, "approval-mode", "", "proposal approval mode: manual|auto")
	f.StringVarP(&format, "output", "o", "", "output format: table|json|yaml")
	f.StringVar(&editor, "editor", "", "(unsupported)")
	f.BoolVar(&noKeep, "no-keep", false, "delete the sandbox after the session")
	f.BoolVar(&noCredWarn, "no-credential-warnings", false, "suppress credential warnings")
	return c
}

type createFlagInput struct {
	name, from, cpu, memory string
	providers, labels, env  []string
	approvalMode, format    string
	noKeep, noCredWarn      bool
	command                 []string
}

func buildCreateFlags(cmd *cobra.Command, in createFlagInput) (sandbox.CreateFlags, error) {
	f := sandbox.CreateFlags{
		Name:                 in.name,
		CPU:                  in.cpu,
		Memory:               in.memory,
		Providers:            in.providers,
		ApprovalMode:         in.approvalMode,
		Output:               in.format,
		Command:              in.command,
		NoCredentialWarnings: in.noCredWarn,
		Workspace:            workspace(),
	}
	if in.noKeep {
		no := false
		f.Keep = &no
	}
	if len(in.labels) > 0 {
		m, err := sandbox.ParseLabels(in.labels)
		if err != nil {
			return f, &UsageError{Err: err}
		}
		f.Labels = m
	}
	if len(in.env) > 0 {
		m, err := sandbox.ParseEnvPairs(in.env)
		if err != nil {
			return f, &UsageError{Err: err}
		}
		f.Env = m
	}
	if in.from != "" {
		img, err := sandbox.ResolveImage(in.from, "", os.Stat)
		if err != nil {
			// Dockerfile/local-build → usage error (exit 2).
			return f, &UsageError{Err: err}
		}
		f.Image = img
	}
	return f, nil
}

// commandArgs returns the trailing COMMAND after "--".
func commandArgs(cmd *cobra.Command, args []string) []string {
	if cmd.ArgsLenAtDash() >= 0 {
		return args[cmd.ArgsLenAtDash():]
	}
	return nil
}

// validateCreateRequest runs the authoritative cpu/memory validators at merge.
func validateCreateRequest(r *sandbox.CreateRequest) error {
	if r.CPU != "" {
		if err := sandbox.ValidateCPU(r.CPU); err != nil {
			return &UsageError{Err: err}
		}
	}
	if r.Memory != "" {
		if err := sandbox.ValidateMemory(r.Memory); err != nil {
			return &UsageError{Err: err}
		}
	}
	return nil
}

// resolveTTY resolves the effective TTY value (explicit override wins; default
// false in the non-interactive create-without-attach path).
func resolveTTY(r *sandbox.CreateRequest) bool {
	if r.TTY != nil {
		return *r.TTY
	}
	return false
}

// loadManifest decodes a -f manifest (- = stdin) and validates it.
func loadManifest(cmd *cobra.Command, path string) (*v1alpha1.Sandbox, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = readAll(cmd.InOrStdin())
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	m, err := v1alpha1.Decode(bytesReader(data))
	if err != nil {
		return nil, err
	}
	if errs := m.Validate(); len(errs) > 0 {
		return nil, &UsageError{Err: fmt.Errorf("manifest invalid: %v", errs[0])}
	}
	return m, nil
}
