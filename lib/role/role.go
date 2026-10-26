// Package role 回答一个问题: **这个用户在本机器人里是谁**。
package role

import (
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// configProvider 按 config.json 的 owners 名单判定机器人主人。
// 它返回 RoleBotOwner 而不是 RoleOwner: 机器人主人和平台下发的群主是两回事,
// 想只给机器人主人用的指令声明 Role: constant.RoleBotOwner 即可。
// 不做缓存: 面板保存后下一条消息就按新名单走。
type configProvider struct{}

func (configProvider) Resolve(userID, groupID string, platform constant.RoleRequired) (constant.RoleRequired, bool) {
	if userID == "" {
		return "", false
	}
	for _, id := range config.OwnerList() {
		if id == userID {
			return constant.RoleBotOwner, true
		}
	}
	return "", false
}

func init() { Use(configProvider{}) }

// Provider 是机器人级角色的一个来源。
type Provider interface {
	Resolve(userID, groupID string, platform constant.RoleRequired) (role constant.RoleRequired, claimed bool)
}

var (
	mu        sync.RWMutex
	providers []Provider
)

// Use 注册一个来源。约定在插件 init() 里调用。
func Use(p Provider) {
	if p == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	providers = append(providers, p)
}

// Resolve 走完整条来源链, 返回最终角色。
func Resolve(userID, groupID string, platform constant.RoleRequired) constant.RoleRequired {
	mu.RLock()
	list := providers
	mu.RUnlock()

	for _, p := range list {
		if resolved, claimed := p.Resolve(userID, groupID, platform); claimed {
			if resolved == "" {
				return constant.RoleMember
			}
			return resolved
		}
	}
	return platformRole(platform)
}

// ResolveAny 与 Resolve 同一条来源链, 但接受发送者的全部标识:
// 同一个群成员在群聊是 member_openid、私聊是 user_openid、跨群还能有 union_openid,
// 名单里填其中任意一个都该被认出来。
func ResolveAny(userIDs []string, groupID string, platform constant.RoleRequired) constant.RoleRequired {
	mu.RLock()
	list := providers
	mu.RUnlock()

	for _, p := range list {
		for _, userID := range userIDs {
			if userID == "" {
				continue
			}
			if resolved, claimed := p.Resolve(userID, groupID, platform); claimed {
				if resolved == "" {
					return constant.RoleMember
				}
				return resolved
			}
		}
	}
	return platformRole(platform)
}

func platformRole(platform constant.RoleRequired) constant.RoleRequired {
	if platform == "" {
		return constant.RoleMember
	}
	return platform
}

// Reset 清空已注册的来源。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	providers = nil
}
