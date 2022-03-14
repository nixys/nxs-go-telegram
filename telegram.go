package tg

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"
)

// MessageSent it's an alias for tgbotapi.Message
type MessageSent tgbotapi.Message

// CommandHandler defines command handler type
type CommandHandler func(t *Telegram, uc UpdateChain, cmd string, args string) (CommandHandlerRes, error)

// Telegram it is a module context structure
type Telegram struct {
	bot             *tgbotapi.BotAPI
	description     Description
	session         session
	usrCtx          interface{}
	redisHost       string
	updateQueueWait time.Duration
}

// Settings contains data to setting up bot
type Settings struct {
	BotSettings     SettingsBot
	RedisHost       string
	UpdateQueueWait time.Duration
}

// SettingsBot contains settings for Telegram bot
type SettingsBot struct {
	BotAPI  string
	Webhook *SettingsBotWebhook
	Proxy   *SettingsBotProxy
}

// SettingsBotWebhook contains settings to set Telegram webhook
type SettingsBotWebhook struct {
	URL      string
	BotToken string
	CertFile string
	WithCert bool
}

// SettingsBotProxy contains proxy settings for Telegram bot
type SettingsBotProxy struct {
	Type     string
	Host     string
	Login    string
	Password string
}

// Description describes bot
type Description struct {

	// Commands contains Telegram commands available for bot.
	// Each record in map it's a command name (without leading
	// '/' character) and its handler
	Commands map[string]CommandMeta

	// States contains the states description.
	// Map key it's a state name that must be set with the
	// tg.SessState() function
	States map[SessionState]State

	// InitHandler is a handler to processing Telegram updates
	// when session has not been started yet.
	// This element returns only next state.
	InitHandler func(t *Telegram, uc UpdateChain) (InitHandlerRes, error)
}

// InitHandlerRes contains data returned by the InitHandler
type InitHandlerRes struct {

	// New state to switch the session.
	// All values of NextState must exist in States map
	// within the bot description
	NextState SessionState
}

// StateHandlerRes contains data returned by the StateHandler
type StateHandlerRes struct {

	// Message contains message text to be sent to user.
	// Message can not be zero length
	Message string

	// Buttons contains buttons for message to be sent to user.
	// If Buttons has zero length message will not contains buttons
	Buttons [][]Button

	// NextState defines next state for current session.
	// NextState will be ignored if MessageHandler defined for state
	NextState SessionState

	// Whether or not stick message. If true appropriate message will
	// be updated when a new state initiate by the `update` of callback type
	StickMessage bool
}

// MessageHandlerRes contains data returned by the MessageHandler
type MessageHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// CallbackHandlerRes contains data returned by the CallbackHandler
type CallbackHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// CommandHandlerRes contains data returned by the CommandHandler
type CommandHandlerRes struct {

	// NextState contains next session state
	NextState SessionState
}

// CommandMeta contains data for command
type CommandMeta struct {
	Description string
	Handler     CommandHandler
}

// State contains session state description
type State struct {

	// Handler to processing new bot state.
	StateHandler func(t *Telegram) (StateHandlerRes, error)

	// Handler to processing messages received from user
	MessageHandler func(t *Telegram, uc UpdateChain) (MessageHandlerRes, error)

	// Handler to processing callbacks received from user for specific state of session
	CallbackHandler func(t *Telegram, uc UpdateChain, identifier string) (CallbackHandlerRes, error)

	// Handler to processing sent message to telegram.
	// E.g. useful for get sent messages ID
	SentHandler func(t *Telegram, messages []MessageSent) error
}

var (
	// ErrCallbackDataFormat contains error "wrong callback data format"
	ErrCallbackDataFormat = errors.New("wrong callback data format")

	// ErrDescriptionState contains error "session state not defined in bot description"
	ErrDescriptionStateMissing = errors.New("session state not defined in bot description")

	// ErrUpdatesChanClosed contains error "updates channel has been closed"
	ErrUpdatesChanClosed = errors.New("updates channel has been closed")
)

// Button contains buttons data for state
type Button struct {

	// Button text
	Text string

	// Defines a button identifier for processing in handler
	Identifier string
}

// File contains file descrition received from Telegram
type File struct {
	FileSize int
	FileName string

	f tgbotapi.File
}

// FileSendStream contains options for sending file to Telegram as stream
type FileSendStream struct {
	FileName string
	FileSize int64
	Caption  string
	Buttons  [][]Button
}

// FileSend contains options for sending file to Telegram
type FileSend struct {
	FilePath string
	Caption  string
	Buttons  [][]Button
}

type callbackData struct {
	S string `json:"s"`
	I string `json:"i"`
}

