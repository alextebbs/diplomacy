package bot

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sammy/diplomacy/internal/db"
	"github.com/sammy/diplomacy/internal/game"
	"github.com/zond/godip"
	"github.com/zond/godip/state"
	"github.com/zond/godip/variants/classical"
)

//go:embed rules.md
var rulesText string

type BuildInfo struct {
	BinaryHash string
	GoVersion  string
	GOOS       string
	GOARCH     string
}

var currentBuild = BuildInfo{BinaryHash: "dev"}

func SetBuildInfo(bi BuildInfo) {
	currentBuild = bi
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleCommand(s, i)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, i)
	}
}

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	switch data.Name {
	case "game":
		b.handleGameCommand(s, i, data.Options)
	case "order":
		b.handleInteractiveOrder(s, i, data.Options)
	case "orderraw":
		b.handleOrderRaw(s, i, data.Options)
	case "orders":
		b.handleViewOrders(s, i, data.Options)
	case "status":
		b.handleStatus(s, i, data.Options)
	case "history":
		b.handleHistory(s, i, data.Options)
	case "ready":
		b.handleReady(s, i, data.Options)
	case "rules":
		respond(s, i, rulesText)
	case "version":
		b.handleVersion(s, i)
	}
}

func (b *Bot) handleGameCommand(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if len(options) == 0 {
		respondEphemeral(s, i, "Please specify a subcommand.")
		return
	}

	sub := options[0]
	switch sub.Name {
	case "create":
		b.handleGameCreate(s, i, sub.Options)
	case "join":
		b.handleGameJoin(s, i, sub.Options)
	case "start":
		b.handleGameStart(s, i, sub.Options)
	case "list":
		b.handleGameList(s, i)
	case "settings":
		b.handleGameSettings(s, i, sub.Options)
	case "export":
		b.handleGameExport(s, i, sub.Options)
	case "import":
		b.handleGameImport(s, i, sub.Options)
	}
}

func (b *Bot) handleGameCreate(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	name := getStringOpt(opts, "name")
	durationStr := getStringOpt(opts, "turn_duration")
	if durationStr == "" {
		durationStr = "24h"
	}

	duration, err := parseDuration(durationStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid turn duration: "+err.Error())
		return
	}

	guildID := i.GuildID
	channelID := i.ChannelID

	g, err := b.mgr.CreateGame(guildID, channelID, name, int64(duration.Seconds()))
	if err != nil {
		respondEphemeral(s, i, "Failed to create game: "+err.Error())
		return
	}

	respond(s, i, fmt.Sprintf(
		"Game **%s** created! (Classical Diplomacy, %s turns)\nUse `/game join name:%s` to join.",
		g.Name, durationStr, g.Name,
	))
}

func (b *Bot) handleGameJoin(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "name")
	power := getStringOpt(opts, "power")

	g, err := b.store.GetGameByName(i.GuildID, gameName)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found: "+gameName)
		return
	}

	userID := i.Member.User.ID
	player, err := b.mgr.JoinGame(g.ID, userID, power)
	if err != nil {
		respondEphemeral(s, i, "Failed to join: "+err.Error())
		return
	}

	respond(s, i, fmt.Sprintf("<@%s> joined **%s** as **%s**!", userID, g.Name, player.Power))
}

func (b *Bot) handleGameStart(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "name")

	g, err := b.store.GetGameByName(i.GuildID, gameName)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found: "+gameName)
		return
	}

	// Update channel to where the start command was issued
	b.store.UpdateGameChannel(g.ID, i.ChannelID)

	if err := b.mgr.StartGame(g.ID); err != nil {
		respondEphemeral(s, i, "Failed to start game: "+err.Error())
		return
	}

	// Reload to get the updated deadline
	g, _ = b.store.GetGame(g.ID)
	deadline := ""
	if g.NextDeadline != nil {
		deadline = fmt.Sprintf("\nFirst deadline: <t:%d:R>", g.NextDeadline.Unix())
	}

	respond(s, i, fmt.Sprintf("Game **%s** has started! Phase: **%s**%s\nUse `/order` to submit your orders.", g.Name, g.Phase, deadline))
}

func (b *Bot) handleGameList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	games, err := b.store.ListActiveGamesByGuild(i.GuildID)
	if err != nil {
		respondEphemeral(s, i, "Failed to list games: "+err.Error())
		return
	}

	if len(games) == 0 {
		respondEphemeral(s, i, "No active games in this server. Use `/game create` to start one.")
		return
	}

	var lines []string
	for _, g := range games {
		players, _ := b.store.CountPlayers(g.ID)
		lines = append(lines, fmt.Sprintf("**%s** - %s (%s, %d/7 players)", g.Name, g.Phase, g.Status, players))
	}

	respond(s, i, "**Games in this server:**\n"+strings.Join(lines, "\n"))
}

