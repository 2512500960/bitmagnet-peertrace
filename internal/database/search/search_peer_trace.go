package search

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/database/query"
	"github.com/bitmagnet-io/bitmagnet/internal/gql/gqlmodel/gen"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
)

var TORRENT_UNRECOGNIZED = model.Torrent{
	Name: "unrecognized",
}

type PeerTraceResult = query.GenericResult[model.PeerTrace]

type PeerTraceSearch interface {
	PeerTrace(ctx context.Context, options ...query.Option) (result PeerTraceResult, err error)
	PeerTraceFiltered(ctx context.Context, input gen.PeerTraceFilterInput) (result PeerTraceResult, err error)
	PeerTraceTorrents(ctx context.Context, ip string) (result gen.PeerTraceTorrentsTraceResult, err error)
	PeerTraceFilteredWithIpDetails(ctx context.Context, infoHash protocol.ID) (gen.PeersLocationByInfohashResult, error)
}

func (s search) PeerTraceTorrents(ctx context.Context, ip string) (result gen.PeerTraceTorrentsTraceResult, err error) {
	// try parse the ip address
	parsed_ip := net.ParseIP(ip)
	if parsed_ip == nil {
		return gen.PeerTraceTorrentsTraceResult{
			TorrentTraces: nil,
		}, nil
	}
	// net.ParseIP will convert ipv4 into ipv4_in_ipv6 format, need to reformat it
	ip = strings.TrimPrefix(parsed_ip.String(), "::ffff:")
	// list all traces this ip has
	peerTracesResult, _ := s.PeerTrace(ctx,
		query.Where(query.DbCriteria{
			Sql: fmt.Sprintf("ip = ('%s')", ip),
		}),
	)
	torrentTracesSlice := make([]gen.PeerTraceTorrentsTrace, 0, len(peerTracesResult.Items))
	// interate the traces, select each torrent associated with that infohash from trace
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
		} else {

			torrentTracesSlice = append(torrentTracesSlice, gen.PeerTraceTorrentsTrace{
				LastSeenTime: peerTrace.LastSeenTime,
				Torrent: model.Torrent{
					InfoHash: infoHash,
					Name:     "unrecognized",
				},
			})
		}
	}
	return gen.PeerTraceTorrentsTraceResult{
		TorrentTraces: torrentTracesSlice,
	}, nil
}
func (s search) PeerTraceFilteredWithIpDetails(ctx context.Context, infoHash protocol.ID) (gen.PeersLocationByInfohashResult, error) {

	peerTraces, err := s.PeerTrace(ctx, query.Where(infoHashCriteria(s.q.PeerTrace.TableName(), infoHash)))
	peers := make([]gen.PeerTraceWithLocation, 0, len(peerTraces.Items))

	for _, r := range peerTraces.Items {
		ip := r.IP
		ip_city, err1 := SearchGeoIPReaderCity.City(net.ParseIP(ip))
		ip_asn, err2 := SearchGeoIPReaderASN.ASN(net.ParseIP(ip))
		Location := gen.IPLocation{}
		Asn := gen.Asn{}
		if err1 == nil {
			Location.City = ip_city.City.Names["en"]
			Location.Country = ip_city.Country.Names["en"]
			Location.Longitude = ip_city.Location.Longitude
			Location.Latitude = ip_city.Location.Latitude
		} else {
			Location.City = "unknown"
			Location.Country = "unknown"
			Location.Longitude = 0
			Location.Latitude = 0
		}
		if err2 == nil {
			Asn.AutonomousSystemNumber = int(ip_asn.AutonomousSystemNumber)
			Asn.AutonomousSystemOrganization = ip_asn.AutonomousSystemOrganization
		} else {
			Asn.AutonomousSystemNumber = 0
			Asn.AutonomousSystemOrganization = "unknown"
		}
		Location.Asn = Asn
		peers = append(peers, gen.PeerTraceWithLocation{
			IP:           r.IP,
			InfoHash:     infoHash,
			LastSeenTime: r.LastSeenTime,
			Location:     Location,
		},
		)
	}
	return gen.PeersLocationByInfohashResult{
		TotalCount: 0,
		Peers:      peers,
	}, err

}
func (s search) PeerTraceFiltered(ctx context.Context, input gen.PeerTraceFilterInput) (PeerTraceResult, error) {
	if input.IP.IsSet() && *input.IP.Value() != "*" && *input.IP.Value() != "" && *input.IP.Value() != "0.0.0.0" && *input.IP.Value() != "::" {
		// try parse the ip address
		parsed_ip := net.ParseIP(*input.IP.Value())
		if parsed_ip == nil {
			return PeerTraceResult{
				Items: nil,
			}, nil
		}
		// net.ParseIP will convert ipv4 into ipv4_in_ipv6 format, need to reformat it
		ip := strings.TrimPrefix(parsed_ip.String(), "::ffff:")
		result, err := s.PeerTrace(ctx,
			query.Where(query.DbCriteria{
				Sql: fmt.Sprintf("ip = ('%s')", ip),
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