// Setup settings up Telegram bot
func Setup(s Settings, description Description, usrCtx interface{}) (Telegram, error) {

	var t Telegram

	bot, err := botConnect(s.BotSettings.BotAPI, s.BotSettings.Proxy)
	if err != nil {
		return t, err
	}

	t.bot = bot
	t.description = description
	t.usrCtx = usrCtx
	t.redisHost = s.RedisHost
	t.updateQueueWait = s.UpdateQueueWait

	if s.BotSettings.Webhook != nil {
		if err := t.setWebhook(s.BotSettings.Webhook); err != nil {
			return t, err
		}
	} else {
		if err := t.delWebhook(); err != nil {
			return t, err
		}
	}

	err = t.commandsSet()

	return t, err
}

func (t *Telegram) SelfIDGet() int64 {
	return t.bot.Self.ID
}

// Processing processes available updates from queue
func (t *Telegram) Processing() error {

	q, err := queueInit(t.redisHost, t.updateQueueWait)
	if err != nil {
		return err
	}
	defer q.queueClose()

	// Get all available updates from queue
	uc, err := q.chainGet()
	if err != nil {
		return err
	}

	t.session = sessionInit(uc.ChatIDGet(), uc.UserIDGet(), t.redisHost)

	return t.stateProcessing(uc)
}

// GetUpdates creates to Telegram API and processes a receiving updates
func (t *Telegram) GetUpdates(ctx context.Context) error {

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	c := t.bot.GetUpdatesChan(u)
	defer t.bot.StopReceivingUpdates()

	for {
		select {
		case <-ctx.Done():
			return nil
		case u, b := <-c:
			if b == false {
				return ErrUpdatesChanClosed
			}
			if err := t.UpdateAbsorb(Update(u)); err != nil {
				return fmt.Errorf("bot add request into queue error: %v", err)
			}
		}
	}
}

// UpdateAbsorb absorbs specified `update` and put it into queue
func (t *Telegram) UpdateAbsorb(update Update) error {

	chatID, userID := updateIDsGet(update)

	if update.CallbackQuery != nil {
		if _, err := t.bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "")); err != nil {
			return err
		}
	}

	if chatID == 0 || userID == 0 {
		return nil
	}

	q, err := queueInit(t.redisHost, t.updateQueueWait)
	if err != nil {
		return err
	}
	defer q.queueClose()

	return q.add(chatID, userID, update)
}

// UserIDGet gets user ID for current session
func (t *Telegram) UserIDGet() int64 {
	return t.session.userID
}

// SlotSave saves data in specified slot within the session
func (t *Telegram) SlotSave(slot string, data interface{}) error {
	return t.session.slotSave(slot, data)
}

// SlotGet gets data from specified slot within the session.
// Use `mapstructure.Decode` to get "structure" data type
func (t *Telegram) SlotGet(slot string) (interface{}, bool, error) {
	return t.session.slotGet(slot)
}

// SlotDel deletes specified slot within the session
func (t *Telegram) SlotDel(slot string) error {
	return t.session.slotDel(slot)
}

// UsrCtxGet gets user context
func (t *Telegram) UsrCtxGet() interface{} {
	return t.usrCtx
}

// DownloadFileStream returns io.ReadCloser to download specified file
func (t *Telegram) DownloadFileStream(file File) (io.ReadCloser, error) {

	// Make request
	req, err := http.NewRequest("GET", file.f.Link(t.bot.Token), nil)
	if err != nil {
		return nil, fmt.Errorf("can't create new request: %v", err)
	}

	// Make request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	client := &http.Client{Transport: tr}

	// Do request
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	// Write body to file
	if res.StatusCode == http.StatusOK {
		return res.Body, nil
	}

	res.Body.Close()

	return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
}

// DownloadFile downloads file from Telegram to specified path
func (t *Telegram) DownloadFile(file File, dstPath string) error {

	lf, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer lf.Close()

	s, err := t.DownloadFileStream(file)
	if err != nil {
		return err
	}
	defer s.Close()

	if _, err := io.Copy(lf, s); err != nil {
		return err
	}

	return nil
}

// UploadPhotoStream uploads file as photo to Telegram by specified reader
func (t *Telegram) UploadPhotoStream(file FileSendStream, r io.Reader) (MessageSent, error) {

	var (
		bm  [][]tgbotapi.InlineKeyboardButton
		ikm tgbotapi.InlineKeyboardMarkup
	)

	reader := tgbotapi.FileReader{
		Name:   file.FileName,
		Reader: r,
	}

	// If buttons set
	if len(file.Buttons) > 0 {
		for _, br := range file.Buttons {
			var b []tgbotapi.InlineKeyboardButton
			for _, be := range br {
				b = append(b, tgbotapi.NewInlineKeyboardButtonData(be.Text, be.Identifier))
			}
			bm = append(bm, b)
		}
		ikm = tgbotapi.NewInlineKeyboardMarkup(bm...)
	}

	// For other examples see: https://github.com/go-telegram-bot-api/telegram-bot-api/blob/master/bot_test.go
	msg := tgbotapi.NewPhoto(t.session.chatIDGet(), reader)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.Caption = file.Caption

	if len(file.Buttons) > 0 {
		msg.ReplyMarkup = &ikm
	}

	m, err := t.bot.Send(msg)

	return MessageSent(m), err
}