func (b *Bot) handleStatus(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "game")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	status, err := b.mgr.GetGameStatus(g)
	if err != nil {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Failed to get status: " + err.Error(),
		})
		return
	}

	mapData, filename, err := b.renderer.RenderMapPNG(g.StateJSON)
	if err != nil {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: status,
		})
		return
	}

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: status,
		Files: []*discordgo.File{
			{
				Name:   filename,
				Reader: bytesReader(mapData),
			},
		},
	})
}

func (b *Bot) handleGameSettings(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "name")
	durationStr := getStringOpt(opts, "turn_duration")

	g, err := b.store.GetGameByName(i.GuildID, gameName)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found: "+gameName)
		return
	}

	duration, err := parseDuration(durationStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid turn duration: "+err.Error())
		return
	}

	if err := b.store.UpdateGameSettings(g.ID, int64(duration.Seconds())); err != nil {
		respondEphemeral(s, i, "Failed to update settings: "+err.Error())
		return
	}

	respond(s, i, fmt.Sprintf("Updated **%s** turn duration to **%s**.", g.Name, durationStr))
}

func (b *Bot) handleGameExport(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "name")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	data, err := b.mgr.ExportState(g)
	if err != nil {
		respondEphemeral(s, i, "Failed to export: "+err.Error())
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Exported **%s** (%s)", g.Name, g.Phase),
			Files: []*discordgo.File{
				{
					Name:   g.Name + ".json",
					Reader: bytesReader(data),
				},
			},
		},
	})
}

func (b *Bot) handleGameImport(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "name")
	durationStr := getStringOpt(opts, "turn_duration")
	if durationStr == "" {
		durationStr = "24h"
	}

	duration, err := parseDuration(durationStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid turn duration: "+err.Error())
		return
	}

	// Get the attachment
	data := i.ApplicationCommandData()
	var attachmentID string
	for _, opt := range opts {
		if opt.Name == "state_file" {
			attachmentID = opt.Value.(string)
		}
	}

	attachment, ok := data.Resolved.Attachments[attachmentID]
	if !ok {
		respondEphemeral(s, i, "Could not find the attached file.")
		return
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		respondEphemeral(s, i, "Failed to download attachment: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		respondEphemeral(s, i, "Failed to read attachment: "+err.Error())
		return
	}

	g, err := b.mgr.ImportGame(i.GuildID, i.ChannelID, gameName, int64(duration.Seconds()), buf.Bytes())
	if err != nil {
		respondEphemeral(s, i, "Failed to import: "+err.Error())
		return
	}

	respond(s, i, fmt.Sprintf("Imported game **%s** at phase **%s**.\nUse `/game join name:%s` to join, then `/game start name:%s`.", g.Name, g.Phase, g.Name, g.Name))
}

func (b *Bot) handleOrderRaw(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	ordersStr := getStringOpt(opts, "orders")
	gameName := getStringOpt(opts, "game")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	player, err := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if err != nil || player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	ordersList, err := game.ParseMultipleOrders(ordersStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid orders: "+err.Error())
		return
	}

	// Clear existing orders for this phase
	if err := b.store.ClearOrdersForPlayerPhase(g.ID, g.Phase, i.Member.User.ID); err != nil {
		respondEphemeral(s, i, "Failed to clear previous orders: "+err.Error())
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	parts := strings.Split(ordersStr, ",")
	for idx, tokens := range ordersList {
		orderText := strings.TrimSpace(parts[idx])
		_ = tokens
		o := &db.Order{
			GameID:    g.ID,
			Phase:     g.Phase,
			UserID:    i.Member.User.ID,
			Power:     player.Power,
			OrderText: orderText,
		}
		if _, err := b.store.SubmitOrder(o); err != nil {
			log.Printf("Failed to submit order: %v", err)
			continue
		}
	}

	content, files := b.buildOrdersSummary(g, i.Member.User.ID)

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Files:   files,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

func (b *Bot) handleViewOrders(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "game")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	content, files := b.buildOrdersSummary(g, i.Member.User.ID)

	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Files:   files,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

func (b *Bot) handleHistory(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "game")
	season := getStringOpt(opts, "season")
	year := getIntOpt(opts, "year")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	if season == "" || year == 0 {
		history, err := b.store.GetPhaseHistory(g.ID)
		if err != nil || len(history) == 0 {
			respondEphemeral(s, i, "No history available yet.")
			return
		}

		var opts []discordgo.SelectMenuOption
		seen := map[string]bool{}
		for _, h := range history {
			label := fmt.Sprintf("%s %d %s", h.Season, h.Year, h.PhaseType)
			value := fmt.Sprintf("%s_%d_%s", h.Season, h.Year, h.PhaseType)
			if seen[value] {
				continue
			}
			seen[value] = true
			opts = append(opts, discordgo.SelectMenuOption{
				Label: label,
				Value: value,
			})
		}
		if len(opts) > 25 {
			opts = opts[:25]
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("**%s** — select a phase to view:", g.Name),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.SelectMenu{
								CustomID:    fmt.Sprintf("history_phase_%d", g.ID),
								Placeholder: "Select phase...",
								Options:     opts,
							},
						},
					},
				},
			},
		})
		return
	}

	phases, err := b.store.GetPhaseBySeasonYear(g.ID, season, int(year))
	if err != nil || len(phases) == 0 {
		respondEphemeral(s, i, fmt.Sprintf("No history found for %s %d.", season, year))
		return
	}

	for _, h := range phases {
		content := fmt.Sprintf("**%s** — %s\n", g.Name, h.Phase)

		if h.OrdersJSON != nil {
			content += "\n**Orders:**\n"
			var allOrders map[string][]string
			if err := json.Unmarshal([]byte(*h.OrdersJSON), &allOrders); err == nil {
				for power, orders := range allOrders {
					content += fmt.Sprintf("*%s:*\n", power)
					for _, o := range orders {
						content += "  " + friendlyOrder(o) + "\n"
					}
				}
			}
		}

		ordersJSON := ""
		resultsJSON := ""
		if h.OrdersJSON != nil {
			ordersJSON = *h.OrdersJSON
		}
		if h.ResultsJSON != nil {
			resultsJSON = *h.ResultsJSON
		}

		svgData, err := b.renderer.RenderHistoryMap(h.StateJSON, ordersJSON, resultsJSON)
		if err != nil {
			respond(s, i, content)
			continue
		}

		pngData, pngErr := game.ConvertSVGToPNG(svgData)
		var file *discordgo.File
		if pngErr == nil {
			file = &discordgo.File{Name: "history.png", Reader: bytesReader(pngData)}
		} else {
			file = &discordgo.File{Name: "history.svg", Reader: bytesReader(svgData)}
		}

		if len(content) > 1900 {
			content = content[:1900] + "\n..."
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Files:   []*discordgo.File{file},
			},
		})
		return
	}
}

