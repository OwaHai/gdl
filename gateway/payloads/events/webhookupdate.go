package events

type WebhookUpdate struct {
	GuildId   uint64          `json:"guild_id,string"`
	ChannelId uint64          `json:"channel_id,string"`
}
