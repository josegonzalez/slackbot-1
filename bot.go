package slackbot

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nlopes/slack"
	logging "github.com/op/go-logging"
)

type SlackBot struct {
	log *logging.Logger

	token  string
	api    *slack.Client
	rtmAPI *slack.RTM

	// Send Outgoing messages easily
	chSender chan slack.OutgoingMessage

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
		channelMap: make(map[string]string),
		userMap:    make(map[string]string),
	}
}

func (bot *SlackBot) SetDebug(debug bool) {
	fn := slack.OptionDebug(debug)
	fn(bot.api)
	if debug {
		logging.SetLevel(logging.INFO, "slackbot")
	} else {
		logging.SetLevel(logging.ERROR, "slackbot")
	}
}

func (bot *SlackBot) Start(url string) (evChan chan interface{}, err error) {
	bot.rtmAPI = bot.api.NewRTM()
	go bot.rtmAPI.ManageConnection()

	go bot.outgoingSink()

	evChan = make(chan interface{})

	go func() {
		bot.log.Info("Starting event loop")
		for {
			select {
			case msg := <-bot.rtmAPI.IncomingEvents:
				switch msg.Data.(type) {

				case *slack.HelloEvent:
					bot.log.Info("Connected to server")
					evChan <- &HelloEvent{}

				case *slack.MessageEvent:
					ev := msg.Data.(*slack.MessageEvent)
					name := ""
					isbot := false

					if ev.Username == "" {
						name, isbot, err = bot.getUsername(ev.User)
						if err != nil && !isbot {
							evChan <- err
							continue
						}
					} else {
						name = ev.Username
					}

					channel, err := bot.getChannelName(ev.Channel)
					if err != nil {
						evChan <- err
						continue
					}
					text := bot.prettifyMessage(ev.Text)

					evChan <- &MessageEvent{
						Sender:  name,
						Channel: channel,
						Text:    text,
						IsBot:   isbot,
					}
				}
			}
		}
	}()

	return evChan, nil
}

func (bot *SlackBot) SendMessage(from, channel, text string) {
	bot.api.PostMessage(channel, slack.MsgOptionText("Hello World!", false), slack.MsgOptionUsername(from))
}

func (bot *SlackBot) outgoingSink() {
	for {
		select {
		case msg := <-bot.chSender:
			bot.rtmAPI.SendMessage(&msg)
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

func (bot *SlackBot) prettifyMessage(msg string) string {
	re := regexp.MustCompile("<(.*?)>")
	matches := re.FindAllString(msg, -1)

	for _, match := range matches {
		splits := strings.Split(match, "|")
		id := splits[0][1:]
		id = id[1 : len(id)-1] // remove the trailing >
		needle := id[:1]

		if len(splits) == 2 {
			// username of channel inside the text
			name := splits[1]
			name = name[:len(name)-1] // remove the trailing >

			// channels start with C
			if needle == "C" {
				msg = strings.Replace(msg, match, "#"+name, -1)

				// username starts with U
			} else if needle == "U" {
				msg = strings.Replace(msg, match, "@"+name, -1)

			}

		} else if len(splits) == 1 {
			// need to fetch channel/username

			// channel starts with C
			if needle == "C" {
				name, err := bot.getChannelName(id)
				if err != nil {
					fmt.Println("Could not get channel name for", id)
					continue
				}
				msg = strings.Replace(msg, match, "#"+name, -1)

				// username starts with U
			} else if needle == "U" {
				name, _, err := bot.getUsername(id)
				if err != nil {
					fmt.Println("Could not get username for", id)
					continue
				}
				msg = strings.Replace(msg, match, "@"+name, -1)
			}
		}
	}

	return msg
}
