package events

import "github.com/Dot-Rar/gdl/objects"

type GuildMemberRemove struct {
	GuildId uint64        `json:"guild_id,string"`
	User    *objects.User `json:"user"`
}
