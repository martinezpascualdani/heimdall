package importsvc

import (
	"reflect"
	"testing"

	"github.com/martinezpascualdani/heimdall/pkg/rirparser"
)

func TestBlockToNormalizedCIDRs_IPv4(t *testing.T) {
	tests := []struct {
		name     string
		rec      *rirparser.Record
		want     []string
		wantErr  bool
	}{
		{
			name: "single /24",
			rec:  &rirparser.Record{Type: rirparser.TypeIPv4, Start: "1.2.3.0", Value: "256"},
			want: []string{"1.2.3.0/24"},
		},
		{
			name: "single /16",
			rec:  &rirparser.Record{Type: rirparser.TypeIPv4, Start: "10.0.0.0", Value: "65536"},
			want: []string{"10.0.0.0/16"},
		},
		{
			name: "512 addresses -> one /23",
			rec:  &rirparser.Record{Type: rirparser.TypeIPv4, Start: "1.0.0.0", Value: "512"},
			want: []string{"1.0.0.0/23"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BlockToNormalizedCIDRs(tt.rec)
			if (err != nil) != tt.wantErr {
				t.Errorf("BlockToNormalizedCIDRs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BlockToNormalizedCIDRs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlockToNormalizedCIDRs_IPv6(t *testing.T) {
	got, err := BlockToNormalizedCIDRs(&rirparser.Record{Type: rirparser.TypeIPv6, Start: "2001:db8::", Value: "32"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "2001:db8::/32" {
		t.Errorf("BlockToNormalizedCIDRs(ipv6) = %v, want [2001:db8::/32]", got)
	}
}
