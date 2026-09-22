package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-online/openshellctl/pkg/auth"
	"github.com/openshift-online/openshellctl/pkg/gatewayconfig"
)

func newTokenCommand() *cobra.Command {
	tok := &cobra.Command{
		Use:   "token",
		Short: "Inspect and manage the gateway auth token",
	}
	tok.AddCommand(newTokenInspectCommand(), newTokenShowCommand(), newTokenRefreshCommand())
	return tok
}

// newTokenInspectCommand decodes a JWT passed as an argument (no gateway needed).
func newTokenInspectCommand() *cobra.Command {
	var output string
	c := &cobra.Command{
		Use:   "inspect <jwt>",
		Short: "Decode a JWT's claims (no signature verification)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			claims, err := auth.Inspect(args[0])
			if err != nil {
				return err
			}
			return writeClaims(cmd.OutOrStdout(), claims, output)
		},
	}
	c.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	return c
}

func writeClaims(w io.Writer, c *auth.Claims, output string) error {
	if output == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(c.Raw)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Issuer:    %s\n", c.Iss)
	fmt.Fprintf(&b, "Subject:   %s\n", c.Sub)
	fmt.Fprintf(&b, "Audience:  %s\n", strings.Join(c.Aud, ", "))
	if c.PreferredUsername != "" {
		fmt.Fprintf(&b, "Username:  %s\n", c.PreferredUsername)
	}
	fmt.Fprintf(&b, "Roles:     %s\n", strings.Join(c.Roles, ", "))
	if c.Exp > 0 {
		fmt.Fprintf(&b, "Expires:   %s\n", time.Unix(c.Exp, 0).UTC().Format(time.RFC3339))
	}
	if c.Iat > 0 {
		fmt.Fprintf(&b, "IssuedAt:  %s\n", time.Unix(c.Iat, 0).UTC().Format(time.RFC3339))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// newTokenShowCommand resolves and prints the active token's metadata.
func newTokenShowCommand() *cobra.Command {
	var output string
	c := &cobra.Command{
		Use:   "show",
		Short: "Show the resolved token (age, expiry, subject, audience, roles)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			src, target, err := resolveTokenSource(cmd)
			if err != nil {
				return err
			}
			tok, err := src.Token(cmd.Context())
			if err != nil {
				return err
			}
			if err := writeToken(cmd.OutOrStdout(), tok, src.Describe(), output); err != nil {
				return err
			}
			// whoami parity: report the gateway's view of the caller (§8.3).
			// Best-effort — a dial/RPC failure is a warning, not an error, so
			// `token show` still succeeds offline.
			reportCurrentUser(cmd, target, src, output)
			return nil
		},
	}
	c.Flags().StringVarP(&output, "output", "o", "text", "output format: text|json")
	return c
}

// reportCurrentUser dials the gateway and prints its CurrentUser view. Failures
// are reported to stderr and do not fail the command (text mode only, to keep
// -o json output machine-parseable).
func reportCurrentUser(cmd *cobra.Command, target *gatewayconfig.Target, src auth.TokenSource, output string) {
	if output != "text" {
		return
	}
	gw, conn, err := dialGateway(target, src)
	if err != nil {
		cmd.PrintErrf("warning: could not reach gateway for whoami: %v\n", err)
		return
	}
	defer func() { _ = conn.Close() }()

	user, err := gw.CurrentUser(cmd.Context())
	if err != nil {
		cmd.PrintErrf("warning: whoami failed: %v\n", err)
		return
	}
	cmd.Printf("\nGateway view (whoami):\n")
	cmd.Printf("  Subject:  %s\n", user.Subject)
	if user.DisplayName != "" {
		cmd.Printf("  Name:     %s\n", user.DisplayName)
	}
	cmd.Printf("  Roles:    %s\n", strings.Join(user.Roles, ", "))
}

func writeToken(w io.Writer, tok *auth.Token, describe, output string) error {
	now := time.Now()
	if output == "json" {
		payload := map[string]any{
			"source":   string(tok.Source),
			"describe": describe,
			"subject":  tok.Subject,
			"issuer":   tok.Issuer,
			"audience": tok.Audience,
			"roles":    tok.Roles,
		}
		if !tok.Expiry.IsZero() {
			payload["expiry"] = tok.Expiry.UTC().Format(time.RFC3339)
			payload["expires_in_seconds"] = int64(tok.ExpiresIn(now).Seconds())
		}
		if !tok.IssuedAt.IsZero() {
			payload["issued_at"] = tok.IssuedAt.UTC().Format(time.RFC3339)
			payload["age_seconds"] = int64(tok.Age(now).Seconds())
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Source:    %s\n", describe)
	fmt.Fprintf(&b, "Subject:   %s\n", tok.Subject)
	fmt.Fprintf(&b, "Issuer:    %s\n", tok.Issuer)
	fmt.Fprintf(&b, "Audience:  %s\n", strings.Join(tok.Audience, ", "))
	fmt.Fprintf(&b, "Roles:     %s\n", strings.Join(tok.Roles, ", "))
	if !tok.IssuedAt.IsZero() {
		fmt.Fprintf(&b, "Age:       %s\n", now.Sub(tok.IssuedAt).Round(time.Second))
	}
	if tok.Expiry.IsZero() {
		fmt.Fprintf(&b, "Expiry:    unknown\n")
	} else {
		fmt.Fprintf(&b, "Expiry:    %s (in %s)\n",
			tok.Expiry.UTC().Format(time.RFC3339), tok.ExpiresIn(now).Round(time.Second))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// newTokenRefreshCommand forces a fresh token, optionally writing it back.
// When the disk bundle is expired and has no refresh token, it falls back to
// the browser login flow automatically.
func newTokenRefreshCommand() *cobra.Command {
	var write bool
	c := &cobra.Command{
		Use:   "refresh",
		Short: "Force a token refresh (client-credentials, refresh-token, or browser login)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if write {
				viper.Set("write-token", true)
			}
			src, target, err := resolveTokenSource(cmd)
			if err != nil {
				return err
			}
			src.Invalidate()
			tok, err := src.Token(cmd.Context())

			// If the token is expired and can't be refreshed, fall back to
			// the browser login flow when we have a named gateway.
			var expired *auth.ErrTokenExpired
			if errors.As(err, &expired) && target != nil && target.Name != "" {
				cmd.PrintErrln("Token expired and no refresh token available. Launching browser login...")
				return loginAndReport(cmd, target)
			}
			if err != nil {
				return err
			}
			cmd.Printf("refreshed token for subject %q (expires %s)\n", tok.Subject, tokExpiryStr(tok))

			if write && tok.Source == auth.SourceClientCredentials {
				w, err := tokenWriterFor(target)
				if err != nil {
					return err
				}
				if w == nil {
					cmd.PrintErrln("warning: --write ignored: no named gateway resolved to write to")
				} else if err := auth.WriteBundle(w, tok); err != nil {
					return fmt.Errorf("write oidc_token.json: %w", err)
				} else {
					cmd.Printf("wrote oidc_token.json for gateway %q (Rust CLI schema)\n", target.Name)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&write, "write", false, "write the refreshed token to oidc_token.json (Rust CLI schema)")
	return c
}

func tokExpiryStr(tok *auth.Token) string {
	if tok.Expiry.IsZero() {
		return "unknown"
	}
	return tok.Expiry.UTC().Format(time.RFC3339)
}
