package dhtcrawler

import (
	"context"
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
}

func NewPeerTrace(params PeerTraceParams) PeerTraceResult {
	return PeerTraceResult{
		PeerTraceInfoHashWithPeersChan: concurrency.NewBatchingChannel[peertrace.PeerTraceInfoHashWithPeers](int(100*params.Config.ScalingFactor), 10, time.Second/100),
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
			).CreateInBatches(records, 200)
			if persistErr != nil {
				c.logger.Errorf("error persisting peer trace: %s", persistErr.Error())
			}
		}

	}
}

func (c *crawler) runPeerTraceRuminate(ctx context.Context) {
	limit := int64(1000)
	total_count := int64(0)
	total_count, _ = c.dao.PeerTrace.WithContext(ctx).Select().Distinct(c.dao.PeerTrace.InfoHash).Count()
	offset := int64(5000000)
	offset = total_count / 2
	time.Sleep(time.Duration(15 * time.Minute.Seconds()))
	for {
		total_count, _ = c.dao.PeerTrace.WithContext(ctx).Select().Count()
		c.logger.Infof("peertrace table has %d infohash now", total_count)
		if offset >= total_count {
			offset = 0
		}
		// select some infohashes from peer_trace table, send them to infohash_triage channel
		peertraces, err := c.dao.PeerTrace.WithContext(ctx).Select(c.dao.PeerTrace.ALL).Limit(int(limit)).Order(
			c.dao.PeerTrace.LastSeenTime.Desc(),
		).
			Offset(int(offset)).Find()
		c.logger.Infof("ruminate %d/%d infohash from peertrace", offset, total_count)
		if err != nil {
			c.logger.Debugf("select infohashes from peertrace error")
		}
		count := 0
		var last_infoash protocol.ID
		for _, peertrace := range peertraces {
			if peertrace.InfoHash == last_infoash {
				continue
			}
			last_infoash = peertrace.InfoHash
			select {
			case <-ctx.Done():
				return
			case c.infoHashTriage.In() <- nodeHasPeersForHash{
				infoHash: peertrace.InfoHash,
				node:     c.kTable.GetClosestNodes(peertrace.InfoHash)[0].Addr(),
			}:
				count++
				c.logger.Infof("ruminate for infohash %s", peertrace.InfoHash)
				if count == 50 {
					time.Sleep(time.Second * 1)
					count = 0
				}
				continue

			}
		}
		offset += limit
	}
}

func (c *crawler) runPeerTraceRuminateMissingHashes(ctx context.Context) {
	limit := int64(1000000)
	total_count := int64(0)
	c.dao.PeerTrace.WithContext(ctx).UnderlyingDB().Raw(
		`select 1 FROM peer_trace
		WHERE info_hash IN (
			SELECT pt.info_hash
			FROM peer_trace pt
			LEFT JOIN torrents t ON pt.info_hash = t.info_hash
			WHERE t.info_hash IS NULL order by last_seen_time  
		) ;`).Count(&total_count)
	offset := int64(5000000)
	offset = total_count / 2
	time.Sleep(time.Duration(15 * time.Minute.Seconds()))
	for {
		c.logger.Infof("peertrace table has %d unrecognized infohash now", total_count)
		if offset >= total_count {
			offset = 0
		}
		// select some infohashes from peer_trace table, send them to infohash_triage channel
		var peertraces []*model.PeerTrace
		c.dao.PeerTrace.WithContext(ctx).UnderlyingDB().Raw(
			fmt.Sprintf(`select * FROM peer_trace
			WHERE info_hash IN (
				SELECT pt.info_hash
				FROM peer_trace pt
				LEFT JOIN torrents t ON pt.info_hash = t.info_hash
				WHERE t.info_hash IS NULL order by last_seen_time 
			) limit %d offset %d;`, limit, offset)).Find(&peertraces)
		c.logger.Infof("ruminate %d/%d infohash from peertrace", offset, total_count)

		count := 0
		var last_infoash protocol.ID
		for _, peertrace := range peertraces {
			if peertrace.InfoHash == last_infoash {
				continue
			}
			last_infoash = peertrace.InfoHash
			select {
			case <-ctx.Done():
				return
			case c.infoHashTriage.In() <- nodeHasPeersForHash{
				infoHash: peertrace.InfoHash,
				node:     c.kTable.GetClosestNodes(peertrace.InfoHash)[0].Addr(),
			}:
				count++
				c.logger.Debugf("ruminate for infohash %s", peertrace.InfoHash)
				if count == 50 {
					time.Sleep(time.Second * 1)
					count = 0
				}
				continue

			}
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
			records = append(records, &model.PeerTrace{
				InfoHash: result.InfoHash,
				IP:       peer_ip,
			})
		}
	}

	return records, nil
}
