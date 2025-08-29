# 电商网站后台接口测试 使用说明文档

## 概述

电商网站后台接口测试
基于gin框架，gorm框架，sqlite3 数据库，redis缓存，jwt认证，日志记录，性能监控，接口文档等功能。

## vps 安装go语言
1. 下载并解压go语言
```sh
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
```
这样 Go 会被安装到 /usr/local/go。

2. 配置环境变量
编辑`~/.bashrc`或`~/.zshrc`：
```sh
export PATH=$PATH:/usr/local/go/bin
```

让配置生效：
```sh
source ~/.bashrc
```

3. 验证安装
```sh
go version
```

输出类似：
```sh
go version go1.25.0 linux/amd64
```

### 一条命令生效
```sh
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -O /tmp/go1.25.0.linux-amd64.tar.gz && \
sudo rm -rf /usr/local/go && \
sudo tar -C /usr/local -xzf /tmp/go1.25.0.linux-amd64.tar.gz && \
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && \
source ~/.bashrc && \
go version
```

# git 推送配置clash代理
```sh
git config --global http.proxy http://127.0.0.1:7890
git config --global https.proxy https://127.0.0.1:7890
```

## 安装依赖

```bash
go mod init your-project
go get -u github.com/gin-gonic/gin
go get -u gorm.io/gorm
go get -u gorm.io/driver/mysql
go get -u github.com/redis/go-redis/v9
```

## 编译
```sh
# 下载依赖
go mod tidy

# 构建可执行文件
go build -o app

# 编译时查看详细信息
go build -v -x main.go 2>&1 | tee compile.log
```

## linux中运行项目
```sh
# 直接运行
./app

# 如果希望后台运行（保持 VPS 会话关闭后仍运行）
nohup ./app > app.log 2>&1 &

```
nohup 可以让程序在后台运行
输出日志在`app.log`


## 快速开始

### 1. 基本配置

```go
package main

import (
	"context"
	"log"
	"time"

	"go_test/gormtool"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `json:"name"`
	Email     string         `json:"email"`
	Age       int            `json:"age"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// 初始化数据库
// dsn := "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"
// db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})

func main() {
	// 初始化数据库连接
	dsn := "host=localhost user=postgres password=123456 dbname=gindemo port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// 初始化 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	// 自定义日志函数
	logger := func(ctx context.Context, operation string, model interface{}, duration time.Duration, err error) {
		log.Printf("Operation: %s, Duration: %v, Error: %v", operation, duration, err)
	}

	// 创建 CRUD 工具
	crudTool := gormtool.NewCRUDTool(db, redisClient, logger)

	r := gin.Default()

	// 使用各种功能
	r.GET("/users/:id", func(c *gin.Context) {
		var user User
		crudTool.GetByID(c, &user)
	})

	r.DELETE("/users/:id", func(c *gin.Context) {
		var user User
		crudTool.SoftDeleteByID(c, &user)
	})

	r.POST("/users/batch", func(c *gin.Context) {
		var users []User
		crudTool.BatchOperation(c, &users, "create")
	})

	r.GET("/metrics", func(c *gin.Context) {
		crudTool.GetMetrics(c)
	})

	// 使用查询构建器
	r.POST("/users/query", func(c *gin.Context) {
		var users []User
		var qb gormtool.QueryBuilder
		if err := c.ShouldBindJSON(&qb); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		crudTool.GetByQueryBuilder(c, &users, &qb)
	})

	// 使用事务
	r.POST("/users/transaction", func(c *gin.Context) {
		crudTool.Transaction(c, func(tx *gorm.DB) error {
			// 在这里执行多个数据库操作
			user1 := User{Name: "User1", Email: "user1@example.com"}
			user2 := User{Name: "User2", Email: "user2@example.com"}

			if err := tx.Create(&user1).Error; err != nil {
				return err
			}
			if err := tx.Create(&user2).Error; err != nil {
				return err
			}
			return nil
		})
	})

	r.Run(":1234")
}
```

### 2. 路由设置示例

```go
func setupRoutes(r *gin.Engine, crudTool *CRUDTool) {
    // 健康检查
    r.GET("/health", func(c *gin.Context) {
        crudTool.HealthCheck(c)
    })

    // 性能监控
    r.GET("/metrics", func(c *gin.Context) {
        crudTool.GetMetrics(c)
    })

    // User 相关路由
    userGroup := r.Group("/users")
    {
        userGroup.GET("/:id", func(c *gin.Context) {
            var user User
            crudTool.GetByID(c, &user)
        })

        userGroup.GET("", func(c *gin.Context) {
            var users []User
            crudTool.GetPaginated(c, &users)
        })

        userGroup.POST("", func(c *gin.Context) {
            var user User
            crudTool.Create(c, &user)
        })

        userGroup.PUT("/:id", func(c *gin.Context) {
            var user User
            crudTool.UpdateByID(c, &user)
        })

        userGroup.DELETE("/:id", func(c *gin.Context) {
            var user User
            crudTool.SoftDeleteByID(c, &user)
        })

        // 高级查询
        userGroup.POST("/query", func(c *gin.Context) {
            var users []User
            var qb gormtool.QueryBuilder
            if err := c.ShouldBindJSON(&qb); err != nil {
                c.JSON(400, gin.H{"error": err.Error()})
                return
            }
            crudTool.GetByQueryBuilder(c, &users, &qb)
        })
    }

    // 事务示例
    r.POST("/transaction", func(c *gin.Context) {
        crudTool.Transaction(c, func(tx *gorm.DB) error {
            // 创建用户
            user := User{Name: "Transaction User", Email: "transaction@example.com", Age: 25}
            if err := tx.Create(&user).Error; err != nil {
                return err
            }

            // 创建产品
            product := Product{Name: "Transaction Product", Price: 99.99}
            if err := tx.Create(&product).Error; err != nil {
                return err
            }

            return nil
        })
    })
}
```

## 核心功能使用示例

### 1. 基本 CRUD 操作

```go
// 查询单个用户
var user User
crudTool.GetByID(c, &user)

