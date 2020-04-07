package cache

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rxdn/gdl/objects/channel"
	"github.com/rxdn/gdl/objects/guild"
	"github.com/rxdn/gdl/objects/guild/emoji"
	"github.com/rxdn/gdl/objects/member"
	"github.com/rxdn/gdl/objects/user"
	"sync"
)

type PgCache struct {
	*pgxpool.Pool
	Options CacheOptions

	// TODO: Should we store self in the DB? Seems kinda redundant
	selfLock sync.RWMutex
	self user.User
}

func NewPgCache(db *pgxpool.Pool, options CacheOptions) PgCache {
	// create schema
	mustRun(db, `CREATE TABLE IF NOT EXISTS guilds("guild_id" int8 NOT NULL UNIQUE, "data" jsonb NOT NULL, PRIMARY KEY("guild_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS channels("channel_id" int8 NOT NULL UNIQUE, "guild_id" int8 NOT NULL, "data" jsonb NOT NULL, PRIMARY KEY("channel_id", "guild_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS users("user_id" int8 NOT NULL UNIQUE, "data" jsonb NOT NULL, PRIMARY KEY("user_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS members("guild_id" int8 NOT NULL, "user_id" int8 NOT NULL, "data" jsonb NOT NULL, PRIMARY KEY("guild_id", "user_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS roles("role_id" int8 NOT NULL UNIQUE, "guild_id" int8 NOT NULL, "data" jsonb NOT NULL, PRIMARY KEY("role_id", "guild_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS emojis("emoji_id" int8 NOT NULL UNIQUE, "guild_id" int8 NOT NULL, "data" jsonb NOT NULL, PRIMARY KEY("emoji_id", "guild_id"));`)
	mustRun(db, `CREATE TABLE IF NOT EXISTS voice_states("guild_id" int8 NOT NULL, "user_id" INT8 NOT NULL, "data" jsonb NOT NULL, PRIMARY KEY("guild_id", "user_id"));`) // we may not have a cached user

	return PgCache{
		Pool: db,
		Options: options,
	}
}

func mustRun(db *pgxpool.Pool, query string) {
	if _, err := db.Exec(context.Background(), query); err != nil {
		panic(err)
	}
}

func (c *PgCache) GetOptions() CacheOptions {
	return c.Options
}

func (c *PgCache) StoreUser(user user.User) {
	if c.Options.Users {
		if encoded, err := json.Marshal(user.ToCachedUser()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO users("user_id", "data") VALUES($1, $2) ON CONFLICT("user_id") DO UPDATE SET "data" = $2;`, user.Id, string(encoded))
		}
	}
}

func (c *PgCache) GetUser(id uint64) (user.User, bool) {
	var user user.CachedUser
	if err := c.QueryRow(context.Background(), `SELECT "data" FROM users WHERE "user_id" = $1;`, id).Scan(&user); err != nil {
		return user.ToUser(id), false
	}

	return user.ToUser(id), true
}

func (c *PgCache) StoreGuild(g guild.Guild) {
	if c.Options.Guilds {
		if encoded, err := json.Marshal(g.ToCachedGuild()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO guilds("guild_id", "data") VALUES($1, $2) ON CONFLICT("guild_id") DO UPDATE SET "data" = $2;`, g.Id, string(encoded))
		}
	}

	for _, channel := range g.Channels {
		channel.GuildId = g.Id
		c.StoreChannel(channel)
	}

	for _, role := range g.Roles {
		c.StoreRole(role, g.Id)
	}

	for _, member := range g.Members {
		c.StoreUser(member.User)
		c.StoreMember(member, g.Id)
	}

	for _, emoji := range g.Emojis {
		c.StoreEmoji(emoji, g.Id)
	}

	for _, voiceState := range g.VoiceStates {
		c.StoreVoiceState(voiceState)
	}
}