func (b *Bot) handleHistoryPhaseSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}

	// value format: Season_Year_PhaseType
	parts := strings.SplitN(values[0], "_", 3)
	if len(parts) < 3 {
		return
	}
	season := parts[0]
	var year int
	fmt.Sscanf(parts[1], "%d", &year)
	phaseType := parts[2]

	// gameID from customID: history_phase_<gameID>
	gameIDStr := strings.TrimPrefix(i.MessageComponentData().CustomID, "history_phase_")
	gameID := parseGameID(gameIDStr)

	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}

	h, err := b.store.GetPhaseBySeasonYearType(g.ID, season, year, phaseType)
	if err != nil || h == nil {
		respondEphemeral(s, i, fmt.Sprintf("No history found for %s %d %s.", season, year, phaseType))
		return
	}

	content := fmt.Sprintf("**%s** — %s %d %s\n", g.Name, h.Season, h.Year, h.PhaseType)

	if h.OrdersJSON != nil {
		content += "\n**Orders:**\n"
		var allOrders map[string][]string
		if err := json.Unmarshal([]byte(*h.OrdersJSON), &allOrders); err == nil {
			for power, orders := range allOrders {
				content += fmt.Sprintf("*%s:*\n", power)
				for _, o := range orders {
					content += "  " + friendlyOrder(o) + "\n"
				}
			}
		}
	}

	ordersJSON := ""
	resultsJSON := ""
	if h.OrdersJSON != nil {
		ordersJSON = *h.OrdersJSON
	}
	if h.ResultsJSON != nil {
		resultsJSON = *h.ResultsJSON
	}

	svgData, err := b.renderer.RenderHistoryMap(h.StateJSON, ordersJSON, resultsJSON)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: content,
			},
		})
		return
	}

	pngData, pngErr := game.ConvertSVGToPNG(svgData)
	var file *discordgo.File
	if pngErr == nil {
		file = &discordgo.File{Name: "history.png", Reader: bytesReader(pngData)}
	} else {
		file = &discordgo.File{Name: "history.svg", Reader: bytesReader(svgData)}
	}

	if len(content) > 1900 {
		content = content[:1900] + "\n..."
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Files:      []*discordgo.File{file},
			Components: []discordgo.MessageComponent{},
		},
	})
}

func (b *Bot) handleReady(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "game")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	if err := b.store.SetPlayerReady(g.ID, i.Member.User.ID, true); err != nil {
		respondEphemeral(s, i, "Failed to set ready: "+err.Error())
		return
	}

	respond(s, i, fmt.Sprintf("<@%s> is **ready** in **%s**!", i.Member.User.ID, g.Name))

	// Check if all players are ready
	adjudicated, err := b.scheduler.CheckReady(g.ID)
	if err != nil {
		log.Printf("Error checking ready state: %v", err)
	}
	if adjudicated {
		// The callback will handle posting results
		log.Printf("All players ready in game %d, adjudicated early", g.ID)
	}
}

