package db

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/net/tsaddr"
)

var mpp = func(pref string) *netip.Prefix {
	p := netip.MustParsePrefix(pref)
	return &p
}

var na = netip.MustParseAddr

var nap = func(pref string) *netip.Addr {
	n := na(pref)
	return &n
}

func TestIPAllocatorSequential(t *testing.T) {
	tests := []struct {
		name   string
		dbFunc func() *HSDatabase

		prefix4  *netip.Prefix
		prefix6  *netip.Prefix
		getCount int
		want4    []netip.Addr
		want6    []netip.Addr
	}{
		{
			name: "simple",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1,

			want4: []netip.Addr{
				na("100.64.0.1"),
			},
			want6: []netip.Addr{
				na("fd7a:115c:a1e0::1"),
			},
		},
		{
			name: "simple-v4",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix4: mpp("100.64.0.0/10"),

			getCount: 1,

			want4: []netip.Addr{
				na("100.64.0.1"),
			},
		},
		{
			name: "simple-v6",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1,

			want6: []netip.Addr{
				na("fd7a:115c:a1e0::1"),
			},
		},
		{
			name: "simple-with-db",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.1"),
					IPv6: nap("fd7a:115c:a1e0::1"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1,

			want4: []netip.Addr{
				na("100.64.0.2"),
			},
			want6: []netip.Addr{
				na("fd7a:115c:a1e0::2"),
			},
		},
		{
			name: "before-after-free-middle-in-db",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.2"),
					IPv6: nap("fd7a:115c:a1e0::2"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 2,

			want4: []netip.Addr{
				na("100.64.0.1"),
				na("100.64.0.3"),
			},
			want6: []netip.Addr{
				na("fd7a:115c:a1e0::1"),
				na("fd7a:115c:a1e0::3"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.dbFunc()

			alloc, _ := NewIPAllocator(context.Background(),
				db,
				tt.prefix4,
				tt.prefix6,
				types.IPAllocationStrategySequential,
			)

			var (
				got4s []netip.Addr
				got6s []netip.Addr
			)

			for range tt.getCount {
				got4, got6, err := alloc.Next(types.DefaultTailnetID)
				if err != nil {
					t.Fatalf("allocating next IP: %s", err)
				}

				if got4 != nil {
					got4s = append(got4s, *got4)
				}

				if got6 != nil {
					got6s = append(got6s, *got6)
				}
			}

			if diff := cmp.Diff(tt.want4, got4s, util.Comparers...); diff != "" {
				t.Errorf("IPAllocator 4s unexpected result (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.want6, got6s, util.Comparers...); diff != "" {
				t.Errorf("IPAllocator 6s unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIPAllocatorRandom(t *testing.T) {
	tests := []struct {
		name   string
		dbFunc func() *HSDatabase

		getCount int

		prefix4 *netip.Prefix
		prefix6 *netip.Prefix
		want4   bool
		want6   bool
	}{
		{
			name: "simple",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1,

			want4: true,
			want6: true,
		},
		{
			name: "simple-v4",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix4: mpp("100.64.0.0/10"),

			getCount: 1,

			want4: true,
			want6: false,
		},
		{
			name: "simple-v6",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1,

			want4: false,
			want6: true,
		},
		{
			name: "generate-lots-of-random",
			dbFunc: func() *HSDatabase {
				return nil
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			getCount: 1000,

			want4: true,
			want6: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.dbFunc()

			alloc, _ := NewIPAllocator(context.Background(), db, tt.prefix4, tt.prefix6, types.IPAllocationStrategyRandom)

			for range tt.getCount {
				got4, got6, err := alloc.Next(types.DefaultTailnetID)
				if err != nil {
					t.Fatalf("allocating next IP: %s", err)
				}

				t.Logf("addrs ipv4: %v, ipv6: %v", got4, got6)

				if tt.want4 {
					if got4 == nil {
						t.Fatalf("expected ipv4 addr, got nil")
					}
				}

				if tt.want6 {
					if got6 == nil {
						t.Fatalf("expected ipv4 addr, got nil")
					}
				}
			}
		})
	}
}

func TestBackfillIPAddresses(t *testing.T) {
	fullNodeP := func(i int) *types.Node {
		v4 := fmt.Sprintf("100.64.0.%d", i)
		v6 := fmt.Sprintf("fd7a:115c:a1e0::%d", i)

		return &types.Node{
			IPv4: nap(v4),
			IPv6: nap(v6),
		}
	}
	tests := []struct {
		name   string
		dbFunc func() *HSDatabase

		prefix4 *netip.Prefix
		prefix6 *netip.Prefix
		want    types.Nodes
	}{
		{
			name: "simple-backfill-ipv6",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.1"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			want: types.Nodes{
				&types.Node{
					IPv4: nap("100.64.0.1"),
					IPv6: nap("fd7a:115c:a1e0::1"),
				},
			},
		},
		{
			name: "simple-backfill-ipv4",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv6: nap("fd7a:115c:a1e0::1"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			want: types.Nodes{
				&types.Node{
					IPv4: nap("100.64.0.1"),
					IPv6: nap("fd7a:115c:a1e0::1"),
				},
			},
		},
		{
			name: "simple-backfill-remove-ipv6",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.1"),
					IPv6: nap("fd7a:115c:a1e0::1"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),

			want: types.Nodes{
				&types.Node{
					IPv4: nap("100.64.0.1"),
				},
			},
		},
		{
			name: "simple-backfill-remove-ipv4",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.1"),
					IPv6: nap("fd7a:115c:a1e0::1"),
				})

				return db
			},

			prefix6: mpp("fd7a:115c:a1e0::/48"),

			want: types.Nodes{
				&types.Node{
					IPv6: nap("fd7a:115c:a1e0::1"),
				},
			},
		},
		{
			name: "multi-backfill-ipv6",
			dbFunc: func() *HSDatabase {
				db := dbForTest(t)
				user := types.User{Name: ""}
				db.DB.Save(&user)

				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.1"),
				})
				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.2"),
				})
				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.3"),
				})
				db.DB.Save(&types.Node{
					User: &user,
					IPv4: nap("100.64.0.4"),
				})

				return db
			},

			prefix4: mpp("100.64.0.0/10"),
			prefix6: mpp("fd7a:115c:a1e0::/48"),

			want: types.Nodes{
				fullNodeP(1),
				fullNodeP(2),
				fullNodeP(3),
				fullNodeP(4),
			},
		},
	}

	comps := append(util.Comparers, cmpopts.IgnoreFields(types.Node{},
		"ID",
		"TenantID",
		"TailnetID",
		"User",
		"UserID",
		"Endpoints",
		"Hostinfo",
		"CreatedAt",
		"UpdatedAt",
	))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tt.dbFunc()

			alloc, err := NewIPAllocator(context.Background(),
				db,
				tt.prefix4,
				tt.prefix6,
				types.IPAllocationStrategySequential,
			)
			if err != nil {
				t.Fatalf("failed to set up ip alloc: %s", err)
			}

			logs, err := db.BackfillNodeIPs(context.Background(), alloc)
			if err != nil {
				t.Fatalf("failed to backfill: %s", err)
			}

			t.Logf("backfill log: \n%s", strings.Join(logs, "\n"))

			got, err := db.ListNodes()
			if err != nil {
				t.Fatalf("failed to get nodes: %s", err)
			}

			if diff := cmp.Diff(tt.want, got, comps...); diff != "" {
				t.Errorf("Backfill unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIPAllocatorNextNoReservedIPs(t *testing.T) {
	db, err := newSQLiteTestDB()
	require.NoError(t, err)

	defer db.Close()

	alloc, err := NewIPAllocator(context.Background(),
		db,
		new(tsaddr.CGNATRange()),
		new(tsaddr.TailscaleULARange()),
		types.IPAllocationStrategySequential,
	)
	if err != nil {
		t.Fatalf("failed to set up ip alloc: %s", err)
	}

	s := alloc.stateFor(types.DefaultTailnetID)

	// Validate that we do not give out 100.100.100.100
	nextQuad100, err := alloc.next(s, na("100.100.100.99"), new(tsaddr.CGNATRange()))
	require.NoError(t, err)
	assert.Equal(t, na("100.100.100.101"), *nextQuad100)

	// Validate that we do not give out fd7a:115c:a1e0::53
	nextQuad100v6, err := alloc.next(s, na("fd7a:115c:a1e0::52"), new(tsaddr.TailscaleULARange()))
	require.NoError(t, err)
	assert.Equal(t, na("fd7a:115c:a1e0::54"), *nextQuad100v6)

	// Validate that we do not give out fd7a:115c:a1e0::53
	nextChrome, err := alloc.next(s, na("100.115.91.255"), new(tsaddr.CGNATRange()))
	t.Logf("chrome: %s", nextChrome.String())
	require.NoError(t, err)
	assert.Equal(t, na("100.115.94.0"), *nextChrome)
}

// TestIPAllocatorPerTailnet proves the core ADR-0002 property: allocation is
// scoped per Tailnet. Two nodes in different Tailnets can be allocated the same
// address, while allocation within a single Tailnet never repeats an address.
func TestIPAllocatorPerTailnet(t *testing.T) {
	const (
		tailnetA uint = 1
		tailnetB uint = 2
		count         = 50
	)

	alloc, err := NewIPAllocator(context.Background(),
		nil,
		mpp("100.64.0.0/10"),
		mpp("fd7a:115c:a1e0::/48"),
		types.IPAllocationStrategySequential,
	)
	require.NoError(t, err)

	seenA4 := make(map[netip.Addr]bool)
	var seqA4, seqB4 []netip.Addr

	// Allocate a run of addresses in Tailnet A, asserting no repeats within it.
	for range count {
		got4, _, err := alloc.Next(tailnetA)
		require.NoError(t, err)
		require.NotNil(t, got4)

		if seenA4[*got4] {
			t.Fatalf("address %s handed out twice within tailnet %d", got4, tailnetA)
		}
		seenA4[*got4] = true
		seqA4 = append(seqA4, *got4)
	}

	// Allocate the same number in Tailnet B: it has its own independent pool,
	// so it must reproduce the identical sequence Tailnet A got.
	for range count {
		got4, _, err := alloc.Next(tailnetB)
		require.NoError(t, err)
		require.NotNil(t, got4)
		seqB4 = append(seqB4, *got4)
	}

	if diff := cmp.Diff(seqA4, seqB4, util.Comparers...); diff != "" {
		t.Errorf("tailnet B did not get the same addresses as tailnet A (-A +B):\n%s", diff)
	}

	// The first address in each Tailnet is the same concrete address, held by
	// two different nodes without either allocator treating it as a collision.
	assert.Equal(t, na("100.64.0.1"), seqA4[0])
	assert.Equal(t, seqA4[0], seqB4[0])
}
