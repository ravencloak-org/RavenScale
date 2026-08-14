package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"sync"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/util"
	"github.com/rs/zerolog/log"
	"go4.org/netipx"
	"gorm.io/gorm"
	"tailscale.com/net/tsaddr"
)

var (
	errGeneratedIPBytesInvalid = errors.New("generated ip bytes are invalid ip")
	errGeneratedIPNotInPrefix  = errors.New("generated ip not in prefix")
	errIPAllocatorNil          = errors.New("ip allocator was nil")
)

// perTailnetState holds the allocation cursor and used-IP set for a single
// Tailnet. Per ADR-0002 every Tailnet gets its own independent 100.64.0.0/10,
// so nodes in different Tailnets may legitimately hold identical addresses.
// Uniqueness is only enforced within a Tailnet, hence one of these per
// tailnet_id.
type perTailnetState struct {
	// Previous IPs handed out for this Tailnet.
	prev4 netip.Addr
	prev6 netip.Addr

	// Set of all IPs handed out within this Tailnet.
	// This might not be in sync with the database,
	// but it is more conservative. If saves to the
	// database fails, the IP will be allocated here
	// until the next restart of Headscale.
	usedIPs netipx.IPSetBuilder
}

// IPAllocator is a singleton responsible for allocating
// IP addresses for nodes and making sure the same
// address is not handed out twice within a Tailnet. There
// can only be one and it needs to be created before any
// other database writes occur.
//
// Per ADR-0002 allocation is scoped by tailnet_id: the used-IP set and the
// allocation cursor live in a [perTailnetState] created lazily on first use of
// a Tailnet.
type IPAllocator struct {
	mu sync.Mutex

	prefix4 *netip.Prefix
	prefix6 *netip.Prefix

	// strategy used for handing out IP addresses.
	strategy types.IPAllocationStrategy

	// tailnets holds the per-Tailnet allocation state keyed by tailnet_id.
	tailnets map[uint]*perTailnetState
}

// stateFor returns the [perTailnetState] for tailnetID, creating and seeding
// it (network/broadcast reserved, cursor at the network address) on first use.
// Callers must hold i.mu.
func (i *IPAllocator) stateFor(tailnetID uint) *perTailnetState {
	if s, ok := i.tailnets[tailnetID]; ok {
		return s
	}

	s := &perTailnetState{}

	// Add network and broadcast addrs to used pool so they
	// are not handed out to nodes.
	if i.prefix4 != nil {
		network4, broadcast4 := util.GetIPPrefixEndpoints(*i.prefix4)
		s.usedIPs.Add(network4)
		s.usedIPs.Add(broadcast4)

		// Use network as starting point, it will be used to call .Next()
		s.prev4 = network4
	}

	if i.prefix6 != nil {
		network6, broadcast6 := util.GetIPPrefixEndpoints(*i.prefix6)
		s.usedIPs.Add(network6)
		s.usedIPs.Add(broadcast6)

		s.prev6 = network6
	}

	i.tailnets[tailnetID] = s

	return s
}

// NewIPAllocator returns a new [IPAllocator] singleton which
// can be used to hand out unique IP addresses within the
// provided IPv4 and IPv6 prefix. It needs to be created
// when headscale starts and needs to finish its read
// transaction before any writes to the database occur.
func NewIPAllocator(
	ctx context.Context,
	db *HSDatabase,
	prefix4, prefix6 *netip.Prefix,
	strategy types.IPAllocationStrategy,
) (*IPAllocator, error) {
	ret := IPAllocator{
		prefix4: prefix4,
		prefix6: prefix6,

		strategy: strategy,

		tailnets: make(map[uint]*perTailnetState),
	}

	// Each node carries its tailnet_id alongside its addresses so the used-IP
	// set can be built per Tailnet (ADR-0002).
	var rows []struct {
		TailnetID uint
		IPv4      sql.NullString
		IPv6      sql.NullString
	}

	if db != nil {
		err := db.Read(ctx, func(rx *gorm.DB) error {
			return rx.Model(&types.Node{}).Select("tailnet_id", "ipv4", "ipv6").Scan(&rows).Error
		})
		if err != nil {
			return nil, fmt.Errorf("reading node addresses from database: %w", err)
		}
	}

	// Fetch all the IP Addresses currently handed out from the Database
	// and add them to the used IP set of their owning Tailnet.
	for _, row := range rows {
		s := ret.stateFor(row.TailnetID)

		for _, addrStr := range []sql.NullString{row.IPv4, row.IPv6} {
			if addrStr.Valid {
				addr, err := netip.ParseAddr(addrStr.String)
				if err != nil {
					return nil, fmt.Errorf("parsing IP address from database: %w", err)
				}

				s.usedIPs.Add(addr)
			}
		}
	}

	// Build the initial IPSet for every Tailnet seen to validate we can use it.
	for tailnetID, s := range ret.tailnets {
		if _, err := s.usedIPs.IPSet(); err != nil {
			return nil, fmt.Errorf(
				"building initial IP Set for tailnet(%d): %w",
				tailnetID,
				err,
			)
		}
	}

	return &ret, nil
}

