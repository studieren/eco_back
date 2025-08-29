// gormtool\crud.go
package gormtool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 常量定义
const (
	CacheTTL = 5 * time.Minute
)

// 扩展的结构定义
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Page    *Pagination `json:"page,omitempty"`
}

// QueryCondition 查询条件结构
type QueryCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // =, !=, >, <, >=, <=, LIKE, IN, NOT IN, BETWEEN
	Value    interface{} `json:"value"`
}

// SortCondition 排序条件
type SortCondition struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // ASC, DESC
}

// QueryBuilder 查询构建器
type QueryBuilder struct {
	Conditions []QueryCondition `json:"conditions"`
	Sorts      []SortCondition  `json:"sorts"`
	Preloads   []string         `json:"preloads"`
}

// CRUDTool 扩展的 CRUD 工具
type CRUDTool struct {
	DB          *gorm.DB
	RedisClient *redis.Client
	Logger      Logger
	EnableLog   bool
}

// DatabaseStats 数据库统计信息结构体
type DatabaseStats struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration"`
	MaxIdleClosed      int64         `json:"max_idle_closed"`
	MaxLifetimeClosed  int64         `json:"max_lifetime_closed"`
}

// NewCRUDTool 创建新的 CRUD 工具
func NewCRUDTool(db *gorm.DB, redisClient *redis.Client, logger Logger) *CRUDTool {
	if logger == nil {
		logger = NewDefaultLogger()
	}

	return &CRUDTool{
		DB:          db,
		RedisClient: redisClient,
		Logger:      logger,
		EnableLog:   true,
	}
}

// 添加日志 辅助方法
// LogOperation 记录操作日志
// ctx: 请求上下文
// operation: 操作名称
// model: 操作的模型
// duration: 操作耗时
// err: 操作错误
// additionalFields: 额外字段
// 日志记录示例
//
//	t.LogOperation(c.Request.Context(), "get_by_id", &User{}, time.Since(start), err, map[string]interface{}{
//		"user_id": c.Param("id"),
//	})
func (t *CRUDTool) LogOperation(ctx context.Context, operation string, model interface{}, duration time.Duration, err error, additionalFields map[string]interface{}) {
	if !t.EnableLog {
		return
	}

	fields := map[string]interface{}{
		"operation": operation,
		"duration":  duration.String(),
		"model":     fmt.Sprintf("%T", model),
	}

	if err != nil {
		fields["error"] = err.Error()
	}

	for k, v := range additionalFields {
		fields[k] = v
	}

	if err != nil {
		t.Logger.Error(ctx, "操作失败", fields)
	} else {
		t.Logger.Info(ctx, "操作成功", fields)
	}
}

// 事务相关方法
type TxFunc func(tx *gorm.DB) error

// WithTransaction 执行事务
func (t *CRUDTool) WithTransaction(ctx context.Context, fn TxFunc) error {
	return t.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// Transaction 事务包装器
func (t *CRUDTool) Transaction(c *gin.Context, fn TxFunc) {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "transaction", nil, time.Since(start), err, nil)
	}()

	err = t.WithTransaction(c.Request.Context(), fn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "事务执行失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "操作成功",
	})
}

