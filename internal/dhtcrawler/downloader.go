package dhtcrawler

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/bitmagnet-io/bitmagnet/internal/concurrency"
	"github.com/bitmagnet-io/bitmagnet/internal/peertrace"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	"go.uber.org/fx"
)

type DownloaderParams struct {
	fx.In
	Config                         Config
	PeerTraceInfoHashWithPeersChan concurrency.BatchingChannel[peertrace.PeerTraceInfoHashWithPeers]
}

type DownloaderResult struct {
	fx.Out
	InfohashToDownloaderChannel concurrency.BufferedConcurrentChannel[protocol.ID]
	TorrentDownloaderClient     *torrent.Client
}
type ipPortAddr struct {
	IP   net.IP
	Port int
}

func tryIpPortFromNetAddr(addr torrent.PeerRemoteAddr) (ipPortAddr, bool) {
	ok := true
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		ok = false
	}
	portI64, err := strconv.ParseInt(port, 10, 0)
	if err != nil {
		ok = false
	}
	return ipPortAddr{net.ParseIP(host), int(portI64)}, ok
}
func (c *crawler) runDownloaderStats(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			//c.torrentDownloaderClient.WriteStatus(&buf)
			c.logger.Infof("downloader hash %d tasks", len(c.torrentDownloaderClient.Torrents()))
		}
	}
}
func (c *crawler) runDownloader(ctx context.Context) {
	go c.runDownloaderStats(ctx)
	_ = c.infohashToDownloaderChannel.Run(ctx, func(infohash protocol.ID) {

		t, new := c.torrentDownloaderClient.AddTorrentInfoHash(infohash.Int160().AsByteArray())
		if !new {
			return
		}
		timeout_ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		c.logger.Infof("added hash %s to Downloader ", infohash)
		defer cancel()
		for {

			var ihwmi infoHashWithMetaInfo

			select {
			case <-ctx.Done():
				return
			case <-timeout_ctx.Done():
				_, exists := c.torrentDownloaderClient.Torrent(t.InfoHash())
				if exists {
					t.Drop()
				}
				c.logger.Infof("downloader timeout for %s", infohash)
				c.peerTracePrune.In() <- peertrace.PeerTracePrune{
					Source:   "ruminate-downloader-timeout",
					InfoHash: infohash,
					Peers:    netip.AddrPort{},
					Criteria: "infohash",
				}
				return
			case <-t.GotInfo():

				var ap netip.AddrPort
				knownSwarm := t.KnownSwarm()

				if len(knownSwarm) > 0 {
					ip_port, ok := tryIpPortFromNetAddr(knownSwarm[0].Addr)
					ip, err := netip.ParseAddr(ip_port.IP.String())
					if !ok || err != nil {
						ap = c.kTable.GetClosestNodes(infohash)[0].Addr()
					}
					ap = netip.AddrPortFrom(ip, uint16(ip_port.Port))
				} else {
					ap = c.kTable.GetClosestNodes(infohash)[0].Addr()
				}
				nhpfh := nodeHasPeersForHash{
					infoHash: infohash,
					node:     ap,
					source:   "ruminate-downloader",
				}
				if t.Info() != nil {
					ihwmi = infoHashWithMetaInfo{
						nodeHasPeersForHash: nhpfh,
						metaInfo:            *t.Info(),
					}
					select {
					case <-ctx.Done():
						return
					case c.persistTorrents.In() <- ihwmi:
						c.logger.Infof("request metainfo with downloader success for %s", infohash)
						_, exists := c.torrentDownloaderClient.Torrent(t.InfoHash())
						if exists {
							t.Drop()
						}
						return
					}
				}
			}
		}
	})

}

func NewDownloader(params DownloaderParams) DownloaderResult {
	torrentDownloaderClientConfig := torrent.NewDefaultClientConfig()
	torrentDownloaderClientConfig.Debug = params.Config.DownloaderDebug
	torrentDownloaderClientConfig.ListenPort = int(params.Config.DownloaderPort)
	//torrentDownloaderClientConfig.DisableIPv4 = false
	torrentDownloaderClientConfig.DisableIPv6 = true
	//listenAddress := params.Config.DownloaderAddr
	// no ipv6!!!

	//if strings.Contains(params.Config.DownloaderAddr, ":") {
	//	listenAddress = fmt.Sprintf("[%s]", params.Config.DownloaderAddr)
	//	torrentDownloaderClientConfig.DisableIPv6 = false
	//	panic("no ipv6 for downloader !!! ")
	//}
	//torrentDownloaderClientConfig.SetListenAddr(fmt.Sprintf("%s:%d", listenAddress, params.Config.DownloaderPort))
	//torrentDownloaderClientConfig.ListenPort = int(params.Config.DownloaderPort)
	/*torrentDownloaderClientConfig.Callbacks.NewPeer = append(torrentDownloaderClientConfig.Callbacks.NewPeer,
		func(p *torrent.Peer) {
			params.PeerTraceInfoHashWithPeersChan.In() <- peertrace.PeerTraceInfoHashWithPeers{
				InfoHash: protocol.ID(p.Torrent().InfoHash()),
				Peers:    [p.Network],
				Source:   "Downloader",
			}
		},
	)*/
	torrentDownloaderClientConfig.DisableAcceptRateLimiting = true
	torrentDownloaderClientConfig.AlwaysWantConns = true
	torrentDownloaderClient, err := torrent.NewClient(torrentDownloaderClientConfig)
	torrentDownloaderClient.AddDhtNodes(params.Config.BootstrapNodes)
	if err != nil {
		panic(err)
	}
	return DownloaderResult{
		InfohashToDownloaderChannel: concurrency.NewBufferedConcurrentChannel[protocol.ID](int(params.Config.DownloaderConcurrency), int(params.Config.DownloaderConcurrency)),
		TorrentDownloaderClient:     torrentDownloaderClient,
	}
}
