package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	seedanceOfficialRouter := router.Group(relayconstant.SeedanceOfficialTasksPath)
	seedanceOfficialRouter.Use(middleware.RouteTag("relay"))
	seedanceOfficialRouter.Use(middleware.TokenAuth())
	{
		seedanceOfficialRouter.POST("", middleware.Distribute(), middleware.BindVirtualCharacter(), controller.RelayTask)
		seedanceOfficialRouter.GET("", controller.ListSeedanceOfficialTasks)
		seedanceOfficialRouter.GET("/:task_id", middleware.Distribute(), controller.RelayTaskFetch)
		seedanceOfficialRouter.DELETE("/:task_id", controller.DeleteSeedanceOfficialTask)
	}

	// API-key virtual character lifecycle endpoints. The dashboard keeps using
	// /api/virtual-characters with TokenOrUserAuth, while external clients get a
	// conventional /v1 entry point backed by the exact same business handlers.
	virtualCharacterV1Router := router.Group("/v1/virtual-characters")
	virtualCharacterV1Router.Use(middleware.RouteTag("relay"))
	virtualCharacterV1Router.Use(middleware.SystemPerformanceCheck())
	virtualCharacterV1Router.Use(middleware.TokenAuth())
	{
		virtualCharacterV1Router.GET("", controller.ListVirtualCharacterGroups)
		virtualCharacterV1Router.POST("", middleware.UserUploadRateLimit(), controller.CreateVirtualCharacter)
		virtualCharacterV1Router.POST("/validation-sessions", controller.CreateVirtualCharacterValidationSession)
		virtualCharacterV1Router.GET("/validation-sessions/:id", controller.GetVirtualCharacterValidationSession)
		virtualCharacterV1Router.DELETE("/validation-sessions/:id", controller.CancelVirtualCharacterValidationSession)
		virtualCharacterV1Router.POST("/:id/asset", middleware.UserUploadRateLimit(), controller.UploadRealPersonVirtualCharacterAsset)
		virtualCharacterV1Router.POST("/:id/sync", controller.SyncRealPersonVirtualCharacter)
		virtualCharacterV1Router.DELETE("/:id", controller.DeleteVirtualCharacterGroup)
		virtualCharacterV1Router.GET("/:id", controller.GetVirtualCharacterGroup)
	}

	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/videos/:video_id/remix", middleware.BindVirtualCharacter(), controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", middleware.BindVirtualCharacter(), controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}
	videoDeleteRouter := router.Group("/v1")
	videoDeleteRouter.Use(middleware.RouteTag("relay"))
	videoDeleteRouter.Use(middleware.TokenAuth())
	{
		videoDeleteRouter.DELETE("/videos/:task_id", controller.DeleteSeedanceOpenAIVideo)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", middleware.BindVirtualCharacter(), controller.RelayTask)
		klingV1Router.POST("/videos/image2video", middleware.BindVirtualCharacter(), controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", middleware.BindVirtualCharacter(), controller.RelayTask)
	}
}