// GetByIDWithRelations 根据ID查询单条记录（支持预加载关系）
func (t *CRUDTool) GetByIDWithRelations(c *gin.Context, model interface{}, relations []string) error {
	start := time.Now()
	var err error

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		t.LogOperation(c.Request.Context(), "get_by_id", model, time.Since(start), err, map[string]interface{}{
			"error_type": "invalid_id",
			"id":         c.Param("id"),
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	// 构建查询
	db := t.DB
	for _, relation := range relations {
		db = db.Preload(relation)
	}

	if err := db.First(model, id).Error; err != nil {
		t.LogOperation(c.Request.Context(), "get_by_id", model, time.Since(start), err, map[string]interface{}{
			"id": id,
		})
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	t.LogOperation(c.Request.Context(), "get_by_id", model, time.Since(start), nil, map[string]interface{}{
		"id":        id,
		"relations": relations,
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "查询成功",
		Data:    model,
	})
	return nil
}

// CreateWithRelations 创建记录（支持关联创建）
func (t *CRUDTool) CreateWithRelations(c *gin.Context, model interface{}, relations []string) error {
	start := time.Now()
	var err error

	if err := c.ShouldBindJSON(model); err != nil {
		t.LogOperation(c.Request.Context(), "create", model, time.Since(start), err,
			map[string]interface{}{"error_type": "bind_error"})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	err = t.WithTransaction(c.Request.Context(), func(tx *gorm.DB) error {
		// 先创建主记录
		if err := tx.Create(model).Error; err != nil {
			return err
		}

		// 逐条追加关联
		for _, rel := range relations {
			field := reflect.Indirect(reflect.ValueOf(model)).FieldByName(rel)
			if !field.IsValid() {
				return fmt.Errorf("invalid relation field: %s", rel)
			}
			assoc := tx.Model(model).Association(rel)
			if assoc == nil {
				return fmt.Errorf("association %s not found", rel)
			}
			if err := assoc.Replace(field.Interface()); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.LogOperation(c.Request.Context(), "create", model, time.Since(start), err,
			map[string]interface{}{"relations": relations})
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建失败",
		})
		return err
	}

	t.LogOperation(c.Request.Context(), "create", model, time.Since(start), nil,
		map[string]interface{}{"relations": relations})

	c.JSON(http.StatusCreated, Response{
		Code:    http.StatusCreated,
		Message: "创建成功",
		Data:    model,
	})
	return nil
}

// UpdateWithRelations 更新记录（支持关联更新）
func (t *CRUDTool) UpdateWithRelations(c *gin.Context, model interface{}, relations []string) error {
	start := time.Now()
	var err error

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		t.LogOperation(c.Request.Context(), "update", model, time.Since(start), err, map[string]interface{}{
			"error_type": "invalid_id",
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	// 先检查记录是否存在
	if err := t.DB.First(model, id).Error; err != nil {
		t.LogOperation(c.Request.Context(), "update", model, time.Since(start), err, map[string]interface{}{
			"id": id,
		})
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	if err := c.ShouldBindJSON(model); err != nil {
		t.LogOperation(c.Request.Context(), "update", model, time.Since(start), err, map[string]interface{}{
			"error_type": "bind_error",
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	err = t.WithTransaction(c.Request.Context(), func(tx *gorm.DB) error {
		if err := tx.Save(model).Error; err != nil {
			return err
		}

		for _, rel := range relations {
			// 通过反射拿到对应字段的值
			field := reflect.Indirect(reflect.ValueOf(model)).FieldByName(rel)
			if !field.IsValid() {
				return fmt.Errorf("invalid relation field: %s", rel)
			}
			if err := tx.Model(model).Association(rel).Replace(field.Interface()); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		t.LogOperation(c.Request.Context(), "update", model, time.Since(start), err, map[string]interface{}{
			"id":        id,
			"relations": relations,
		})
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新失败",
		})
		return err
	}

	// 清除缓存
	cacheKey := t.GenerateCacheKey(model, id)
	t.DeleteFromCache(c.Request.Context(), cacheKey)

	t.LogOperation(c.Request.Context(), "update", model, time.Since(start), nil, map[string]interface{}{
		"id":        id,
		"relations": relations,
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "更新成功",
		Data:    model,
	})
	return nil
}

// GetRelated 获取关联记录
func (t *CRUDTool) GetRelated(c *gin.Context, model interface{}, associationName string, result interface{}) error {
	start := time.Now()
	var err error

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		t.LogOperation(c.Request.Context(), "get_related", model, time.Since(start), err, map[string]interface{}{
			"error_type": "invalid_id",
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	// 先获取主记录
	if err := t.DB.First(model, id).Error; err != nil {
		t.LogOperation(c.Request.Context(), "get_related", model, time.Since(start), err, map[string]interface{}{
			"id": id,
		})
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	// 获取关联记录
	if err := t.DB.Model(model).Association(associationName).Find(result); err != nil {
		t.LogOperation(c.Request.Context(), "get_related", model, time.Since(start), err, map[string]interface{}{
			"id":          id,
			"association": associationName,
		})
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取关联记录失败",
		})
		return err
	}

	t.LogOperation(c.Request.Context(), "get_related", model, time.Since(start), nil, map[string]interface{}{
		"id":          id,
		"association": associationName,
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "获取关联记录成功",
		Data:    result,
	})
	return nil
}

// AddRelation 添加关联关系
func (t *CRUDTool) AddRelation(c *gin.Context, model interface{}, associationName string, relatedModel interface{}) error {
	start := time.Now()
	var err error

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		t.LogOperation(c.Request.Context(), "add_relation", model, time.Since(start), err, map[string]interface{}{
			"error_type": "invalid_id",
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	if err := c.ShouldBindJSON(relatedModel); err != nil {
		t.LogOperation(c.Request.Context(), "add_relation", model, time.Since(start), err, map[string]interface{}{
			"error_type": "bind_error",
		})
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	// 先获取主记录
	if err := t.DB.First(model, id).Error; err != nil {
		t.LogOperation(c.Request.Context(), "add_relation", model, time.Since(start), err, map[string]interface{}{
			"id": id,
		})
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	// 添加关联
	if err := t.DB.Model(model).Association(associationName).Append(relatedModel); err != nil {
		t.LogOperation(c.Request.Context(), "add_relation", model, time.Since(start), err, map[string]interface{}{
			"id":          id,
			"association": associationName,
		})
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "添加关联失败",
		})
		return err
	}

	t.LogOperation(c.Request.Context(), "add_relation", model, time.Since(start), nil, map[string]interface{}{
		"id":          id,
		"association": associationName,
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "添加关联成功",
	})
	return nil
}

// 缓存相关方法
func (t *CRUDTool) GenerateCacheKey(model interface{}, id interface{}) string {
	return fmt.Sprintf("%T:%v", model, id)
}

func (t *CRUDTool) GetFromCache(ctx context.Context, key string, result interface{}) bool {
	if t.RedisClient == nil {
		return false
	}

	data, err := t.RedisClient.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if err := json.Unmarshal([]byte(data), result); err != nil {
		return false
	}

	return true
}

func (t *CRUDTool) SetToCache(ctx context.Context, key string, data interface{}) error {
	if t.RedisClient == nil {
		return nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return t.RedisClient.Set(ctx, key, jsonData, CacheTTL).Err()
}

func (t *CRUDTool) DeleteFromCache(ctx context.Context, key string) error {
	if t.RedisClient == nil {
		return nil
	}

	return t.RedisClient.Del(ctx, key).Err()
}

// 查询构建器方法
func (t *CRUDTool) BuildQuery(db *gorm.DB, qb *QueryBuilder) *gorm.DB {
	if qb == nil {
		return db
	}

	// 构建条件
	for _, cond := range qb.Conditions {
		switch cond.Operator {
		case "=", "!=", ">", "<", ">=", "<=":
			db = db.Where(fmt.Sprintf("%s %s ?", cond.Field, cond.Operator), cond.Value)
		case "LIKE":
			db = db.Where(fmt.Sprintf("%s LIKE ?", cond.Field), "%"+cond.Value.(string)+"%")
		case "IN":
			db = db.Where(fmt.Sprintf("%s IN (?)", cond.Field), cond.Value)
		case "NOT IN":
			db = db.Where(fmt.Sprintf("%s NOT IN (?)", cond.Field), cond.Value)
		case "BETWEEN":
			if values, ok := cond.Value.([]interface{}); ok && len(values) == 2 {
				db = db.Where(fmt.Sprintf("%s BETWEEN ? AND ?", cond.Field), values[0], values[1])
			}
		}
	}

	// 构建排序
	for _, sort := range qb.Sorts {
		db = db.Order(fmt.Sprintf("%s %s", sort.Field, sort.Direction))
	}

	// 构建预加载
	for _, preload := range qb.Preloads {
		db = db.Preload(preload)
	}

	return db
}

// 核心 CRUD 方法（带缓存和监控）
func (t *CRUDTool) GetByID(c *gin.Context, model interface{}, preloads ...string) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "get_by_id", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	// 尝试从缓存获取
	cacheKey := t.GenerateCacheKey(model, id)
	if t.GetFromCache(c.Request.Context(), cacheKey, model) {
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "查询成功（缓存）",
			Data:    model,
		})
		return nil
	}

	db := t.DB
	for _, preload := range preloads {
		db = db.Preload(preload)
	}

	if err := db.First(model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	// 设置缓存
	t.SetToCache(c.Request.Context(), cacheKey, model)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "查询成功",
		Data:    model,
	})
	return nil
}

// GetByIDWithSoftDelete 支持软删除的查询
func (t *CRUDTool) GetByIDWithSoftDelete(c *gin.Context, model interface{}, preloads ...string) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "get_by_id_soft_delete", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	db := t.DB.Unscoped() // 包含已删除的记录
	for _, preload := range preloads {
		db = db.Preload(preload)
	}

	if err := db.First(model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "查询成功",
		Data:    model,
	})
	return nil
}

// GetByQueryBuilder 使用查询构建器（支持分页）
func (t *CRUDTool) GetByQueryBuilder(c *gin.Context, models interface{}, qb *QueryBuilder) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "get_by_query_builder", models, time.Since(start), err, nil)
	}()

	// 分页参数处理
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pagesize", "10")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	db := t.BuildQuery(t.DB, qb)

	// 获取总数
	var total int64
	if err := db.Model(models).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "查询失败",
		})
		return err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := db.Limit(pageSize).Offset(offset).Find(models).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "查询失败",
		})
		return err
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "查询成功",
		Data:    models,
		Page: &Pagination{
			Page:     page,
			PageSize: pageSize,
			Total:    int(total),
		},
	})
	return nil
}

