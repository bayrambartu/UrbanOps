ALTER TABLE work_orders DROP CONSTRAINT work_orders_status_check;
ALTER TABLE work_orders ADD CONSTRAINT work_orders_status_check CHECK(status IN ('open','assigned','in_progress','on_hold','resolved','closed'));
ALTER TABLE work_orders ADD COLUMN hold_reason text NOT NULL DEFAULT '';
ALTER TABLE work_orders ADD COLUMN reassignment_requested boolean NOT NULL DEFAULT false;
ALTER TABLE work_orders ADD COLUMN reassignment_reason text NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN previous_assignee_id bigint REFERENCES users(id);
ALTER TABLE audit_log ADD COLUMN next_assignee_id bigint REFERENCES users(id);
CREATE INDEX work_orders_assignee_status ON work_orders(assignee_id,status);
