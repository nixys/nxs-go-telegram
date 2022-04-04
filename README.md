# nxs-go-telegram

`Nxs-go-telegram` based on [Go Telegram Bot API](github.com/go-telegram-bot-api/telegram-bot-api) and has built-in sessions allows you to develop complex bots. Your bot may use either `webhook` or `get update` modes.

The main idea of `nxs-go-telegram` based on a states and handlers. Each handler does some actions and returns (switches bot to) a new state. First user interaction with bot starts a session. During a session life cycle bot has a certain state.

General `nxs-go-telegram` components are follows:
- `Queue`
- `Updates chain`
- `Session`

## General components

### Queue

Every `update` from Telegram received by the bot seves in the appropriate queue in Redis. Queues are separated by a `chat ID` and `user ID` got from `updates`. After `update` arrived to bot it puts into queue. Each queue contains one or more `updates` with protected time interval (stored in `meta`). After new `update` adds into the queue this interval increases for specified amount of time. A queue can be processed only after protected time interval has been reached.

In accordance with the selected mode (`webhook` or `get update`) you are able to choose following methods to put obtained `update` into the queue:
- For `webhook` mode
  After new `update` arrived to an endpoint in your app's API use the `tg.UpdateAbsorb()` to add it into queue.
- For `get update` mode
  Use `tg.GetUpdates()` to get available updates from Telegram. This function creates and listens a special channel and calls `tg.UpdateAbsorb()` for every `updates`.

### Updates chain

In order to processing an `update` queues use `Processing()` method for your bot. This method will lookups an available queue with reached protected time interval and wolly extracts into `updates chain`. `Updates chain` will got a type by the first `update` in the queue. All `updates` with different types will be dropped while extracted. After an `updates chain` has been formed it will be processed in accordance with bot description and current session state.

At this time `updates chain` can take the following values:
- `Message`
- `Callback`

### Sessions

Bot's behaviour based on session model and described by different states. As a `queue`, `session` defines by `chat ID` and `user ID` and has the following values:
- `State`: it is a name of stated defined in bot description.
- `Slots`: in other words it is a build-in storage. You may put and get to/from the specified slot any data you want on every state of session. You may operate with `slots` within an any handler.

## Bot description

`Bot description` defines following elements:
- `Commands`
- `States`
- `InitHandler`

Note that it is not recommended to send messages to user directly from any handler.

### Commands

It is a set of Telegram commands that can be used by users. Each `command` has the following properties:
- `Command`: string, defines a command name (excluding leading '/' character).
- `Description`: string, defines a command description.
- `Handler`: function, determines a function that will be done when user execute appropriate command. Handler function does defined actions and returns a new state bot will be switched to.

After your app has been started defined commands will be automatically set for your bot.

### States

Each session `state` has following properties:
- `StateHandler`
- `MessageHandler`
- `CallbackHandler`
- `SentHandler`

Library has two special states you may use within a your handlers:
- `tg.SessStateBreak()`: do not switch a session into new state (stay in a current state).
- `tg.SessStateDestroy()`: destroy session. It will be clear all session data including session state and slots. Bot will go to its original state.

In other cases to switch session to state you want use `tg.SessState(botNewState)`.

#### StateHandler

This handler called when session switched to appropriate state. The main goal of this handler is a prepare message (incl. text and buttons) will be sent to user and define a new session state. If `MessageHandler` defined for state a session will not be switched to a new state and specified new `state` will be ignored.

Note that after any user actions bot will switched its states until goes a state with `break` next state or defined `MessageHandler`.

#### MessageHandler

This handler is called for an appropriate state when user sends a message. After message has been processed handler must returns a new session state.

If this handler is defined bot can change its state only in following cases:
- User send a message to bot
- User click some button
- User execute some defined command

#### CallbackHandler

This handler is called for an appropriate state when user send a callback (click the button). After callback has been processed handler must returns a new session state.

If this handler is not defined bot will ignore any user buttons click for appropriate state.

#### SentHandler

This handler is called for an appropriate state after message prepared in `StateHandler` is sent to user. It useful for get sent messages ID.

### InitHandler

