package slackbot

type (
	// Sucessfully Connected Event
	HelloEvent struct {
	}

	// Event to notify a message in slack
	MessageEvent struct {
		Sender  string
		Channel string
		Text    string
		IsBot   bool
	}
)
