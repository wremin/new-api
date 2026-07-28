package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSetAssetsRouterNoPanic 验证素材路由的注册方式不会触发 gin 路由树冲突。
//
// 这是 PRD M0 的第一项验证：/v1/assets 下同时需要静态段（batch、groups）
// 与素材 officialId，若分别注册 /:id 与静态兄弟节点，gin 可能 panic。
// 当前实现改用单一 catch-all，本测试锁定这一行为，防止后续有人改回分别注册。
func TestSetAssetsRouterNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetAssetsRouter panicked during route registration: %v", r)
		}
	}()

	engine := gin.New()
	SetAssetsRouter(engine)

	routes := engine.Routes()
	if len(routes) == 0 {
		t.Fatal("expected assets routes to be registered, got none")
	}

	// 至少要有 POST /v1/assets、GET /v1/assets 与 catch-all
	want := map[string]bool{
		"POST /v1/assets":         false,
		"GET /v1/assets":          false,
		"GET /v1/assets/*action":  false,
		"POST /v1/assets/*action": false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("expected route %q to be registered", key)
		}
	}
}

// TestAssetsRouterCoexistsWithVideoRouter 验证素材路由与既有 /v1 路由共存时不冲突。
// 两者都挂在 /v1 下，注册顺序变化不应导致 panic。
func TestAssetsRouterCoexistsWithVideoRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering assets router alongside video router panicked: %v", r)
		}
	}()

	engine := gin.New()
	SetVideoRouter(engine)
	SetAssetsRouter(engine)
}
