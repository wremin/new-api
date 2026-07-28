package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetAssetsRouter 注册 Seedance 2.0 素材库接口。
//
// 路径与上游 seegen.ai 完全一致，客户端只需把 base_url 换成本服务地址。
//
// 路由注册方式说明：/v1/assets 下同时存在静态段（batch、groups）与素材 officialId，
// 若分别注册 /:id 与静态兄弟节点，gin 的路由树可能因通配段与静态段冲突而 panic。
// 因此这里只注册一个 catch-all，由控制器内部分发，与仓库中
// relay-router.go 的 POST /models/*path 是同一种模式。
func SetAssetsRouter(router *gin.Engine) {
	assetsRouter := router.Group("/v1/assets")
	assetsRouter.Use(middleware.RouteTag("relay"))
	// TokenOrUserAuth：API 客户端用 Bearer token，控制台素材库页面用会话，
	// 与 /v1/videos/:task_id/content 的做法一致，避免为前端再开一套 /api/assets 路由。
	assetsRouter.Use(middleware.TokenOrUserAuth())
	assetsRouter.Use(middleware.AssetsRateLimit())
	{
		assetsRouter.POST("", controller.RelayAssetCreate)
		assetsRouter.GET("", controller.RelayAssetList)
		// /batch、/batch/template、/groups、/<officialId>
		assetsRouter.Any("/*action", controller.RelayAssetDispatch)
	}
}
