// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.

package model

import (
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
)

const TableNamePeerTrace = "peer_trace"

// PeerTrace mapped from table <peer_trace>
type PeerTrace struct {
	IP           string      `gorm:"column:ip;primaryKey;<-:create" json:"ip"`
	InfoHash     protocol.ID `gorm:"column:info_hash;primaryKey" json:"infoHash"`
	LastSeenTime time.Time   `gorm:"column:last_seen_time;default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC;<-:create" json:"lastSeenTime"`
}

// TableName PeerTrace's table name
func (*PeerTrace) TableName() string {
	return TableNamePeerTrace
}
