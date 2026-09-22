// Package v1alpha1 defines the openshellctl manifest types
// (openshell.managed.openshift.io/v1alpha1), shared between the CLI and the
// operator. Manifests are a thin, Kubernetes-style envelope over the fields a
// sandbox create accepts (spec §5.4).
package v1alpha1

// Group/version identifiers.
const (
	Group      = "openshell.managed.openshift.io"
	Version    = "v1alpha1"
	APIVersion = Group + "/" + Version
)

// Kinds. Provider is reserved (no spec yet).
const (
	KindSandbox  = "Sandbox"
	KindProvider = "Provider"
)

// TypeMeta is the apiVersion/kind envelope.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// ObjectMeta carries name/workspace/labels.
type ObjectMeta struct {
	Name      string            `json:"name,omitempty"`      // --name (empty → server-generated)
	Workspace string            `json:"workspace,omitempty"` // --workspace (default "default")
	Labels    map[string]string `json:"labels,omitempty"`    // --label
}

// Sandbox is a manifest describing a sandbox to create.
type Sandbox struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta  `json:"metadata"`
	Spec     SandboxSpec `json:"spec"`
}

// ProviderRef names a provider to attach.
type ProviderRef struct {
	Name string `json:"name"`
}

// GPU requests GPU resources. An empty struct ({}) means the driver default
// (bare --gpu with no COUNT); Count set means an explicit count.
type GPU struct {
	Count *uint32 `json:"count,omitempty"`
}

// Resources is the CPU/memory/GPU request.
type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	GPU    *GPU   `json:"gpu,omitempty"`
}

// Upload is a client-side file upload (ignored by the operator).
type Upload struct {
	Local     string `json:"local"`
	Dest      string `json:"dest,omitempty"`      // "" → sandbox workdir
	GitIgnore *bool  `json:"gitignore,omitempty"` // nil/true = filter; false = --no-git-ignore
}

// SessionOpts groups client-side session behavior that controls what happens
// after sandbox creation. These are not part of the sandbox resource itself —
// the gateway and operator ignore them. They exist so that a single manifest
// file can fully describe "create sandbox X and behave like this."
type SessionOpts struct {
	NoKeep       bool   `json:"noKeep,omitempty"`       // delete sandbox when session ends
	Detach       bool   `json:"detach,omitempty"`       // return after create, do not attach
	Forward      string `json:"forward,omitempty"`      // [bind:]port to forward
	ApprovalMode string `json:"approvalMode,omitempty"` // manual|auto
	Output       string `json:"output,omitempty"`       // table|json|yaml
}

// SandboxSpec is the create input.
type SandboxSpec struct {
	Image                string            `json:"image,omitempty"`
	Command              []string          `json:"command,omitempty"`
	TTY                  *bool             `json:"tty,omitempty"`
	Env                  map[string]string `json:"env,omitempty"`
	ProviderRefs         []ProviderRef     `json:"providerRefs,omitempty"`
	Resources            *Resources        `json:"resources,omitempty"`
	DriverConfig         map[string]any    `json:"driverConfig,omitempty"`
	Policy               map[string]any    `json:"policy,omitempty"`
	PolicyFile           string            `json:"policyFile,omitempty"`
	AutoProviders        *bool             `json:"autoProviders,omitempty"`
	NoCredentialWarnings bool              `json:"noCredentialWarnings,omitempty"`

	// client-side orchestration (ignored by the operator)
	Upload      []Upload     `json:"upload,omitempty"`
	SessionOpts *SessionOpts `json:"sessionOpts,omitempty"`

	// Deprecated: use sessionOpts.approvalMode. Kept for backward compat
	// during v1alpha1; removed when the schema stabilizes.
	ApprovalMode string `json:"approvalMode,omitempty"`
	// Deprecated: use sessionOpts.noKeep (inverted). Kept for backward compat.
	Keep *bool `json:"keep,omitempty"`
	// Deprecated: use sessionOpts.detach. Kept for backward compat.
	Detach bool `json:"detach,omitempty"`
	// Deprecated: use sessionOpts.forward. Kept for backward compat.
	Forward string `json:"forward,omitempty"`
}
