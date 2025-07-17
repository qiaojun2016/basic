package queue

import (
	"context"
	"github.com/go-redis/redis/v9"
)

type StreamMQ struct {
	client *redis.Client
}

func NewStreamMQ() *StreamMQ {
	return &StreamMQ{}
}

func (q *StreamMQ) SendMsg(ctx context.Context, string, msg *Msg) error {
	return nil
}