func (b *Bot) handleVersion(s *discordgo.Session, i *discordgo.InteractionCreate) {
	bi := currentBuild
	msg := fmt.Sprintf("**Diplomacy Bot**\nBinary SHA-256:\n```\n%s\n```\nGo: `%s` | OS/Arch: `%s/%s`",
		bi.BinaryHash, bi.GoVersion, bi.GOOS, bi.GOARCH)

	msg += "\n\nTo verify:\n" +
		"```\ngit clone https://github.com/alextebbs/diplomacy.git\ncd diplomacy\n./verify.sh " + bi.BinaryHash + "\n```"

	respond(s, i, msg)
}

// handleInteractiveOrder starts the interactive order submission flow
func (b *Bot) handleInteractiveOrder(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameName := getStringOpt(opts, "game")

	g, err := b.mgr.ResolveGame(i.GuildID, i.Member.User.ID, gameName)
	if err != nil {
		respondEphemeral(s, i, err.Error())
		return
	}

	player, err := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if err != nil || player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	state, err := game.DeserializeState(g.StateJSON)
	if err != nil {
		respondEphemeral(s, i, "Failed to load game state.")
		return
	}

	options := game.GetPossibleOrders(state, godip.Nation(player.Power))
	if options == nil {
		respondEphemeral(s, i, "No orders available for you this phase.")
		return
	}

	// Build unit selection menu
	units, _, _, _, _, _ := state.Dump()
	var menuOptions []discordgo.SelectMenuOption
	for prov, unit := range units {
		if unit.Nation != godip.Nation(player.Power) {
			continue
		}
		label := fmt.Sprintf("%s in %s", unit.Type, provName(string(prov)))
		menuOptions = append(menuOptions, discordgo.SelectMenuOption{
			Label: label,
			Value: string(prov),
		})
	}

	if len(menuOptions) == 0 {
		respondEphemeral(s, i, "You have no units to order this phase.")
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s** - %s\nSelect a unit to order:", g.Name, g.Phase),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("order_unit_%d", g.ID),
							Placeholder: "Select a unit...",
							Options:     menuOptions,
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

func (b *Bot) handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	switch {
	case strings.HasPrefix(customID, "order_unit_"):
		b.handleUnitSelection(s, i)
	case strings.HasPrefix(customID, "order_action_"):
		b.handleActionSelection(s, i)
	case strings.HasPrefix(customID, "order_dest_"):
		b.handleDestSelection(s, i)
	case strings.HasPrefix(customID, "order_suptgt_"):
		b.handleSupportTargetSelection(s, i)
	case strings.HasPrefix(customID, "order_supact_"):
		b.handleSupportActionSelection(s, i)
	case strings.HasPrefix(customID, "order_supdst_"):
		b.handleSupportDestSelection(s, i)
	case strings.HasPrefix(customID, "order_cvytgt_"):
		b.handleConvoyTargetSelection(s, i)
	case strings.HasPrefix(customID, "order_cvydst_"):
		b.handleConvoyDestSelection(s, i)
	case strings.HasPrefix(customID, "history_phase_"):
		b.handleHistoryPhaseSelection(s, i)
	}
}

func (b *Bot) handleUnitSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}

	province := values[0]
	customID := i.MessageComponentData().CustomID
	gameIDStr := strings.TrimPrefix(customID, "order_unit_")

	// Offer order type selection
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Unit at **%s** - choose an action:", provName(province)),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("order_action_%s_%s", gameIDStr, province),
							Placeholder: "Select action...",
							Options: []discordgo.SelectMenuOption{
								{Label: "Hold", Value: "Hold"},
								{Label: "Move", Value: "Move"},
								{Label: "Support", Value: "Support"},
								{Label: "Convoy", Value: "Convoy"},
							},
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