// Next allocates the next free IPv4/IPv6 pair within the given Tailnet. Two
// nodes in different Tailnets may receive the same address (ADR-0002).
func (i *IPAllocator) Next(tailnetID uint) (*netip.Addr, *netip.Addr, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.stateFor(tailnetID)

	var (
		err  error
		ret4 *netip.Addr
		ret6 *netip.Addr
	)

	if i.prefix4 != nil {
		ret4, err = i.next(s, s.prev4, i.prefix4)
		if err != nil {
			return nil, nil, fmt.Errorf("allocating IPv4 address: %w", err)
		}

		s.prev4 = *ret4
	}

	if i.prefix6 != nil {
		ret6, err = i.next(s, s.prev6, i.prefix6)
		if err != nil {
			return nil, nil, fmt.Errorf("allocating IPv6 address: %w", err)
		}

		s.prev6 = *ret6
	}

	return ret4, ret6, nil
}

var ErrCouldNotAllocateIP = errors.New("failed to allocate IP")

// allocateNext4 allocates the next IPv4 within tailnetID under i.mu, advancing
// that Tailnet's prev4 so a run of allocations (e.g. BackfillNodeIPs) does not
// rescan already-issued addresses, and so prev4 is read under the lock rather
// than in the caller's frame.
func (i *IPAllocator) allocateNext4(tailnetID uint) (*netip.Addr, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.stateFor(tailnetID)

	ret, err := i.next(s, s.prev4, i.prefix4)
	if err != nil {
		return nil, err
	}

	s.prev4 = *ret

	return ret, nil
}

// allocateNext6 mirrors allocateNext4 for the IPv6 prefix.
func (i *IPAllocator) allocateNext6(tailnetID uint) (*netip.Addr, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.stateFor(tailnetID)

	ret, err := i.next(s, s.prev6, i.prefix6)
	if err != nil {
		return nil, err
	}

	s.prev6 = *ret

	return ret, nil
}

func (i *IPAllocator) next(s *perTailnetState, prev netip.Addr, prefix *netip.Prefix) (*netip.Addr, error) {
	var (
		err error
		ip  netip.Addr
	)

	switch i.strategy {
	case types.IPAllocationStrategySequential:
		// Get the first IP in our prefix
		ip = prev.Next()
	case types.IPAllocationStrategyRandom:
		ip, err = randomNext(*prefix)
		if err != nil {
			return nil, fmt.Errorf("getting random IP: %w", err)
		}
	}

	// TODO(kradalby): maybe this can be done less often.
	set, err := s.usedIPs.IPSet()
	if err != nil {
		return nil, err
	}

	// Walk forward from the starting address until a free, non-reserved
	// address inside the prefix is found. The random strategy only picks the
	// starting point at random and then scans deterministically: this keeps
	// the loop finite, so an exhausted prefix returns ErrCouldNotAllocateIP
	// instead of re-drawing in-prefix addresses forever under i.mu.
	start := ip
	for {
		if prefix.Contains(ip) && !set.Contains(ip) && !isTailscaleReservedIP(ip) {
			s.usedIPs.Add(ip)

			return &ip, nil
		}

		ip = ip.Next()

		switch i.strategy {
		case types.IPAllocationStrategySequential:
			// Sequential allocation never wraps: walking past the end of
			// the prefix means the pool is exhausted.
			if !prefix.Contains(ip) {
				return nil, ErrCouldNotAllocateIP
			}
		case types.IPAllocationStrategyRandom:
			// Random allocation wraps within the prefix so every address is
			// examined exactly once; returning to the start means the prefix
			// is exhausted.
			if !prefix.Contains(ip) {
				ip = prefix.Masked().Addr()
			}

			if ip == start {
				return nil, ErrCouldNotAllocateIP
			}
		}
	}
}

