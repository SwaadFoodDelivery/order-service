package repository

import "github.com/redis/go-redis/v9"

type RedisRepository struct{ rc *redis.Client }

func NewRedisRepository(rc *redis.Client) *RedisRepository { return &RedisRepository{rc: rc} }
