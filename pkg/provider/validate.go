package provider

import "fmt"

// ValidateCreateFlags checks `provider create`'s client-side requirements
// before any network call: --name and --type are required, the three
// credential sources openshellctl doesn't implement yet are rejected, and
// otherwise at least one --credential is required. It returns the first
// violation found, or nil.
func ValidateCreateFlags(name, providerType string, credentials []string, fromExisting, fromGcloudADC, runtimeCredentials bool) error {
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if providerType == "" {
		return fmt.Errorf("--type is required")
	}
	if err := validateCredentialSource("create", fromExisting, fromGcloudADC, runtimeCredentials); err != nil {
		return err
	}
	if len(credentials) == 0 {
		return fmt.Errorf("at least one --credential is required (or use --from-existing/--from-gcloud-adc/--runtime-credentials, not yet supported in openshellctl)")
	}
	return nil
}

// ValidateUpdateFlags checks `provider update`'s client-side requirements:
// only the three unsupported credential sources need rejecting (NAME is
// required by cobra.ExactArgs, and no --credential is required since a
// partial update with nothing but e.g. --global-profile is valid).
func ValidateUpdateFlags(fromExisting, fromGcloudADC, runtimeCredentials bool) error {
	return validateCredentialSource("update", fromExisting, fromGcloudADC, runtimeCredentials)
}

// validateCredentialSource rejects the three provider credential sources
// openshellctl doesn't implement yet (reading local state, gcloud ADC, or
// gateway-resolved runtime credentials), naming whichever was passed.
// subcommand ("create" or "update") is interpolated into the hint so it
// points at the upstream invocation the caller actually ran — `provider
// update` and `provider create` each have their own, separate versions of
// these three flags upstream, so the hint must not hardcode one or the other.
func validateCredentialSource(subcommand string, fromExisting, fromGcloudADC, runtimeCredentials bool) error {
	switch {
	case fromExisting:
		return fmt.Errorf("--from-existing is not yet supported in openshellctl; run `openshell provider %s --from-existing ...` directly", subcommand)
	case fromGcloudADC:
		return fmt.Errorf("--from-gcloud-adc is not yet supported in openshellctl; run `openshell provider %s --from-gcloud-adc ...` directly", subcommand)
	case runtimeCredentials:
		return fmt.Errorf("--runtime-credentials is not yet supported in openshellctl; run `openshell provider %s --runtime-credentials ...` directly", subcommand)
	default:
		return nil
	}
}
