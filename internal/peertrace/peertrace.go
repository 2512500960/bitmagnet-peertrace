package peertrace

import (
	"net/netip"

	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
)

// used when receiving announcement/get_peer requests, or on GetPeers Request brought peers for infohash
type PeerTraceInfoHashWithPeers struct {
	Source   string
	InfoHash protocol.ID
	Peers    []netip.AddrPort
}
