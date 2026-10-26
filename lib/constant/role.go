package constant

// RoleRequired 定义自定义类型
type RoleRequired string

const (
	// RoleBotOwner 机器人主人: 只认 config.json 的 owners 名单, 不认平台下发的群主。
	RoleBotOwner RoleRequired = "bot_owner"
	// RoleOwner 群主(平台下发 member_role=owner); 机器人主人比它更高, 同样满足。
	RoleOwner  RoleRequired = "owner"
	RoleAdmin  RoleRequired = "admin"
	RoleMember RoleRequired = "member"
)

func (require RoleRequired) CanUse(user RoleRequired) bool {
	if require == "" {
		return true
	}
	switch require {
	case RoleBotOwner:
		return user == RoleBotOwner
	case RoleOwner:
		return user == RoleOwner || user == RoleBotOwner
	case RoleAdmin:
		return user == RoleAdmin || user == RoleOwner || user == RoleBotOwner
	default:
		return true
	}
}
