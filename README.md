# bitmagnet

A self-hosted BitTorrent indexer, DHT crawler, content classifier and torrent search engine with web UI, GraphQL API and Servarr stack integration.

Visit the website at [bitmagnet.io](https://bitmagnet.io).


To compile : go build -ldflags "-s -w -X github.com/2512500960/bitmagnet/internal/version.GitTag=$(git describe --tags --always --dirty)"