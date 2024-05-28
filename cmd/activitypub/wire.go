//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/totegamma/ccworld-ap-bridge/x/activitypub"

	"github.com/totegamma/concurrent/x/agent"
	"github.com/totegamma/concurrent/x/association"
	"github.com/totegamma/concurrent/x/auth"
	"github.com/totegamma/concurrent/x/character"
	"github.com/totegamma/concurrent/x/domain"
	"github.com/totegamma/concurrent/x/entity"
	"github.com/totegamma/concurrent/x/jwt"
	"github.com/totegamma/concurrent/x/key"
	"github.com/totegamma/concurrent/x/message"
	"github.com/totegamma/concurrent/x/socket"
	"github.com/totegamma/concurrent/x/stream"
	"github.com/totegamma/concurrent/x/userkv"
	"github.com/totegamma/concurrent/x/util"
)

var jwtServiceProvider = wire.NewSet(jwt.NewService, jwt.NewRepository)
var domainServiceProvider = wire.NewSet(domain.NewService, domain.NewRepository)
var entityServiceProvider = wire.NewSet(entity.NewService, entity.NewRepository, SetupJwtService)
var streamServiceProvider = wire.NewSet(stream.NewService, stream.NewRepository, SetupEntityService, SetupDomainService)
var associationServiceProvider = wire.NewSet(association.NewService, association.NewRepository, SetupStreamService, SetupMessageService, SetupKeyService)
var characterServiceProvider = wire.NewSet(character.NewService, character.NewRepository, SetupKeyService)
var authServiceProvider = wire.NewSet(auth.NewService, SetupEntityService, SetupDomainService, SetupKeyService)
var messageServiceProvider = wire.NewSet(message.NewService, message.NewRepository, SetupStreamService, SetupKeyService)
var keyServiceProvider = wire.NewSet(key.NewService, key.NewRepository, SetupEntityService)
var userKvServiceProvider = wire.NewSet(userkv.NewService, userkv.NewRepository)

func SetupActivitypubHandler(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config, apConfig activitypub.APConfig, manager socket.Manager, version string) *activitypub.Handler {
	wire.Build(
		activitypub.NewHandler,
		activitypub.NewRepository,
		SetupMessageService,
		SetupAssociationService,
		SetupEntityService,
	)
	return &activitypub.Handler{}
}

func SetupJwtService(rdb *redis.Client) jwt.Service {
	wire.Build(jwtServiceProvider)
	return nil
}

func SetupKeyService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config) key.Service {
	wire.Build(keyServiceProvider)
	return nil
}

func SetupMessageService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, manager socket.Manager, config util.Config) message.Service {
	wire.Build(messageServiceProvider)
	return nil
}

func SetupCharacterService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config) character.Service {
	wire.Build(characterServiceProvider)
	return nil
}

func SetupAssociationService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, manager socket.Manager, config util.Config) association.Service {
	wire.Build(associationServiceProvider)
	return nil
}

func SetupStreamService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, manager socket.Manager, config util.Config) stream.Service {
	wire.Build(streamServiceProvider)
	return nil
}

func SetupDomainService(db *gorm.DB, config util.Config) domain.Service {
	wire.Build(domainServiceProvider)
	return nil
}

func SetupEntityService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config) entity.Service {
	wire.Build(entityServiceProvider)
	return nil
}

func SetupSocketHandler(rdb *redis.Client, manager socket.Manager, config util.Config) socket.Handler {
	wire.Build(socket.NewHandler, socket.NewService)
	return nil
}

func SetupAgent(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config) agent.Agent {
	wire.Build(agent.NewAgent, SetupEntityService, SetupDomainService)
	return nil
}

func SetupAuthService(db *gorm.DB, rdb *redis.Client, mc *memcache.Client, config util.Config) auth.Service {
	wire.Build(authServiceProvider)
	return nil
}

func SetupUserkvService(rdb *redis.Client) userkv.Service {
	wire.Build(userKvServiceProvider)
	return nil
}

func SetupSocketManager(mc *memcache.Client, db *gorm.DB, rdb *redis.Client, config util.Config) socket.Manager {
	wire.Build(socket.NewManager)
	return nil
}
