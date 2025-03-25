package search

import (
	"github.com/bitmagnet-io/bitmagnet/internal/boilerplate/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/oschwald/geoip2-golang"
	"go.uber.org/fx"
)

var SearchGeoIPReaderCity *geoip2.Reader
var SearchGeoIPReaderASN *geoip2.Reader

type Search interface {
	ContentSearch
	QueueJobSearch
	TorrentSearch
	TorrentContentSearch
	TorrentFilesSearch
	PeerTraceSearch
}

type search struct {
	q *dao.Query
}

type Params struct {
	fx.In
	Query                 lazy.Lazy[*dao.Query]
	SearchGeoIPReaderCity *geoip2.Reader `name:"geoip_city"`
	SearchGeoIPReaderASN  *geoip2.Reader `name:"geoip_asn"`
}

type Result struct {
	fx.Out
	Search lazy.Lazy[Search]
}

func New(params Params) Result {
	SearchGeoIPReaderCity = params.SearchGeoIPReaderCity
	SearchGeoIPReaderASN = params.SearchGeoIPReaderASN
	return Result{
		Search: lazy.New(func() (Search, error) {
			q, err := params.Query.Get()
			if err != nil {
				return nil, err
			}
			return &search{
				q: q,
			}, nil
		}),
	}
}
