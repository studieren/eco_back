package main

// ubuntu 后台执行的方法 nohup ./eco_back > eco_back.log 2>&1 &
import (
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/studieren/eco_back/gormtool"
	"github.com/studieren/eco_back/models"
	"gorm.io/driver/sqlite"

	// "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	db *gorm.DB
	// rdb    *redis.Client
	cruder *gormtool.CRUDTool
)

// 初始化 DB、Redis、CRUDTool
func init() {
	var err error
	// dsn := "host=localhost user=postgres password=123456 dbname=gindemo port=5432 sslmode=disable"
	// db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	db, err = gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	// 自动迁移
	db.AutoMigrate(&models.User{}, &models.Profile{}, &models.Tag{})

	// rdb = redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	// cruder = gormtool.NewCRUDTool(db, rdb, nil) // 使用默认 logger, 使用 Redis
	cruder = gormtool.NewCRUDTool(db, nil, nil) // 不使用 Redis
}

func main() {
	r := gin.Default()
	r.Use(cors.Default())
	// 1) 事务级联创建：User + Profile + Tags
	r.POST("/users", createUserWithEverything)

	// 2) 查询所有
	r.GET("/users", getAllUsers)
	r.GET("/users/:id", getUserByID)

	// 3) 更新 User + 同步更新关联 Tags（事务 + 缓存失效）
	r.PUT("/users/:id", updateUserWithTags)

	// 4) 软删除（级联 tags 不会删除，仅 user）
	r.DELETE("/users/:id", softDeleteUser)

	// 5) 恢复软删除
	r.PUT("/users/:id/restore", restoreUser)

	// 6) 批量硬删除（危险操作演示）
	r.DELETE("/users/batch/hard", batchHardDelete)

	// 7) 指标监控
	r.GET("/metrics", cruder.GetMetrics)

	r.Run(":1234")
}

/*
	------------------------------------------------
	  1. 事务级联创建

------------------------------------------------
*/
func createUserWithEverything(c *gin.Context) {
	type payload struct {
		models.User
		Profile models.Profile `json:"profile"`
		Tags    []models.Tag   `json:"tags"`
	}
	var req payload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}

	err := cruder.WithTransaction(c.Request.Context(), func(tx *gorm.DB) error {
		// 1. 创建 user
		if err := tx.Create(&req.User).Error; err != nil {
			return err
		}
		// 2. 创建 profile
		req.Profile.UserID = req.User.ID
		if err := tx.Create(&req.Profile).Error; err != nil {
			return err
		}
		// 3. 创建/附加 tags
		return tx.Model(&req.User).Association("Tags").Append(req.Tags)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"msg": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": req.User})
}

/*
	------------------------------------------------
	  2. 复杂条件 + 分页 + 预加载

------------------------------------------------
*/
// func queryUsers(c *gin.Context) {
// 	qb := &gormtool.QueryBuilder{
// 		Conditions: []gormtool.QueryCondition{
// 			// {Field: "age", Operator: ">=", Value: 18},
// 			{Field: "name", Operator: "LIKE", Value: c.Query("name")},
// 		},
// 		Sorts: []gormtool.SortCondition{
// 			{Field: "created_at", Direction: "DESC"},
// 		},
// 		Preloads: []string{"Profile", "Tags"},
// 	}
// 	var users []models.User
// 	_ = cruder.GetByQueryBuilder(c, &users, qb) // 出错已在内部返回
// }

func getAllUsers(c *gin.Context) {
	var users []models.User
	cruder.GetByQueryBuilder(c, &users, &gormtool.QueryBuilder{})
}

func getUserByID(c *gin.Context) {
	var user models.User

	// 调用 CRUD 方法（注意：GetByID 内部已处理 HTTP 响应）
	err := cruder.GetByID(c, &user)

	// 记录错误（便于排查问题，即使响应已发送）
	if err != nil {
		// 可以使用日志库（如 zap、logrus）记录错误详情
		log.Printf("获取用户失败: %v, ID: %s", err, c.Param("id"))
		// c.JSON(http.StatusNotFound, gin.H{"msg": err.Error()})
		// 无需再次发送响应，因为 GetByID 已处理
		return
	}

	// 若需要在成功后添加额外逻辑（如统计、权限二次校验等）
	// 注意：此时响应已由 GetByID 发送，不能再调用 c.JSON
}

/*
	------------------------------------------------
	  3. 更新（事务 + 关联覆盖 + 缓存失效）

------------------------------------------------
*/
func updateUserWithTags(c *gin.Context) {
	var user models.User
	// 先查出来（软删除除外）
	if err := cruder.DB.First(&user, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"msg": err.Error()})
		return
	}
	// 绑定 JSON
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}
	// 事务更新
	err := cruder.WithTransaction(c.Request.Context(), func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		// 前端把完整的 tags 传过来 -> 直接 Replace
		return tx.Model(&user).Association("Tags").Replace(user.Tags)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"msg": err.Error()})
		return
	}
	// 清除缓存
	cruder.DeleteFromCache(c.Request.Context(), cruder.GenerateCacheKey(&models.User{}, user.ID))
	c.JSON(http.StatusOK, gin.H{"data": user})
}

/*
	------------------------------------------------
	  4. 软删除（自动触发缓存失效）

------------------------------------------------
*/
func softDeleteUser(c *gin.Context) {
	var user models.User
	_ = cruder.SoftDeleteByID(c, &user)
}

/*
	------------------------------------------------
	  5. 恢复软删除

------------------------------------------------
*/
func restoreUser(c *gin.Context) {
	var user models.User
	_ = cruder.RestoreSoftDelete(c, &user)
}

/*
	------------------------------------------------
	  6. 批量硬删除（事务）

------------------------------------------------
*/
func batchHardDelete(c *gin.Context) {
	type IDs struct {
		IDs []uint `json:"ids"`
	}
	var ids IDs
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}
	err := cruder.WithTransaction(c.Request.Context(), func(tx *gorm.DB) error {
		return tx.Unscoped().Delete(&models.User{}, ids.IDs).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"affected": len(ids.IDs)})
}
