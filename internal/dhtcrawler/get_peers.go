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
		pfh, pfhErr := c.requestPeersForHash(ctx, req)
		//pfh, pfhErr := c.requestPeersForHashIterative(ctx, req)
		if pfhErr != nil {
			return
		}
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

func (c *crawler) requestPeersForHashIterative(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithPeers, error) {
	try_time := 1
	peers := make([]netip.AddrPort, 0)
	c.logger.Debugf("trying to get peers for %s", req.infoHash)
	for try_time < 10 && len(peers) < 8 {
		try_time += 1
		res, err := c.client.GetPeers(ctx, req.node, req.infoHash)
		if err != nil {
			c.kTable.BatchCommand(ktable.DropAddr{Addr: req.node.Addr(), Reason: fmt.Errorf("failed to get peers: %w", err)})
			break
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
		if len(res.Nodes) > 0 && len(res.Values) == 0 {
			req.node = c.kTable.GetClosestNodes(req.infoHash)[0].Addr()
		}
		peers = append(peers, res.Values...)

	}
	c.logger.Infof("found %d peers for %s", len(peers), req.infoHash)
	if len(peers) == 0 {
		return infoHashWithPeers{}, errors.New("no peers found")
	}
	return infoHashWithPeers{
		nodeHasPeersForHash: req,
		peers:               peers,
	}, nil
}
