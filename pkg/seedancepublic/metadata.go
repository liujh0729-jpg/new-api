package seedancepublic

import "strings"

// Java's private supplier metadata may also occur in old user log payloads.
// Keep NewAPI's own public billing modes used by the charge breakdown UI.
func privateMetadataField(key string, value any) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	switch key {
	case "aipddmeta", "basesource", "provider", "providerid", "providername", "billingscope",
		"estimatedawcoin", "basestatus", "costawcoinpersecond", "apikey", "authorization",
		"accesstoken", "credential", "credentialsource", "billingsnapshot", "financesnapshot":
		return true
	case "billingmode":
		return value != "task_pricing" && value != "tiered_expr" && value != "token"
	}
	return false
}
