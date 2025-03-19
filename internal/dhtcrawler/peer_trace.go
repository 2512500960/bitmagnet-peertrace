package dhtcrawler

import (
	"context"
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/peertrace"
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

			records, err := createPeerTraceModel(is)
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

func createPeerTraceModel(
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

			// is ip is ipv4_in_ipv6, reformat it to ipv4, strip leading "::ffff:"
			if strings.HasPrefix(peer_ip, "::ffff:") {
				// 去除前缀
				peer_ip = strings.TrimPrefix(peer_ip, "::ffff:")
			}
			key := infoHash + "|" + peer_ip
			if seen[key] {
				continue
			}
			seen[key] = true
			records = append(records, &model.PeerTrace{
				InfoHash: result.InfoHash[:],
				IP:       peer_ip,
			})
		}
	}

	return records, nil
}
