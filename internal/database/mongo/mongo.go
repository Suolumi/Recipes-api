package mongo

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"recipes/internal/config"
	"recipes/internal/database"
)

type Client struct {
	client *mongo.Client
	db     *mongo.Database
}

func New(cfg *config.DatabaseConfig) (database.Database, error) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancelFunc()
	m, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.DefaultAddr))
	if err != nil {
		return nil, err
	}

	client := &Client{
		client: m,
		db:     m.Database(cfg.Name),
	}
	if err := client.db.Client().Ping(ctx, nil); err != nil {
		_ = m.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

func (c *Client) RawDatabase() *mongo.Database { return c.db }

func (c *Client) Close(ctx context.Context) error { return c.client.Disconnect(ctx) }
