package rest

import (
	"fmt"
	"github.com/rxdn/gdl/objects"
	"github.com/rxdn/gdl/rest/request"
	"github.com/rxdn/gdl/rest/routes"
	"github.com/rxdn/gdl/utils"
)

func ListGuildEmojis(token string, guildId uint64) ([]*objects.Emoji, error) {
	endpoint := request.Endpoint{
		RequestType: request.GET,
		ContentType: request.Nil,
		Endpoint:    fmt.Sprintf("/guilds/%d/emojis", guildId),
	}

	var emojis []*objects.Emoji
	err, _ := endpoint.Request(token, &routes.RouteManager.GetEmojiRoute(guildId).Ratelimiter, nil, &emojis)
	return emojis, err
}

func GetGuildEmoji(token string, guildId, emojiId uint64) (*objects.Emoji, error) {
	endpoint := request.Endpoint{
		RequestType: request.GET,
		ContentType: request.Nil,
		Endpoint:    fmt.Sprintf("/guilds/%d/emojis/%d", guildId, emojiId),
	}

	var emoji objects.Emoji
	if err, _ := endpoint.Request(token, &routes.RouteManager.GetEmojiRoute(guildId).Ratelimiter, nil, &emoji); err != nil {
		return nil, err
	}

	return &emoji, nil
}

type CreateEmojiData struct {
	Name  string
	Image Image
	Roles []uint64 // roles for which this emoji will be whitelisted
}

func CreateGuildEmoji(token string, guildId uint64, data CreateEmojiData) (*objects.Emoji, error) {
	endpoint := request.Endpoint{
		RequestType: request.POST,
		ContentType: data.Image.ContentType,
		Endpoint:    fmt.Sprintf("/guilds/%d/emojis", guildId),
	}

	imageData, err := data.Image.Encode(); if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"name": data.Name,
		"image": imageData,
		"roles": utils.Uint64StringSlice(data.Roles),
	}

	var emoji objects.Emoji
	if err, _ := endpoint.Request(token, &routes.RouteManager.GetEmojiRoute(guildId).Ratelimiter, body, &emoji); err != nil {
		return nil, err
	}

	return &emoji, nil
}

// updating Image is not permitted
func ModifyGuildEmoji(token string, guildId, emojiId uint64, data CreateEmojiData) (*objects.Emoji, error) {
	endpoint := request.Endpoint{
		RequestType: request.PATCH,
		ContentType: request.Nil,
		Endpoint:    fmt.Sprintf("/guilds/%d/emojis/%d", guildId, emojiId),
	}

	body := map[string]interface{}{
		"name": data.Name,
		"roles": utils.Uint64StringSlice(data.Roles),
	}

	var emoji objects.Emoji
	if err, _ := endpoint.Request(token, &routes.RouteManager.GetEmojiRoute(guildId).Ratelimiter, body, &emoji); err != nil {
		return nil, err
	}

	return &emoji, nil
}

func DeleteGuildEmoji(token string, guildId, emojiId uint64) error {
	endpoint := request.Endpoint{
		RequestType: request.DELETE,
		ContentType: request.Nil,
		Endpoint:    fmt.Sprintf("/guilds/%d/emojis/%d", guildId, emojiId),
	}

	err, _ := endpoint.Request(token, &routes.RouteManager.GetEmojiRoute(guildId).Ratelimiter, nil, nil)
	return err
}
