package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Name string `gorm:"column:name"`
	Age  int    `gorm:"column:age"`
	Tags []Tag  `gorm:"many2many:user_tags;"`
}

func (User) TableName() string { return "users" }

type Profile struct {
	gorm.Model
	UserID uint
	Avatar string `json:"avatar"`
	Bio    string `json:"bio"`
}

func (Profile) TableName() string { return "profiles" }

type Tag struct {
	gorm.Model
	Name string `json:"name" gorm:"uniqueIndex"`
}

func (Tag) TableName() string { return "tags" }
