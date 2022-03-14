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
- `Name`: string, defines a command name (excluding leading '/' character).
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

Coming soon ...
