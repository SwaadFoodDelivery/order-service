CREATE TABLE users (user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), phone VARCHAR(15) NOT NULL, role user_role NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE otp_requests (otp_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), phone VARCHAR(15) NOT NULL, otp_hash VARCHAR(255) NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE sessions (session_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), user_id UUID NOT NULL REFERENCES users(user_id), is_active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