// UploadPhoto uploads file as photo to Telegram
func (t *Telegram) UploadPhoto(file FileSend) (MessageSent, error) {

	f, err := os.Open(file.FilePath)
	if err != nil {
		return MessageSent{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return MessageSent{}, err
	}

	return t.UploadPhotoStream(FileSendStream{
		FileName: path.Base(file.FilePath),
		FileSize: stat.Size(),
		Caption:  file.Caption,
		Buttons:  file.Buttons,
	}, f)
}

// stateProcessing processes current session state.
// It's initial point to route processing into appropriate state
// in accordance with update chain
func (t *Telegram) stateProcessing(uc UpdateChain) error {

	// Check `update` is a defined command
	b, err := t.commandProcessing(uc)
	if b == true {
		// If command were found
		return err
	}

	switch uc.TypeGet() {
	case UpdateTypeMessage:
		return t.messageProcessing(uc)
	case UpdateTypeCallback:
		return t.callbackProcessing(uc)
	}

	return nil
}

// setWebhook sets Telegram webhook
func (t *Telegram) setWebhook(s *SettingsBotWebhook) error {

	var (
		wh  tgbotapi.WebhookConfig
		err error
	)

	if s == nil {
		return nil
	}

	// Prepare webhook URL
	whURL := s.URL
	if whURL[len(whURL)-1] != '/' {
		whURL += "/"
	}
	whURL += s.BotToken

	// Set webhook (each time when server starting)
	if s.WithCert == true {
		wh, err = tgbotapi.NewWebhookWithCert(whURL, tgbotapi.FilePath(s.CertFile))
		if err != nil {
			return fmt.Errorf("Telegram bot set webhook error: %v", err)
		}
	} else {
		wh, err = tgbotapi.NewWebhook(whURL)
		if err != nil {
			return fmt.Errorf("Telegram bot set webhook error: %v", err)
		}
	}

	if _, err := t.bot.Request(wh); err != nil {
		return fmt.Errorf("Telegram bot set webhook error: %v", err)
	}

	return nil
}

func (t *Telegram) delWebhook() error {
	if _, err := t.bot.Request(tgbotapi.DeleteWebhookConfig{}); err != nil {
		return fmt.Errorf("Telegram bot delete webhook error: %v", err)
	}
	return nil
}

// Set specified bot commands
func (t *Telegram) commandsSet() error {

	var bcmds []tgbotapi.BotCommand

	for c, m := range t.description.Commands {
		bcmds = append(bcmds, tgbotapi.BotCommand{
			Command:     c,
			Description: m.Description,
		})
	}

	// Set specified commands
	if _, err := t.bot.Request(tgbotapi.NewSetMyCommands(bcmds...)); err != nil {
		return fmt.Errorf("Telegram bot set commands error: %v", err)
	}

	return nil
}

// initProcessing processes session init state
func (t *Telegram) initProcessing(uc UpdateChain) error {

	if t.description.InitHandler == nil {
		return nil
	}

	// Call initHandler
	r, err := t.description.InitHandler(t, uc)
	if err != nil {
		return err
	}

	return t.stateSwitch(r.NextState, 0)
}

// commandProcessing lookups and processes command (if described) by message text from Telegram update.
func (t *Telegram) commandProcessing(uc UpdateChain) (bool, error) {

	// Check update contains command
	cmd, args := uc.commandCheck()
	if len(cmd) == 0 {
		return false, nil
	}

	// Check specified command defined in bot description
	s, b := t.description.Commands[cmd]
	if b == false {
		return false, nil
	}

	// Check handler defined for command
	if s.Handler == nil {
		return true, nil
	}

	r, err := s.Handler(t, uc, cmd, args)
	if err != nil {
		return true, err
	}

	return true, t.stateSwitch(r.NextState, 0)
}

// messageProcessing processes update chain with `message` type
func (t *Telegram) messageProcessing(uc UpdateChain) error {

	// Get current session
	state, e, err := t.session.stateGet()
	if err != nil {
		return err
	}

	// If session does not exist
	if e == false {
		return t.initProcessing(uc)
	}

	// Get state description
	s, b := t.description.States[state]
	if b == false {
		return ErrDescriptionStateMissing
	}

	if s.MessageHandler == nil {
		return nil
	}

	r, err := s.MessageHandler(t, uc)
	if err != nil {
		return err
	}

	return t.stateSwitch(r.NextState, 0)
}

// callbackProcessing processes update chain with `callback` type
func (t *Telegram) callbackProcessing(uc UpdateChain) error {

	state, identifier, err := callbackDataGet(uc)
	if err != nil {
		return err
	}

	// Get state description
	s, b := t.description.States[state]
	if b == false {
		return ErrDescriptionStateMissing
	}

	if s.CallbackHandler == nil {
		return nil
	}

	r, err := s.CallbackHandler(t, uc, identifier)
	if err != nil {
		return err
	}

	return t.stateSwitch(r.NextState, uc.updates[0].CallbackQuery.Message.MessageID)

}

func (t *Telegram) stateSwitch(newState SessionState, messageID int) error {

	var mID int

	switch newState {
	case sessionBreak:
		return nil
	case sessionDestroy:
		return t.session.destroy()
	}

	s, b := t.description.States[newState]
	if b == false {
		return ErrDescriptionStateMissing
	}

	// Put session into new state
	if err := t.session.stateSet(newState); err != nil {
		return err
	}

	if s.StateHandler == nil {
		// Do nothing if state handler not defined
		return nil
	}

	hr, err := s.StateHandler(t)
	if err != nil {
		return err
	}

	if hr.StickMessage == true {
		mID = messageID
	}

	// Send message to user if set
	if len(hr.Message) > 0 {

		msgs, err := t.sendMessage(t.session.chatIDGet(), mID, hr.Message, hr.Buttons, newState)
		if err != nil {
			return err
		}

		if s.SentHandler != nil {
			if err := s.SentHandler(t, msgs); err != nil {
				return err
			}
		}
	}

	// Do not switch next state if message handler defined
	if s.MessageHandler != nil {
		return nil
	}

	return t.stateSwitch(hr.NextState, mID)

}

// sendMessage sends specified message to client
// Messages can be of two types: either new message, or edit existing message (if messageID is set)
func (t *Telegram) sendMessage(chatID int64, messageID int, message string, buttons [][]Button, state SessionState) ([]MessageSent, error) {

	var (
		bm  [][]tgbotapi.InlineKeyboardButton
		ikm tgbotapi.InlineKeyboardMarkup
		mr  tgbotapi.Message
		err error
	)

	// If buttons set
	if len(buttons) > 0 {
		for _, br := range buttons {
			var b []tgbotapi.InlineKeyboardButton
			for _, be := range br {

				d, err := callbackDataSet(state, be.Identifier)
				if err != nil {
					return []MessageSent{}, err
				}

				b = append(b, tgbotapi.NewInlineKeyboardButtonData(be.Text, d))
			}
			bm = append(bm, b)
		}

		ikm = tgbotapi.NewInlineKeyboardMarkup(bm...)
	}

	if messageID == 0 {
		msg := tgbotapi.NewMessage(chatID, message)
		msg.ParseMode = tgbotapi.ModeMarkdown

		if len(buttons) > 0 {
			msg.ReplyMarkup = ikm
		}

		mr, err = t.bot.Send(msg)
	} else {
		msg := tgbotapi.NewEditMessageText(chatID, messageID, message)
		msg.ParseMode = tgbotapi.ModeMarkdown

		if len(buttons) > 0 {
			msg.ReplyMarkup = &ikm
		}

		mr, err = t.bot.Send(msg)
	}

	return []MessageSent{MessageSent(mr)}, err
}

// botConnect sets up Telegram bot
func botConnect(botAPI string, p *SettingsBotProxy) (*tgbotapi.BotAPI, error) {

	if p == nil {
		return tgbotapi.NewBotAPI(botAPI)
	}

	switch p.Type {
	case "socks5":
		auth := proxy.Auth{
			User:     p.Login,
			Password: p.Password,
		}

		dialer, err := proxy.SOCKS5("tcp", p.Host, &auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("connect to proxy error: %v", err)
		}

		t := &http.Transport{Dial: dialer.Dial}

		return tgbotapi.NewBotAPIWithClient(botAPI, tgbotapi.APIEndpoint, &http.Client{Transport: t})
	}

	return nil, fmt.Errorf("unknown proxy type")
}

func callbackDataGet(uc UpdateChain) (SessionState, string, error) {

	var d callbackData

	data := uc.callbackDataGet()
	if len(data) == 0 {
		return sessionBreak, "", nil
	}

	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return sessionBreak, "", err
	}

	return SessionState{d.S}, d.I, nil
}

func callbackDataSet(state SessionState, identifier string) (string, error) {

	d := callbackData{
		S: state.state,
		I: identifier,
	}

	b, err := json.Marshal(&d)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
