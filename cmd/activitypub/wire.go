//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/totegamma/ccworld-ap-bridge/x/activitypub"
	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/entity"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/stream"
)

func SetupActivitypubHandler(db *gorm.DB, rdb *redis.Client, config activitypub.APConfig) *activitypub.Handler {
	wire.Build(activitypub.NewHandler, activitypub.NewRepository, message.NewService, message.NewRepository, association.NewService, association.NewRepository, entity.NewService, entity.NewRepository, stream.NewService, stream.NewRepository)
	return &activitypub.Handler{}
}