func (b *Bot) handleActionSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}

	action := values[0]
	customID := i.MessageComponentData().CustomID
	parts := strings.SplitN(strings.TrimPrefix(customID, "order_action_"), "_", 2)
	if len(parts) < 2 {
		return
	}
	gameIDStr := parts[0]
	province := parts[1]

	if action == "Hold" {
		b.submitInteractiveOrder(s, i, gameIDStr, province, "Hold", "")
		return
	}

	if action == "Move" {
		// Build destination menu from valid options
		gameID := parseGameID(gameIDStr)
		g, err := b.store.GetGame(gameID)
		if err != nil || g == nil {
			respondEphemeral(s, i, "Game not found.")
			return
		}

		st, err := game.DeserializeState(g.StateJSON)
		if err != nil {
			respondEphemeral(s, i, "Failed to load game state.")
			return
		}

		player, _ := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
		if player == nil {
			respondEphemeral(s, i, "You are not in this game.")
			return
		}

		dests := getMoveDestinations(st, godip.Nation(player.Power), godip.Province(province))
		if len(dests) == 0 {
			respondEphemeral(s, i, fmt.Sprintf("No valid move destinations for unit at %s.", provName(province)))
			return
		}

		var destOptions []discordgo.SelectMenuOption
		for _, d := range dests {
			destOptions = append(destOptions, discordgo.SelectMenuOption{
				Label: provName(string(d)),
				Value: string(d),
			})
		}

		// Discord select menus max out at 25 options
		if len(destOptions) > 25 {
			destOptions = destOptions[:25]
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Move **%s** to:", provName(province)),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.SelectMenu{
								CustomID:    fmt.Sprintf("order_dest_%s_%s_Move", gameIDStr, province),
								Placeholder: "Select destination...",
								Options:     destOptions,
							},
						},
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if action == "Support" {
		gameID := parseGameID(gameIDStr)
		g, err := b.store.GetGame(gameID)
		if err != nil || g == nil {
			respondEphemeral(s, i, "Game not found.")
			return
		}
		st, err := game.DeserializeState(g.StateJSON)
		if err != nil {
			respondEphemeral(s, i, "Failed to load game state.")
			return
		}

		// Show all neighboring units that this unit could support
		targets := getSupportableUnits(st, godip.Province(province))
		if len(targets) == 0 {
			respondEphemeral(s, i, fmt.Sprintf("No units nearby to support from %s.", provName(province)))
			return
		}

		var opts []discordgo.SelectMenuOption
		for _, t := range targets {
			opts = append(opts, discordgo.SelectMenuOption{
				Label: fmt.Sprintf("%s in %s", t.Type, provName(string(t.Province))),
				Value: string(t.Province),
			})
		}
		if len(opts) > 25 {
			opts = opts[:25]
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("**%s** supports — which unit?", provName(province)),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.SelectMenu{
								CustomID:    fmt.Sprintf("order_suptgt_%s_%s", gameIDStr, province),
								Placeholder: "Select unit to support...",
								Options:     opts,
							},
						},
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if action == "Convoy" {
		gameID := parseGameID(gameIDStr)
		g, err := b.store.GetGame(gameID)
		if err != nil || g == nil {
			respondEphemeral(s, i, "Game not found.")
			return
		}
		st, err := game.DeserializeState(g.StateJSON)
		if err != nil {
			respondEphemeral(s, i, "Failed to load game state.")
			return
		}

		armies := getConvoyableArmies(st, godip.Province(province))
		if len(armies) == 0 {
			respondEphemeral(s, i, fmt.Sprintf("No armies can be convoyed by fleet at %s.", provName(province)))
			return
		}

		var opts []discordgo.SelectMenuOption
		for _, a := range armies {
			opts = append(opts, discordgo.SelectMenuOption{
				Label: fmt.Sprintf("Army in %s", provName(string(a))),
				Value: string(a),
			})
		}
		if len(opts) > 25 {
			opts = opts[:25]
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("**Fleet %s** convoys — which army?", provName(province)),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.SelectMenu{
								CustomID:    fmt.Sprintf("order_cvytgt_%s_%s", gameIDStr, province),
								Placeholder: "Select army to convoy...",
								Options:     opts,
							},
						},
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
}

func (b *Bot) handleDestSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}

	dest := values[0]
	customID := i.MessageComponentData().CustomID
	// Format: order_dest_<gameID>_<province>_<action>
	trimmed := strings.TrimPrefix(customID, "order_dest_")
	parts := strings.SplitN(trimmed, "_", 3)
	if len(parts) < 3 {
		return
	}
	gameIDStr := parts[0]
	province := parts[1]

	b.submitInteractiveOrder(s, i, gameIDStr, province, "Move", dest)
}

// Support flow: user picked which unit to support
func (b *Bot) handleSupportTargetSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	targetProv := values[0]
	trimmed := strings.TrimPrefix(i.MessageComponentData().CustomID, "order_suptgt_")
	parts := strings.SplitN(trimmed, "_", 2)
	if len(parts) < 2 {
		return
	}
	gameIDStr := parts[0]
	srcProv := parts[1]

	// Ask: support hold or support move?
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s** supports **%s** — hold or move?", provName(srcProv), provName(targetProv)),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("order_supact_%s_%s_%s", gameIDStr, srcProv, targetProv),
							Placeholder: "Support hold or support move?",
							Options: []discordgo.SelectMenuOption{
								{Label: "Support Hold", Value: "hold"},
								{Label: "Support Move", Value: "move"},
							},
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