// use withMembers with extreme caution!
func (c *PgCache) GetGuild(id uint64, withUserData bool) (guild.Guild, bool) {
	var cachedGuild guild.CachedGuild

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM guilds WHERE "guild_id" = $1;`, id).Scan(&cachedGuild); err != nil {
		return cachedGuild.ToGuild(id), false
	}

	g := cachedGuild.ToGuild(id)

	g.Channels = c.getChannels(id)
	g.Roles = c.getRoles(id)
	g.Members = c.getMembers(id, withUserData)
	g.Emojis = c.getEmojis(id,)
	g.VoiceStates = c.getVoiceStates(id)

	return g, true
}

func (c *PgCache) getChannels(guildId uint64) []channel.Channel {
	if !c.Options.Channels {
		return nil
	}

	rows, err := c.Query(context.Background(), `SELECT "channel_id", "data" FROM channels WHERE "guild_id" = $1;`, guildId)
	defer rows.Close()
	if err != nil {
		return nil
	}

	var channels []channel.Channel

	for rows.Next() {
		var channelId uint64
		var data channel.CachedChannel

		if err := rows.Scan(&channelId, &data); err != nil {
			continue
		}

		channels = append(channels, data.ToChannel(channelId))
	}

	return channels
}

func (c *PgCache) getRoles(guildId uint64) []guild.Role {
	if !c.Options.Roles {
		return nil
	}

	rows, err := c.Query(context.Background(), `SELECT "role_id", "data" FROM roles WHERE "guild_id" = $1;`, guildId)
	defer rows.Close()
	if err != nil {
		return nil
	}

	var roles []guild.Role

	for rows.Next() {
		var roleId uint64
		var data guild.CachedRole

		if err := rows.Scan(&roleId, &data); err != nil {
			continue
		}

		roles = append(roles, data.ToRole(roleId))
	}

	return roles
}

func (c *PgCache) getMembers(guildId uint64, withUserData bool) []member.Member {
	if !c.Options.Members {
		return nil
	}

	rows, err := c.Query(context.Background(), `SELECT "user_id", "data" FROM members WHERE "guild_id" = $1;`, guildId)
	defer rows.Close()
	if err != nil {
		return nil
	}

	var members []member.Member

	for rows.Next() {
		var userId uint64
		var data member.CachedMember

		if err := rows.Scan(&userId, &data); err != nil {
			continue
		}

		var userData user.User
		if withUserData {
			userData, _ = c.GetUser(userId)
		} else {
			userData = user.User{
				Id: userId,
			}
		}

		members = append(members, data.ToMember(userData))
	}

	return members
}

func (c *PgCache) getEmojis(guildId uint64) []emoji.Emoji {
	if !c.Options.Emojis {
		return nil
	}

	rows, err := c.Query(context.Background(), `SELECT "emoji_id", "data" FROM emojis WHERE "guild_id" = $1;`, guildId)
	defer rows.Close()
	if err != nil {
		return nil
	}

	var emojis []emoji.Emoji

	for rows.Next() {
		var emojiId uint64
		var data emoji.CachedEmoji

		if err := rows.Scan(&emojiId, &data); err != nil {
			continue
		}

		user, _ := c.GetUser(data.User)
		emojis = append(emojis, data.ToEmoji(emojiId, user))
	}

	return emojis
}

func (c *PgCache) getVoiceStates(guildId uint64) []guild.VoiceState {
	if !c.Options.VoiceStates {
		return nil
	}

	rows, err := c.Query(context.Background(), `SELECT "user_id", "data" FROM voice_states WHERE "guild_id" = $1;`, guildId)
	defer rows.Close()
	if err != nil {
		return nil
	}

	var states []guild.VoiceState

	for rows.Next() {
		var userId uint64
		var data guild.CachedVoiceState

		if err := rows.Scan(&userId, &data); err != nil {
			continue
		}

		member, _ := c.GetMember(guildId, userId)

		states = append(states, data.ToVoiceState(guildId, member))
	}

	return states
}

// TODO: FIX
func (c *PgCache) GetGuilds() []guild.Guild {
	var guilds []guild.Guild

	rows, err := c.Query(context.Background(), `SELECT * FROM guilds;`)
	defer rows.Close()
	if err != nil {
		return nil
	}

	if err := rows.Scan(&guilds); err != nil {
		return nil
	}

	return guilds
}

func (c *PgCache) DeleteGuild(id uint64) {
	if c.Options.Guilds {
		_, _ = c.Exec(context.Background(), `DELETE FROM guilds WHERE "guild_id" = $1;`, id)
	}
}

func (c *PgCache) GetGuildCount() int {
	var count int
	_ = c.QueryRow(context.Background(), "SELECT COUNT(*) FROM guilds;").Scan(&count)
	return count
}

func (c *PgCache) StoreMember(m member.Member, guildId uint64) {
	if c.Options.Members {
		if encoded, err := json.Marshal(m.ToCachedMember()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO members("guild_id", "user_id", "data") VALUES($1, $2, $3) ON CONFLICT("guild_id", "user_id") DO UPDATE SET "data" = $3;`, guildId, m.User.Id, string(encoded))
		}
	}
}

func (c *PgCache) GetMember(guildId, userId uint64) (member.Member, bool) {
	var cachedMember member.CachedMember
	if !c.Options.Members {
		return cachedMember.ToMember(user.User{Id: userId}), false
	}

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM members WHERE "guild_id" = $1 AND "user_id" = $2;`, guildId, userId).Scan(&cachedMember); err != nil {
		return cachedMember.ToMember(user.User{Id: userId}), false
	}

	// fill user field
	user, _ := c.GetUser(userId)
	return cachedMember.ToMember(user), true
}

func (c *PgCache) DeleteMember(userId, guildId uint64) {
	if c.Options.Members {
		_, _ = c.Exec(context.Background(), `DELETE FROM members WHERE "guild_id" = $1 AND "user_id" = $2;`, guildId, userId)
	}
}

func (c *PgCache) StoreChannel(ch channel.Channel) {
	if c.Options.Channels {
		if encoded, err := json.Marshal(ch.ToCachedChannel()); err == nil {
			_, err = c.Exec(context.Background(), `INSERT INTO channels("channel_id", "guild_id", "data") VALUES($1, $2, $3) ON CONFLICT("channel_id") DO UPDATE SET "data" = $3;`, ch.Id, ch.GuildId, string(encoded))
		}
	}
}

func (c *PgCache) GetChannel(id uint64) (channel.Channel, bool) {
	var ch channel.CachedChannel
	if !c.Options.Channels {
		return ch.ToChannel(id), false
	}

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM channels WHERE "channel_id" = $1;`, id).Scan(&ch); err != nil {
		return ch.ToChannel(id), false
	}

	return ch.ToChannel(id), true
}

