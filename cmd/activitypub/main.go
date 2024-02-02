//go:generate go run github.com/google/wire/cmd/wire gen .
package main

import (
	"context"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"

	"github.com/bradfitz/gomemcache/memcache"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/totegamma/ccworld-ap-bridge/x/activitypub"
	"github.com/totegamma/concurrent/x/auth"
	"github.com/totegamma/concurrent/x/socket"
	"github.com/totegamma/concurrent/x/util"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"gorm.io/plugin/opentelemetry/tracing"
)

var (
	version      = "unknown"
	buildMachine = "unknown"
	buildTime    = "unknown"
	goVersion    = "unknown"
)

func main() {
	e := echo.New()
	config := util.Config{}
	configPath := os.Getenv("CONCURRENT_CONFIG")
	if configPath == "" {
		configPath = "/etc/concurrent/config.yaml"
	}

	err := config.Load(configPath)
	if err != nil {
		e.Logger.Fatal(err)
	}

	apConf := activitypub.APConfig{}
	apConfPath := os.Getenv("GATEWAY_CONFIG")
	if apConfPath == "" {
		apConfPath = "/etc/concurrent/activitypub.yaml"
	}
	err = apConf.Load(apConfPath)
	if err != nil {
		e.Logger.Fatal(err)
	}

	log.Print("ConcurrentWorld Activitypub Bridge ", version, " starting...")
	log.Print("Config loaded! I am: ", config.Concurrent.CCID)

	log.Print("ApConfig loaded! Proxy: ", apConf.ProxyCCID)

	e.HidePort = true
	e.HideBanner = true

	if config.Server.EnableTrace {
		cleanup, err := setupTraceProvider(config.Server.TraceEndpoint, config.Concurrent.FQDN+"/ccapi", version)
		if err != nil {
			panic(err)
		}
		defer cleanup()

		skipper := otelecho.WithSkipper(
			func(c echo.Context) bool {
				return c.Path() == "/metrics" || c.Path() == "/health"
			},
		)
		e.Use(otelecho.Middleware(config.Concurrent.FQDN, skipper))
	}

	e.Use(echoprometheus.NewMiddleware("ccapi"))
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Binder = &activitypub.Binder{}

	db, err := gorm.Open(postgres.Open(config.Server.Dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	sqlDB, err := db.DB() // for pinging
	if err != nil {
		panic("failed to connect database")
	}
	defer sqlDB.Close()

	err = db.Use(tracing.NewPlugin(
		tracing.WithDBName("postgres"),
	))
	if err != nil {
		panic("failed to setup tracing plugin")
	}

	mc := memcache.New(config.Server.MemcachedAddr)
	log.Println("config.Server.MemcachedAddr", config.Server.MemcachedAddr)
	if err != nil {
		panic("failed to connect memcached")
	}
	defer mc.Close()

	// Migrate the schema
	log.Println("start migrate")
	db.AutoMigrate(
		&activitypub.ApEntity{},
		&activitypub.ApPerson{},
		&activitypub.ApFollow{},
		&activitypub.ApFollower{},
		&activitypub.ApObjectReference{},
	)

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Server.RedisAddr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	err = redisotel.InstrumentTracing(
		rdb,
		redisotel.WithAttributes(
			attribute.KeyValue{
				Key:   "db.name",
				Value: attribute.StringValue("redis"),
			},
		),
	)
	if err != nil {
		panic("failed to setup tracing plugin")
	}

	var emptyManager socket.Manager

	authService := SetupAuthService(db, rdb, config)
	activitypubHandler := SetupActivitypubHandler(
		db,
		rdb,
		mc,
		config,
		apConf,
		emptyManager,
		version,
	)

	e.GET("/.well-known/webfinger", activitypubHandler.WebFinger)
	e.GET("/.well-known/nodeinfo", activitypubHandler.NodeInfoWellKnown)

	ap := e.Group("/ap")
	ap.GET("/nodeinfo/2.0", activitypubHandler.NodeInfo)
	ap.GET("/acct/:id", activitypubHandler.User)
	ap.POST("/acct/:id/inbox", activitypubHandler.Inbox)
	ap.POST("/acct/:id/outbox", activitypubHandler.PrintRequest)
	ap.GET("/note/:id", activitypubHandler.Note)

	ap.GET("/api/entity/:ccid", activitypubHandler.GetEntityID)
	ap.GET("/api/person/:id", activitypubHandler.GetPerson)

	ap.POST("/inbox", activitypubHandler.Inbox)

	// should be restricted
	apR := ap.Group("", auth.ParseJWT)
	apR.POST("/api/entity", activitypubHandler.CreateEntity, authService.Restrict(auth.ISLOCAL))      // ISLOCAL
	apR.PUT("/api/person", activitypubHandler.UpdatePerson, authService.Restrict(auth.ISLOCAL))       // ISLOCAL
	apR.GET("/api/resolve/:id", activitypubHandler.ResolvePerson, authService.Restrict(auth.ISLOCAL)) // ISLOCAL
	apR.POST("/api/follow/:id", activitypubHandler.Follow, authService.Restrict(auth.ISLOCAL))        // ISLOCAL
	apR.DELETE("/api/follow/:id", activitypubHandler.UnFollow, authService.Restrict(auth.ISLOCAL))    // ISLOCAL
	apR.GET("/api/stats", activitypubHandler.GetStats, authService.Restrict(auth.ISLOCAL))            // ISLOCAL
	apR.GET("/api/import", activitypubHandler.ImportNote, authService.Restrict(auth.ISLOCAL))         // ISLOCAL

	e.GET("/health", func(c echo.Context) (err error) {
		ctx := c.Request().Context()

		err = sqlDB.Ping()
		if err != nil {
			return c.String(http.StatusInternalServerError, "db error")
		}

		err = rdb.Ping(ctx).Err()
		if err != nil {
			return c.String(http.StatusInternalServerError, "redis error")
		}

		return c.String(http.StatusOK, "ok")
	})

	e.GET("/metrics", echoprometheus.NewHandler())

	go activitypubHandler.StartMessageWorker()
	go activitypubHandler.StartAssociationWorker(apConf.Proxy.NotificationStream)

	e.Logger.Fatal(e.Start(":8000"))
}

func setupTraceProvider(endpoint string, serviceName string, serviceVersion string) (func(), error) {

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)

	if err != nil {
		return nil, err
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(serviceVersion),
	)

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(tracerProvider)

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)

	cleanup := func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown tracer provider: %v", err)
		}
	}
	return cleanup, nil
}
