package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type Material struct {
	Id          int            `json:"id"`
	UserId      int            `json:"user_id" gorm:"index"`
	Name        string         `json:"name" gorm:"type:varchar(191);index"`
	Type        string         `json:"type" gorm:"type:varchar(16);index"`
	MimeType    string         `json:"mime_type" gorm:"type:varchar(128)"`
	FileName    string         `json:"file_name" gorm:"type:varchar(255)"`
	Url         string         `json:"url" gorm:"type:varchar(1024)"`
	StorageType string         `json:"storage_type" gorm:"type:varchar(16);default:oss"`
	FilePath    string         `json:"file_path" gorm:"type:varchar(1024)"`
	FileSize    int64          `json:"file_size" gorm:"bigint"`
	Width       *int           `json:"width,omitempty"`
	Height      *int           `json:"height,omitempty"`
	Duration    *float64       `json:"duration,omitempty"`
	Status      int            `json:"status" gorm:"default:1"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

const (
	MaterialTypeImage = "image"
	MaterialTypeVideo = "video"
	MaterialTypeAudio = "audio"

	StorageTypeOSS   = "oss"
	StorageTypeLocal = "local"
)

func GetMaterialsByUser(userId int, startIdx int, num int) (materials []*Material, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			err = fmt.Errorf("panic in GetMaterialsByUser: %v", r)
		}
	}()

	err = tx.Model(&Material{}).Where("user_id = ?", userId).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = tx.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&materials).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return materials, total, nil
}

func SearchMaterialsByUser(userId int, keyword string, typeFilter string, startIdx int, num int) (materials []*Material, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			err = fmt.Errorf("panic in SearchMaterialsByUser: %v", r)
		}
	}()

	query := tx.Model(&Material{}).Where("user_id = ?", userId)

	if keyword != "" {
		nameCol := "`name`"
		if common.UsingPostgreSQL {
			nameCol = `"name"`
		}
		query = query.Where(nameCol+" LIKE ?", "%"+keyword+"%")
	}
	typeFilters := normalizeMaterialTypeFilters(typeFilter)
	if len(typeFilters) == 1 {
		query = query.Where("type = ?", typeFilters[0])
	} else if len(typeFilters) > 1 {
		query = query.Where("type IN ?", typeFilters)
	}

	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&materials).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return materials, total, nil
}

func normalizeMaterialTypeFilters(typeFilter string) []string {
	allowed := map[string]bool{
		MaterialTypeImage: true,
		MaterialTypeVideo: true,
		MaterialTypeAudio: true,
	}
	seen := make(map[string]bool)
	filters := make([]string, 0, 3)
	for _, item := range strings.Split(typeFilter, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if !allowed[item] || seen[item] {
			continue
		}
		seen[item] = true
		filters = append(filters, item)
	}
	return filters
}

func GetMaterialByIdAndUser(id int, userId int) (*Material, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	material := Material{Id: id}
	err := DB.Where("user_id = ?", userId).First(&material, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("素材不存在")
		}
		return nil, err
	}
	return &material, nil
}

func (m *Material) Insert() error {
	return DB.Create(m).Error
}

func (m *Material) UpdateName() error {
	return DB.Model(m).Select("name", "updated_time").Updates(m).Error
}

func (m *Material) Delete() error {
	return DB.Delete(m).Error
}

func DeleteMaterialByIdAndUser(id int, userId int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	material, err := GetMaterialByIdAndUser(id, userId)
	if err != nil {
		return err
	}
	return material.Delete()
}
