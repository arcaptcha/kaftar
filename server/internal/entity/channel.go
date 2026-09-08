package entity

type Channel string

const (
	ChannelEmail      Channel = "email"
	ChannelSMS        Channel = "sms"
	ChannelTelegram   Channel = "telegram"
	ChannelDiscord    Channel = "discord"
	ChannelMattermost Channel = "mattermost"
	ChannelBale       Channel = "bale"
	ChannelHTTP       Channel = "http"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelEmail,
		ChannelSMS,
		ChannelTelegram,
		ChannelDiscord,
		ChannelMattermost,
		ChannelBale,
		ChannelHTTP:
		return true
	default:
		return false
	}
}

func (c Channel) String() string {
	return string(c)
}
