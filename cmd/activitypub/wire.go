//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/totegamma/ccworld-ap-bridge/x/activitypub"
	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/auth"
	"github.com/totegamma/concurrent/x/domain"
	"github.com/totegamma/concurrent/x/entity"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/stream"
	"github.com/totegamma/concurrent/x/util"
	"github.com/bradfitz/gomemcache/memcache"
)

func SetupAuthService(db *gorm.DB, config util.Config) auth.Service {
	wire.Build(auth.NewService, entity.NewService, entity.NewRepository, domain.NewService, domain.NewRepository)
	return nil
}

func SetupActivitypubHandler(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config, apConfig activitypub.APConfig) *activitypub.Handler {
	wire.Build(
		activitypub.NewHandler,
		activitypub.NewRepository,
		message.NewService,
		message.NewRepository,
		entity.NewService,
		entity.NewRepository,
		association.NewService,
		association.NewRepository,
		stream.NewService,
		stream.NewRepository,
	)
	return &activitypub.Handler{}
}
