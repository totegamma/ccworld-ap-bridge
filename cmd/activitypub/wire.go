//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/totegamma/ccworld-ap-bridge/x/activitypub"
	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/auth"
	"github.com/totegamma/concurrent/x/domain"
	"github.com/totegamma/concurrent/x/entity"
	"github.com/totegamma/concurrent/x/jwt"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/socket"
	"github.com/totegamma/concurrent/x/stream"
	"github.com/totegamma/concurrent/x/util"
)

var jwtServiceProvider = wire.NewSet(jwt.NewService, jwt.NewRepository)
var entityServiceProvider = wire.NewSet(entity.NewService, entity.NewRepository, jwtServiceProvider)
var streamServiceProvider = wire.NewSet(stream.NewService, stream.NewRepository, entityServiceProvider)
var messageServiceProvider = wire.NewSet(message.NewService, message.NewRepository, streamServiceProvider)
var associationServiceProvider = wire.NewSet(association.NewService, association.NewRepository, messageServiceProvider)

func SetupAuthService(db *gorm.DB, rdb *redis.Client, config util.Config) auth.Service {
	wire.Build(jwtServiceProvider, auth.NewService, auth.NewRepository, entity.NewService, entity.NewRepository, domain.NewService, domain.NewRepository)
	return nil
}

func SetupActivitypubHandler(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config, apConfig activitypub.APConfig, manager socket.Manager, version string) *activitypub.Handler {
	wire.Build(
		activitypub.NewHandler,
		activitypub.NewRepository,
		associationServiceProvider,
	)
	return &activitypub.Handler{}
}
