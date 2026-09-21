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
	var noGitIgnore bool
	c := &cobra.Command{
		Use:   "upload LOCAL[:DEST] [NAME]",
		Short: "Upload files to a sandbox",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := args[0]
			local, dest := parseUploadSpec(spec)

			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				sbName, err := resolveSandboxName(args[1:], target, ws)
				if err != nil {
					return err
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
	return c
}

func newSandboxDownloadCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "download REMOTE[:DEST] [NAME]",
		Short: "Download files from a sandbox",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := args[0]
			remote, dest := parseDownloadSpec(spec)

			return withGatewayTarget(cmd, func(gw gateway.Gateway, target *gatewayconfig.Target) error {
				ws := workspace()
				sbName, err := resolveSandboxName(args[1:], target, ws)
				if err != nil {
					return err
				}
				defer saveLastSandbox(target, ws, sbName)

				tc := transfer.New(gw, nil, wallClock{})
				return tc.Download(cmd.Context(), ws, sbName, remote, dest, func(msg string) {
					fmt.Fprintln(cmd.ErrOrStderr(), msg)
				})
			})
		},
	}
	return c
}

// parseUploadSpec splits "local[:dest]" — colon separates only when not part of
// a Windows drive letter (irrelevant here, but defensive). The last colon wins
// (rfind semantics matching the upstream CLI).
func parseUploadSpec(spec string) (local, dest string) {
	i := strings.LastIndex(spec, ":")
	if i < 0 {
		return spec, ""
	}
	return spec[:i], spec[i+1:]
}

// parseDownloadSpec splits "remote[:dest]".
func parseDownloadSpec(spec string) (remote, dest string) {
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
