# CI and scheduled-job examples

A complete, runnable example of using `openshellctl` from a Kubernetes
`CronJob` instead of interactively. Uses the service-account auth path (see
the README's
["Quick start (service account)"](../README.md#service-account-ci--fire-and-forget)) —
no browser, no interactive login, a client secret does all the work.

## Kubernetes CronJob

This mirrors the pattern used by
[`openshift-online/rosa-agent`](https://github.com/openshift-online/rosa-agent)'s
own scheduled jobs (see its `sandbox/skills/*/​*-cron.yaml` manifests for the
live, production version this example is adapted from) — generalized here
with placeholder names instead of that repo's specific endpoints/secrets.

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: my-scheduled-job
spec:
  schedule: "0 3 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      activeDeadlineSeconds: 3600
      backoffLimit: 0
      template:
        spec:
          serviceAccountName: my-job-sa
          containers:
            - name: my-scheduled-job
              image: my-registry/my-image:latest
              command: ["/bin/bash", "-c"]
              args:
                - |
                  set -euo pipefail

                  # openshellctl reads OPENSHELL_GATEWAY_ENDPOINT, OPENSHELL_OIDC_ISSUER,
                  # OPENSHELL_OIDC_CLIENT_ID, OPENSHELL_OIDC_AUDIENCE, and
                  # OPENSHELL_OIDC_CLIENT_SECRET directly from the environment below —
                  # no manual token minting needed. `gateway add` authenticates via
                  # client credentials as part of registration and sets the gateway
                  # active for every command that follows.
                  openshellctl gateway add "${OPENSHELL_GATEWAY_ENDPOINT}" --name my-gw

                  # Preflight: confirms reachability, a valid token, and that every
                  # provider this job needs actually exists — before any sandbox is
                  # created. A deliberately-wrong --provider name fails here, not
                  # confusingly later at sandbox create.
                  openshellctl doctor --provider my-provider

                  # --replace deletes-and-waits-for-gone any sandbox left over from a
                  # prior run (a timed-out pod, a crashed job) before creating, so a
                  # real failure aborts loudly instead of racing a deletion still in
                  # flight.
                  openshellctl sandbox create \
                    --replace \
                    --name my-job \
                    --from my-registry/my-sandbox-image:latest \
                    --provider my-provider \
                    --no-keep \
                    --no-tty \
                    -- my-command --with --args
              env:
                - name: HOME
                  value: /tmp
                - name: OPENSHELL_GATEWAY_ENDPOINT
                  value: "https://gateway.example.com"
                - name: OPENSHELL_OIDC_ISSUER
                  value: "https://issuer.example.com/realms/my-realm"
                - name: OPENSHELL_OIDC_CLIENT_ID
                  value: "my-service-account-client-id"
                - name: OPENSHELL_OIDC_AUDIENCE
                  value: "my-service-account-audience"
                - name: OPENSHELL_OIDC_CLIENT_SECRET
                  valueFrom:
                    secretKeyRef:
                      name: my-oidc-secret
                      key: OPENSHELL_OIDC_CLIENT_SECRET
          restartPolicy: Never
```

Notes:

- `HOME` must point somewhere writable in the container (`/tmp` above) —
  `gateway add` and the resulting token cache write under
  `$HOME/.config/openshell/gateways/<name>/`.
- `OPENSHELL_OIDC_CLIENT_SECRET` arrives as a plain environment variable
  sourced from a Kubernetes `Secret`. If that secret is itself populated
  from Vault (e.g. via an External Secrets Operator `ExternalSecret`, as in
  rosa-agent's setup), that happens entirely outside this manifest — the
  Pod never talks to Vault directly, so openshellctl's own native
  `--vault-kv-mount`/`--vault-kv-path` flags (see the README's "4.
  Vault-sourced config") are not used here.
- No `curl`, `python3`, `getent`, or upstream `openshell` binary invocation
  appears anywhere — `gateway add`, `doctor`, and `sandbox create --replace`
  are natively implemented in Go.