// Support flow: user picked hold vs move
func (b *Bot) handleSupportActionSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	supAction := values[0]
	// Format: order_supact_<gameID>_<src>_<target>
	trimmed := strings.TrimPrefix(i.MessageComponentData().CustomID, "order_supact_")
	parts := strings.SplitN(trimmed, "_", 3)
	if len(parts) < 3 {
		return
	}
	gameIDStr := parts[0]
	srcProv := parts[1]
	targetProv := parts[2]

	if supAction == "hold" {
		b.submitSupportOrder(s, i, gameIDStr, srcProv, targetProv, "")
		return
	}

	// Support move — need to pick where the target is moving to
	gameID := parseGameID(gameIDStr)
	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}
	st, err := game.DeserializeState(g.StateJSON)
	if err != nil {
		respondEphemeral(s, i, "Failed to load game state.")
		return
	}

	units, _, _, _, _, _ := st.Dump()
	targetUnit, ok := units[godip.Province(targetProv)]
	if !ok {
		targetUnit, ok = units[godip.Province(targetProv).Super()]
	}
	var nation godip.Nation
	if ok {
		nation = targetUnit.Nation
	}

	dests := getMoveDestinations(st, nation, godip.Province(targetProv))
	if len(dests) == 0 {
		respondEphemeral(s, i, fmt.Sprintf("No valid move destinations for unit at %s.", provName(targetProv)))
		return
	}

	var opts []discordgo.SelectMenuOption
	for _, d := range dests {
		opts = append(opts, discordgo.SelectMenuOption{
			Label: provName(string(d)),
			Value: string(d),
		})
	}
	if len(opts) > 25 {
		opts = opts[:25]
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**%s** supports **%s** moving to:", provName(srcProv), provName(targetProv)),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("order_supdst_%s_%s_%s", gameIDStr, srcProv, targetProv),
							Placeholder: "Select destination...",
							Options:     opts,
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

// Support flow: user picked the move destination
func (b *Bot) handleSupportDestSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	dest := values[0]
	trimmed := strings.TrimPrefix(i.MessageComponentData().CustomID, "order_supdst_")
	parts := strings.SplitN(trimmed, "_", 3)
	if len(parts) < 3 {
		return
	}
	gameIDStr := parts[0]
	srcProv := parts[1]
	targetProv := parts[2]

	b.submitSupportOrder(s, i, gameIDStr, srcProv, targetProv, dest)
}

// Convoy flow: user picked which army to convoy
func (b *Bot) handleConvoyTargetSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	armyProv := values[0]
	trimmed := strings.TrimPrefix(i.MessageComponentData().CustomID, "order_cvytgt_")
	parts := strings.SplitN(trimmed, "_", 2)
	if len(parts) < 2 {
		return
	}
	gameIDStr := parts[0]
	fleetProv := parts[1]

	// Get valid convoy destinations for this army via this fleet
	gameID := parseGameID(gameIDStr)
	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}
	st, err := game.DeserializeState(g.StateJSON)
	if err != nil {
		respondEphemeral(s, i, "Failed to load game state.")
		return
	}

	player, _ := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	dests := getConvoyDestinations(st, godip.Nation(player.Power), godip.Province(fleetProv), godip.Province(armyProv))
	if len(dests) == 0 {
		respondEphemeral(s, i, "No valid convoy destinations.")
		return
	}

	var opts []discordgo.SelectMenuOption
	for _, d := range dests {
		opts = append(opts, discordgo.SelectMenuOption{
			Label: provName(string(d)),
			Value: string(d),
		})
	}
	if len(opts) > 25 {
		opts = opts[:25]
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("**Fleet %s** convoys **Army %s** to:", provName(fleetProv), provName(armyProv)),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    fmt.Sprintf("order_cvydst_%s_%s_%s", gameIDStr, fleetProv, armyProv),
							Placeholder: "Select destination...",
							Options:     opts,
						},
					},
				},
			},
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
}

// Convoy flow: user picked the destination
func (b *Bot) handleConvoyDestSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		return
	}
	dest := values[0]
	trimmed := strings.TrimPrefix(i.MessageComponentData().CustomID, "order_cvydst_")
	parts := strings.SplitN(trimmed, "_", 3)
	if len(parts) < 3 {
		return
	}
	gameIDStr := parts[0]
	fleetProv := parts[1]
	armyProv := parts[2]

	b.submitConvoyOrder(s, i, gameIDStr, fleetProv, armyProv, dest)
}

func (b *Bot) submitSupportOrder(s *discordgo.Session, i *discordgo.InteractionCreate, gameIDStr, srcProv, targetProv, dest string) {
	gameID := parseGameID(gameIDStr)
	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}
	st, err := game.DeserializeState(g.StateJSON)
	if err != nil {
		respondEphemeral(s, i, "Failed to load game state.")
		return
	}
	player, _ := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	srcUnit := getUnitType(st, godip.Province(srcProv))
	targetUnit := getUnitType(st, godip.Province(targetProv))

	var orderText string
	if dest == "" {
		orderText = fmt.Sprintf("%s %s S %s %s", srcUnit, strings.ToUpper(srcProv), targetUnit, strings.ToUpper(targetProv))
	} else {
		orderText = fmt.Sprintf("%s %s S %s %s - %s", srcUnit, strings.ToUpper(srcProv), targetUnit, strings.ToUpper(targetProv), strings.ToUpper(dest))
	}

	b.submitOrderText(s, i, g, player, orderText)
}

func (b *Bot) submitConvoyOrder(s *discordgo.Session, i *discordgo.InteractionCreate, gameIDStr, fleetProv, armyProv, dest string) {
	gameID := parseGameID(gameIDStr)
	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}
	player, _ := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	orderText := fmt.Sprintf("F %s C A %s - %s", strings.ToUpper(fleetProv), strings.ToUpper(armyProv), strings.ToUpper(dest))
	b.submitOrderText(s, i, g, player, orderText)
}