func randomNext(pfx netip.Prefix) (netip.Addr, error) {
	rang := netipx.RangeOfPrefix(pfx)
	fromIP, toIP := rang.From(), rang.To()

	var from, to big.Int

	from.SetBytes(fromIP.AsSlice())
	to.SetBytes(toIP.AsSlice())

	// Find the max, this is how we can do "random range",
	// get the "max" as 0 -> to - from and then add back from
	// after.
	tempMax := big.NewInt(0).Sub(&to, &from)

	// A single-address prefix (/32 or /128) has from == to, so tempMax is 0 and
	// rand.Int would panic on a non-positive bound. Return the sole address.
	if tempMax.Sign() <= 0 {
		return fromIP, nil
	}

	out, err := rand.Int(rand.Reader, tempMax)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("generating random IP: %w", err)
	}

	valInRange := big.NewInt(0).Add(&from, out)

	// big.Int.Bytes() strips leading zero bytes, so a value with a zero high
	// byte yields a too-short slice that AddrFromSlice rejects. Pad to the
	// prefix's address width.
	ip, ok := netip.AddrFromSlice(valInRange.FillBytes(make([]byte, len(fromIP.AsSlice()))))
	if !ok {
		return netip.Addr{}, errGeneratedIPBytesInvalid
	}

	if !pfx.Contains(ip) {
		return netip.Addr{}, fmt.Errorf(
			"%w: ip(%s) not in prefix(%s)",
			errGeneratedIPNotInPrefix,
			ip.String(),
			pfx.String(),
		)
	}

	return ip, nil
}

func isTailscaleReservedIP(ip netip.Addr) bool {
	return tsaddr.ChromeOSVMRange().Contains(ip) ||
		tsaddr.TailscaleServiceIP() == ip ||
		tsaddr.TailscaleServiceIPv6() == ip
}

// BackfillNodeIPs will take a database transaction, and
// iterate through all of the current nodes ([types.Node]) in headscale
// and ensure it has IP addresses according to the current
// configuration.
// This means that if both IPv4 and IPv6 is set in the
// config, and some nodes are missing that type of IP,
// it will be added.
// If a prefix type has been removed (IPv4 or IPv6), it
// will remove the IPs in that family from the node.
func (db *HSDatabase) BackfillNodeIPs(ctx context.Context, i *IPAllocator) ([]string, error) {
	var (
		err error
		ret []string
	)

	err = db.Write(ctx, func(tx *gorm.DB) error {
		if i == nil {
			return fmt.Errorf("backfilling IPs: %w", errIPAllocatorNil)
		}

		log.Trace().Caller().Msgf("starting to backfill IPs")

		nodes, err := ListNodes(tx)
		if err != nil {
			return fmt.Errorf("listing nodes to backfill IPs: %w", err)
		}

		for _, node := range nodes {
			log.Trace().Caller().EmbedObject(node).Msg("ip backfill check started because node found in database")

			changed := false
			// IPv4 prefix is set, but node ip is missing, alloc
			if i.prefix4 != nil && node.IPv4 == nil {
				ret4, err := i.allocateNext4(node.TailnetID)
				if err != nil {
					return fmt.Errorf("allocating IPv4 for node(%d): %w", node.ID, err)
				}

				node.IPv4 = ret4
				changed = true

				ret = append(ret, fmt.Sprintf("assigned IPv4 %q to Node(%d) %q", ret4.String(), node.ID, node.Hostname))
			}

			// IPv6 prefix is set, but node ip is missing, alloc
			if i.prefix6 != nil && node.IPv6 == nil {
				ret6, err := i.allocateNext6(node.TailnetID)
				if err != nil {
					return fmt.Errorf("allocating IPv6 for node(%d): %w", node.ID, err)
				}

				node.IPv6 = ret6
				changed = true

				ret = append(ret, fmt.Sprintf("assigned IPv6 %q to Node(%d) %q", ret6.String(), node.ID, node.Hostname))
			}

			// IPv4 prefix is not set, but node has IP, remove
			if i.prefix4 == nil && node.IPv4 != nil {
				ret = append(ret, fmt.Sprintf("removing IPv4 %q from Node(%d) %q", node.IPv4.String(), node.ID, node.Hostname))
				node.IPv4 = nil
				changed = true
			}

			// IPv6 prefix is not set, but node has IP, remove
			if i.prefix6 == nil && node.IPv6 != nil {
				ret = append(ret, fmt.Sprintf("removing IPv6 %q from Node(%d) %q", node.IPv6.String(), node.ID, node.Hostname))
				node.IPv6 = nil
				changed = true
			}

			if changed {
				// Use Updates() with Select() to only update IP fields, avoiding overwriting
				// other fields like Expiry. We need Select() because Updates() alone skips
				// zero values, but we DO want to update IPv4/IPv6 to nil when removing them.
				err := tx.Model(node).Select("ipv4", "ipv6").Updates(node).Error
				if err != nil {
					return fmt.Errorf("saving node(%d) after adding IPs: %w", node.ID, err)
				}
			}
		}

		return nil
	})

	return ret, err
}

// FreeIPs releases the given addresses back into the pool of the given
// Tailnet. A freed address only frees it within its own Tailnet (ADR-0002).
func (i *IPAllocator) FreeIPs(tailnetID uint, ips []netip.Addr) {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.stateFor(tailnetID)

	for _, ip := range ips {
		s.usedIPs.Remove(ip)
	}
}
