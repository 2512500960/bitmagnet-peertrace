package dhtcrawler

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/peertrace"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/dht/ktable"
)

func (c *crawler) runGetPeers(ctx context.Context) {
	_ = c.getPeers.Run(ctx, func(req nodeHasPeersForHash) {
		pfh, pfhErr := c.requestPeersForHashDepthe2(ctx, req)
		if pfhErr != nil {
			return
		}
		//c.logger.Infof("find %d peers for %s", len(pfh.peers), req.infoHash)
		peers := make([]netip.AddrPort, 0, len(pfh.peers))
		hashPeers := make([]ktable.HashPeer, 0, len(pfh.peers))
		for _, p := range pfh.peers {
			peers = append(peers, p)
			hashPeers = append(hashPeers, ktable.HashPeer{
				Addr: p,
			})
		}
		c.kTable.BatchCommand(
			ktable.PutHash{ID: req.infoHash, Peers: hashPeers},
		)
		select {
		case <-ctx.Done():
			return
		case c.requestMetaInfo.In() <- infoHashWithPeers{
			nodeHasPeersForHash: req,
			peers:               peers,
		}:
		}

		select {
		case <-ctx.Done():
			return
		case c.peerTraceInfoHashWithPeers.In() <- peertrace.PeerTraceInfoHashWithPeers{
			Source:   "GetPeers",
			InfoHash: req.infoHash,
			Peers:    peers,
		}:
			return
		}
	})
}

func (c *crawler) requestPeersForHash(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithPeers, error) {
	res, err := c.client.GetPeers(ctx, req.node, req.infoHash)
	if err != nil {
		c.kTable.BatchCommand(ktable.DropAddr{Addr: req.node.Addr(), Reason: fmt.Errorf("failed to get peers: %w", err)})
		return infoHashWithPeers{}, err
	} else {
		c.kTable.BatchCommand(ktable.PutNode{ID: res.ID, Addr: req.node, Options: []ktable.NodeOption{ktable.NodeResponded()}})
	}
	if len(res.Nodes) > 0 {
		// block the channel for up to a second in an attempt to add the nodes to the discoveredNodes channel
		cancelCtx, cancel := context.WithTimeout(ctx, time.Second)
		for _, n := range res.Nodes {
			select {
			case <-cancelCtx.Done():
				break
			case c.discoveredNodes.In() <- ktable.NewNode(n.ID, n.Addr):
				continue
			}
		}
		cancel()
	}
	if len(res.Values) < 1 {
		return infoHashWithPeers{}, errors.New("no peers found")
	}
	return infoHashWithPeers{
		nodeHasPeersForHash: req,
		peers:               res.Values,
	}, nil
}

func (c *crawler) requestPeersForHashDepthe2(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithPeers, error) {
	level1InfoHashWithPeers, err := c.requestPeersForHash(ctx, req)
	peers := make([]netip.AddrPort, 0)
	peerMap := make(map[string]struct{}) // 用于存储已经存在的peer
	if err != nil {
		return level1InfoHashWithPeers, err
	}
	for _, p := range level1InfoHashWithPeers.peers {
		if p == req.node {
			continue
		}
		peerMap[p.String()] = struct{}{} // 将peer添加到map中
		peers = append(peers, p)
	}
	for _, p := range level1InfoHashWithPeers.peers {
		if p == req.node {
			continue
		}
		level2InfoHashWithPeers, err := c.requestPeersForHash(ctx, nodeHasPeersForHash{
			node:     p,
			infoHash: req.infoHash,
		})
		if err != nil {
			continue
		}
		for _, peer := range level2InfoHashWithPeers.peers {
			if _, exists := peerMap[peer.String()]; !exists {
				peerMap[peer.String()] = struct{}{}
				peers = append(peers, peer)
			}
		}
		if len(peers) > 200 {
			break
		}
		select {
		case <-ctx.Done():
			return infoHashWithPeers{}, ctx.Err()
		default:
			continue
		}
	}
	return infoHashWithPeers{
		nodeHasPeersForHash: req,
		peers:               peers,
	}, nil
}
