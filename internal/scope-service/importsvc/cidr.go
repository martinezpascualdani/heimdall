package importsvc

import (
	"fmt"
	"net"
	"strconv"

	"github.com/martinezpascualdani/heimdall/pkg/rirparser"
)

// BlockToNormalizedCIDRs converts an RIR IP record (Start + Value) to a minimal set of normalized CIDR strings.
// IPv4: Start = first address, Value = count of addresses.
// IPv6: Start = prefix, Value = prefix length.
// Returns nil slice on error or if no valid CIDR; caller should skip or store empty.
func BlockToNormalizedCIDRs(rec *rirparser.Record) ([]string, error) {
	if rec == nil {
		return nil, nil
	}
	return NormalizeBlockCIDRs(rec.Start, rec.Value, string(rec.Type))
}

// NormalizeBlockCIDRs converts (start, value, addressFamily) to normalized CIDRs.
// addressFamily is "ipv4" or "ipv6". Used for backfilling existing blocks.
func NormalizeBlockCIDRs(start, value, addressFamily string) ([]string, error) {
	switch addressFamily {
	case "ipv4":
		return ipv4BlockToCIDRs(start, value)
	case "ipv6":
		return ipv6BlockToCIDRs(start, value)
	default:
		return nil, nil
	}
}

func ipv4BlockToCIDRs(startStr, countStr string) ([]string, error) {
	ip := net.ParseIP(startStr)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("invalid ipv4 start: %s", startStr)
	}
	count, err := strconv.ParseUint(countStr, 10, 64)
	if err != nil || count == 0 {
		return nil, fmt.Errorf("invalid ipv4 count: %s", countStr)
	}
	start := ipToUint32(ip.To4())
	end := start + uint32(count) - 1
	if end < start {
		return nil, fmt.Errorf("ipv4 range overflow")
	}
	return rangeIPv4ToCIDRs(start, end)
}

func ipToUint32(ip net.IP) uint32 {
	v := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	return v
}

func uint32ToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// rangeIPv4ToCIDRs returns the minimal set of CIDRs covering [start, end] (inclusive).
func rangeIPv4ToCIDRs(start, end uint32) ([]string, error) {
	var out []string
	for start <= end {
		// Largest step = 2^n such that (start & (step-1)) == 0 and start+step-1 <= end
		step := uint32(1)
		for step != 0 && start+step-1 <= end && (start&(step-1)) == 0 {
			step <<= 1
		}
		if step != 0 {
			step >>= 1
		}
		if step == 0 {
			step = 1
		}
		prefixLen := 32 - trailingZeros32(step)
		ip := uint32ToIP(start)
		out = append(out, fmt.Sprintf("%s/%d", ip.String(), prefixLen))
		start += step
	}
	return out, nil
}

func trailingZeros32(x uint32) int {
	if x == 0 {
		return 32
	}
	n := 0
	for x&1 == 0 {
		n++
		x >>= 1
	}
	return n
}

func ipv6BlockToCIDRs(prefixStr, lengthStr string) ([]string, error) {
	ip := net.ParseIP(prefixStr)
	if ip == nil || ip.To16() == nil {
		return nil, fmt.Errorf("invalid ipv6 prefix: %s", prefixStr)
	}
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length < 0 || length > 128 {
		return nil, fmt.Errorf("invalid ipv6 length: %s", lengthStr)
	}
	// Normalize: mask the prefix to the given length
	mask := net.CIDRMask(length, 128)
	normIP := ip.Mask(mask)
	return []string{fmt.Sprintf("%s/%d", normIP.String(), length)}, nil
}