// 查询所有用户（带预加载）
var users []User
crudTool.GetAll(c, &users, "Orders", "Profile")

// 分页查询
var users []User
crudTool.GetPaginated(c, &users)

// 创建用户
var user User
crudTool.Create(c, &user)

// 更新用户
var user User
crudTool.UpdateByID(c, &user)

// 软删除
var user User
crudTool.SoftDeleteByID(c, &user)

// 硬删除
var user User
crudTool.HardDeleteByID(c, &user)

// 恢复软删除
var user User
crudTool.RestoreSoftDelete(c, &user)
```

### 2. 高级查询构建器

```go
// 构建复杂查询
qb := QueryBuilder{
    Conditions: []QueryCondition{
        {
            Field:    "age",
            Operator: ">",
            Value:    18,
        },
        {
            Field:    "name",
            Operator: "LIKE",
            Value:    "John",
        },
    },
    Sorts: []SortCondition{
        {
            Field:     "created_at",
            Direction: "DESC",
        },
    },
    Preloads: []string{"Orders", "Profile"},
}

var users []User
crudTool.GetByQueryBuilder(c, &users, &qb)
```

### 3. 批量操作

```go
// 批量创建
users := []User{
    {Name: "User1", Email: "user1@example.com", Age: 20},
    {Name: "User2", Email: "user2@example.com", Age: 25},
}
crudTool.BatchOperation(c, &users, "create")

// 批量更新
crudTool.BatchOperation(c, &users, "update")

// 批量删除
crudTool.BatchOperation(c, &users, "delete")
```

### 4. 条件查询

```go
// 条件查询
var users []User
crudTool.GetByCondition(c, &users, "age > ? AND name LIKE ?", 18, "%John%")

// 带条件的分页查询
crudTool.GetPaginatedWithCondition(c, &users, "age > ?", 18)
```

## API 请求示例

### 1. 创建用户
```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name": "John Doe", "email": "john@example.com", "age": 30}'
```

### 2. 分页查询用户
```bash
curl "http://localhost:8080/users?page=1&pageSize=10"
```

### 3. 高级查询
```bash
curl -X POST http://localhost:8080/users/query \
  -H "Content-Type: application/json" \
  -d '{
    "conditions": [
      {"field": "age", "operator": ">", "value": 18},
      {"field": "name", "operator": "LIKE", "value": "John"}
    ],
    "sorts": [
      {"field": "created_at", "direction": "DESC"}
    ]
  }'
```

### 4. 健康检查
```bash
curl http://localhost:8080/health
```

### 5. 性能指标
```bash
curl http://localhost:8080/metrics
```

## 配置说明

### 数据库连接池配置
```go
// 最大打开连接数
crudTool.ConfigureConnectionPool(100, 10, time.Hour)
```

### Redis 配置
```go
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: "your_password",
    DB:       0,
})
```

### 自定义日志
```go
logger := func(ctx context.Context, operation string, model interface{}, duration time.Duration, err error) {
    // 自定义日志逻辑
    log.Printf("操作: %s, 耗时: %v, 错误: %v", operation, duration, err)
}
```

## 错误处理

所有操作都包含统一的错误处理，返回格式：
```json
{
    "code": 400,
    "message": "错误信息",
    "data": null
}
```

## 响应格式

成功响应：
```json
{
    "code": 200,
    "message": "操作成功",
    "data": {...},
    "page": {
        "page": 1,
        "pageSize": 10,
        "total": 100
    }
}
```

## 注意事项

1. **模型要求**：所有模型需要实现 GORM 的标准字段
2. **软删除**：需要包含 `DeletedAt gorm.DeletedAt` 字段
3. **缓存**：Redis 是可选的，如果没有配置会自动降级
4. **事务**：确保在事务中处理所有相关操作
5. **连接池**：根据实际负载调整连接池参数

这个工具库可以显著提高开发效率，减少重复代码，同时提供企业级应用所需的高级功能。
