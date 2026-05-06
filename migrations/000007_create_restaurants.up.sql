CREATE TABLE restaurants (restaurant_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), owner_id UUID NOT NULL REFERENCES users(user_id), name VARCHAR(200) NOT NULL, cuisine_types TEXT[] DEFAULT '{}', location GEOMETRY(POINT,4326) NOT NULL, status VARCHAR(50) NOT NULL DEFAULT 'verification_pending', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE INDEX idx_restaurants_location ON restaurants USING GIST(location);
CREATE INDEX idx_restaurants_cuisine ON restaurants USING GIN(cuisine_types);