// submitOrderText is the shared submission path for all interactive orders.
func (b *Bot) submitOrderText(s *discordgo.Session, i *discordgo.InteractionCreate, g *db.Game, player *db.Player, orderText string) {
	o := &db.Order{
		GameID:    g.ID,
		Phase:     g.Phase,
		UserID:    i.Member.User.ID,
		Power:     player.Power,
		OrderText: orderText,
	}

	if _, err := b.store.SubmitOrder(o); err != nil {
		respondEphemeral(s, i, "Failed to submit order: "+err.Error())
		return
	}

	content, files := b.buildOrdersSummary(g, i.Member.User.ID)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Files:      files,
			Components: []discordgo.MessageComponent{},
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

// buildOrdersSummary returns the text + map image for all of a player's current orders.
func (b *Bot) buildOrdersSummary(g *db.Game, userID string) (string, []*discordgo.File) {
	orders, err := b.store.GetOrdersForPlayerPhase(g.ID, g.Phase, userID)
	if err != nil || len(orders) == 0 {
		return fmt.Sprintf("Your orders for **%s** (%s):\n_(none)_", g.Name, g.Phase), nil
	}

	var lines []string
	var orderStrings []string
	for _, o := range orders {
		lines = append(lines, "- "+friendlyOrder(o.OrderText))
		orderStrings = append(orderStrings, o.OrderText)
	}
	content := fmt.Sprintf("Your orders for **%s** (%s):\n%s", g.Name, g.Phase, strings.Join(lines, "\n"))

	var files []*discordgo.File
	svgData, err := b.renderer.RenderMapWithOrders(g.StateJSON, orderStrings)
	if err == nil {
		pngData, pngErr := game.ConvertSVGToPNG(svgData)
		if pngErr == nil {
			files = []*discordgo.File{{Name: "orders.png", Reader: bytesReader(pngData)}}
		} else {
			files = []*discordgo.File{{Name: "orders.svg", Reader: bytesReader(svgData)}}
		}
	}

	return content, files
}

func (b *Bot) submitInteractiveOrder(s *discordgo.Session, i *discordgo.InteractionCreate, gameIDStr, province, action, dest string) {
	gameID := parseGameID(gameIDStr)
	g, err := b.store.GetGame(gameID)
	if err != nil || g == nil {
		respondEphemeral(s, i, "Game not found.")
		return
	}

	st, err := game.DeserializeState(g.StateJSON)
	if err != nil {
		respondEphemeral(s, i, "Failed to load game state.")
		return
	}

	player, err := b.store.GetPlayerByUserAndGame(g.ID, i.Member.User.ID)
	if err != nil || player == nil {
		respondEphemeral(s, i, "You are not in this game.")
		return
	}

	unitType := getUnitType(st, godip.Province(province))

	var orderText string
	switch action {
	case "Hold":
		orderText = fmt.Sprintf("%s %s H", unitType, strings.ToUpper(province))
	case "Move":
		orderText = fmt.Sprintf("%s %s - %s", unitType, strings.ToUpper(province), strings.ToUpper(dest))
	default:
		orderText = fmt.Sprintf("%s %s H", unitType, strings.ToUpper(province))
	}

	b.submitOrderText(s, i, g, player, orderText)
}

// Helpers

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	})
}

