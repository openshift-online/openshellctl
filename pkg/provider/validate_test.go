package provider

import (
	"strings"
	"testing"
)

func TestValidateCreateFlags(t *testing.T) {
	cases := []struct {
		name                                            string
		providerName, providerType                      string
		credentials                                     []string
		fromExisting, fromGcloudADC, runtimeCredentials bool
		wantErr                                         string
	}{
		{
			name:         "valid",
			providerName: "p1",
			providerType: "github",
			credentials:  []string{"TOKEN=abc"},
		},
		{
			name:         "missing name",
			providerType: "github",
			credentials:  []string{"TOKEN=abc"},
			wantErr:      "--name",
		},
		{
			name:         "missing type",
			providerName: "p1",
			credentials:  []string{"TOKEN=abc"},
			wantErr:      "--type",
		},
		{
			name:         "from-existing not supported",
			providerName: "p1",
			providerType: "github",
			fromExisting: true,
			wantErr:      "--from-existing",
		},
		{
			name:         "from-existing hint names 'create', not 'update'",
			providerName: "p1",
			providerType: "github",
			fromExisting: true,
			wantErr:      "provider create --from-existing",
		},
		{
			name:          "from-gcloud-adc not supported",
			providerName:  "p1",
			providerType:  "github",
			fromGcloudADC: true,
			wantErr:       "--from-gcloud-adc",
		},
		{
			name:               "runtime-credentials not supported",
			providerName:       "p1",
			providerType:       "github",
			runtimeCredentials: true,
			wantErr:            "--runtime-credentials",
		},
		{
			name:         "no credential source at all",
			providerName: "p1",
			providerType: "github",
			wantErr:      "--credential",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateFlags(tc.providerName, tc.providerType, tc.credentials,
				tc.fromExisting, tc.fromGcloudADC, tc.runtimeCredentials)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateUpdateFlags(t *testing.T) {
	cases := []struct {
		name                                            string
		fromExisting, fromGcloudADC, runtimeCredentials bool
		wantErr                                         string
	}{
		{name: "valid, nothing set"},
		{name: "from-existing not supported", fromExisting: true, wantErr: "--from-existing"},
		{name: "from-existing hint names 'update', not 'create'", fromExisting: true, wantErr: "provider update --from-existing"},
		{name: "from-gcloud-adc not supported", fromGcloudADC: true, wantErr: "--from-gcloud-adc"},
		{name: "runtime-credentials not supported", runtimeCredentials: true, wantErr: "--runtime-credentials"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUpdateFlags(tc.fromExisting, tc.fromGcloudADC, tc.runtimeCredentials)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
