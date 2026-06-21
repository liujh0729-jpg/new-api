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
	SourceType  string         `json:"source_type" gorm:"type:varchar(16);index;default:material"`
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

	MaterialSourceTypeUpload   = "material"
	MaterialSourceTypeAIOutput = "ai_output"

	StorageTypeOSS   = "oss"
	StorageTypeLocal = "local"
)

type MaterialSearchFilters struct {
	Keyword          string
	TypeFilter       string
	SourceTypeFilter string
	CreatedAfter     int64
	CreatedBefore    int64
}

func GetMaterialsByUser(userId int, startIdx int, num int) (materials []*Material, total int64, err error) {
	return SearchMaterialsByUser(userId, MaterialSearchFilters{}, startIdx, num)
}

func SearchMaterialsByUser(userId int, filters MaterialSearchFilters, startIdx int, num int) (materials []*Material, total int64, err error) {
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

	query := buildMaterialQuery(tx.Model(&Material{}).Where("user_id = ?", userId), filters)

	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	query = buildMaterialQuery(tx.Model(&Material{}).Where("user_id = ?", userId), filters)
	err = query.Order("id DESC").Offset(startIdx).Limit(num).Find(&materials).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return materials, total, nil
}

func buildMaterialQuery(query *gorm.DB, filters MaterialSearchFilters) *gorm.DB {
	keyword := strings.TrimSpace(filters.Keyword)
	if keyword != "" {
		nameCol := "`name`"
		if common.UsingPostgreSQL {
			nameCol = `"name"`
		}
		query = query.Where(nameCol+" LIKE ?", "%"+keyword+"%")
	}

	typeFilters := normalizeMaterialTypeFilters(filters.TypeFilter)
	if len(typeFilters) == 1 {
		query = query.Where("type = ?", typeFilters[0])
	} else if len(typeFilters) > 1 {
		query = query.Where("type IN ?", typeFilters)
	}

	sourceTypeFilters := normalizeMaterialSourceTypeFilters(filters.SourceTypeFilter)
	query = applyMaterialSourceTypeFilter(query, sourceTypeFilters)

	if filters.CreatedAfter > 0 {
		query = query.Where("created_time >= ?", filters.CreatedAfter)
	}
	if filters.CreatedBefore > 0 {
		query = query.Where("created_time <= ?", filters.CreatedBefore)
	}

	return query
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

func normalizeMaterialSourceTypeFilters(sourceTypeFilter string) []string {
	allowed := map[string]bool{
		MaterialSourceTypeUpload:   true,
		MaterialSourceTypeAIOutput: true,
	}
	seen := make(map[string]bool)
	filters := make([]string, 0, 2)
	for _, item := range strings.Split(sourceTypeFilter, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if !allowed[item] || seen[item] {
			continue
		}
		seen[item] = true
		filters = append(filters, item)
	}
	return filters
}

func applyMaterialSourceTypeFilter(query *gorm.DB, sourceTypes []string) *gorm.DB {
	if len(sourceTypes) == 0 || len(sourceTypes) == 2 {
		return query
	}
	if sourceTypes[0] == MaterialSourceTypeUpload {
		return query.Where("(source_type = ? OR source_type = ? OR source_type IS NULL)", MaterialSourceTypeUpload, "")
	}
	return query.Where("source_type = ?", sourceTypes[0])
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

func CreateGeneratedMaterial(material *Material) (*Material, error) {
	if material == nil {
		return nil, errors.New("material is nil")
	}
	if material.UserId == 0 {
		return nil, errors.New("user id is required")
	}
	if strings.TrimSpace(material.Url) == "" {
		return nil, errors.New("material url is required")
	}
	material.SourceType = MaterialSourceTypeAIOutput

	var existing Material
	err := DB.Where(
		"user_id = ? AND url = ? AND source_type = ?",
		material.UserId,
		material.Url,
		MaterialSourceTypeAIOutput,
	).First(&existing).Error
	if err == nil {
		updates := map[string]interface{}{}
		if existing.FileSize <= 0 && material.FileSize > 0 {
			updates["file_size"] = material.FileSize
			existing.FileSize = material.FileSize
		}
		if strings.TrimSpace(existing.MimeType) == "" && strings.TrimSpace(material.MimeType) != "" {
			updates["mime_type"] = material.MimeType
			existing.MimeType = material.MimeType
		}
		if len(updates) > 0 {
			updates["updated_time"] = common.GetTimestamp()
			if updateErr := DB.Model(&existing).Updates(updates).Error; updateErr != nil {
				return nil, updateErr
			}
			if updatedAt, ok := updates["updated_time"].(int64); ok {
				existing.UpdatedTime = updatedAt
			}
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err = DB.Create(material).Error; err != nil {
		return nil, err
	}
	return material, nil
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

func EnsureMaterialSourceTypeDefault() error {
	if !DB.Migrator().HasColumn(&Material{}, "source_type") {
		return nil
	}
	return DB.Model(&Material{}).
		Where("source_type = ? OR source_type IS NULL", "").
		Update("source_type", MaterialSourceTypeUpload).Error
}
