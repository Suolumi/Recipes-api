package mongo

import (
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/net/context"
	"recipes/internal/config"
	"recipes/internal/database"
)

type Client struct {
	client *mongo.Client
	db     *mongo.Database
}

func New(cfg *config.DatabaseConfig) (database.Database, error) {
	ctx, cancelFunc := context.WithTimeout(context.TODO(), cfg.Timeout)
	defer cancelFunc()
	m, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.DefaultAddr))
	if err != nil {
		return nil, err
	}

	return &Client{
		client: m,
		db:     m.Database(cfg.Name),
	}, nil
}
