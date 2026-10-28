CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS users (
 id bigserial PRIMARY KEY, email text NOT NULL UNIQUE,
 name text NOT NULL, password_hash text NOT NULL,
 role text NOT NULL CHECK(role IN ('admin','operator','technician'))
);
CREATE TABLE IF NOT EXISTS devices (
 id text PRIMARY KEY, name text NOT NULL, kind text NOT NULL,
 location text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS faults (
 id bigserial PRIMARY KEY, device_id text NOT NULL REFERENCES devices(id),
 fault_code text NOT NULL, description text NOT NULL,
 severity text NOT NULL CHECK(severity IN ('low','medium','high','critical')),
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved')),
 occurrences integer NOT NULL DEFAULT 1,
 first_seen timestamptz NOT NULL, last_seen timestamptz NOT NULL,
 resolved_at timestamptz, resolution text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS faults_active ON faults(device_id,fault_code) WHERE status='open';
CREATE TABLE IF NOT EXISTS events (
 event_id text PRIMARY KEY, fault_id bigint NOT NULL REFERENCES faults(id),
 payload jsonb NOT NULL, received_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS work_orders (
 id bigserial PRIMARY KEY, fault_id bigint NOT NULL UNIQUE REFERENCES faults(id),
 assignee_id bigint REFERENCES users(id),
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','assigned','in_progress','resolved','closed')),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS audit_log (
 id bigserial PRIMARY KEY, work_order_id bigint NOT NULL REFERENCES work_orders(id),
 actor_id bigint NOT NULL REFERENCES users(id), action text NOT NULL,
 note text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS documents (
 id bigserial PRIMARY KEY, title text NOT NULL, content text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS chunks (
 id bigserial PRIMARY KEY, document_id bigint NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 content text NOT NULL, embedding vector NOT NULL, model text NOT NULL
);