func getStringOpt(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

func getIntOpt(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) int64 {
	for _, o := range opts {
		if o.Name == name {
			return o.IntValue()
		}
	}
	return 0
}

// getUnitType returns "A" or "F" for the unit at the given province.
func getUnitType(s *state.State, prov godip.Province) string {
	units, _, _, _, _, _ := s.Dump()
	if u, ok := units[prov]; ok {
		if u.Type == godip.Fleet {
			return "F"
		}
		return "A"
	}
	if u, ok := units[prov.Super()]; ok {
		if u.Type == godip.Fleet {
			return "F"
		}
		return "A"
	}
	return "A"
}

type unitInfo struct {
	Province godip.Province
	Type     godip.UnitType
	Nation   godip.Nation
}

// getSupportableUnits returns all units adjacent to src that could be supported.
func getSupportableUnits(s *state.State, src godip.Province) []unitInfo {
	units, _, _, _, _, _ := s.Dump()
	edges := s.Graph().Edges(src, false)
	var result []unitInfo
	seen := map[godip.Province]bool{}

	for prov, unit := range units {
		if prov.Super() == src.Super() {
			continue
		}
		provSuper := prov.Super()
		if seen[provSuper] {
			continue
		}
		// Check if src is adjacent to this unit's province
		for neighbor := range edges {
			if neighbor.Super() == provSuper {
				seen[provSuper] = true
				result = append(result, unitInfo{Province: prov, Type: unit.Type, Nation: unit.Nation})
				break
			}
		}
	}
	return result
}

// getConvoyableArmies returns armies that the fleet at fleetProv can convoy.
func getConvoyableArmies(s *state.State, fleetProv godip.Province) []godip.Province {
	units, _, _, _, _, _ := s.Dump()
	edges := s.Graph().Edges(fleetProv, false)
	var result []godip.Province

	for prov, unit := range units {
		if unit.Type != godip.Army {
			continue
		}
		for neighbor := range edges {
			if neighbor.Super() == prov.Super() {
				result = append(result, prov)
				break
			}
		}
	}
	return result
}

// getConvoyDestinations returns valid destinations for an army convoyed by the given fleet.
// Options tree for Convoy: Province → "Convoy" → SrcProvince → convoyed unit province → destination
func getConvoyDestinations(s *state.State, nation godip.Nation, fleetProv, armyProv godip.Province) []godip.Province {
	opts := s.Phase().Options(s, nation)
	if opts == nil {
		return nil
	}

	provOpts, ok := opts[fleetProv]
	if !ok {
		provOpts, ok = opts[fleetProv.Super()]
		if !ok {
			return nil
		}
	}

	var dests []godip.Province
	for orderType, srcMap := range provOpts {
		orderName, ok := orderType.(godip.OrderType)
		if !ok || string(orderName) != "Convoy" {
			continue
		}
		for _, convoyMap := range srcMap {
			if convoyMap == nil {
				continue
			}
			for unitKey, destMap := range convoyMap {
				up, ok := unitKey.(godip.Province)
				if !ok || up.Super() != armyProv.Super() {
					continue
				}
				if destMap == nil {
					continue
				}
				for destKey := range destMap {
					if d, ok := destKey.(godip.Province); ok {
						dests = append(dests, d)
					}
				}
			}
		}
	}
	return dests
}

func parseGameID(s string) int64 {
	var id int64
	fmt.Sscanf(s, "%d", &id)
	return id
}

// getMoveDestinations extracts valid move destinations for a unit from godip's Options tree.
// The Options tree is: Province → OrderType → SrcProvince → Destination
func getMoveDestinations(s *state.State, nation godip.Nation, src godip.Province) []godip.Province {
	opts := s.Phase().Options(s, nation)
	if opts == nil {
		return nil
	}

	provOpts, ok := opts[src]
	if !ok {
		provOpts, ok = opts[src.Super()]
		if !ok {
			return nil
		}
	}

	var dests []godip.Province
	for orderType, srcMap := range provOpts {
		orderName, ok := orderType.(godip.OrderType)
		if !ok {
			continue
		}
		if string(orderName) != "Move" {
			continue
		}
		for _, destMap := range srcMap {
			if destMap == nil {
				continue
			}
			for destKey := range destMap {
				if d, ok := destKey.(godip.Province); ok {
					dests = append(dests, d)
				}
			}
		}
	}
	return dests
}

// provName returns the friendly name for a province abbreviation,
// falling back to the uppercase abbreviation if not found.
func provName(prov string) string {
	name, ok := classical.ClassicalVariant.ProvinceLongNames[godip.Province(strings.ToLower(prov))]
	if ok {
		return name
	}
	// Try the super province (strip coast suffix)
	p := godip.Province(strings.ToLower(prov))
	name, ok = classical.ClassicalVariant.ProvinceLongNames[p.Super()]
	if ok {
		return name
	}
	return strings.ToUpper(prov)
}

// friendlyOrder converts a raw order like "A PAR - BUR" into
// "Army Paris → Burgundy" for display to users.
func friendlyOrder(raw string) string {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return raw
	}

	unitTypes := map[string]string{"A": "Army", "F": "Fleet"}
	unitName := unitTypes[parts[0]]
	if unitName == "" {
		return raw
	}

	if len(parts) < 2 {
		return raw
	}

	src := provName(parts[1])

	if len(parts) == 3 && parts[2] == "H" {
		return fmt.Sprintf("%s %s Hold", unitName, src)
	}
	if len(parts) >= 4 && parts[2] == "-" {
		dst := provName(parts[3])
		return fmt.Sprintf("%s %s → %s", unitName, src, dst)
	}
	if len(parts) >= 4 && parts[2] == "S" {
		rest := make([]string, 0, len(parts)-3)
		for _, p := range parts[3:] {
			if p == "-" {
				rest = append(rest, "→")
			} else if unitTypes[p] != "" {
				rest = append(rest, unitTypes[p])
			} else {
				rest = append(rest, provName(p))
			}
		}
		return fmt.Sprintf("%s %s Support %s", unitName, src, strings.Join(rest, " "))
	}
	if len(parts) >= 4 && parts[2] == "C" {
		rest := make([]string, 0, len(parts)-3)
		for _, p := range parts[3:] {
			if p == "-" {
				rest = append(rest, "→")
			} else if unitTypes[p] != "" {
				rest = append(rest, unitTypes[p])
			} else {
				rest = append(rest, provName(p))
			}
		}
		return fmt.Sprintf("%s %s Convoy %s", unitName, src, strings.Join(rest, " "))
	}

	return raw
}

func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid duration (use e.g. 24h, 12h, 1h30m)", s)
	}
	if d < 1*time.Minute {
		return 0, fmt.Errorf("turn duration must be at least 1 minute")
	}
	return d, nil
}