This handler is called when session has not been started yet. The main goal for this handler it's a do some initial actions (eg. check auth or something like) and return a first state session will be swiched to.

## Example of usage

You can find the example of very simple bot below. Bot asks to user several simple questions and sends summary.

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tg "github.com/nixys/nxs-go-telegram"
)

func main() {

	// Example of user context used within the bot handlers
	botCompany := "Nixys"

	// Bot description
	botDescription := tg.Description{
		Commands: []tg.Command{
			{
				Command:     "sayhello",
				Description: "Bot says you hello and begins a conversation",
				Handler:     sayHelloCmd,
			},
			{
				Command:     "destroy",
				Description: "Destroy current session",
				Handler:     destroyCmd,
			},
		},
		InitHandler: botInit,
		States: map[tg.SessionState]tg.State{
			tg.SessState("hello"): {
				StateHandler: helloState,
			},
			tg.SessState("name"): {
				StateHandler:   nameState,
				MessageHandler: nameMsg,
			},
			tg.SessState("gender"): {
				StateHandler:    genderState,
				CallbackHandler: genderCallback,
			},
			tg.SessState("age"): {
				StateHandler:   ageState,
				MessageHandler: ageMsg,
			},
			tg.SessState("info"): {
				StateHandler: infoState,
			},
			tg.SessState("bye"): {
				StateHandler: byeState,
			},
		},
	}

	// Initialize the bot
	bot, err := tg.Init(tg.Settings{
		BotSettings: tg.SettingsBot{
			BotAPI: os.Getenv("BOT_API_TOKEN"),
		},
		RedisHost: os.Getenv("REDIS_HOST"),
	}, botDescription, botCompany)
	if err != nil {
		fmt.Println("bot setup error:", err)
		return
	}

	// Runtime the bot
	runtime(bot)
}

func runtime(bot tg.Telegram) {

	fmt.Println("bot started")

	// Updates runtime
	ctxUpdates, cfUpdates := context.WithCancel(context.Background())
	chUpdates := make(chan error)

	// Queue runtime
	ctxQueue, cfQueue := context.WithCancel(context.Background())
	chQueue := make(chan error)

	// Defer call cancel functions
	defer cfUpdates()
	defer cfQueue()

	go runtimeBotUpdates(ctxUpdates, bot, chUpdates)
	go runtimeBotQueue(ctxQueue, bot, chQueue)

	for {
		select {
		case e := <-chUpdates:
			fmt.Println("got error from runtime update:", e)
			return
		case e := <-chQueue:
			fmt.Println("got error from runtime queue:", e)
			return
		}
	}
}

// runtimeBotUpdates checks updates at Telegram and put it into queue
func runtimeBotUpdates(ctx context.Context, bot tg.Telegram, ch chan error) {
	if err := bot.GetUpdates(ctx); err != nil {
		if err == tg.ErrUpdatesChanClosed {
			ch <- nil
		} else {
			ch <- err
		}
	} else {
		ch <- nil
	}
}

// runtimeBotQueue processes an updaates from queue
func runtimeBotQueue(ctx context.Context, bot tg.Telegram, ch chan error) {
	timer := time.NewTimer(time.Duration(1) * time.Second)
	for {
		select {
		case <-timer.C:
			if err := bot.Processing(); err != nil {
				ch <- err
			}
			timer.Reset(time.Duration(1) * time.Second)
		case <-ctx.Done():
			return
		}
	}
}

// botInit represents InitHandler for bot
func botInit(t *tg.Telegram, s *tg.Session) (tg.InitHandlerRes, error) {
	return tg.InitHandlerRes{
		NextState: tg.SessState("name"),
	}, nil
}

// sayHelloCmd handles a `/sayhello` command
func sayHelloCmd(t *tg.Telegram, s *tg.Session, cmd string, args string) (tg.CommandHandlerRes, error) {
	return tg.CommandHandlerRes{
		NextState: tg.SessState("hello"),
	}, nil
}

// destroyCmd handles a `/destroy` command
func destroyCmd(t *tg.Telegram, s *tg.Session, cmd string, args string) (tg.CommandHandlerRes, error) {
	return tg.CommandHandlerRes{
		NextState: tg.SessState("bye"),
	}, nil
}

