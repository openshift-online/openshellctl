package sandbox

// builtinProfileCredentials maps (uppercased) builtin-profile credential env var
// names to the provider suggestions the CLI prints (Appendix A.5, derived from
// crates/openshell-providers/src/profiles.rs). Keyed by the uppercased env var
// so the lookup is case-insensitive.
//
// This mirrors the documented examples (GITHUB_TOKEN/GH_TOKEN); it is a subset
// of the upstream profile table and is extended as profiles are added. Keys not
// listed here still trip the keyword heuristic in matchesCredentialKeyword.
var builtinProfileCredentials = map[string][]CredentialSuggestion{
	"GITHUB_TOKEN": {{Type: "github", Key: "GITHUB_TOKEN"}, {Type: "copilot", Key: "GITHUB_TOKEN"}},
	"GH_TOKEN":     {{Type: "github", Key: "GH_TOKEN"}, {Type: "copilot", Key: "GH_TOKEN"}},
}
