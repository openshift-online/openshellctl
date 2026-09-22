package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/openshellctl/pkg/gateway"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
	"github.com/openshift-online/openshellctl/pkg/transfer"
)

func newSandboxUploadCommand() *cobra.Command {
	var (
		noGitIgnore bool
		file        string
	)
	c := &cobra.Command{
		Use:   "upload NAME LOCAL_PATH [DEST]",
		Short: "Upload files to a sandbox",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			sbName, local, dest, err := parseUploadArgs(cmd, file, args)
			if err != nil {
				return err
			}

			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				if sbName == "" {
					resolved, rerr := resolveSandboxName(nil, target, ws)
					if rerr != nil {
						return rerr
					}
					sbName = resolved
				}
				defer saveLastSandbox(target, ws, sbName)

				fsys, fsPath, err := resolveUploadFS(local)
				if err != nil {
					return err
				}

				tc := transfer.New(gw, fsys, wallClock{})
				return tc.Upload(cmd.Context(), ws, sbName, fsPath, dest, !noGitIgnore, func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				})
			})
		},
	}
	c.Flags().BoolVar(&noGitIgnore, "no-git-ignore", false, "disable .gitignore filtering for directory uploads")
	c.Flags().StringVarP(&file, "file", "f", "", "manifest file to read sandbox name from (- for stdin)")
	return c
}

func newSandboxDownloadCommand() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use:   "download NAME SANDBOX_PATH [DEST]",
		Short: "Download files from a sandbox",
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			sbName, remote, dest, err := parseDownloadArgs(cmd, file, args)
			if err != nil {
				return err
			}

			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				if sbName == "" {
					resolved, rerr := resolveSandboxName(nil, target, ws)
					if rerr != nil {
						return rerr
					}
					sbName = resolved
				}
				defer saveLastSandbox(target, ws, sbName)

				tc := transfer.New(gw, nil, wallClock{})
				return tc.Download(cmd.Context(), ws, sbName, remote, dest, func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				})
			})
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "manifest file to read sandbox name from (- for stdin)")
	return c
}

// parseUploadArgs resolves (name, local, dest) from the Rust-CLI-style positional
// args: NAME LOCAL_PATH [DEST]. When -f is used, NAME is omitted from positional
// args: LOCAL_PATH [DEST].
func parseUploadArgs(cmd *cobra.Command, file string, args []string) (name, local, dest string, err error) {
	if file != "" {
		names, ferr := namesFromManifest(cmd, file)
		if ferr != nil {
			return "", "", "", ferr
		}
		name = names[0]
		switch len(args) {
		case 1:
			local = args[0]
		case 2:
			local, dest = args[0], args[1]
		default:
			return "", "", "", &UsageError{Err: fmt.Errorf("with -f, expected LOCAL_PATH [DEST], got %d args", len(args))}
		}
		return name, local, dest, nil
	}
	switch len(args) {
	case 2:
		return args[0], args[1], "", nil
	case 3:
		return args[0], args[1], args[2], nil
	default:
		return "", "", "", &UsageError{Err: fmt.Errorf("expected NAME LOCAL_PATH [DEST], got %d args", len(args))}
	}
}

// parseDownloadArgs resolves (name, remote, dest) from the Rust-CLI-style
// positional args: NAME SANDBOX_PATH [DEST]. When -f is used, NAME is omitted:
// SANDBOX_PATH [DEST].
func parseDownloadArgs(cmd *cobra.Command, file string, args []string) (name, remote, dest string, err error) {
	if file != "" {
		names, ferr := namesFromManifest(cmd, file)
		if ferr != nil {
			return "", "", "", ferr
		}
		name = names[0]
		switch len(args) {
		case 1:
			remote = args[0]
		case 2:
			remote, dest = args[0], args[1]
		default:
			return "", "", "", &UsageError{Err: fmt.Errorf("with -f, expected SANDBOX_PATH [DEST], got %d args", len(args))}
		}
		return name, remote, dest, nil
	}
	switch len(args) {
	case 2:
		return args[0], args[1], "", nil
	case 3:
		return args[0], args[1], args[2], nil
	default:
		return "", "", "", &UsageError{Err: fmt.Errorf("expected NAME SANDBOX_PATH [DEST], got %d args", len(args))}
	}
}

// splitColonSpec splits "a:b" on the last colon — used by create's --upload flag
// where the format is LOCAL:DEST (no sandbox name, since the sandbox is being
// created). Not used by the upload/download subcommands which take separate
// positional args.
func splitColonSpec(spec string) (string, string) {
	i := strings.LastIndex(spec, ":")
	if i < 0 {
		return spec, ""
	}
	return spec[:i], spec[i+1:]
}

// resolveUploadFS builds an fs.FS and a relative fsPath suitable for
// transfer.Upload. Absolute paths are rooted at "/"; relative paths at cwd.
func resolveUploadFS(local string) (fs.FS, string, error) {
	abs, err := filepath.Abs(local)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve path %q: %w", local, err)
	}
	if filepath.IsAbs(local) {
		return transfer.OSFS("/"), strings.TrimPrefix(filepath.Clean(local), "/"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return nil, "", err
	}
	return transfer.OSFS(cwd), filepath.ToSlash(rel), nil
}
