package setting

import (
	"github.com/QuantumNous/new-api/common"
)

var autoGroups = []string{
	"default",
}

var DefaultUseAutoGroup = false

// TokenGroupLockedToUserGroupEnabled 开启后，API Key 分组锁定为用户自身分组：
// 创建/更新时强制 Token.Group = User.Group，请求鉴权时以 User.Group 计费，并禁用 auto / 跨分组重试。
var TokenGroupLockedToUserGroupEnabled = true

func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	autoGroups = make([]string, 0)
	return common.Unmarshal([]byte(jsonString), &autoGroups)
}

func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(autoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	return autoGroups
}
