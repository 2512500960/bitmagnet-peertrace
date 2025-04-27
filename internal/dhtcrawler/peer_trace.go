package dhtcrawler

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/peertrace"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	"go.uber.org/fx"
	"gorm.io/gorm/clause"
)

type PeerTraceParams struct {
	fx.In
	Config Config
}

type PeerTraceResult struct {
	fx.Out
	PeerTraceInfoHashWithPeersChan concurrency.BatchingChannel[peertrace.PeerTraceInfoHashWithPeers]
	PeerTracePruneChan             concurrency.BatchingChannel[peertrace.PeerTracePrune]
}

func NewPeerTrace(params PeerTraceParams) PeerTraceResult {
	return PeerTraceResult{
		PeerTraceInfoHashWithPeersChan: concurrency.NewBatchingChannel[peertrace.PeerTraceInfoHashWithPeers](int(100*params.Config.ScalingFactor), 10, time.Second/100),
		PeerTracePruneChan:             concurrency.NewBatchingChannel[peertrace.PeerTracePrune](int(100*params.Config.ScalingFactor), 10, time.Second/100),
	}
}

func (c *crawler) runPeerTrace(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case is := <-c.peerTraceInfoHashWithPeers.Out():
			c.logger.Debug(is)

			records, err := c.createPeerTraceModel(is)
			if err != nil {
				c.logger.Debug(err)
			}
			persistErr := c.dao.WithContext(ctx).PeerTrace.Clauses(
				clause.OnConflict{
					Columns: []clause.Column{
						{Name: c.dao.PeerTrace.IP.ColumnName().String()},
						{Name: c.dao.PeerTrace.InfoHash.ColumnName().String()},
					},
					DoUpdates: clause.AssignmentColumns([]string{
						c.dao.PeerTrace.LastSeenTime.ColumnName().String(),
					}),
				},
			).CreateInBatches(records, 500)
			if persistErr != nil {
				c.logger.Errorf("error persisting peer trace: %s", persistErr.Error())
			}
		}

	}
}

func (c *crawler) runPeerTracePrune(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case prunes := <-c.peerTracePrune.Out():
			for _, prune := range prunes {
				if prune.Criteria == "infohash" {
					c.dao.PeerTrace.WithContext(ctx).Where(c.dao.PeerTrace.InfoHash.Eq(prune.InfoHash)).Delete()
				} else if prune.Criteria == "peer" {
					c.dao.PeerTrace.WithContext(ctx).Where(c.dao.PeerTrace.IP.Eq(prune.Peers.Addr().String())).Delete()
				}
				c.logger.Debugf("prune %s", prune)
			}
		}

	}
}

func (c *crawler) runPeerTraceRuminate(ctx context.Context) {
	limit := int64(100000)
	total_count := int64(0)
	total_count, _ = c.dao.Torrent.WithContext(ctx).Select().Distinct(c.dao.Torrent.InfoHash).Count()
	offset := int64(5000000)
	offset = total_count / 2
	time.Sleep(time.Duration(15 * time.Minute.Seconds()))
	for {
		total_count, _ = c.dao.PeerTrace.WithContext(ctx).Select().Count()
		c.logger.Infof("torrent table has %d infohash now", total_count)
		if offset >= total_count {
			offset = 0
		}
		// select some infohashes from peer_trace table, send them to infohash_triage channel
		torrents, err := c.dao.Torrent.WithContext(ctx).Select(c.dao.Torrent.ALL).Limit(int(limit)).Order(
			c.dao.Torrent.CreatedAt.Desc()).Offset(int(offset)).Find()
		c.logger.Infof("ruminate %d/%d infohash from torrent", offset, total_count)
		if err != nil {
			c.logger.Debugf("select infohashes from Torrent error")
		}
		count := 0
		var last_infoash protocol.ID
		for _, torrent := range torrents {
			if torrent.InfoHash == last_infoash {
				continue
			}
			last_infoash = torrent.InfoHash
			select {
			case <-ctx.Done():
				return
			case c.infoHashTriage.In() <- nodeHasPeersForHash{
				infoHash: torrent.InfoHash,
				node:     c.kTable.GetClosestNodes(torrent.InfoHash)[0].Addr(),
				source:   "ruminate",
			}:
				count++
				c.logger.Infof("ruminate for infohash %s", torrent.InfoHash)
				if count == 5 {
					time.Sleep(time.Second * 1)
					count = 0
				}
				continue

			}
		}
		offset += limit
	}
}
func (c *crawler) runPeerTraceRuminateMissingHashesTest(ctx context.Context) {
	ihs := []string{
		"d471223e517199c5159a39ab5ceade4b716b264f",
		"b223cef3c2949831daf95bc38e3e6da7be4ba794",
		"7e78a8eb1c593e3e273118e616abb11c8c8b8734",
		"7b471f0f3ade0af976f9ed6e558a71d161bad5a9",
		"9e55796991851fa65c4514cb51cce2d359a2b268",
		"b375c0d21ed9700bbcc437a68e79ec7a4e6add1b",
		"469de1d2bb3bb30ba93c50d0c8aa8bb0e33f8b54",
		"c31bfe13b3350582f2cc0ceb9a2c83e06f52023c",
		"9e7291161fc67b6e87f0c54fc01ded7af15650e7",
		"475cfe09a9f47b6a7b56641b2b26deef479b9687",
		"d4b1273a5d78e4c78d2d01c3b5711d076b5b3e2d",
		"9a2ce5ee4451203c49d384cf2b18cfa7757442cc",
		"d6fbfb63eaa25d09076b680d68025fcaa8623e81",
		"d4854fbe91a4af8ca4a30fbbbbf3c9009b861c71",
		"e68ac5799c56411170ec77ae587b2ce6b5b1f497",
		"9ea62f2050d286045fca7e9d69141c58aa0ff12b",
		"e7e77350468387a70438f15ff25824202f6970ba",
		"b3b84605ad80fbbacc6ddd8d5091accee63256f1",
		"38cb2b8434a630df9fd8f55f672091d1c6712992",
		"9a3f745c14822ab03c405e5f4b3efa47a5552cb0",
		"e00327cea90ff5bbe06b5a4b4edfcb5cf12371fc",
		"ba95754dec509b8649fd192b2f1d8a1a370507de",
		"9a3c3191f6c71fb7b0683c0ff9be252d4a3d0ec9",
		"3a8f7b9695664286bcc950b573297d95bbb3e57e",
		"3fd8486d22e8b7e858f30d588b5a0d8c3076aafe",
		"abee05f6b68ef39598df5672637e12338118c617",
		"46d39e6fab08eeee82d28df9336e7925283e10cb",
		"39ebab5aabbcfc04b6d12e9346cd4ee60ac2c7d0",
		"fb098ce1702892f05b91698e5427f31d074eef65",
		"3dc8117b7cb98e253bb6be5b5b97c8273bae2b80",
		"9a844c6f3221e516ab9967d842b793a8a66a7212",
		"b30502c7eca3919da837d826f0937af6a992b676",
		"b954412c1645796339f83b0f9f111e866bbfe20c",
		"afbc1202c1d414ed5402e6a029381f81b4ff4a26",
	}
	for _, ih := range ihs {
		var id protocol.ID
		if _, err := hex.Decode(id[:], []byte(ih)); err != nil {
			fmt.Printf("Error decoding hex string %s: %v\n", ih, err)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case c.infohashToDownloaderChannel.In() <- id:
			continue
		}
	}
}

