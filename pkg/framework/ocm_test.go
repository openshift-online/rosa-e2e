package framework

import (
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

func TestIsZeroEgressEnabled(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]string
		typedValue *bool
		want       bool
		wantErr    bool
	}{
		{
			name:       "enabled property",
			properties: map[string]string{"zero_egress": "true"},
			want:       true,
		},
		{
			name:       "disabled property takes precedence",
			properties: map[string]string{"zero_egress": "false"},
			typedValue: boolPointer(true),
		},
		{
			name:       "invalid property",
			properties: map[string]string{"zero_egress": "invalid"},
			wantErr:    true,
		},
		{
			name:       "typed field fallback",
			typedValue: boolPointer(true),
			want:       true,
		},
		{
			name: "not enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := cmv1.NewCluster()
			if tt.properties != nil {
				builder.Properties(tt.properties)
			}
			if tt.typedValue != nil {
				builder.AWS(cmv1.NewAWS().ZeroEgress(cmv1.NewZeroEgress().Enabled(*tt.typedValue)))
			}

			cluster, err := builder.Build()
			if err != nil {
				t.Fatalf("building cluster: %v", err)
			}

			got, err := IsZeroEgressEnabled(cluster)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("detecting zero egress: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %t, want %t", got, tt.want)
			}
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
}
