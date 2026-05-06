CREATE TABLE audit_logs (audit_id BIGSERIAL PRIMARY KEY, actor_id UUID REFERENCES users(user_id) ON DELETE SET NULL, actor_role user_role, action VARCHAR(40) NOT NULL, entity_type VARCHAR(50) NOT NULL, entity_id VARCHAR(64) NOT NULL, before JSONB, after JSONB, ip_address INET, user_agent TEXT, occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()) PARTITION BY RANGE (occurred_at);
CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;
REVOKE UPDATE, DELETE ON audit_logs FROM PUBLIC;