// Hello

// helloState represents StateHandler for `hello` state
func helloState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {
	c := t.UsrCtxGet().(string)
	return tg.StateHandlerRes{
		Message:   "Hello! I'm a bot created by " + c + " developers",
		NextState: tg.SessState("name"),
	}, nil
}

// Name

// nameState represents StateHandler for `name` state
func nameState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {
	return tg.StateHandlerRes{
		Message: "Please, enter your name",
	}, nil
}

// nameMsg represents MessageHandler for `name` state
func nameMsg(t *tg.Telegram, s *tg.Session) (tg.MessageHandlerRes, error) {

	m := s.UpdateChain().MessageTextGet()
	if len(m) == 0 {
		return tg.MessageHandlerRes{}, fmt.Errorf("empty message")
	}

	if err := s.SlotSave("name", m[0]); err != nil {
		return tg.MessageHandlerRes{}, err
	}

	return tg.MessageHandlerRes{
		NextState: tg.SessState("gender"),
	}, nil
}

// Gender

// genderState represents StateHandler for `gender` state
func genderState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {
	return tg.StateHandlerRes{
		Message: "Select your gender",
		Buttons: [][]tg.Button{
			{
				{
					Text:       "Male",
					Identifier: "male",
				},
			},
			{
				{
					Text:       "Female",
					Identifier: "female",
				},
			},
		},
		NextState: tg.SessStateBreak(),
	}, nil
}

// genderCallback represents CallbackHandler for `gender` state
func genderCallback(t *tg.Telegram, s *tg.Session, identifier string) (tg.CallbackHandlerRes, error) {
	switch identifier {
	case "male":
		if err := s.SlotSave("gender", "Male"); err != nil {
			return tg.CallbackHandlerRes{}, err
		}
	case "female":
		if err := s.SlotSave("gender", "Female"); err != nil {
			return tg.CallbackHandlerRes{}, err
		}
	}
	return tg.CallbackHandlerRes{
		NextState: tg.SessState("age"),
	}, nil
}

// Age

// ageState represents StateHandler for `age` state
func ageState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {
	return tg.StateHandlerRes{
		Message:      "How old are you?",
		StickMessage: true,
	}, nil
}

// ageMsg represents MessageHandler for `age` state
func ageMsg(t *tg.Telegram, s *tg.Session) (tg.MessageHandlerRes, error) {

	m := s.UpdateChain().MessageTextGet()
	if len(m) == 0 {
		return tg.MessageHandlerRes{}, fmt.Errorf("empty message")
	}

	if err := s.SlotSave("age", m[0]); err != nil {
		return tg.MessageHandlerRes{}, err
	}

	return tg.MessageHandlerRes{
		NextState: tg.SessState("info"),
	}, nil
}

// Info

// infoState represents StateHandler for `info` state
func infoState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {

	var (
		name   string
		gender string
		age    string
	)

	if _, err := s.SlotGet("name", &name); err != nil {
		return tg.StateHandlerRes{}, err
	}

	if _, err := s.SlotGet("gender", &gender); err != nil {
		return tg.StateHandlerRes{}, err
	}

	if _, err := s.SlotGet("age", &age); err != nil {
		return tg.StateHandlerRes{}, err
	}

	m := fmt.Sprintf("Info:\n"+
		"  Name: %s\n"+
		"  Gender: %s\n"+
		"  Age: %s",
		name,
		gender,
		age)

	return tg.StateHandlerRes{
		Message:   m,
		NextState: tg.SessState("bye"),
	}, nil
}

// Bye

// byeState represents StateHandler for `bye` state
func byeState(t *tg.Telegram, s *tg.Session) (tg.StateHandlerRes, error) {
	return tg.StateHandlerRes{
		Message:   "Bye!",
		NextState: tg.SessStateDestroy(),
	}, nil
}
```

Run:

```
go mod init test
go mod tidy
BOT_API_TOKEN="YOUR_BOT_API_TOKEN" REDIS_HOST="YOUR_REDIS_HOST_AND_PORT" go run main.go
```