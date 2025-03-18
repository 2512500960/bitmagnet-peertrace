-- +goose Up
-- +goose StatementBegin

CREATE TABLE peer_trace 
(
  "ip" inet NOT NULL,
  "info_hash" bytea NOT NULL,
  "last_seen_time" timestamptz(6) DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'UTC'::text)
)
;
ALTER TABLE peer_trace ADD CONSTRAINT "peer_trace_pkey" PRIMARY KEY ("ip", "info_hash");
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

drop table peer_trace;
-- +goose StatementEnd