// Create 创建记录（带缓存失效）
func (t *CRUDTool) Create(c *gin.Context, model interface{}) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "create", model, time.Since(start), err, nil)
	}()

	if err := c.ShouldBindJSON(model); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	if err := t.DB.Create(model).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建失败",
		})
		return err
	}

	c.JSON(http.StatusCreated, Response{
		Code:    http.StatusCreated,
		Message: "创建成功",
		Data:    model,
	})
	return nil
}

// UpdateByID 更新记录（带缓存失效）
func (t *CRUDTool) UpdateByID(c *gin.Context, model interface{}) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "update_by_id", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	// 先检查记录是否存在
	if err := t.DB.First(model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "记录不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "查询失败",
			})
		}
		return err
	}

	if err := c.ShouldBindJSON(model); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	if err := t.DB.Save(model).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新失败",
		})
		return err
	}

	// 清除缓存
	cacheKey := t.GenerateCacheKey(model, id)
	t.DeleteFromCache(c.Request.Context(), cacheKey)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "更新成功",
		Data:    model,
	})
	return nil
}

// SoftDeleteByID 软删除
func (t *CRUDTool) SoftDeleteByID(c *gin.Context, model interface{}) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "soft_delete", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	result := t.DB.Delete(model, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除失败",
		})
		return result.Error
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "记录不存在",
		})
		return gorm.ErrRecordNotFound
	}

	// 清除缓存
	cacheKey := t.GenerateCacheKey(model, id)
	t.DeleteFromCache(c.Request.Context(), cacheKey)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "删除成功",
	})
	return nil
}

