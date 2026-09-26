package verifiers

import "testing"

func TestVerifyIPInCIDR(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		cidr    string
		wantErr bool
	}{
		{name: "vpc ip in machine cidr", ip: "10.0.3.17", cidr: "10.0.0.0/16", wantErr: false},
		{name: "ip outside machine cidr", ip: "192.168.1.5", cidr: "10.0.0.0/16", wantErr: true},
		{name: "invalid ip", ip: "not-an-ip", cidr: "10.0.0.0/16", wantErr: true},
		{name: "invalid cidr", ip: "10.0.0.1", cidr: "not-a-cidr", wantErr: true},
		{name: "empty ip", ip: "", cidr: "10.0.0.0/16", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyIPInCIDR(tt.ip, tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyIPInCIDR(%q, %q) error = %v, wantErr %v", tt.ip, tt.cidr, err, tt.wantErr)
			}
		})
	}
}