func (c *PgCache) DeleteChannel(channelId, guildId uint64) {
	if c.Options.Channels {
		_, _ = c.Exec(context.Background(), `DELETE FROM channels WHERE "channel_id" = $1;`, channelId)
	}
}

func (c *PgCache) StoreRole(role guild.Role, guildId uint64) {
	if c.Options.Roles {
		if encoded, err := json.Marshal(role.ToCachedRole()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO roles("role_id", "guild_id", "data") VALUES($1, $2, $3) ON CONFLICT("role_id", "guild_id") DO UPDATE SET "data" = $3;`, role.Id, guildId, string(encoded))
		}
	}
}

func (c *PgCache) GetRole(id uint64) (guild.Role, bool) {
	var role guild.CachedRole
	if !c.Options.Roles {
		return role.ToRole(id), false
	}

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM roles WHERE "role_id" = $1;`, id).Scan(&role); err != nil {
		return role.ToRole(id), false
	}

	return role.ToRole(id), true
}

func (c *PgCache) DeleteRole(roleId, guildId uint64) {
	if c.Options.Roles {
		_, _ = c.Exec(context.Background(), `DELETE FROM roles WHERE "role_id" = $1;`, roleId)
	}
}

func (c *PgCache) StoreEmoji(emoji emoji.Emoji, guildId uint64) {
	if c.Options.Emojis {
		if encoded, err := json.Marshal(emoji.ToCachedEmoji()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO emojis("emoji_id", "guild_id", "data") VALUES($1, $2, $3) ON CONFLICT("emoji_id") DO UPDATE SET "data" = $3;`, emoji.Id, guildId, string(encoded))
		}
	}
}

func (c *PgCache) GetEmoji(id uint64) (emoji.Emoji, bool) {
	var cachedEmoji emoji.CachedEmoji
	if !c.Options.Emojis {
		return cachedEmoji.ToEmoji(id, user.User{}), false
	}

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM emojis WHERE "emoji_id" = $1;`, id).Scan(&cachedEmoji); err != nil {
		return cachedEmoji.ToEmoji(id, user.User{}), false
	}

	// fill user field
	user, _ := c.GetUser(cachedEmoji.User)

	return cachedEmoji.ToEmoji(id, user), true
}

func (c *PgCache) DeleteEmoji(emojiId, guildId uint64) {
	if c.Options.Emojis {
		_, _ = c.Exec(context.Background(), `DELETE FROM emojis WHERE "emoji_id" = $1;`, emojiId)
	}
}

func (c *PgCache) StoreVoiceState(state guild.VoiceState) {
	if c.Options.VoiceStates {
		if encoded, err := json.Marshal(state.ToCachedVoiceState()); err == nil {
			_, _ = c.Exec(context.Background(), `INSERT INTO voice_states("guild_id", "user_id", "data") VALUES($1, $2, $3) ON CONFLICT("guild_id", "user_id") DO UPDATE SET "data" = $3;`, state.GuildId, state.UserId, string(encoded))
		}
	}
}

func (c *PgCache) GetVoiceState(userId, guildId uint64) (guild.VoiceState, bool) {
	fakeMember := member.Member{
		User: user.User{
			Id: userId,
		},
	}

	var cachedVoiceState guild.CachedVoiceState
	if !c.Options.VoiceStates {
		return cachedVoiceState.ToVoiceState(guildId, fakeMember), false
	}

	if err := c.QueryRow(context.Background(), `SELECT "data" FROM voice_states WHERE "guild_id" = $1 AND "user_id" = $2;`, guildId, userId).Scan(&cachedVoiceState); err != nil {
		return cachedVoiceState.ToVoiceState(guildId, fakeMember), false
	}

	// fill user field
	member, _ := c.GetMember(guildId, userId)
	return cachedVoiceState.ToVoiceState(guildId, member), true
}

func (c *PgCache) StoreSelf(self user.User) {
	c.selfLock.Lock()
	c.self = self
	c.selfLock.Unlock()
}

func (c *PgCache) GetSelf() (user.User, bool) {
	c.selfLock.RLock()
	self := c.self
	c.selfLock.RUnlock()

	return self, self.Id != 0
}