// HardDeleteByID 硬删除
func (t *CRUDTool) HardDeleteByID(c *gin.Context, model interface{}) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "hard_delete", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	result := t.DB.Unscoped().Delete(model, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除失败",
		})
		return result.Error
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "记录不存在",
		})
		return gorm.ErrRecordNotFound
	}

	// 清除缓存
	cacheKey := t.GenerateCacheKey(model, id)
	t.DeleteFromCache(c.Request.Context(), cacheKey)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "永久删除成功",
	})
	return nil
}

// RestoreSoftDelete 恢复软删除的记录
func (t *CRUDTool) RestoreSoftDelete(c *gin.Context, model interface{}) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "restore", model, time.Since(start), err, nil)
	}()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的ID",
		})
		return err
	}

	result := t.DB.Unscoped().Model(model).Where("id = ?", id).Update("deleted_at", nil)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "恢复失败",
		})
		return result.Error
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "记录不存在",
		})
		return gorm.ErrRecordNotFound
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "恢复成功",
	})
	return nil
}

// 批量操作（区分软删除和硬删除）
func (t *CRUDTool) BatchOperation(c *gin.Context, models interface{}, operation string) error {
	start := time.Now()
	var err error

	defer func() {
		t.LogOperation(c.Request.Context(), "batch_"+operation, models, time.Since(start), err, nil)
	}()

	if err := c.ShouldBindJSON(models); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return err
	}

	var result *gorm.DB
	switch operation {
	case "create":
		result = t.DB.Create(models)
	case "update":
		result = t.DB.Save(models)
	case "soft_delete":
		result = t.DB.Delete(models)
	case "hard_delete":
		result = t.DB.Unscoped().Delete(models)
	default:
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "不支持的批量操作",
		})
		return fmt.Errorf("unsupported batch operation")
	}

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "批量操作失败",
		})
		return result.Error
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "批量操作成功",
		Data:    result.RowsAffected,
	})
	return nil
}

// GetMetrics 获取性能指标
func (t *CRUDTool) GetMetrics(c *gin.Context) {
	metrics := gin.H{}

	// 获取数据库统计信息
	if sqlDB, err := t.DB.DB(); err == nil {
		stats := sqlDB.Stats()
		dbStats := DatabaseStats{
			MaxOpenConnections: stats.MaxOpenConnections,
			OpenConnections:    stats.OpenConnections,
			InUse:              stats.InUse,
			Idle:               stats.Idle,
			WaitCount:          stats.WaitCount,
			WaitDuration:       stats.WaitDuration,
			MaxIdleClosed:      stats.MaxIdleClosed,
			MaxLifetimeClosed:  stats.MaxLifetimeClosed,
		}
		metrics["database"] = dbStats
	} else {
		metrics["database"] = "无法获取数据库统计信息: " + err.Error()
	}

	// 获取 Redis 统计信息
	metrics["redis"] = t.getRedisStats(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"code":    http.StatusOK,
		"message": "性能指标获取成功",
		"data":    metrics,
	})
}

// getRedisStats 获取 Redis 统计信息
func (t *CRUDTool) getRedisStats(ctx context.Context) interface{} {
	if t.RedisClient == nil {
		return "Redis 未配置"
	}

	// 获取 Redis 信息
	info, err := t.RedisClient.Info(ctx).Result()
	if err != nil {
		return "无法获取 Redis 信息: " + err.Error()
	}

	// 解析 Redis 信息为更结构化的格式
	redisStats := make(map[string]string)
	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			redisStats[parts[0]] = parts[1]
		}
	}

	return redisStats
}
