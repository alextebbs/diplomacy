package testutil

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

// MockDiscordSession captures Discord API calls for testing.
type MockDiscordSession struct {
	mu             sync.Mutex
	Responses      []*discordgo.InteractionResponse
	Messages       []*discordgo.MessageSend
	SentChannelIDs []string
}

func NewMockDiscordSession() *MockDiscordSession {
	return &MockDiscordSession{}
}

func (m *MockDiscordSession) InteractionRespond(interaction *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses = append(m.Responses, resp)
	return nil
}

func (m *MockDiscordSession) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentChannelIDs = append(m.SentChannelIDs, channelID)
	m.Messages = append(m.Messages, &discordgo.MessageSend{Content: content})
	return &discordgo.Message{}, nil
}

func (m *MockDiscordSession) ChannelMessageSendComplex(channelID string, msg *discordgo.MessageSend) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SentChannelIDs = append(m.SentChannelIDs, channelID)
	m.Messages = append(m.Messages, msg)
	return &discordgo.Message{}, nil
}

func (m *MockDiscordSession) GetResponseCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Responses)
}

func (m *MockDiscordSession) LastResponse() *discordgo.InteractionResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Responses) == 0 {
		return nil
	}
	return m.Responses[len(m.Responses)-1]
}

func (m *MockDiscordSession) LastResponseContent() string {
	resp := m.LastResponse()
	if resp == nil || resp.Data == nil {
		return ""
	}
	return resp.Data.Content
}

func (m *MockDiscordSession) LastResponseIsEphemeral() bool {
	resp := m.LastResponse()
	if resp == nil || resp.Data == nil {
		return false
	}
	return resp.Data.Flags == discordgo.MessageFlagsEphemeral
}

func (m *MockDiscordSession) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses = nil
	m.Messages = nil
	m.SentChannelIDs = nil
}
