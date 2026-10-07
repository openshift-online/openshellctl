package gatewayconfig

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Env is the injected environment. Tests pass fakes; the real constructor wires
// Getenv to os.Getenv and the two FS to os.DirFS over the resolved trees.
type Env struct {
	Getenv  func(string) string // XDG_CONFIG_HOME, HOME, OPENSHELL_SYSTEM_GATEWAY_DIR
	UserFS  fs.FS               // rooted at the user config tree ($XDG_CONFIG_HOME/openshell)
	SysFS   fs.FS               // rooted at the system base (/etc/openshell or override)
	UserDir string              // absolute path of the user config tree ("" when unknown/test)
	SysDir  string              // absolute path of the system config tree ("" when unknown/test)
}

// Source identifies which tree a resolved gateway came from.
type Source string

// Config tree sources.
const (
	SourceUser   Source = "user"
	SourceSystem Source = "system"
)

// Resolved is a gateway's metadata plus the tree it came from.
type Resolved struct {
	Name     string
	Dir      string // absolute dir on the source tree (best-effort; "" if unknown)
	Source   Source
	Metadata Metadata
	FS       fs.FS // sub-FS rooted at the gateway dir (oidc_token.json, mtls/, last_sandbox)
}

// Info summarises a gateway for List.
type Info struct {
	Name   string
	Source Source
	Active bool
}

// Load reads gateways/<name>/metadata.json from the user tree, then the system
// tree; the user entry shadows the system entry even when it is present but
// invalid (upstream user_entry_shadows_system, metadata.rs:108-132).
func Load(env Env, name string) (*Resolved, error) {
	if err := ValidateGatewayName(name); err != nil {
		return nil, err
	}

	if env.UserFS != nil {
		m, present, perr := readMetadataFile(env.UserFS, name)
		if perr != nil {
			return nil, perr
		}
		if present {
			return newResolved(env.UserFS, name, SourceUser, m, env.UserDir), nil
		}
	}
	if env.SysFS != nil {
		m, present, perr := readMetadataFile(env.SysFS, name)
		if perr != nil {
			return nil, perr
		}
		if present {
			return newResolved(env.SysFS, name, SourceSystem, m, env.SysDir), nil
		}
	}
	return nil, &GatewayNotFoundError{Name: name}
}

func newResolved(root fs.FS, name string, src Source, m Metadata, rootDir string) *Resolved {
	sub, err := fs.Sub(root, GatewayDir(name))
	if err != nil {
		sub = nil
	}
	var dir string
	if rootDir != "" {
		dir = filepath.Join(rootDir, GatewayDir(name))
	}
	return &Resolved{
		Name:     name,
		Dir:      dir,
		Source:   src,
		Metadata: m,
		FS:       sub,
	}
}

// ActiveGateway reads the active_gateway file (config-root level, not under
// gateways/), user tree then system tree; content is trimmed and validated; an
// empty or invalid value yields ErrNoActiveGateway (metadata.rs:243-276).
func ActiveGateway(env Env) (string, error) {
	if name, ok := readActive(env.UserFS); ok {
		return name, nil
	}
	if name, ok := readActive(env.SysFS); ok {
		return name, nil
	}
	return "", &NoActiveGatewayError{}
}

func readActive(fsys fs.FS) (string, bool) {
	if fsys == nil {
		return "", false
	}
	b, err := fs.ReadFile(fsys, "active_gateway")
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(b))
	if name == "" {
		return "", false
	}
	if ValidateGatewayName(name) != nil {
		return "", false // upstream ignores an invalid stored name
	}
	return name, true
}

// List enumerates gateways from both trees (user shadows system by name),
// marking the active one.
func List(env Env) ([]Info, error) {
	active, _ := ActiveGateway(env)
	seen := map[string]bool{}
	var out []Info

	add := func(fsys fs.FS, src Source) {
		if fsys == nil {
			return
		}
		entries, err := fs.ReadDir(fsys, "gateways")
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if seen[name] {
				continue
			}
			if _, present, _ := readMetadataFile(fsys, name); !present {
				continue
			}
			seen[name] = true
			out = append(out, Info{Name: name, Source: src, Active: name == active})
		}
	}
	add(env.UserFS, SourceUser)
	add(env.SysFS, SourceSystem)
	return out, nil
}

// normalizeEndpoint strips all trailing slashes (upstream
// normalize_gateway_endpoint = trim_end_matches('/'), main.rs:58-60).
func normalizeEndpoint(endpoint string) string {
	return strings.TrimRight(endpoint, "/")
}

// FindByEndpoint matches a (slash-normalised) endpoint against the active
// gateway first, then all gateways (main.rs:62-76).
func FindByEndpoint(env Env, endpoint string) (string, bool, error) {
	target := normalizeEndpoint(endpoint)

	if active, err := ActiveGateway(env); err == nil {
		if r, lerr := Load(env, active); lerr == nil {
			if normalizeEndpoint(r.Metadata.GatewayEndpoint) == target {
				return r.Metadata.Name, true, nil
			}
		}
	}

	infos, _ := List(env)
	for _, info := range infos {
		r, err := Load(env, info.Name)
		if err != nil {
			continue
		}
		if normalizeEndpoint(r.Metadata.GatewayEndpoint) == target {
			return r.Metadata.Name, true, nil
		}
	}
	return "", false, nil
}

// ResolveInput is the caller-merged endpoint/name (from flags + env).
type ResolveInput struct{ Endpoint, Name string }

// Target is a resolved gateway context.
type Target struct {
	Name     string    // "" when endpoint given and no metadata matched (falls back to endpoint)
	Endpoint string    //
	Resolved *Resolved // nil when endpoint-only
}

// Resolve mirrors resolve_gateway (main.rs:78-126). Precedence:
//   - endpoint given: endpoint used directly; name = Name flag > find-by-endpoint > raw endpoint.
//   - else: name = Name flag > active file; then metadata is loaded (unknown → error).
//
// Note: the OPENSHELL_GATEWAY env var is merged into ResolveInput.Name by the
// caller, matching upstream's flag-or-env plumbing.
func Resolve(env Env, in ResolveInput) (*Target, error) {
	if in.Endpoint != "" {
		name := in.Name
		if name == "" {
			if found, ok, _ := FindByEndpoint(env, in.Endpoint); ok {
				name = found
			}
		}
		t := &Target{Name: name, Endpoint: in.Endpoint}
		if name == "" {
			t.Name = in.Endpoint // upstream falls back to the raw endpoint string
		} else if r, err := Load(env, name); err == nil {
			t.Resolved = r
		}
		return t, nil
	}

	name := in.Name
	if name == "" {
		active, err := ActiveGateway(env)
		if err != nil {
			return nil, &NoActiveGatewayError{}
		}
		name = active
	}

	r, err := Load(env, name)
	if err != nil {
		return nil, &UnknownGatewayError{Name: name}
	}
	return &Target{
		Name:     r.Metadata.Name,
		Endpoint: r.Metadata.GatewayEndpoint,
		Resolved: r,
	}, nil
}
