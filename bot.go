package slackbot

import (
	"fmt"
	"time"

	"github.com/nlopes/slack"
	logging "github.com/op/go-logging"
)

type SlackBot struct {
	log *logging.Logger

	token string
	api   *slack.Slack
	wsAPI *slack.SlackWS

	// Send Outgoing messages easily
	chSender chan slack.OutgoingMessage
	// Get events from Slack api
	chRecv chan slack.SlackEvent

	channelMap map[string]string
	userMap    map[string]string

	// skip these messages as they were sent by us
	msgSkipId map[int]bool
}

func New(token string) *SlackBot {
	return &SlackBot{
		log:        logging.MustGetLogger("slackbot"),
		token:      token,
		api:        slack.New(token),
		chSender:   make(chan slack.OutgoingMessage),
		chRecv:     make(chan slack.SlackEvent),
		channelMap: make(map[string]string),
		userMap:    make(map[string]string),
	}
}

func (bot *SlackBot) SetDebug(debug bool) {
	bot.api.SetDebug(debug)
	if debug {
		logging.SetLevel(logging.INFO, "slackbot")
	} else {
		logging.SetLevel(logging.ERROR, "slackbot")
	}
}

func (bot *SlackBot) Start(url string) (evChan chan interface{}, err error) {
	bot.wsAPI, err = bot.api.StartRTM("", url)
	if err != nil {
		return nil, err
	}

	go bot.wsAPI.HandleIncomingEvents(bot.chRecv)
	go bot.wsAPI.Keepalive(20 * time.Second)

	go bot.outgoingSink()

	evChan = make(chan interface{})

	go func() {
		bot.log.Info("Starting event loop")
		for {
			select {
			case msg := <-bot.chRecv:
				switch msg.Data.(type) {

				case slack.HelloEvent:
					bot.log.Info("Connected to server")
					evChan <- &HelloEvent{}

				case *slack.MessageEvent:
					ev := msg.Data.(*slack.MessageEvent)
					name := ""
					isbot := false
					if ev.Username == "" {
						name, isbot, err = bot.getUsername(ev.UserId)
						if err != nil && !isbot {
							evChan <- err
							continue
						}
					} else {
						name = ev.Username
					}

					channel, err := bot.getChannelName(ev.ChannelId)
					if err != nil {
						evChan <- err
						continue
					}

					evChan <- &MessageEvent{
						Sender:  name,
						Channel: channel,
						Text:    ev.Text,
						IsBot:   isbot,
					}
				}
			}
		}
	}()

	return evChan, nil
}

func (bot *SlackBot) SendMessage(from, channel, text string) {
	params := slack.PostMessageParameters{
		Text:     text,
		Username: from,
	}
	bot.api.PostMessage(channel, text, params)
}

func (bot *SlackBot) outgoingSink() {
	for {
		select {
		case msg := <-bot.chSender:
			bot.wsAPI.SendMessage(&msg)
		}
	}
}

func (bot *SlackBot) getChannelName(channelId string) (string, error) {
	if val, ok := bot.channelMap[channelId]; ok {
		return val, nil
	}

	info, err := bot.api.GetChannelInfo(channelId)
	if err != nil {
		bot.log.Warning("Could not fetch channel info")
		return "", fmt.Errorf("Could not fetch channel name: %s", err.Error())
	}

	bot.channelMap[channelId] = info.Name

	return info.Name, nil
}

func (bot *SlackBot) getUsername(userId string) (name string, isbot bool, err error) {
	isbot = false

	if val, ok := bot.userMap[userId]; ok {
		name = val
		return
	}

	info, err := bot.api.GetUserInfo(userId)
	if err != nil {
		bot.log.Warning("Could not fetch user info")
		err = fmt.Errorf("Could not get username: %s", err.Error())
		return
	}

	if info.IsBot {
		isbot = true
		return
	}

	bot.userMap[userId] = info.Name
	name = info.Name
	return
}
