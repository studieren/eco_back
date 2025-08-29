以下是为电商网站后端接口设计的PostgreSQL数据库文档，包含核心表结构、关系及说明：

### 数据库整体设计
数据库名称：`ecommerce_db`
字符集：`UTF8`
排序规则：`en_US.UTF-8`

### 核心表结构设计

#### 用户表（users）
存储系统用户信息，包括买家、卖家和管理员
```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,  
    email VARCHAR(100) NOT NULL UNIQUE,   
    phone VARCHAR(20) UNIQUE,             
    password_hash VARCHAR(255) NOT NULL,   
    last_login_at TIMESTAMP WITH TIME ZONE, 
    login_ip VARCHAR(45),             
    status VARCHAR(20) NOT NULL DEFAULT 'active' 
        CHECK (status IN ('active', 'inactive', 'locked', 'deleted')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_profiles (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(100),               -- 真实姓名
    avatar_url VARCHAR(255),              -- 头像URL
    gender VARCHAR(10) CHECK (gender IN ('male', 'female', 'other')),
    birth_date DATE,                      -- 出生日期
    bio TEXT,                             -- 个人简介
    language VARCHAR(10) DEFAULT 'zh-CN', -- 偏好语言
    timezone VARCHAR(50) DEFAULT 'Asia/Shanghai', -- 时区
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### 角色表
```sql
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,     -- 角色名称（如：admin, seller, buyer_vip）
    code VARCHAR(50) NOT NULL UNIQUE,     -- 角色编码（用于权限判断）
    description TEXT,                     -- 角色描述
    is_default BOOLEAN DEFAULT FALSE,     -- 是否默认角色（新用户自动分配）
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### 权限表
```sql
CREATE TABLE permissions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,    -- 权限名称（如：商品管理）
    code VARCHAR(100) NOT NULL UNIQUE,    -- 权限编码（如：product:manage）
    module VARCHAR(50) NOT NULL,          -- 所属模块（如：product, order）
    description TEXT,                     -- 权限描述
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### 用户角色关联表
```sql
CREATE TABLE user_roles (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by INTEGER REFERENCES users(id), -- 分配人ID
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
```

### 角色权限关联表
```sql
CREATE TABLE role_permissions (
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_perms_role_id ON role_permissions(role_id);

-- 插入基础角色
INSERT INTO roles (name, code, description, is_default) VALUES
('普通买家', 'buyer', '基础购物权限', true),
('高级买家', 'buyer_vip', '包含折扣和优先客服', false),
('商家', 'seller', '商品管理和订单处理权限', false),
('系统管理员', 'admin', '全系统管理权限', false);

-- 插入基础权限（示例）
INSERT INTO permissions (name, code, module, description) VALUES
('查看商品', 'product:view', 'product', '浏览商品列表和详情'),
('下单', 'order:create', 'order', '创建订单'),
('管理商品', 'product:manage', 'product', 'CRUD商品信息'),
('处理退款', 'order:refund', 'order', '审核和处理退款申请');
```

2. 权限模型说明
采用 "用户 - 角色 - 权限" 三级模型（RBAC 模型）：

用户：登录系统的实体（买家、卖家、管理员等）
角色：一组权限的集合（如 "商品管理员" 包含商品 CRUD 权限）
权限：具体操作许可（如 "删除商品"、"查看订单"）

优势：
灵活分配：一个用户可拥有多个角色，一个角色可包含多个权限
易于管理：通过角色批量管理用户权限，无需逐个设置
扩展性强：新增角色或权限时不影响现有结构
3. 关键功能实现建议
3.1 用户注册与登录流程
注册时：
在users表创建账号（必须包含用户名 / 邮箱 / 手机号中的至少一个）
在user_profiles表创建关联的用户详情
自动分配默认角色（通过roles.is_default=true）
登录时：
支持多因素登录（用户名 / 邮箱 / 手机号 + 密码）
验证成功后更新last_login_at和login_ip
生成 JWT 令牌（包含用户 ID 和角色信息）

#### 2. 买家信息表（buyers）
扩展买家用户的详细信息
性别：
- 0 保密 Prefer not to say
- 1 男 Male
- 2 女 Female
```sql
CREATE TABLE buyers (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    full_name VARCHAR(100),
    avatar_url VARCHAR(255),
    gender TINYINT NOT NULL DEFAULT 0,
    birth_date DATE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 3. 卖家信息表（sellers）
扩展卖家用户的详细信息
```sql
CREATE TABLE sellers (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    store_name VARCHAR(100) NOT NULL,
    description TEXT,
    logo_url VARCHAR(255),
    business_license VARCHAR(255),
    verified BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 4. 地址表（addresses）
存储用户的账单/发货地址
```sql
CREATE TABLE addresses (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_name VARCHAR(100) NOT NULL,
    recipient_phone VARCHAR(20) NOT NULL,
    country VARCHAR(50) NOT NULL,
    province VARCHAR(50) NOT NULL,
    city VARCHAR(50) NOT NULL,
    district VARCHAR(50) NOT NULL,
    detail_address_one TEXT NOT NULL,
    detail_address_two TEXT NOT NULL,
    postal_code VARCHAR(20),
    address_type VARCHAR(20) NOT NULL CHECK (address_type IN ('bill', 'delivery')),
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 5. 商品分类表（categories）
存储商品分类信息，支持多级分类
```sql
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    parent_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    level INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER DEFAULT 0,
    icon_url VARCHAR(255),
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 6. 商品表（products）
存储商品基本信息
```sql
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    seller_id INTEGER NOT NULL REFERENCES sellers(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    subtitle VARCHAR(255),
    main_image_url VARCHAR(255),
    price DECIMAL(10, 2) NOT NULL CHECK (price >= 0),
    market_price DECIMAL(10, 2) CHECK (market_price >= 0),
    stock INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
    sales_count INTEGER NOT NULL DEFAULT 0 CHECK (sales_count >= 0),
    rating DECIMAL(2, 1) DEFAULT 0 CHECK (rating >= 0 AND rating <= 5),
    rating_count INTEGER DEFAULT 0 CHECK (rating_count >= 0),
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'deleted')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 7. 商品详情表（product_details）
存储商品的详细描述信息
```sql
CREATE TABLE product_details (
    product_id INTEGER PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    description TEXT,
    specifications JSONB, -- 存储商品规格，如{"颜色": "红色", "尺寸": "XL"}
    packing_list TEXT,
    after_sale_service TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 8. 商品图片表（product_images）
存储商品的多图片信息
```sql
CREATE TABLE product_images (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    image_url VARCHAR(255) NOT NULL,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 9. 订单表（orders）
存储订单基本信息
```sql
CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    order_no VARCHAR(50) NOT NULL UNIQUE, -- 订单编号，如ORD20230828001
    buyer_id INTEGER NOT NULL REFERENCES buyers(id) ON DELETE RESTRICT,
    total_amount DECIMAL(10, 2) NOT NULL CHECK (total_amount >= 0),
    payment_amount DECIMAL(10, 2) NOT NULL CHECK (payment_amount >= 0),
    freight DECIMAL(10, 2) NOT NULL DEFAULT 0 CHECK (freight >= 0),
    discount_amount DECIMAL(10, 2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending_payment', 'paid', 'shipped', 'delivered', 'cancelled', 'refunded')),
    payment_method VARCHAR(20) CHECK (payment_method IN ('alipay', 'wechat', 'credit_card', 'bank_transfer')),
    payment_time TIMESTAMP WITH TIME ZONE,
    ship_time TIMESTAMP WITH TIME ZONE,
    delivery_time TIMESTAMP WITH TIME ZONE,
    address_id INTEGER NOT NULL REFERENCES addresses(id) ON DELETE RESTRICT,
    recipient_name VARCHAR(100) NOT NULL,
    recipient_phone VARCHAR(20) NOT NULL,
    shipping_address TEXT NOT NULL,
    note TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 10. 订单项表（order_items）
存储订单中的商品明细
```sql
CREATE TABLE order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    seller_id INTEGER NOT NULL REFERENCES sellers(id) ON DELETE RESTRICT,
    product_name VARCHAR(200) NOT NULL,
    product_image_url VARCHAR(255),
    price DECIMAL(10, 2) NOT NULL CHECK (price >= 0),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    subtotal DECIMAL(10, 2) NOT NULL CHECK (subtotal >= 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 11. 购物车表（shopping_carts）
存储用户的购物车信息
```sql
CREATE TABLE shopping_carts (
    id SERIAL PRIMARY KEY,
    buyer_id INTEGER NOT NULL REFERENCES buyers(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    selected BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(buyer_id, product_id)
);
```

#### 12. 商品评价表（reviews）
存储用户对商品的评价
```sql
CREATE TABLE reviews (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    order_item_id INTEGER REFERENCES order_items(id) ON DELETE SET NULL,
    buyer_id INTEGER NOT NULL REFERENCES buyers(id) ON DELETE RESTRICT,
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    content TEXT,
    images JSONB, -- 存储评价图片URL数组
    status VARCHAR(20) DEFAULT 'published' CHECK (status IN ('pending', 'published', 'rejected')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 13. 优惠券表（coupons）
存储优惠券信息
```sql
CREATE TABLE coupons (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('percentage', 'fixed_amount')),
    value DECIMAL(10, 2) NOT NULL CHECK (value > 0),
    min_purchase DECIMAL(10, 2) DEFAULT 0 CHECK (min_purchase >= 0),
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    total_quantity INTEGER NOT NULL CHECK (total_quantity > 0),
    used_quantity INTEGER NOT NULL DEFAULT 0 CHECK (used_quantity >= 0),
    status VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'expired')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

#### 14. 用户优惠券表（user_coupons）
存储用户领取的优惠券
```sql
CREATE TABLE user_coupons (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    coupon_id INTEGER NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'unused' CHECK (status IN ('unused', 'used', 'expired')),
    order_id INTEGER REFERENCES orders(id) ON DELETE SET NULL,
    obtained_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(user_id, coupon_id)
);
```

### 索引设计
```sql
-- 用户表索引
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_user_type ON users(user_type);

-- 商品表索引
CREATE INDEX idx_products_seller_id ON products(seller_id);
CREATE INDEX idx_products_category_id ON products(category_id);
CREATE INDEX idx_products_status ON products(status);

-- 订单表索引
CREATE INDEX idx_orders_buyer_id ON orders(buyer_id);
CREATE INDEX idx_orders_order_no ON orders(order_no);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);

-- 订单项表索引
CREATE INDEX idx_order_items_order_id ON order_items(order_id);
CREATE INDEX idx_order_items_product_id ON order_items(product_id);
CREATE INDEX idx_order_items_seller_id ON order_items(seller_id);

-- 购物车表索引
CREATE INDEX idx_shopping_carts_buyer_id ON shopping_carts(buyer_id);

-- 评价表索引
CREATE INDEX idx_reviews_product_id ON reviews(product_id);
CREATE INDEX idx_reviews_buyer_id ON reviews(buyer_id);
```

### 数据库关系图说明
- 用户(users)是核心表，通过user_type区分买家、卖家和管理员
- 买家(buyers)和卖家(sellers)表与用户表是一对一关系
- 一个用户可以有多个地址，地址表(addresses)与用户表是多对一关系
- 商品分类(categories)支持多级分类，通过parent_id实现自关联
- 商品(products)属于某个分类，与分类表是多对一关系
- 商品与卖家是多对一关系，一个卖家可以有多个商品
- 订单(orders)与买家是多对一关系，一个买家可以有多个订单
- 一个订单包含多个订单项(order_items)，订单项与商品是多对一关系
- 购物车(shopping_carts)与买家是多对一关系，与商品是多对一关系
- 优惠券(coupons)可以被多个用户领取，通过用户优惠券表(user_coupons)实现多对多关系

### 扩展建议
1. 对于高并发场景，可以考虑对热门商品表进行读写分离
2. 对于订单历史数据，可以考虑按时间分区存储
3. 对于商品搜索功能，可以考虑集成PostgreSQL的全文搜索功能或Elasticsearch
4. 可以添加触发器自动更新商品的销售数量、评分等统计信息
