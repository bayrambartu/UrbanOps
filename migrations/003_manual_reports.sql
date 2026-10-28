ALTER TABLE events ADD COLUMN source text NOT NULL DEFAULT 'device' CHECK(source IN ('device','simulator','manual'));
ALTER TABLE events ADD COLUMN reporter_id bigint REFERENCES users(id);
ALTER TABLE events ADD COLUMN location_detail text NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN field_note text NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN request_fingerprint text NOT NULL DEFAULT '';
ALTER TABLE events ADD CONSTRAINT events_reporter_source CHECK ((source='manual')=(reporter_id IS NOT NULL));
CREATE INDEX events_reporter_received ON events(reporter_id,received_at DESC) WHERE source='manual';
CREATE INDEX events_fault_received ON events(fault_id,received_at,event_id);
