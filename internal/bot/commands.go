package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) registerCommands() {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "game",
			Description: "Manage Diplomacy games",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "create",
					Description: "Create a new Diplomacy game",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Name for this game",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
						{
							Name:        "turn_duration",
							Description: "Time per turn (e.g. 24h, 48h, 1h)",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    false,
						},
					},
				},
				{
					Name:        "join",
					Description: "Join an existing game",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Game name",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
						{
							Name:        "power",
							Description: "Which power to play",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    false,
							Choices: []*discordgo.ApplicationCommandOptionChoice{
								{Name: "Austria", Value: "Austria"},
								{Name: "England", Value: "England"},
								{Name: "France", Value: "France"},
								{Name: "Germany", Value: "Germany"},
								{Name: "Italy", Value: "Italy"},
								{Name: "Russia", Value: "Russia"},
								{Name: "Turkey", Value: "Turkey"},
							},
						},
					},
				},
				{
					Name:        "start",
					Description: "Start a pending game",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Game name",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
					},
				},
				{
					Name:        "list",
					Description: "List games in this server",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "settings",
					Description: "Update game settings",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Game name",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
						{
							Name:        "turn_duration",
							Description: "New turn duration (e.g. 24h, 12h)",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
					},
				},
				{
					Name:        "export",
					Description: "Export game state as JSON",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Game name (optional if you're in only one game)",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    false,
						},
					},
				},
				{
					Name:        "import",
					Description: "Import a game from a JSON state file",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "name",
							Description: "Name for the new game",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    true,
						},
						{
							Name:        "state_file",
							Description: "JSON state file to import",
							Type:        discordgo.ApplicationCommandOptionAttachment,
							Required:    true,
						},
						{
							Name:        "turn_duration",
							Description: "Time per turn (e.g. 24h, 48h)",
							Type:        discordgo.ApplicationCommandOptionString,
							Required:    false,
						},
					},
				},
			},
		},
		{
			Name:        "order",
			Description: "Submit orders for the current phase",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "orderraw",
			Description: "Submit orders using standard notation",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "orders",
					Description: "Orders in notation (e.g. A PAR - BUR, F BRE - ENG)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "orders",
			Description: "View your currently submitted orders",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "status",
			Description: "Show game status and map",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "history",
			Description: "View a historical phase",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "season",
					Description: "Season",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Spring", Value: "Spring"},
						{Name: "Fall", Value: "Fall"},
						{Name: "Winter", Value: "Winter"},
					},
				},
				{
					Name:        "year",
					Description: "Year (e.g. 1901)",
					Type:        discordgo.ApplicationCommandOptionInteger,
					Required:    false,
				},
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "ready",
			Description: "Mark yourself as ready to advance",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "game",
					Description: "Game name (optional if you're in only one game)",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    false,
				},
			},
		},
		{
			Name:        "rules",
			Description: "Show the rules of Diplomacy",
		},
		{
			Name:        "version",
			Description: "Show bot version and build info",
		},
	}

	wantNames := map[string]bool{}
	for _, cmd := range commands {
		wantNames[cmd.Name] = true
		created, err := b.session.ApplicationCommandCreate(b.appID, "", cmd)
		if err != nil {
			log.Printf("Failed to register command %s: %v", cmd.Name, err)
			continue
		}
		b.commands = append(b.commands, created)
	}

	// Remove stale commands that are no longer defined
	existing, err := b.session.ApplicationCommands(b.appID, "")
	if err == nil {
		for _, cmd := range existing {
			if !wantNames[cmd.Name] {
				log.Printf("Removing stale command: %s", cmd.Name)
				b.session.ApplicationCommandDelete(b.appID, "", cmd.ID)
			}
		}
	}

	log.Printf("Registered %d commands", len(b.commands))
}

func (b *Bot) removeCommands() {
	for _, cmd := range b.commands {
		if err := b.session.ApplicationCommandDelete(b.appID, "", cmd.ID); err != nil {
			log.Printf("Failed to remove command %s: %v", cmd.Name, err)
		}
	}
}
