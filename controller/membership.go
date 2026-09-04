package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type createMembershipLevelRequest struct {
	Code          string `json:"code" binding:"required"`
	DisplayName   string `json:"display_name" binding:"required"`
	MultiplierPPM int64  `json:"multiplier_ppm" binding:"required"`
	Rank          int    `json:"rank"`
	SortOrder     int    `json:"sort_order"`
	Enabled       *bool  `json:"enabled"`
}

type updateMembershipLevelRequest struct {
	DisplayName   string `json:"display_name" binding:"required"`
	MultiplierPPM int64  `json:"multiplier_ppm" binding:"required"`
	Rank          int    `json:"rank"`
	SortOrder     int    `json:"sort_order"`
	Enabled       bool   `json:"enabled"`
}

type createUserMembershipRequest struct {
	UserId            int    `json:"user_id" binding:"required"`
	MembershipLevelId int    `json:"membership_level_id" binding:"required"`
	StartsAt          int64  `json:"starts_at"`
	EndsAt            int64  `json:"ends_at"`
	Note              string `json:"note"`
}

type applyLegacyVIPMigrationRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

func GetSelfMembership(c *gin.Context) {
	snapshot, err := model.ResolveUserMembership(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, snapshot)
}

func GetMembershipLevels(c *gin.Context) {
	includeArchived := c.Query("include_archived") == "true"
	levels, err := model.ListMembershipLevels(includeArchived)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, levels)
}

func CreateMembershipLevel(c *gin.Context) {
	var request createMembershipLevelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	level := &model.MembershipLevel{
		Code: request.Code, DisplayName: request.DisplayName,
		MultiplierPPM: request.MultiplierPPM, Rank: request.Rank,
		SortOrder: request.SortOrder, Enabled: enabled,
	}
	if err := model.CreateMembershipLevel(level); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("created membership level %s", level.Code))
	common.ApiSuccess(c, level)
}

func UpdateMembershipLevel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid membership level id"))
		return
	}
	var request updateMembershipLevelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	level := &model.MembershipLevel{
		Id: id, DisplayName: request.DisplayName, MultiplierPPM: request.MultiplierPPM,
		Rank: request.Rank, SortOrder: request.SortOrder, Enabled: request.Enabled,
	}
	if err := model.UpdateMembershipLevel(level); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetMembershipLevelById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("updated membership level %s", updated.Code))
	common.ApiSuccess(c, updated)
}

func ArchiveMembershipLevel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid membership level id"))
		return
	}
	level, err := model.GetMembershipLevelById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.ArchiveMembershipLevel(id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("archived membership level %s", level.Code))
	common.ApiSuccess(c, nil)
}

func GetUserMemberships(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userId <= 0 {
		common.ApiError(c, errors.New("invalid user id"))
		return
	}
	if err := ensureMembershipTargetPermission(c, userId); err != nil {
		common.ApiError(c, err)
		return
	}
	grants, err := model.ListUserMemberships(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	current, err := model.ResolveUserMembership(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"current": current, "grants": grants})
}

func CreateUserMembership(c *gin.Context) {
	var request createUserMembershipRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := ensureMembershipTargetPermission(c, request.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	grant := &model.UserMembership{
		UserId: request.UserId, MembershipLevelId: request.MembershipLevelId,
		StartsAt: request.StartsAt, EndsAt: request.EndsAt,
		Status: model.UserMembershipStatusActive, Source: model.UserMembershipSourceAdmin,
		Note: request.Note, CreatedBy: c.GetInt("id"),
	}
	if err := model.CreateUserMembership(grant); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(request.UserId, model.LogTypeManage, fmt.Sprintf("membership grant #%d created by admin #%d", grant.Id, c.GetInt("id")))
	common.ApiSuccess(c, grant)
}

func RevokeUserMembership(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, errors.New("invalid membership grant id"))
		return
	}
	grant, err := model.GetUserMembershipById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := ensureMembershipTargetPermission(c, grant.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RevokeUserMembership(id, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(grant.UserId, model.LogTypeManage, fmt.Sprintf("membership grant #%d revoked by admin #%d", id, c.GetInt("id")))
	common.ApiSuccess(c, nil)
}

func GetLegacyVIPMigrationPreflight(c *gin.Context) {
	preflight, err := model.PreflightLegacyVIPGroupMigration()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preflight)
}

func ApplyLegacyVIPMigration(c *gin.Context) {
	var request applyLegacyVIPMigrationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(request.Confirmation) != "MIGRATE_LEGACY_VIP_GROUPS" {
		common.ApiError(c, errors.New("migration confirmation does not match"))
		return
	}
	result, err := model.ApplyLegacyVIPGroupMigration()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, fmt.Sprintf("migrated %d legacy VIP users to memberships", result.MigratedUsers))
	common.ApiSuccess(c, result)
}

func ensureMembershipTargetPermission(c *gin.Context, userId int) error {
	target, err := model.GetUserById(userId, false)
	if err != nil {
		return err
	}
	role := c.GetInt("role")
	if role != common.RoleRootUser && role <= target.Role {
		return errors.New("cannot manage membership for a user with the same or higher role")
	}
	return nil
}
