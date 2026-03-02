package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"
	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/game"
)

type Bot struct {
	session   *discordgo.Session
	appID     string
	mgr       *game.Manager
	store     *db.Store
	scheduler *game.Scheduler
	renderer  *game.Renderer
	commands  []*discordgo.ApplicationCommand
}

func New(token, appID string, mgr *game.Manager, store *db.Store, scheduler *game.Scheduler) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	renderer, err := game.NewRenderer()
	if err != nil {
		return nil, err
	}

	b := &Bot{
		session:   session,
		appID:     appID,
		mgr:       mgr,
		store:     store,
		scheduler: scheduler,
		renderer:  renderer,
	}

	session.AddHandler(b.handleInteraction)

	scheduler.SetCallback(b.onPhaseProcessed)

	return b, nil
}

func (b *Bot) Start() error {
	if err := b.session.Open(); err != nil {
		return err
	}

	b.registerCommands()
	b.scheduler.Start()

	log.Println("Bot started successfully")
	return nil
}

func (b *Bot) Stop() {
	b.scheduler.Stop()
	b.session.Close()
}

func (b *Bot) onPhaseProcessed(g *db.Game, results map[string]string, gameOver bool) {
	if g.ChannelID == "" {
		return
	}

	var content string
	if gameOver {
		content = "**Game Over!** "
	}
	content += "Phase resolved: **" + g.Phase + "**\n\n"

	for prov, result := range results {
		content += provName(prov) + ": " + result + "\n"
	}

	// Render and attach the map
	mapData, filename, err := b.renderer.RenderMapPNG(g.StateJSON)
	if err != nil {
		log.Printf("Failed to render map for game %d: %v", g.ID, err)
		b.session.ChannelMessageSend(g.ChannelID, content)
		return
	}

	// Truncate content if too long for Discord
	if len(content) > 1900 {
		content = content[:1900] + "\n..."
	}

	b.session.ChannelMessageSendComplex(g.ChannelID, &discordgo.MessageSend{
		Content: content,
		Files: []*discordgo.File{
			{
				Name:   filename,
				Reader: bytesReader(mapData),
			},
		},
	})
}
