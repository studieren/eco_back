#!/usr/bin/env python3
import sqlite3
import random
import datetime
import os
from faker import Faker

faker = Faker()

DB_FILE = "test.db"

# 如果旧库存在，先删除（调试用，可注释）
if os.path.exists(DB_FILE):
    os.remove(DB_FILE)

conn = sqlite3.connect(DB_FILE)
cur = conn.cursor()

# ------------------ 建表 ------------------
cur.executescript("""
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT UNIQUE NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    age        INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS profiles (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    avatar     TEXT,
    bio        TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME
);

CREATE TABLE IF NOT EXISTS user_tags (
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    tag_id  INTEGER REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tag_id)
);
""")

conn.commit()


# ------------------ 工具函数 ------------------
def now_pair():
    """
    返回 (created_at, updated_at)，保证 updated_at >= created_at
    """
    created = faker.date_time_this_decade()
    updated = created + datetime.timedelta(
        seconds=random.randint(0, 86400 * 30)  # 最多晚 30 天
    )
    return created.strftime("%Y-%m-%d %H:%M:%S"), updated.strftime("%Y-%m-%d %H:%M:%S")


def deleted_or_null():
    """
    90% 概率返回 NULL（未软删除），10% 概率返回一个软删除时间
    """
    if random.random() < 0.9:
        return None
    return faker.date_time_this_year().strftime("%Y-%m-%d %H:%M:%S")


# ------------------ 插入 tags ------------------
tags = []
for _ in range(500):
    created, updated = now_pair()
    deleted = deleted_or_null()
    name = faker.word() + str(random.randint(1000, 9999))  # 保证唯一
    cur.execute(
        "INSERT INTO tags(name, created_at, updated_at, deleted_at) VALUES (?,?,?,?)",
        (name, created, updated, deleted),
    )
    tags.append(cur.lastrowid)

conn.commit()

# ------------------ 插入 users + profiles + 关联 ------------------
for _ in range(500):
    # users
    created, updated = now_pair()
    deleted = deleted_or_null()
    name = faker.name()
    age = random.randint(18, 65)
    cur.execute(
        "INSERT INTO users(name, age, created_at, updated_at, deleted_at) VALUES (?,?,?,?,?)",
        (name, age, created, updated, deleted),
    )
    user_id = cur.lastrowid

    # profiles
    created, updated = now_pair()
    deleted_prof = deleted_or_null()
    avatar = faker.image_url()
    bio = faker.text(max_nb_chars=120)
    cur.execute(
        "INSERT INTO profiles(user_id, avatar, bio, created_at, updated_at, deleted_at) VALUES (?,?,?,?,?,?)",
        (user_id, avatar, bio, created, updated, deleted_prof),
    )

    # user_tags：随机 1~5 个标签
    chosen = random.sample(tags, k=random.randint(1, 5))
    for tag_id in chosen:
        try:
            cur.execute(
                "INSERT INTO user_tags(user_id, tag_id) VALUES (?,?)", (user_id, tag_id)
            )
        except sqlite3.IntegrityError:
            # 重复关联忽略
            pass

conn.commit()
cur.close()
conn.close()

print("✅ SQLite3 数据插入完成，共 500 条用户、500 条档案、500 条标签及中间表。")
