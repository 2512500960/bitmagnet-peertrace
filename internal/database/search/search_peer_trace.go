package search

import (
	"context"
	"fmt"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/database/query"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

type PeerTraceResult = query.GenericResult[model.PeerTrace]

type PeerTraceSearch interface {
	PeerTrace(ctx context.Context, options ...query.Option) (result PeerTraceResult, err error)
	PeerTraceFiltered(ctx context.Context, input gen.PeerTraceFilterInput) (result PeerTraceResult, err error)
	PeerTraceTorrents(ctx context.Context, ip string) (result gen.PeerTraceTorrentsTraceResult, err error)
}

func (s search) PeerTraceTorrents(ctx context.Context, ip string) (result gen.PeerTraceTorrentsTraceResult, err error) {
	peerTracesResult, _ := s.PeerTrace(ctx,
		query.Where(query.DbCriteria{
			Sql: fmt.Sprintf("ip = ('%s')", ip),
		}),
	)
	torrentTracesSlice := make([]gen.PeerTraceTorrentsTrace, 0, len(peerTracesResult.Items))
	for _, peerTrace := range peerTracesResult.Items {
		infoHash := peerTrace.InfoHash
		torrent_result, err := s.Torrents(ctx, query.Where(infoHashCriteria(s.q.Torrent.TableName(), infoHash)))
		if err != nil {
			continue
		}
		if len(torrent_result.Items) > 0 {
			torrentTracesSlice = append(torrentTracesSlice, gen.PeerTraceTorrentsTrace{
				LastSeenTime: peerTrace.LastSeenTime,
				Torrent:      torrent_result.Items[0],
			})
		}
	}
	return gen.PeerTraceTorrentsTraceResult{
		TorrentTraces: torrentTracesSlice,
	}, nil
}
func (s search) PeerTraceFiltered(ctx context.Context, input gen.PeerTraceFilterInput) (PeerTraceResult, error) {
	if input.IP.IsSet() && *input.IP.Value() != "*" && *input.IP.Value() != "" && *input.IP.Value() != "0.0.0.0" && *input.IP.Value() != "::" {
		result, err := s.PeerTrace(ctx,
			query.Where(query.DbCriteria{
				Sql: fmt.Sprintf("ip = ('%s')", *input.IP.Value()),
			}),
		)
		return result, err
	}
	if input.InfoHash.IsSet() {
		result, err := s.PeerTrace(ctx, query.Where(infoHashCriteria(s.q.PeerTrace.TableName(), *input.InfoHash.Value())))
		return result, err
	}
	return PeerTraceResult{
		Items: nil,
	}, nil
}

func (s search) PeerTrace(ctx context.Context, options ...query.Option) (result PeerTraceResult, err error) {
	return query.GenericQuery[model.PeerTrace](
		ctx,
		s.q,
		query.Options(append([]query.Option{query.SelectAll()}, options...)...),
		model.TableNamePeerTrace,
		func(ctx context.Context, q *dao.Query) query.SubQuery {
			return query.GenericSubQuery[dao.IPeerTraceDo]{
				SubQuery: q.PeerTrace.WithContext(ctx).ReadDB().Order(q.PeerTrace.LastSeenTime.Desc()),
			}
		},
	)
}
