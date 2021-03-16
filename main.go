package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"launchpad.net/xmlpath"
)

func mustEnv(name string) (string, bool) {
	value := os.Getenv(name)
	if value == "" {
		log.Println("must env ", name)
		return "", false
	}
	return value, true
}

func main() {
	isValid := true

	botToken, ok := mustEnv("SLACK_POKEMON_DICT_BOT_TOKEN")
	if !ok {
		isValid = false
	}
	appToken, ok := mustEnv("SLACK_POKEMON_DICT_APP_TOKEN")
	if !ok {
		isValid = false
	}
	if !isValid {
		return
	}

	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	client := socketmode.New(
		api,
	)
	go runner(api, client)

	fmt.Println("[INFO] slack-police")
	fmt.Println("[INFO] run websocket")
	client.Run()

}

func runner(api *slack.Client, client *socketmode.Client) {
	for evt := range client.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			fmt.Println("Connecting to Slack with Socket Mode...")
		case socketmode.EventTypeConnectionError:
			fmt.Println("Connection failed. Retrying later...")
		case socketmode.EventTypeConnected:
			fmt.Println("Connected to Slack with Socket Mode.")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				fmt.Printf("Ignored %+v\n", evt)
				continue
			}
			client.Ack(*evt.Request)

			switch eventsAPIEvent.Type {
			case slackevents.CallbackEvent:
				procInnerEvent(api, eventsAPIEvent.InnerEvent)
			}
		}
	}
}

func procInnerEvent(api *slack.Client, event slackevents.EventsAPIInnerEvent) {
	log.Println("received: ", event.Type)
	rgxIgnoreID := regexp.MustCompile(`<.*?>`)
	rgx := regexp.MustCompile(`[0-9]+`)

	switch ev := event.Data.(type) {
	case *slackevents.MessageEvent:
		log.Println("MessageEvent")
		if ev.BotID != "" {
			break
		}

		log.Println(ev.Text)
		b := []byte(ev.Text)
		text := rgxIgnoreID.ReplaceAll(b, []byte(""))
		idstr := fmt.Sprintf("%s", rgx.Find(text))
		if idstr == "" {
			break
		}

		id, err := strconv.Atoi(idstr)
		if err != nil {
			log.Println(err.Error())
			break
		}

		imageUrl, err := getPokemon(id)
		if err != nil {
			log.Println(err.Error())
			break
		}

		log.Println(
			api.PostMessage(
				ev.Channel,
				slack.MsgOptionText(
					fmt.Sprintf("%s", imageUrl),
					false,
				),
			),
		)
	}
}

func getPokemon(id int) (string, error) {
	url := fmt.Sprintf("https://zukan.pokemon.co.jp/detail/%03d", id)
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err.Error())
		return "", err
	}
	defer resp.Body.Close()

	path := xmlpath.MustCompile(`//*[@id="json-data"]`)
	content, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		log.Println(err.Error())
		return "", err
	}

	jsonBytes, ok := path.Bytes(content)
	if !ok {
		return "", fmt.Errorf("path.Bytes(content)")
	}

	var pokemon = PokemonDictData{}
	if err := json.Unmarshal(jsonBytes, &pokemon); err != nil {
		log.Println(err.Error())
		return "", err
	}
	return pokemon.Pokemon.ImageSmall, nil
}

type PokemonDictData struct {
	Pokemon Pokemon `json:"pokemon"`
}

type Pokemon struct {
	ImageLarge  string `json:"image_l"`
	ImageMidium string `json:"image_m"`
	ImageSmall  string `json:"image_s"`
}
