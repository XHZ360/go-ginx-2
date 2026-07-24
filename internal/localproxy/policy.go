package localproxy

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

var ErrTargetDenied = errors.New("local target is not allowed")

var DefaultAllowlist = []AllowlistEntry{
	{CIDR: "127.0.0.1/32"},
	{CIDR: "::1/128"},
}

type policySnapshot struct {
	entries []normalizedEntry
}

type normalizedEntry struct {
	value  AllowlistEntry
	prefix netip.Prefix
}

type Policy struct {
	repository AllowlistRepository
	snapshot   atomic.Pointer[policySnapshot]
}

func LoadPolicy(ctx context.Context, repository AllowlistRepository) (*Policy, error) {
	if repository == nil {
		return nil, errors.New("allowlist repository is required")
	}
	entries, err := repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load local target allowlist: %w", err)
	}
	normalized, err := normalizeEntries(entries)
	if err != nil {
		return nil, fmt.Errorf("load local target allowlist: %w", err)
	}
	policy := &Policy{repository: repository}
	policy.snapshot.Store(&policySnapshot{entries: normalized})
	return policy, nil
}

func (policy *Policy) ValidateTarget(_ context.Context, host string, port int) error {
	if policy == nil || policy.snapshot.Load() == nil {
		return fmt.Errorf("%w: policy is unavailable", ErrTargetDenied)
	}
	address, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return fmt.Errorf("%w: host must be an IP address", ErrTargetDenied)
	}
	address = address.Unmap()
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%w: port is invalid", ErrTargetDenied)
	}
	for _, entry := range policy.snapshot.Load().entries {
		if !entry.prefix.Contains(address) {
			continue
		}
		if entry.value.PortStart == 0 || port >= entry.value.PortStart && port <= entry.value.PortEnd {
			return nil
		}
	}
	return fmt.Errorf("%w: target does not match the allowlist", ErrTargetDenied)
}

func (policy *Policy) Snapshot() AllowlistSnapshot {
	if policy == nil || policy.snapshot.Load() == nil {
		return AllowlistSnapshot{}
	}
	current := policy.snapshot.Load().entries
	entries := make([]AllowlistEntry, len(current))
	for index := range current {
		entries[index] = current[index].value
	}
	return AllowlistSnapshot{Entries: entries}
}

func (policy *Policy) Replace(ctx context.Context, input AllowlistInput) error {
	if policy == nil || policy.repository == nil {
		return errors.New("allowlist policy is unavailable")
	}
	normalized, err := normalizeEntries(input.Entries)
	if err != nil {
		return err
	}
	values := make([]AllowlistEntry, len(normalized))
	for index := range normalized {
		values[index] = normalized[index].value
	}
	if err := policy.repository.Replace(ctx, values); err != nil {
		return err
	}
	policy.snapshot.Store(&policySnapshot{entries: normalized})
	return nil
}

func normalizeEntries(entries []AllowlistEntry) ([]normalizedEntry, error) {
	if len(entries) == 0 {
		return nil, errors.New("allowlist must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(entries))
	result := make([]normalizedEntry, 0, len(entries))
	for index, entry := range entries {
		prefix, err := parsePrefix(entry.CIDR)
		if err != nil {
			return nil, fmt.Errorf("allowlist entry %d: %w", index, err)
		}
		if entry.PortStart == 0 && entry.PortEnd == 0 {
			// Zero bounds mean all ports.
		} else if entry.PortStart <= 0 || entry.PortStart > 65535 || entry.PortEnd <= 0 || entry.PortEnd > 65535 || entry.PortStart > entry.PortEnd {
			return nil, fmt.Errorf("allowlist entry %d: invalid port range", index)
		}
		value := AllowlistEntry{CIDR: prefix.String(), PortStart: entry.PortStart, PortEnd: entry.PortEnd}
		key := value.CIDR + ":" + strconv.Itoa(value.PortStart) + ":" + strconv.Itoa(value.PortEnd)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, normalizedEntry{value: value, prefix: prefix})
	}
	slices.SortFunc(result, func(left, right normalizedEntry) int {
		return strings.Compare(left.value.CIDR+fmt.Sprintf(":%05d:%05d", left.value.PortStart, left.value.PortEnd), right.value.CIDR+fmt.Sprintf(":%05d:%05d", right.value.PortStart, right.value.PortEnd))
	})
	return result, nil
}

func parsePrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if address, err := netip.ParseAddr(value); err == nil {
		address = address.Unmap()
		return netip.PrefixFrom(address, address.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, errors.New("CIDR must be an IP address or prefix")
	}
	address := prefix.Addr().Unmap()
	bits := prefix.Bits()
	if address.Is4() && prefix.Addr().Is4In6() {
		bits -= 96
	}
	if bits < 0 || bits > address.BitLen() {
		return netip.Prefix{}, errors.New("CIDR prefix length is invalid")
	}
	return netip.PrefixFrom(address, bits).Masked(), nil
}

var _ LocalTargetPolicy = (*Policy)(nil)