func (c *crawler) runPeerTraceRuminateMissingHashes(ctx context.Context) {
	c.logger.Info("start ruminate missing hashes")
	limit := int64(100000)
	offset := int64(0)
	//time.Sleep(time.Duration(15 * time.Minute.Seconds()))
	for {
		// select some infohashes from peer_trace table, send them to infohash_triage channel
		var peertraces []*model.PeerTrace = nil
		c.dao.PeerTrace.WithContext(ctx).UnderlyingDB().Raw(
			fmt.Sprintf(`select * FROM peer_trace
			WHERE info_hash IN (
				SELECT pt.info_hash
				FROM peer_trace pt
				LEFT JOIN torrents t ON pt.info_hash = t.info_hash
				WHERE t.info_hash IS NULL order by last_seen_time
			) limit %d;`, limit)).Find(&peertraces)
		if int64(len(peertraces)) < limit {
			//c.logger.Infof("no more infohashes to ruminate")
			time.Sleep(time.Duration(15 * time.Minute))
			continue
		}
		c.logger.Infof("ruminate %d infohash from peertrace", len(peertraces))

		batchSize := 300
		for i := 0; i < len(peertraces); i += batchSize {
			groupEnd := i + batchSize
			if groupEnd > len(peertraces) {
				groupEnd = len(peertraces)
			}
			var last_infohash protocol.ID
			for j := i; j < groupEnd; j++ {
				peertrace := peertraces[j]
				if peertrace.InfoHash == last_infohash {
					continue
				}
				last_infohash = peertrace.InfoHash
				select {
				case <-ctx.Done():
					return
				case c.infoHashTriage.In() <- nodeHasPeersForHash{
					infoHash: peertrace.InfoHash,
					node:     c.kTable.GetClosestNodes(peertrace.InfoHash)[0].Addr(),
					source:   "ruminate",
				}:
					c.logger.Infof("`ruminate` for infohash %s", peertrace.InfoHash)
					continue
				}
			}
			time.Sleep(time.Second * 15)

		}
		offset += limit
	}
}

func (c *crawler) filterPeerTraceByIP(peer netip.AddrPort) (filter bool) {
	country, err := c.SearchGeoIPReaderCity.Country(net.ParseIP(peer.Addr().String()))
	if err == nil && country.Country.IsoCode == "CN" {
		//c.logger.Debugf("%s is in TargetArea, will record it", peer.Addr().String())
		return true
	} else {
		//c.logger.Debugf("%s is not in TargetArea, will not record it", peer.Addr().String())
		return false
	}

}
func (c *crawler) createPeerTraceModel(
	results []peertrace.PeerTraceInfoHashWithPeers,
) ([]*model.PeerTrace, error) {
	size := 0
	for _, result := range results {
		size += len(result.Peers)
	}

	records := make([]*model.PeerTrace, 0, size)
	seen := make(map[string]bool)
	for _, result := range results {
		infoHash := string(result.InfoHash[:])
		for _, peer := range result.Peers {
			peer_ip := peer.Addr().String()
			if peer_ip == "invalid IP" {
				continue
			}
			if !c.filterPeerTraceByIP(peer) {
				continue
			}
			// if peer_ip is ipv4_in_ipv6, reformat it to ipv4, strip leading "::ffff:"

			peer_ip = strings.TrimPrefix(peer_ip, "::ffff:")
			key := infoHash + "|" + peer_ip
			if seen[key] {
				continue
			}
			seen[key] = true
			//c.logger.Infof("peer trace %s %s", result.InfoHash, peer_ip)
			records = append(records, &model.PeerTrace{
				InfoHash: result.InfoHash,
				IP:       peer_ip,
			})
		}
	}

	return records, nil
}
